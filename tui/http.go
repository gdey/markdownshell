package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/httpblock"
	"github.com/gdey/markdownshell/run"
)

var httpTabStyle = lipgloss.NewStyle().Bold(true).Underline(true)

// httpOutput draws an http block's response under it: the status, the
// body or, after H, the headers, and the last run's checks with a tick
// or a cross. text is the response as the output holds it, masked as
// the file saves it; r is the block's result, for its checks. ok is
// false when text isn't a response, so the output is drawn as usual.
func (m *Model) httpOutput(b *doc.Block, r *run.Result, text string) (string, bool) {
	status, headers, body, ok := httpblock.Split(text)
	if !ok {
		return "", false
	}
	col := okCol
	if code := strings.SplitN(status, " ", 2)[0]; code == "" || code[0] >= '4' {
		col = failCol
	}
	tabs := []string{"Body", fmt.Sprintf("Headers (%d)", strings.Count(headers, "\n"))}
	show := 0
	if m.httpHeaders[b] {
		show = 1
	}
	tabs[show] = httpTabStyle.Render(tabs[show])
	tabs[1-show] = tagStyle.Render(tabs[1-show])
	lines := []string{lipgloss.NewStyle().Foreground(col).Bold(true).Render(status) + "  " + tabs[0] + "  " + tabs[1] + "  " + helpStyle.Render("H: "+map[int]string{0: "headers", 1: "body"}[show])}
	pane := body
	if show == 1 {
		pane = headers
	}
	pane = strings.TrimRight(clean(pane), "\n")
	if pane == "" {
		pane = helpStyle.Render(map[int]string{0: "(no body)", 1: "(no headers)"}[show])
	}
	lines = append(lines, pane)
	if r != nil && r.Ran && r.HTTP != nil && len(r.HTTP.Checks) > 0 {
		lines = append(lines, "")
		for _, c := range r.HTTP.Checks {
			if c.OK {
				lines = append(lines, lipgloss.NewStyle().Foreground(okCol).Render("✓")+" "+c.Text)
			} else {
				lines = append(lines, stderrStyle.Render("✗ "+c.Text+": got "+c.Got))
			}
		}
		for _, e := range r.HTTP.CaptureErrs {
			lines = append(lines, stderrStyle.Render("✗ "+e))
		}
	}
	return strings.Join(lines, "\n"), true
}

// savedResponse is the response saved after http block b, as its
// output fence holds it, or "".
func savedResponse(b *doc.Block, src []byte) string {
	var lines []string
	fence := ""
	for _, l := range strings.SplitAfter(string(src[b.Output.Start:b.Output.End]), "\n") {
		l = strings.TrimPrefix(l, b.Prefix)
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

// toggleHeaders switches the current http block's response between its
// body and its headers.
func (m *Model) toggleHeaders() {
	i := m.cur()
	if i < 0 || !m.d.Blocks[i].IsHTTP() {
		m.status = "H shows an http block's response headers"
		return
	}
	if m.httpHeaders == nil {
		m.httpHeaders = map[*doc.Block]bool{}
	}
	b := m.d.Blocks[i]
	m.httpHeaders[b] = !m.httpHeaders[b]
}

// copyCurl copies the current http block's request as a curl command
// line, with its placeholders filled in as running it now would.
func (m *Model) copyCurl() tea.Cmd {
	i := m.cur()
	if i < 0 || !m.d.Blocks[i].IsHTTP() {
		m.status = "Y copies an http block as curl"
		return nil
	}
	b := m.d.Blocks[i]
	req, err := b.Request()
	if err != nil {
		m.status = "copy as curl: " + err.Error()
		return nil
	}
	lookup := run.LookupAt(run.NewLookup(m.d, m.results, m.opts.Run.Environ), m.d, m.results, b.Span.Start)
	if full, err := req.Expand(func(s string) (string, error) { return doc.Expand(s, lookup) }); err == nil {
		req = full
	}
	text, label := httpblock.Curl(req), m.d.Label(b)+" as curl"
	return func() tea.Msg {
		return copiedMsg{label: label, lines: 1, err: setClipboard(text)}
	}
}
