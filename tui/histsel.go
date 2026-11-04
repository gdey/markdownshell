package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The History pane's lines can be selected and copied, as in a
// terminal's copy mode: V in the file, or shift+↑ in the pane, selects
// the last line; ↑/↓ (or k/j) move the selection's end, and y or enter
// copies the lines as text, without their colors or links.

// histSel is a selection of lines in the pane's log, from anchor to
// cur, which may come before it.
type histSel struct {
	anchor, cur int
	// focus is whether the keys were in the pane, to give them back.
	focus bool
}

var histSelStyle = lipgloss.NewStyle().Reverse(true)

// selectHistory starts a selection on the log's last line. It reports
// why it can't, or "".
func (m *Model) selectHistory() string {
	h := &m.hist
	switch {
	case !h.open:
		return "History is closed; h opens it"
	case h.running:
		return "a command runs in History"
	case len(h.log) == 0:
		return "nothing in History to select"
	}
	last := len(h.log) - 1
	h.sel = &histSel{anchor: last, cur: last, focus: h.focus}
	h.input.Blur()
	return ""
}

// selRange is the selection's first and last lines.
func (s *histSel) selRange() (int, int) { return min(s.anchor, s.cur), max(s.anchor, s.cur) }

// selKey handles a key while lines are selected.
func (m *Model) selKey(k tea.KeyMsg) tea.Cmd {
	h := &m.hist
	s := h.sel
	page := max(m.histRows()/2, 1)
	move := func(to int) { s.cur = max(0, min(to, len(h.log)-1)) }
	switch k.String() {
	case "up", "k", "shift+up":
		move(s.cur - 1)
	case "down", "j", "shift+down":
		move(s.cur + 1)
	case "ctrl+u", "pgup":
		move(s.cur - page)
	case "ctrl+d", "pgdown":
		move(s.cur + page)
	case "g", "home":
		move(0)
	case "G", "end":
		move(len(h.log) - 1)
	case "o":
		s.anchor, s.cur = s.cur, s.anchor
	case "V", "v":
		// Start again from the line the end is on.
		s.anchor = s.cur
	case "y", "enter":
		text := m.selText()
		m.endSel()
		return func() tea.Msg {
			return copiedMsg{lines: strings.Count(text, "\n") + 1, err: setClipboard(text)}
		}
	case "esc", "q", "ctrl+c":
		m.endSel()
		if h.focus {
			return h.input.Focus()
		}
	}
	return nil
}

// endSel ends the selection, giving the keys back to where they were.
func (m *Model) endSel() {
	h := &m.hist
	if h.sel == nil {
		return
	}
	h.focus = h.sel.focus
	h.sel = nil
	if h.focus {
		h.input.Focus()
	}
}

// selText is the selected lines as plain text: what they show, without
// colors, links or the spaces that end them.
func (m *Model) selText() string {
	from, to := m.hist.sel.selRange()
	lines := make([]string, 0, to-from+1)
	for _, l := range m.hist.log[from : to+1] {
		lines = append(lines, strings.TrimRight(ansi.Strip(l), " \t"))
	}
	return strings.Join(lines, "\n")
}

// selView is the pane's rows while lines are selected: the log, scrolled
// to show the selection's end, with the selected lines drawn reversed.
func (m *Model) selView(rows int) []string {
	h := &m.hist
	from, to := h.sel.selRange()
	top := max(len(h.log)-rows, 0)
	top = min(top, h.sel.cur)
	top = max(top, h.sel.cur-rows+1)
	var lines []string
	for i := top; i < len(h.log) && len(lines) < rows; i++ {
		l := h.log[i]
		if i >= from && i <= to {
			l = histSelStyle.Render(ansi.Strip(l))
		}
		lines = append(lines, l)
	}
	return lines
}
