// Package httpblock reads and runs http blocks: a request written the
// way it goes over the wire, an expected status, and Hurl's checks and
// captures (see docs/http/design.md).
//
//	```http {#login}
//	POST {{$API}}/sessions
//	Content-Type: application/json
//
//	{"user": "j"}
//
//	HTTP 201
//	[Asserts]
//	jsonpath $.token exists
//	[Captures]
//	token: jsonpath $.token
//	```
package httpblock

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Lang is the language of an http block's fence.
const Lang = "http"

// Request is an http block as written. Its text still holds the
// placeholders; Expand fills them in.
type Request struct {
	Method string
	URL    string
	// Headers are the request's headers, in order.
	Headers []Pair
	// Query are [QueryStringParams] lines, added to the URL.
	Query []Pair
	// Form are [FormParams] lines, sent as a form body.
	Form []Pair
	// Body is the raw body, without the blank lines around it. "" when
	// there is none.
	Body string
	// BodyFile is the file named by an @file body, relative to the
	// document. "" when the body isn't one.
	BodyFile string
	// Insecure and Follow are [Options]' insecure and follow.
	Insecure, Follow bool
	// Timeout is [Options]' timeout, or 0.
	Timeout time.Duration
	// Status is the status HTTP NNN expects, or 0 when there is no
	// HTTP line or it says HTTP *.
	Status int
	// StatusLine is the HTTP line as written, or "".
	StatusLine string
	Asserts    []Assert
	Captures   []Capture
}

// Pair is a name and value, as a header or a parameter.
type Pair struct {
	Name, Value string
}

// Query is what a check or a capture reads from the response:
// status, header NAME, jsonpath PATH, body, regex PATTERN or duration,
// optionally followed by the count filter.
type Query struct {
	What string // status, header, jsonpath, body, regex, duration
	Arg  string // the header name, path or pattern, unquoted
	// Count is the count filter: the number of items the query found.
	Count bool
}

// Assert is one line of [Asserts]: a query, a predicate and, for
// those that compare, a value.
type Assert struct {
	Query
	Not bool
	Op  string
	// Value is the value to compare with: a JSON literal, or a bare
	// word, as written.
	Value string
	// Text is the line as written, to report it.
	Text string
}

// Capture is one line of [Captures]: name: QUERY.
type Capture struct {
	Name string
	Query
	// Secret is set by secret before the query, or redact after it.
	Secret bool
}

// Section names a request or response takes, and their Hurl
// shorthands.
var sections = map[string]string{
	"QueryStringParams": "query", "Query": "query",
	"FormParams": "form", "Form": "form",
	"Options":  "options",
	"Asserts":  "asserts",
	"Captures": "captures",
}

// Unsupported section names, with why, so a pasted Hurl file gets a
// clear error instead of a strange body.
var unsupported = map[string]string{
	"MultipartFormData": "multipart bodies aren't supported yet",
	"Multipart":         "multipart bodies aren't supported yet",
	"Cookies":           "set a Cookie header instead",
	"BasicAuth":         "set an Authorization header instead",
}

// Methods are the request methods an http block takes.
var Methods = []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "CONNECT"}

// Whats are the queries checks and captures read, and Ops the
// predicates checks use, in Hurl's words.
var (
	Whats    = []string{"status", "header", "jsonpath", "body", "regex", "duration"}
	Ops      = []string{"==", "!=", "<", "<=", ">", ">=", "contains", "startsWith", "endsWith", "matches", "exists", "isEmpty", "isNumber", "isString", "isBoolean", "isCollection"}
	unaryOps = map[string]bool{"exists": true, "isEmpty": true, "isNumber": true, "isString": true, "isBoolean": true, "isCollection": true}
)

var (
	statusRE  = regexp.MustCompile(`^HTTP(?:/[0-9.]+)?\s+(\d{3}|\*)\s*$`)
	sectionRE = regexp.MustCompile(`^\[([A-Za-z]+)\]\s*$`)
	headerRE  = regexp.MustCompile(`^([!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+)\s*:\s?(.*)$`)
	pairRE    = regexp.MustCompile(`^([^:\s][^:]*?)\s*:\s?(.*)$`)
	captureRE = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.*)$`)
)

// Error is a problem in an http block's text, on a line counted from 1
// at the block's first body line.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("http block line %d: %s", e.Line, e.Msg) }

// Parse reads an http block's body.
func Parse(src string) (*Request, error) {
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	r := &Request{}
	i := 0
	skip := func() {
		for i < len(lines) && (strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(strings.TrimSpace(lines[i]), "#")) {
			i++
		}
	}
	skip()
	if i == len(lines) {
		return nil, &Error{1, "no request: start with a line such as GET https://example.com"}
	}
	method, url, ok := strings.Cut(strings.TrimSpace(lines[i]), " ")
	url = strings.TrimSpace(url)
	if !ok || url == "" || strings.ContainsAny(url, " \t") {
		return nil, &Error{i + 1, fmt.Sprintf("want METHOD URL, got %q", lines[i])}
	}
	if !isMethod(method) {
		return nil, &Error{i + 1, fmt.Sprintf("unknown method %q; want one of %s", method, strings.Join(Methods, ", "))}
	}
	r.Method, r.URL = method, url
	i++

	// Headers: the lines right after the request line, up to a blank
	// line or a section.
	for ; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if t == "" || sectionRE.MatchString(t) || statusRE.MatchString(t) {
			break
		}
		if strings.HasPrefix(t, "#") {
			continue
		}
		m := headerRE.FindStringSubmatch(t)
		if m == nil {
			return nil, &Error{i + 1, fmt.Sprintf("want a header, Name: value, or a blank line before the body; got %q", l)}
		}
		r.Headers = append(r.Headers, Pair{m[1], strings.TrimSpace(m[2])})
	}

	// The request's sections and body, up to the HTTP line or a
	// response section.
	var body []string
	bodyLine := 0
	section := ""
	response := false
	for ; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if m := statusRE.FindStringSubmatch(t); m != nil {
			if r.StatusLine != "" {
				return nil, &Error{i + 1, "a second HTTP line; an http block makes one request"}
			}
			r.StatusLine = t
			if m[1] != "*" {
				r.Status, _ = strconv.Atoi(m[1])
			}
			response, section = true, ""
			continue
		}
		if m := sectionRE.FindStringSubmatch(t); m != nil && (section != "body" || sections[m[1]] != "" || unsupported[m[1]] != "") {
			name := m[1]
			if why, ok := unsupported[name]; ok {
				return nil, &Error{i + 1, fmt.Sprintf("[%s]: %s", name, why)}
			}
			kind, ok := sections[name]
			if !ok {
				return nil, &Error{i + 1, fmt.Sprintf("unknown section [%s]", name)}
			}
			switch {
			case response && (kind == "query" || kind == "form" || kind == "options"):
				return nil, &Error{i + 1, fmt.Sprintf("[%s] belongs to the request, above the HTTP line", name)}
			case !response && (kind == "asserts" || kind == "captures") && r.StatusLine == "":
				// Hurl allows checks without an HTTP line; so do we.
				response = true
			}
			section = kind
			continue
		}
		switch section {
		case "":
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if response {
				return nil, &Error{i + 1, fmt.Sprintf("after the HTTP line, want [Asserts] or [Captures]; got %q", l)}
			}
			if body != nil || r.BodyFile != "" {
				return nil, &Error{i + 1, "a second body"}
			}
			if f, ok := strings.CutPrefix(t, "@"); ok && !strings.ContainsAny(f, " \t") && f != "" {
				r.BodyFile = f
				continue
			}
			section, bodyLine = "body", i+1
			body = append(body, l)
		case "body":
			body = append(body, l)
		case "query", "form":
			if t == "" {
				// A blank line ends a request section: a body may follow.
				section = ""
				continue
			}
			if strings.HasPrefix(t, "#") {
				continue
			}
			m := pairRE.FindStringSubmatch(t)
			if m == nil {
				return nil, &Error{i + 1, fmt.Sprintf("want name: value, got %q", l)}
			}
			p := Pair{unquote(m[1]), unquote(strings.TrimSpace(m[2]))}
			if section == "query" {
				r.Query = append(r.Query, p)
			} else {
				r.Form = append(r.Form, p)
			}
		case "options":
			if t == "" {
				section = ""
				continue
			}
			if strings.HasPrefix(t, "#") {
				continue
			}
			if err := r.option(t); err != nil {
				return nil, &Error{i + 1, err.Error()}
			}
		case "asserts":
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			a, err := ParseAssert(t)
			if err != nil {
				return nil, &Error{i + 1, err.Error()}
			}
			r.Asserts = append(r.Asserts, a)
		case "captures":
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			c, err := parseCapture(t)
			if err != nil {
				return nil, &Error{i + 1, err.Error()}
			}
			for _, o := range r.Captures {
				if o.Name == c.Name {
					return nil, &Error{i + 1, fmt.Sprintf("%s is captured twice", c.Name)}
				}
			}
			r.Captures = append(r.Captures, c)
		}
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	if len(body) > 0 {
		r.Body = strings.Join(body, "\n")
	}
	if r.Body != "" && len(r.Form) > 0 || r.BodyFile != "" && len(r.Form) > 0 {
		return nil, &Error{bodyLine, "a request has a body or [FormParams], not both"}
	}
	return r, nil
}

func isMethod(m string) bool {
	for _, x := range Methods {
		if m == x {
			return true
		}
	}
	return false
}

// option reads an [Options] line.
func (r *Request) option(line string) error {
	m := pairRE.FindStringSubmatch(line)
	if m == nil {
		return fmt.Errorf("want name: value, got %q", line)
	}
	name, value := m[1], strings.TrimSpace(m[2])
	flag := func() (bool, error) {
		b, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("%s: want true or false, got %q", name, value)
		}
		return b, nil
	}
	var err error
	switch name {
	case "insecure":
		r.Insecure, err = flag()
	case "location", "follow":
		r.Follow, err = flag()
	case "timeout":
		r.Timeout, err = time.ParseDuration(value)
		if err == nil && r.Timeout <= 0 {
			err = fmt.Errorf("timeout must be more than 0")
		}
		if err != nil {
			err = fmt.Errorf("timeout: %v", err)
		}
	case "retry", "retry-interval":
		err = fmt.Errorf("%s isn't supported yet; loop in a sh block with mdsh ctl for now", name)
	default:
		err = fmt.Errorf("unknown option %q; want insecure, follow or timeout", name)
	}
	return err
}

// ParseAssert reads one [Asserts] line: QUERY [count] [not] OP [VALUE].
func ParseAssert(line string) (Assert, error) {
	a := Assert{Text: line}
	q, rest, err := parseQuery(line)
	if err != nil {
		return a, err
	}
	a.Query = q
	word, rest := nextWord(rest)
	if word == "not" {
		a.Not = true
		word, rest = nextWord(rest)
	}
	if word == "" {
		return a, fmt.Errorf("%q: want a predicate after %s, such as == or exists", line, q.What)
	}
	if !isOp(word) {
		return a, fmt.Errorf("%q: unknown predicate %q; want one of %s", line, word, strings.Join(Ops, ", "))
	}
	a.Op = word
	rest = strings.TrimSpace(rest)
	switch {
	case unaryOps[word] && rest != "":
		return a, fmt.Errorf("%q: %s takes no value", line, word)
	case !unaryOps[word] && rest == "":
		return a, fmt.Errorf("%q: %s needs a value", line, word)
	}
	if word == "matches" {
		if _, err := regexp.Compile(unquote(rest)); err != nil {
			return a, fmt.Errorf("%q: %v", line, err)
		}
	}
	a.Value = rest
	return a, nil
}

func isOp(w string) bool {
	for _, o := range Ops {
		if w == o {
			return true
		}
	}
	return false
}

// parseCapture reads one [Captures] line: name: [secret] QUERY [redact].
func parseCapture(line string) (Capture, error) {
	m := captureRE.FindStringSubmatch(line)
	if m == nil {
		return Capture{}, fmt.Errorf("want name: QUERY, such as token: jsonpath $.token; got %q", line)
	}
	c := Capture{Name: m[1]}
	rest := m[2]
	if w, after := nextWord(rest); w == "secret" {
		c.Secret, rest = true, after
	}
	q, after, err := parseQuery(rest)
	if err != nil {
		return c, err
	}
	if w, more := nextWord(after); w == "redact" {
		c.Secret, after = true, more
	}
	if strings.TrimSpace(after) != "" {
		return c, fmt.Errorf("%q: unexpected %q after the query", line, strings.TrimSpace(after))
	}
	c.Query = q
	return c, nil
}

// parseQuery reads a query from the start of s and returns the rest.
func parseQuery(s string) (Query, string, error) {
	var q Query
	word, rest := nextWord(s)
	switch word {
	case "status", "body", "duration":
	case "header", "jsonpath", "regex":
		arg, more, err := nextArg(rest)
		if err != nil {
			return q, "", fmt.Errorf("%s: %v", word, err)
		}
		if arg == "" {
			return q, "", fmt.Errorf("%s needs %s", word, map[string]string{"header": "a name", "jsonpath": "a path, such as $.id", "regex": "a pattern"}[word])
		}
		switch word {
		case "jsonpath":
			if _, err := compilePath(arg); err != nil {
				return q, "", err
			}
		case "regex":
			if _, err := regexp.Compile(arg); err != nil {
				return q, "", fmt.Errorf("regex: %v", err)
			}
		}
		q.Arg, rest = arg, more
	case "":
		return q, "", fmt.Errorf("want a query: %s", strings.Join(Whats, ", "))
	default:
		return q, "", fmt.Errorf("unknown query %q; want one of %s", word, strings.Join(Whats, ", "))
	}
	q.What = word
	if w, more := nextWord(rest); w == "count" {
		q.Count, rest = true, more
	}
	return q, rest, nil
}

// nextWord splits s at its first run of spaces.
func nextWord(s string) (word, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

// nextArg reads a query's argument: a quoted string, or a bare word.
func nextArg(s string) (arg, rest string, err error) {
	s = strings.TrimLeft(s, " \t")
	if strings.HasPrefix(s, `"`) {
		for i := 1; i < len(s); i++ {
			switch s[i] {
			case '\\':
				i++
			case '"':
				v, err := strconv.Unquote(s[:i+1])
				if err != nil {
					return "", "", fmt.Errorf("bad quoted %s", s[:i+1])
				}
				return v, s[i+1:], nil
			}
		}
		return "", "", fmt.Errorf("no closing quote in %s", s)
	}
	arg, rest = nextWord(s)
	return arg, rest, nil
}

// unquote drops the quotes around a quoted string, if it is one.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if v, err := strconv.Unquote(s); err == nil {
			return v
		}
	}
	return s
}
