package tui

import (
	"context"
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// View is how a file is shown, as in mdsh-gui's View menu. The TUI keeps
// the last one used, so the next file opens the same way.
type View struct {
	// Outline shows the headings and blocks down the left.
	Outline bool `json:"outline"`
	// HideCode and HideOutput leave out the code and the output of
	// blocks.
	HideCode   bool `json:"hideCode"`
	HideOutput bool `json:"hideOutput"`
}

// viewMsg says the view changed, so the app can keep it.
type viewMsg View

// entry is a line of the outline: a heading, or a block the cursor stops
// on.
type entry struct {
	level int    // heading level, or 0 for a block
	text  string // the heading's text
	block int    // index into d.Blocks, for a block
}

// The outline's width, and the narrowest the document is let get
// beside it.
const outlineWidth, minDocWidth = 28, 40

// showOutline reports whether the outline is drawn: it is on and the
// terminal is wide enough for it.
func (m *Model) showOutline() bool {
	return m.shown.Outline && m.width-outlineWidth-1 >= minDocWidth
}

// docWidth is the width the document is drawn at.
func (m *Model) docWidth() int {
	if m.showOutline() {
		return m.width - outlineWidth - 1
	}
	return m.width
}

// entries lists the document's headings and the blocks the cursor stops
// on, in the order they come. Headings are ATX headings (# …) outside
// fenced blocks, their output and the front matter.
func (m *Model) entries() []entry {
	src := m.d.Src
	// skip are the byte ranges that hold no headings, in order.
	type span struct{ from, to int }
	skip := []span{{m.d.FrontMatter.Start, m.d.FrontMatter.End}}
	at := map[int]int{} // start of a block the cursor stops on → its index
	for _, i := range m.nav {
		b := m.d.Blocks[i]
		at[b.Span.Start] = i
	}
	for _, b := range m.d.Blocks {
		skip = append(skip, span{b.Span.Start, max(b.Span.End, b.Output.End)})
	}
	var out []entry
	k := 0
	for pos := 0; pos < len(src); {
		end := strings.IndexByte(string(src[pos:]), '\n')
		if end < 0 {
			end = len(src)
		} else {
			end += pos + 1
		}
		for k < len(skip) && skip[k].to <= pos {
			k++
		}
		inSkip := false
		for _, s := range skip[k:] {
			if s.from <= pos && pos < s.to {
				inSkip = true
			}
		}
		if i, ok := at[pos]; ok {
			out = append(out, entry{block: i})
		} else if !inSkip {
			if level, text := heading(string(src[pos:end])); level > 0 {
				out = append(out, entry{level: level, text: text})
			}
		}
		pos = end
	}
	return out
}

// heading returns the level and text of an ATX heading line, or 0.
func heading(line string) (int, string) {
	line = strings.TrimRight(line, "\r\n")
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return 0, ""
	}
	n := len(t) - len(strings.TrimLeft(t, "#"))
	if n == 0 || n > 6 || (len(t) > n && t[n] != ' ' && t[n] != '\t') {
		return 0, ""
	}
	text := strings.TrimSpace(t[n:])
	// A closing run of #s goes, if a space comes before it.
	if s := strings.TrimRight(text, "#"); s == "" || strings.HasSuffix(s, " ") {
		text = strings.TrimSpace(s)
	}
	return n, text
}

// mark is a block's state in the outline, as in mdsh-gui: ✓ ran, ✗
// failed, ? waiting for an answer, … running.
func (m *Model) mark(i int) string {
	r := m.results[i]
	switch {
	case m.active == i, m.bg[i]:
		return lipgloss.NewStyle().Foreground(runningCol).Render("…")
	case m.prompt != nil && m.index[m.prompt.block] == i:
		return lipgloss.NewStyle().Foreground(runningCol).Render("?")
	case r.Ran && (r.Err != nil || r.Failed() || errors.Is(r.Err, context.Canceled)):
		return lipgloss.NewStyle().Foreground(failCol).Render("✗")
	case r.Ran && !r.Block.IsInclude():
		return lipgloss.NewStyle().Foreground(okCol).Render("✓")
	case r.Skipped && run.Runnable(r.Block):
		return tagStyle.Render("–")
	}
	return " "
}

// outline draws the outline h lines high, keeping the block under the
// cursor in view.
func (m *Model) outline(h int) string {
	w := outlineWidth
	var lines []string
	at := 0
	level := 0
	for _, e := range m.entries() {
		if e.level > 0 {
			level = e.level
			indent := strings.Repeat(" ", min(e.level-1, 3))
			lines = append(lines, truncate(indent+titleStyle.Render(e.text), w))
			continue
		}
		b := m.d.Blocks[e.block]
		name := m.name(b)
		if b.IsInclude() {
			name = "include " + b.Attrs["file"]
		}
		indent := strings.Repeat(" ", min(level, 4))
		line := indent + m.mark(e.block) + " " + name
		if m.cur() == e.block {
			at = len(lines)
			line = indent + m.mark(e.block) + " " + pickedStyle.Render(name)
		}
		lines = append(lines, truncate(line, w))
	}
	top := max(min(at-h/2, len(lines)-h), 0)
	lines = lines[top:min(top+h, len(lines))]
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n"))
}

// name is how the outline and footer name b: #id, or "block at line N"
// when mdsh made its id up.
func (m *Model) name(b *doc.Block) string {
	if l := m.d.Label(b); l != b.ID {
		return "block " + l
	}
	return "#" + b.ID
}

func truncate(s string, w int) string { return ansi.Truncate(s, w, "…") }

// toggle flips one part of the view and redraws.
func (m *Model) toggle(f *bool) func() tea.Msg {
	*f = !*f
	m.md = map[string]string{}
	m.vp.Width = m.docWidth()
	m.fit()
	m.refresh()
	if i := m.cur(); i >= 0 {
		m.moveTo(i)
	}
	v := m.shown
	return func() tea.Msg { return viewMsg(v) }
}
