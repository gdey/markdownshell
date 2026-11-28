package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdey/markdownshell/copyable"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termsafe"
)

// copyMark is how a copyable text's number shows in output: ⧉1 in
// reverse video. It only turns reverse video on and off, so the
// output's own colors carry on around it.
func copyMark(n int) string { return "\x1b[7m⧉" + strconv.Itoa(n) + "\x1b[27m" }

// markCopyable puts a mark before each text output s offers to copy
// (see package copyable): before a region's text, which shows as it
// is, and, for an OSC 52 text, which doesn't show, in place of the
// sequence with a word on what it is. The sequences themselves are
// taken out, so clean and termsafe.Styled never see them.
func markCopyable(s string) string {
	if !copyable.Has(s) {
		return s
	}
	var b strings.Builder
	for _, p := range copyable.Split(s) {
		switch p.Kind {
		case copyable.Plain:
			b.WriteString(p.Text)
		case copyable.Open:
			b.WriteString(copyMark(p.N) + " ")
		case copyable.Hidden:
			n := utf8.RuneCountInString(p.Text)
			unit := " chars"
			if n == 1 {
				unit = " char"
			}
			b.WriteString(copyMark(p.N) + " text to copy, " + strconv.Itoa(n) + unit + " (c)\n")
		}
	}
	return b.String()
}

// rawOutput is the output block i shows once it has run: what it
// printed this session, or the output saved in the file.
func (m *Model) rawOutput(i int) string {
	if r := m.results[i]; r != nil && r.Ran {
		if r.Block.IsInclude() || r.HTTP != nil || r.Image != "" || run.OutKind(r.Block) == "markdown" {
			return ""
		}
		return string(r.Stdout)
	}
	b := m.d.Blocks[i]
	if !b.HasOutput() || b.IsHTTP() {
		return ""
	}
	cur := b.CurrentOutput()
	return saved(m.d.Src[cur.Start:cur.End])
}

// copyOffered puts a text the output of the block under the cursor
// offers to copy on the clipboard. With more than one, each press of c
// copies the next, in order, starting again after the last.
func (m *Model) copyOffered() tea.Cmd {
	i := m.cur()
	if i < 0 {
		return nil
	}
	texts := copyable.Texts(m.rawOutput(i))
	if len(texts) == 0 {
		m.status = "the block's output offers nothing to copy"
		return nil
	}
	k := 0
	if m.copyAt.block == i {
		k = (m.copyAt.next) % len(texts)
	}
	m.copyAt.block, m.copyAt.next = i, k+1
	text := texts[k]
	label := m.d.Label(m.d.Blocks[i])
	of := ""
	if len(texts) > 1 {
		of = " " + strconv.Itoa(k+1) + " of " + strconv.Itoa(len(texts))
	}
	return func() tea.Msg {
		return copiedMsg{label: label, lines: strings.Count(text, "\n") + 1, offered: "⧉" + strconv.Itoa(k+1) + of, err: setClipboard(text)}
	}
}

// offeredStatus is the status line after copyOffered.
func offeredStatus(msg copiedMsg) string {
	return "copied text " + msg.offered + " from " + termsafe.Visible(msg.label) + "'s output"
}
