package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
)

// A background block marked tty runs on a terminal of its own, drawn in
// its card, while other blocks run: an editor, say, open on a file that
// another block runs. Keys go to it while it has the focus, which it
// takes as it starts; ctrl+g gives the focus back to the file, with
// the block still running, and t on its card takes it again.

// bgPaneMsg says background block's terminal opened, or with end, that
// it closed as the block ended.
type bgPaneMsg struct {
	block *doc.Block
	p     *pane
	end   bool
}

// bgDrawnMsg says what background terminal p shows changed.
type bgDrawnMsg struct {
	m *Model
	p *pane
}

// backgroundTerminal is run's BackgroundTerminal: a pane of its own for
// b, as big as the one tty blocks run in.
func (m *Model) backgroundTerminal(b *doc.Block) (*env.Terminal, func()) {
	p := newPane()
	p.setSize(m.pane.size())
	// The model keeps which block it is; take readies its screen.
	p.take(0)
	m.send(bgPaneMsg{block: b, p: p})
	return p.env(), func() {
		p.release()
		// Wake watchBg, which stops now the pane is released.
		select {
		case p.drawn <- struct{}{}:
		default:
		}
		m.send(bgPaneMsg{block: b, p: p, end: true})
	}
}

// watchBg waits for background terminal p's screen to change.
func (m *Model) watchBg(p *pane) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-p.drawn:
			return bgDrawnMsg{m, p}
		case <-m.done:
			return nil
		}
	}
}

// bgPane takes in that a background terminal opened or closed.
func (m *Model) bgPane(msg bgPaneMsg) tea.Cmd {
	i, ok := m.index[msg.block]
	if !ok {
		return nil
	}
	if msg.end {
		if m.bgPanes[i] == msg.p {
			delete(m.bgPanes, i)
			if m.bgFocus == i {
				m.bgFocus = -1
			}
		}
		m.refresh()
		return nil
	}
	m.bgPanes[i] = msg.p
	m.fit()
	// Someone started it to type in it, unless a tty block has the
	// keys now.
	if m.termBlock == -1 {
		m.bgFocus = i
		m.helping, m.procsOpen = false, false
		m.moveTo(i)
		m.follow(i)
	}
	m.refresh()
	return m.watchBg(msg.p)
}

// bgDrawn redraws for a background terminal that changed.
func (m *Model) bgDrawn(msg bgDrawnMsg) tea.Cmd {
	if msg.p.running() == -1 {
		return nil
	}
	m.refresh()
	if m.bgFocus >= 0 && m.bgPanes[m.bgFocus] == msg.p {
		m.follow(m.bgFocus)
	}
	return m.watchBg(msg.p)
}

// bgKey sends k to the background terminal with the focus, or with
// ctrl+g gives the focus back to the file. It reports whether it took
// the key.
func (m *Model) bgKey(k tea.KeyMsg) bool {
	if m.bgFocus == -1 {
		return false
	}
	p := m.bgPanes[m.bgFocus]
	if p == nil {
		m.bgFocus = -1
		return false
	}
	if k.String() == "ctrl+g" {
		m.status = m.name(m.d.Blocks[m.bgFocus]) + " runs on in the background; t types in it again, enter stops it"
		m.bgFocus = -1
		m.refresh()
		return true
	}
	p.typeKey(k)
	return true
}

// focusBg gives the keys to the selected block's background terminal.
func (m *Model) focusBg() {
	i := m.cur()
	if i < 0 || m.bgPanes[i] == nil {
		m.status = "t types in a background tty block's terminal, and this block has none running"
		return
	}
	m.bgFocus = i
	m.follow(i)
	m.refresh()
}

// bgPaneView draws background block i's terminal in its card: in the
// running color while it has the keys, dimmer while it doesn't.
func (m *Model) bgPaneView(i int) string {
	p := m.bgPanes[i]
	if p == nil {
		return ""
	}
	col := runningCol
	if m.bgFocus != i {
		col = dimCol
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(col).Render(p.screen())
}
