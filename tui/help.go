package tui

import (
	"errors"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
)

// helpItem is one key binding in the help panel that ? opens.
type helpItem struct {
	group string
	keys  string // the keys, as shown
	// key is what picking the item presses.
	key tea.KeyMsg
	// what says what the key does. It can depend on the state, as q's
	// does.
	what func(m *Model) string
	// off, when set, says why the key does nothing now. Such items show
	// greyed.
	off func(m *Model) string
}

func runeKey(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func says(s string) func(*Model) string { return func(*Model) string { return s } }

// helpItems are the TUI's key bindings, as the help panel lists them.
// Keep them in step with key.
var helpItems = []helpItem{
	{group: "Move", keys: "j/k ↓/↑", key: tea.KeyMsg{Type: tea.KeyDown}, what: says("next block (k: the one before)"), off: noBlocks},
	{group: "Move", keys: "g/G", key: runeKey("g"), what: says("first block (G: last)"), off: noBlocks},
	{group: "Move", keys: "space/pgup", key: tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}, what: says("page down (pgup, ctrl+b: up)")},
	{group: "Move", keys: "ctrl+d/ctrl+u", key: tea.KeyMsg{Type: tea.KeyCtrlD}, what: says("half a page down (ctrl+u: up)")},

	{group: "Run", keys: "enter r", key: tea.KeyMsg{Type: tea.KeyEnter}, what: says("run the block"), off: needIdleBlock},
	{group: "Run", keys: "a", key: runeKey("a"), what: says("run from this block to the end"), off: needIdleBlock},
	{group: "Run", keys: "s", key: runeKey("s"), what: says("run the section the block is in"), off: noSection},
	{group: "Run", keys: "i :", key: runeKey("i"), what: says("type a new sh block and run it"), off: busy},
	{group: "Run", keys: "h", key: runeKey("h"), what: says("History: a shell whose commands are added to the file"), off: func(m *Model) string {
		if m.running && !m.hist.open {
			return "a block is running"
		}
		return ""
	}},
	{group: "Run", keys: "ctrl+c", key: tea.KeyMsg{Type: tea.KeyCtrlC}, what: func(m *Model) string {
		if m.running {
			return "stop the run"
		}
		return "quit"
	}},
	{group: "Run", keys: "ctrl+]", key: tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}, what: says("stop a tty block running in its card"), off: says("only while a tty block runs")},
	{group: "Run", keys: ",", key: runeKey(","), what: says("run options: arguments, step, stop on error"), off: busy},

	{group: "Edit", keys: "e", key: runeKey("e"), what: says("edit the block in $EDITOR"), off: noEdit},
	{group: "Edit", keys: "y", key: runeKey("y"), what: says("copy the block's code to the clipboard"), off: noBlocks},
	{group: "Edit", keys: "E", key: runeKey("E"), what: says("edit the whole file in $EDITOR"), off: busy},
	{group: "Edit", keys: "J/K alt+↓/↑", key: runeKey("J"), what: says("move the block down (K: up)"), off: noMove},
	{group: "Edit", keys: "u", key: runeKey("u"), what: says("undo the last move"), off: noUndo},

	{group: "File", keys: "w ctrl+s", key: runeKey("w"), what: says("save output to the file"), off: noSave},
	{group: "File", keys: "W", key: runeKey("W"), what: says("save as another file"), off: busy},
	{group: "File", keys: "R", key: runeKey("R"), what: says("reload the file from disk"), off: busy},
	{group: "File", keys: "M", key: runeKey("M"), what: says("keep yours when the file changed on disk"), off: func(m *Model) string {
		if !m.changed {
			return "the file hasn't changed on disk"
		}
		return ""
	}},
	{group: "File", keys: "p", key: runeKey("p"), what: says("create a report: PDF, HTML or markdown"), off: busy},
	{group: "File", keys: "q ctrl+w", key: runeKey("q"), what: func(m *Model) string {
		if m.back {
			return "close the file (ctrl+w: the tab)"
		}
		return "quit (ctrl+w: close the tab)"
	}, off: busy},

	{group: "View", keys: "v", key: runeKey("v"), what: says("show the block's image full screen"), off: noImage},
	{group: "View", keys: "o", key: runeKey("o"), what: says("show or hide the outline")},
	{group: "View", keys: "C", key: runeKey("C"), what: says("show or hide code")},
	{group: "View", keys: "O", key: runeKey("O"), what: says("show or hide output")},
	{group: "View", keys: "?", key: runeKey("?"), what: says("this help")},

	{group: "Tabs", keys: "]/[", key: runeKey("]"), what: says("next tab ([: the one before)"), off: func(m *Model) string {
		if m.tabs < 2 {
			return "there's one tab"
		}
		return ""
	}},
	{group: "Tabs", keys: "ctrl+t", key: tea.KeyMsg{Type: tea.KeyCtrlT}, what: says("new tab")},
}

func noBlocks(m *Model) string {
	if len(m.nav) == 0 {
		return "the file has no blocks"
	}
	return ""
}

func busy(m *Model) string {
	if m.running {
		return "a block is running"
	}
	return ""
}

func needIdleBlock(m *Model) string {
	if s := busy(m); s != "" {
		return s
	}
	return noBlocks(m)
}

func noSection(m *Model) string {
	if s := needIdleBlock(m); s != "" {
		return s
	}
	if _, ok := doc.SectionAt(m.d.Headings(), m.d.Blocks[m.cur()].Span.Start); !ok {
		return "the block isn't under a heading"
	}
	return ""
}

func noEdit(m *Model) string {
	if s := needIdleBlock(m); s != "" {
		return s
	}
	if m.d.Blocks[m.cur()].IsInclude() {
		return "an include block's lines come from its file"
	}
	return ""
}

func noMove(m *Model) string {
	if m.running || len(m.bg) > 0 {
		return "a block is running"
	}
	if s := noBlocks(m); s != "" {
		return s
	}
	b := m.d.Blocks[m.cur()]
	_, up := m.d.StepTarget(b, -1, false)
	_, down := m.d.StepTarget(b, 1, false)
	if errors.Is(up, doc.ErrNoMove) && errors.Is(down, doc.ErrNoMove) {
		return "there's nowhere to move the block"
	}
	return ""
}

func noUndo(m *Model) string {
	switch {
	case len(m.undo) == 0:
		return "nothing to undo"
	case m.running || len(m.bg) > 0:
		return "a block is running"
	}
	return ""
}

func noSave(m *Model) string {
	if m.opts.Save == nil {
		return "this file can't be saved"
	}
	return ""
}

func noImage(m *Model) string {
	if s := noBlocks(m); s != "" {
		return s
	}
	if m.imageOf(m.cur()) == "" {
		return "the block has no image"
	}
	return ""
}

// whyOff is why item n does nothing now, or "".
func (m *Model) whyOff(n int) string {
	if it := helpItems[n]; it.off != nil {
		return it.off(m)
	}
	return ""
}

// openHelp shows the help panel, with the first key picked.
func (m *Model) openHelp() {
	m.helping, m.helpAt = true, 0
}

// helped handles a key while the help panel is up: ↑/↓ pick a key,
// enter presses it, and esc, q or ? closes the panel.
func (m *Model) helped(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	last := len(helpItems) - 1
	switch k.String() {
	case "esc", "q", "?", "ctrl+c":
		m.helping = false
	case "j", "down", "tab", "ctrl+n":
		m.helpAt = min(m.helpAt+1, last)
	case "k", "up", "shift+tab", "ctrl+p":
		m.helpAt = max(m.helpAt-1, 0)
	case "g", "home":
		m.helpAt = 0
	case "G", "end":
		m.helpAt = last
	case "pgdown", "ctrl+f", " ", "ctrl+d":
		m.helpAt = min(m.helpAt+max(m.vp.Height/2, 1), last)
	case "pgup", "ctrl+b", "ctrl+u":
		m.helpAt = max(m.helpAt-max(m.vp.Height/2, 1), 0)
	case "enter":
		if why := m.whyOff(m.helpAt); why != "" {
			m.status = helpItems[m.helpAt].keys + ": " + why
			break
		}
		m.helping = false
		key := helpItems[m.helpAt].key
		if key.String() == "ctrl+t" {
			// The app opens tabs.
			return m, func() tea.Msg { return key }
		}
		return m.key(key)
	}
	m.refresh()
	return m, nil
}

var (
	helpGroupStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	helpKeyStyle   = lipgloss.NewStyle().Bold(true)
	helpOffStyle   = lipgloss.NewStyle().Foreground(dimCol).Faint(true)
)

// helpView draws the help panel w wide and h high, scrolled so the key
// picked shows.
func (m *Model) helpView(w, h int) string {
	keysW := 0
	for _, it := range helpItems {
		keysW = max(keysW, lipgloss.Width(it.keys))
	}
	lines := []string{titleStyle.Render("Keys") + helpStyle.Render("  pick one and press enter to do it, or esc to close")}
	at, group := 0, ""
	for n, it := range helpItems {
		if it.group != group {
			group = it.group
			lines = append(lines, "", "  "+helpGroupStyle.Render(group))
		}
		keys := it.keys + strings.Repeat(" ", keysW-lipgloss.Width(it.keys))
		mark := "    "
		if n == m.helpAt {
			mark = "  " + pickedStyle.Render("▸ ")
			at = len(lines)
		}
		line := mark
		if why := m.whyOff(n); why != "" {
			line += helpOffStyle.Render(keys + "  " + it.what(m) + "  (" + why + ")")
		} else if n == m.helpAt {
			line += pickedStyle.Render(keys) + "  " + pickedStyle.Render(it.what(m))
		} else {
			line += helpKeyStyle.Render(keys) + "  " + it.what(m)
		}
		lines = append(lines, truncate(line, w))
	}
	// Scroll to show the pick.
	top := 0
	if len(lines) > h {
		top = min(max(at-h/2, 0), len(lines)-h)
	}
	shown := lines[top:min(top+h, len(lines))]
	for len(shown) < h {
		shown = append(shown, "")
	}
	return lipgloss.NewStyle().Width(w).Render(strings.Join(shown, "\n"))
}
