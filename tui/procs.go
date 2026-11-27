package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/termsafe"
)

// The processes panel, which P opens: each of the file's shells, with
// the processes under it as a tree (blocks running, background blocks,
// and whatever a block or the History pane left running, such as
// cmd &), how long each has run, and x to kill the one picked. It is
// read again every second while it is up.

// procRow is one line of the panel: a shell, or a process under one.
type procRow struct {
	depth int
	pid   int
	shell bool // a shell, which x doesn't kill
	text  string
	secs  int
}

// procsMsg is the tick that reads the processes again.
type procsMsg struct{ m *Model }

const procsEvery = time.Second

func (m *Model) openProcs() tea.Cmd {
	m.procsOpen, m.procsAt = true, 0
	m.readProcs()
	// Start on the first process rather than its shell.
	if len(m.procs) > 1 {
		m.procsAt = 1
	}
	return m.procsTick()
}

func (m *Model) procsTick() tea.Cmd {
	return tea.Tick(procsEvery, func(time.Time) tea.Msg { return procsMsg{m} })
}

// readProcs lists the processes again, keeping the pick on the same
// process if it still runs.
func (m *Model) readProcs() {
	was := 0
	if m.procsAt < len(m.procs) {
		was = m.procs[m.procsAt].pid
	}
	m.procs = m.procs[:0]
	for _, sh := range m.sess.Processes() {
		name := "shell"
		if sh.Env != "" {
			name = "env " + sh.Env
		}
		m.procs = append(m.procs, procRow{pid: sh.PID, shell: true, text: name, secs: sh.Secs})
		m.addProcs(sh.Children, 1)
	}
	m.procsAt = min(m.procsAt, max(len(m.procs)-1, 0))
	for i, r := range m.procs {
		if r.pid == was {
			m.procsAt = i
		}
	}
}

func (m *Model) addProcs(ps []*env.Proc, depth int) {
	for _, p := range ps {
		text := termsafe.Visible(p.Command)
		if p.Block != "" {
			text = "◐ " + termsafe.Visible(p.Block) + "  " + text
			if s := statusWords(m.blockRecordsByLabel(p.Block)); s != "" {
				text += "  " + s
			}
		}
		m.procs = append(m.procs, procRow{depth: depth, pid: p.PID, text: text, secs: p.Secs})
		m.addProcs(p.Children, depth+1)
	}
}

// procsKey handles a key while the panel is up: ↑/↓ pick a process, x
// kills it, and esc, q or P closes the panel.
func (m *Model) procsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	last := max(len(m.procs)-1, 0)
	switch k.String() {
	case "esc", "q", "P", "ctrl+c":
		m.procsOpen = false
	case "j", "down", "tab", "ctrl+n":
		m.procsAt = min(m.procsAt+1, last)
	case "k", "up", "shift+tab", "ctrl+p":
		m.procsAt = max(m.procsAt-1, 0)
	case "g", "home":
		m.procsAt = 0
	case "G", "end":
		m.procsAt = last
	case "x", "delete":
		switch {
		case m.procsAt >= len(m.procs):
		case m.procs[m.procsAt].shell:
			m.status = "a shell isn't killed here; R restarts the shells"
		case m.sess.KillProcess(m.procs[m.procsAt].pid):
			m.status = fmt.Sprintf("killed %d", m.procs[m.procsAt].pid)
			m.readProcs()
		default:
			m.status = fmt.Sprintf("%d had already ended", m.procs[m.procsAt].pid)
			m.readProcs()
		}
	}
	m.refresh()
	return m, nil
}

// runFor says how long a process has run: 42s, 3m 07s, 2h 05m, 4d 3h.
func runFor(secs int) string {
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm %02ds", secs/60, secs%60)
	case secs < 86400:
		return fmt.Sprintf("%dh %02dm", secs/3600, secs/60%60)
	}
	return fmt.Sprintf("%dd %dh", secs/86400, secs/3600%24)
}

var procPIDStyle = lipgloss.NewStyle().Foreground(dimCol)

// procsView draws the panel w wide and h high, scrolled so the pick
// shows.
func (m *Model) procsView(w, h int) string {
	lines := []string{titleStyle.Render("Processes") + helpStyle.Render("  what runs under this file's shells, and for how long"), ""}
	if len(m.procs) == 0 {
		lines = append(lines, helpStyle.Render("  No shell runs for this file yet. Once a block runs, its shell shows here."))
	}
	timeW := 7
	at := 0
	for n, r := range m.procs {
		mark := "  "
		if n == m.procsAt {
			mark = pickedStyle.Render("▸ ")
			at = len(lines)
		}
		left := mark + strings.Repeat("  ", r.depth) + procPIDStyle.Render(fmt.Sprint(r.pid)) + " "
		text := r.text
		switch {
		case r.shell:
			text = helpKeyStyle.Render(text)
		case n == m.procsAt:
			text = pickedStyle.Render(text)
		}
		t := runFor(r.secs)
		room := w - timeW - 1
		line := truncate(left+text, room)
		pad := max(room-lipgloss.Width(line), 0)
		lines = append(lines, line+strings.Repeat(" ", pad)+" "+helpStyle.Render(fmt.Sprintf("%*s", timeW, t)))
	}
	top := 0
	if len(lines) > h {
		top = min(max(at-h/2, 0), len(lines)-h)
		lines = lines[top : top+h]
	}
	// The panel fills the document's place, so the footer stays put.
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
