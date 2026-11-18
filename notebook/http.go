package notebook

import (
	"errors"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/httpblock"
	"github.com/gdey/markdownshell/run"
)

// HTTPCell is what the view shows of an http block's response, over
// its output: the status, the body and the headers as two tabs, and the
// checks with a tick or a cross. Headers are masked as the output is.
type HTTPCell struct {
	// Status is the status line's code and text, such as 201 Created,
	// and Code its number; both are empty when there is no response.
	Status string `json:"status,omitempty"`
	Code   int    `json:"code,omitempty"`
	// Headers and Body are the response's parts, as the output shows
	// them: the body pretty-printed, the credential headers masked.
	Headers string `json:"headers,omitempty"`
	Body    string `json:"body,omitempty"`
	// Checks are the last run's checks, in order: none for a response
	// read from the file.
	Checks []HTTPCheck `json:"checks,omitempty"`
}

// HTTPCheck is one check's result.
type HTTPCheck struct {
	Text string `json:"text"`
	OK   bool   `json:"ok"`
	Got  string `json:"got,omitempty"`
}

// httpCell is the response view for http block b, from its run r this
// session, else from the output saved after it, out (a piece of
// render.Pieces). It is nil for any other block, and for an http block
// with no response to show.
func httpCell(b *doc.Block, r *run.Result, out []byte) *HTTPCell {
	if !b.IsHTTP() {
		return nil
	}
	var text string
	switch {
	case r != nil && r.Ran && r.HTTP != nil:
		text = string(r.Stdout)
	case r != nil && r.Ran:
		return nil
	default:
		text = fenceBody(string(out), b.Prefix)
	}
	status, headers, body, ok := httpblock.Split(text)
	if !ok {
		return nil
	}
	c := &HTTPCell{Status: status, Headers: headers, Body: body}
	code, _, _ := strings.Cut(status, " ")
	for _, ch := range code {
		if ch < '0' || ch > '9' {
			c.Code = 0
			break
		}
		c.Code = c.Code*10 + int(ch-'0')
	}
	if r != nil && r.HTTP != nil {
		for _, ch := range r.HTTP.Checks {
			hc := HTTPCheck{Text: ch.Text, OK: ch.OK}
			if !ch.OK {
				hc.Got = ch.Got
			}
			c.Checks = append(c.Checks, hc)
		}
	}
	return c
}

// fenceBody is the body of the first output fence in md, a block's
// output as markdown, with prefix taken off each line; "" if there is
// none.
func fenceBody(md, prefix string) string {
	var lines []string
	fence := ""
	for _, l := range strings.SplitAfter(md, "\n") {
		l = strings.TrimPrefix(l, prefix)
		t := strings.TrimRight(l, "\r\n")
		if fence == "" {
			if strings.HasPrefix(t, "```") && strings.Contains(t, "output") && !strings.Contains(t, "stream=stderr") {
				fence = t[:len(t)-len(strings.TrimLeft(t, "`"))]
			}
			continue
		}
		if strings.TrimSpace(t) == fence {
			return strings.Join(lines, "")
		}
		lines = append(lines, l)
	}
	return ""
}

// Curl is http block i's request as a curl command line, with its
// placeholders filled in as running it now would: for Copy as curl. A
// placeholder with no value yet is left as written.
func (n *Notebook) Curl(i int) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if i < 0 || i >= len(n.d.Blocks) || !n.d.Blocks[i].IsHTTP() {
		return "", errNotHTTP
	}
	b := n.d.Blocks[i]
	req, err := b.Request()
	if err != nil {
		return "", err
	}
	lookup := run.LookupAt(run.NewLookup(n.d, n.results, environ()), n.d, n.results, b.Span.Start)
	if full, err := req.Expand(func(s string) (string, error) { return doc.Expand(s, lookup) }); err == nil {
		req = full
	}
	return httpblock.Curl(req), nil
}

var errNotHTTP = errors.New("not an http block")
