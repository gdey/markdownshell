package httpblock

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultTimeout is how long a request may take when neither the
// block's timeout= nor [Options] says.
const DefaultTimeout = 30 * time.Second

// MaxBody is the most of a response body mdsh reads; the rest is
// dropped, and the output says so.
const MaxBody = 8 << 20

// Expand returns a copy of r with fill run over every text that can hold
// a placeholder: the URL, header values, parameters, the body, the
// body file's name and the values checks compare with. A header or a
// URL that fills in with a line break is an error, so a value can't add
// a header of its own.
func (r *Request) Expand(fill func(string) (string, error)) (*Request, error) {
	out := *r
	var err error
	one := func(s string, what string, single bool) string {
		if err != nil {
			return ""
		}
		var v string
		v, err = fill(s)
		if err == nil && single && strings.ContainsAny(v, "\r\n") {
			err = fmt.Errorf("%s fills in with a line break", what)
		}
		return v
	}
	out.URL = one(r.URL, "the URL", true)
	pairs := func(ps []Pair, what string) []Pair {
		if ps == nil {
			return nil
		}
		o := make([]Pair, len(ps))
		for i, p := range ps {
			o[i] = Pair{p.Name, one(p.Value, what+" "+p.Name, what == "header")}
		}
		return o
	}
	out.Headers = pairs(r.Headers, "header")
	out.Query = pairs(r.Query, "parameter")
	out.Form = pairs(r.Form, "form field")
	out.Body = one(r.Body, "the body", false)
	if r.BodyFile != "" {
		out.BodyFile = one(r.BodyFile, "the body file", true)
	}
	if r.Asserts != nil {
		out.Asserts = make([]Assert, len(r.Asserts))
		for i, a := range r.Asserts {
			a.Value = one(a.Value, "a check", false)
			a.Arg = one(a.Arg, "a check", true)
			out.Asserts[i] = a
		}
	}
	return &out, err
}

// Config is how Do makes a request.
type Config struct {
	// Timeout overrides [Options]' timeout and DefaultTimeout when set.
	Timeout time.Duration
	// ReadFile reads an @file body, as the document's includes are
	// read. Nil refuses @file bodies.
	ReadFile func(name string) ([]byte, error)
	// Transport, if set, makes the request instead of a transport with
	// the system's proxy and certificates. Tests use it.
	Transport http.RoundTripper
}

// Response is what a request got back, with its checks' and captures'
// results.
type Response struct {
	Proto  string // HTTP/1.1
	Status string // 201 Created
	Code   int
	Header http.Header
	Body   []byte
	// Truncated is set when the body was longer than MaxBody.
	Truncated bool
	Duration  time.Duration
	// Checks are the status check, if any, then each [Asserts] line,
	// in order.
	Checks []Check
	// Vars are the captures' values; Secret names the captures marked
	// secret.
	Vars   map[string]string
	Secret []string
	// CaptureErrs are the captures that found nothing, said as for
	// stderr.
	CaptureErrs []string
}

// Check is one check's result.
type Check struct {
	Text string // as written: HTTP 200, or the [Asserts] line
	OK   bool
	// Got is what the response had, when the check failed.
	Got string
}

// Failed reports whether a check failed or a capture found nothing.
func (r *Response) Failed() bool {
	for _, c := range r.Checks {
		if !c.OK {
			return true
		}
	}
	return len(r.CaptureErrs) > 0
}

// Failures are the lines a failed run reports on stderr: one per failed
// check, then one per capture that found nothing.
func (r *Response) Failures() []string {
	var out []string
	for _, c := range r.Checks {
		if !c.OK {
			if strings.HasPrefix(c.Text, "HTTP") {
				out = append(out, fmt.Sprintf("%s: got %s", c.Text, c.Got))
			} else {
				out = append(out, fmt.Sprintf("assert %s: got %s", c.Text, c.Got))
			}
		}
	}
	return append(out, r.CaptureErrs...)
}

// Do makes the request r, an expanded Request, and runs its checks and
// captures on the response. The error is for a request that got no
// response; failed checks are in the Response.
func Do(ctx context.Context, r *Request, cfg Config) (*Response, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, fmt.Errorf("URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL %s: want http:// or https://", r.URL)
	}
	if len(r.Query) > 0 {
		q := u.Query()
		for _, p := range r.Query {
			q.Add(p.Name, p.Value)
		}
		u.RawQuery = q.Encode()
	}
	var body io.Reader
	var data []byte
	contentType := ""
	switch {
	case len(r.Form) > 0:
		f := url.Values{}
		for _, p := range r.Form {
			f.Add(p.Name, p.Value)
		}
		data = []byte(f.Encode())
		contentType = "application/x-www-form-urlencoded"
	case r.BodyFile != "":
		if cfg.ReadFile == nil {
			return nil, errors.New("@" + r.BodyFile + ": this run can't read files")
		}
		if data, err = cfg.ReadFile(r.BodyFile); err != nil {
			return nil, fmt.Errorf("@%s: %w", r.BodyFile, err)
		}
	case r.Body != "":
		data = []byte(r.Body)
		if json.Valid(data) {
			contentType = "application/json"
		}
	}
	if data != nil {
		body = bytes.NewReader(data)
	}
	timeout := DefaultTimeout
	if r.Timeout > 0 {
		timeout = r.Timeout
	}
	if cfg.Timeout > 0 {
		timeout = cfg.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), body)
	if err != nil {
		return nil, err
	}
	for _, h := range r.Headers {
		if strings.EqualFold(h.Name, "Host") {
			req.Host = h.Value
			continue
		}
		req.Header.Add(h.Name, h.Value)
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "mdsh")
	}
	tr := cfg.Transport
	if tr == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		if r.Insecure {
			t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		tr = t
	}
	client := &http.Client{Transport: tr}
	if !r.Follow {
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s %s: no response after %s", r.Method, redactURL(u), timeout)
		}
		return nil, err
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody+1))
	out := &Response{
		Proto:    resp.Proto,
		Status:   resp.Status,
		Code:     resp.StatusCode,
		Header:   resp.Header,
		Body:     got,
		Duration: time.Since(start),
	}
	if err != nil {
		return nil, fmt.Errorf("reading the response: %w", err)
	}
	if len(got) > MaxBody {
		out.Body, out.Truncated = got[:MaxBody], true
	}
	out.check(r)
	out.capture(r)
	return out, nil
}

// redactURL is u without its query or user info, for an error.
func redactURL(u *url.URL) string {
	c := *u
	c.RawQuery, c.User, c.Fragment = "", nil, ""
	return c.String()
}

// check runs r's status check and asserts.
func (resp *Response) check(r *Request) {
	if r.StatusLine != "" && r.Status != 0 {
		resp.Checks = append(resp.Checks, Check{Text: r.StatusLine, OK: resp.Code == r.Status, Got: fmt.Sprint(resp.Code)})
	}
	for _, a := range r.Asserts {
		ok, got := resp.assert(a)
		resp.Checks = append(resp.Checks, Check{Text: a.Text, OK: ok, Got: got})
	}
}

// capture runs r's captures.
func (resp *Response) capture(r *Request) {
	for _, c := range r.Captures {
		v, found, err := resp.query(c.Query)
		switch {
		case err != nil:
			resp.CaptureErrs = append(resp.CaptureErrs, fmt.Sprintf("capture %s: %v", c.Name, err))
			continue
		case !found:
			resp.CaptureErrs = append(resp.CaptureErrs, fmt.Sprintf("capture %s: %s found nothing", c.Name, c.Query))
			continue
		}
		if resp.Vars == nil {
			resp.Vars = map[string]string{}
		}
		resp.Vars[c.Name] = text(v)
		if c.Secret {
			resp.Secret = append(resp.Secret, c.Name)
		}
	}
}

func (q Query) String() string {
	s := q.What
	if q.Arg != "" {
		s += " " + q.Arg
	}
	if q.Count {
		s += " count"
	}
	return s
}
