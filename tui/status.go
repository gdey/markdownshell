package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termsafe"
	"github.com/gdey/markdownshell/termstatus"
)

// The terminal mdsh -i runs in is told what it is doing, with the
// Program Status Protocol (OSC 7501, package termstatus): working while
// a block runs, blocked while a card asks a question, done or error once
// the run ends, and idle again at the next key, since the result has
// been seen then. Options.Status is the tracker, shared by every tab.

// statusAsking tells the terminal what the file waits on: the confirm
// prompt, else the question being asked, else nothing, so the run is
// working again.
func (m *Model) statusAsking() {
	st := m.opts.Status
	if st == nil {
		return
	}
	if p := m.prompt; p != nil {
		switch {
		case p.of != nil && p.why != "":
			st.Blocked(termstatus.Permission, "Run "+m.d.Label(p.block)+" first, for "+m.d.Label(p.of)+"? It is switched off")
		case p.of != nil:
			st.Blocked(termstatus.Permission, "Run "+m.d.Label(p.block)+" first, for "+m.d.Label(p.of)+"?")
		default:
			st.Blocked(termstatus.Permission, "Run "+m.d.Label(p.block)+"?")
		}
		return
	}
	if a := m.asking(); a != nil {
		kind := termstatus.Question
		if a.q.Secret {
			kind = termstatus.Auth
		}
		st.Blocked(kind, m.d.Label(a.q.Block)+": "+a.q.Text)
		return
	}
	st.Unblocked()
}

// statusStarted tells the terminal a run started, before its first
// block does.
func (m *Model) statusStarted() {
	m.runFailed = false
	m.opts.Status.Working(termstatus.NoProgress, "")
}

// statusEvent follows the run's events: each block starting, and what
// a block says with mdsh ctl status.
func (m *Model) statusEvent(e run.Event) {
	st := m.opts.Status
	if st == nil {
		return
	}
	switch e.Kind {
	case run.Started:
		st.Working(termstatus.NoProgress, m.d.Label(e.Block))
	case run.Status:
		p := termstatus.NoProgress
		if e.Progress.Of > 0 {
			p = e.Progress.Step * 100 / e.Progress.Of
		}
		st.Working(p, m.d.Label(e.Block)+": "+e.Progress.Text)
	case run.Done:
		if e.Result.Failed() {
			m.runFailed = true
			m.runFailure = fmt.Sprintf("%s failed, exit %d", m.d.Label(e.Block), e.Result.ExitStatus())
		}
	}
}

// statusEnded tells the terminal how the run ended.
func (m *Model) statusEnded(err error) {
	st := m.opts.Status
	if st == nil {
		return
	}
	switch {
	case errors.Is(err, context.Canceled):
		// Stopped by the user, who is here.
		st.Idle()
	case err != nil:
		st.Error(err.Error())
	case m.runFailed:
		st.Error(m.runFailure)
	default:
		st.Done("")
	}
}

// statusSeen notes a sign of life from the user: a result on show has
// been seen.
func (m *Model) statusSeen() {
	if st := m.opts.Status; st.Settled() {
		st.Idle()
	}
}

// The programs blocks run report what they are doing too, on the
// terminals mdsh -i draws for them: a tty block's pane, a background tty
// block's, the History pane's shell (env.Terminal.Status, and the
// shell's "status" events). Their records show on the block's card, the
// History bar and the Processes panel, and go up to the terminal mdsh -i
// runs in as children of its own record, so an inbox there reads
// "mdsh › terraform: blocked".

// statusMsg says what the program in p reports changed.
type statusMsg struct{ p *pane }

// onStatus takes a change in what the program in p reports: the records
// are drawn with the block, and sent up under the block's name.
func (m *Model) onStatus(p *pane) {
	key := "tty"
	if i := p.running(); i >= 0 && i < len(m.d.Blocks) {
		key = termstatus.Name(m.d.Label(m.d.Blocks[i]))
	}
	m.forwardStatus(p, key, p.records())
}

// liveStatusKey names the History shell's records for the terminal
// mdsh -i runs in: the History block of the command running, else the
// shell.
func (m *Model) liveStatusKey() string {
	lt := m.lterm
	if lt.cur >= 0 && lt.cur < len(m.d.Blocks) {
		return termstatus.Name(m.d.Label(m.d.Blocks[lt.cur]))
	}
	return "history"
}

// forwardStatus sends recs, what the program in p reports, up to the
// terminal mdsh -i runs in as records under key, and clears those that
// went since last time. nil recs says the program ended.
func (m *Model) forwardStatus(p *pane, key string, recs []termstatus.Report) {
	st := m.opts.Status
	if st == nil {
		return
	}
	had := m.forwarded[p]
	now := map[string]bool{}
	for _, r := range recs {
		id := key
		if r.ID != "" {
			id += "/" + r.ID
		}
		now[id] = true
		st.Child(key, r)
	}
	for id := range had {
		if !now[id] {
			st.ClearChild(id)
		}
	}
	if len(now) == 0 {
		delete(m.forwarded, p)
	} else {
		m.forwarded[p] = now
	}
}

// blockRecords is what the program running in block i's terminal
// reports about itself, or nil.
func (m *Model) blockRecords(i int) []termstatus.Report {
	if m.pane.running() == i {
		return m.pane.records()
	}
	if p := m.bgPanes[i]; p != nil {
		return p.records()
	}
	return nil
}

// blockRecordsByLabel is blockRecords for the block labelled label, as
// the Processes panel names blocks.
func (m *Model) blockRecordsByLabel(label string) []termstatus.Report {
	for i, b := range m.d.Blocks {
		if m.d.Label(b) == label {
			return m.blockRecords(i)
		}
	}
	return nil
}

// statusChip is block i's program status for its header, after the
// running badge: " · working 42% building", " · waiting: permission
// Apply?", or "" with nothing reported.
func (m *Model) statusChip(i int) string {
	recs := m.blockRecords(i)
	words := statusWords(recs)
	if words == "" {
		return ""
	}
	col := runningCol
	switch statusRoot(recs).State {
	case termstatus.Error:
		col = failCol
	case termstatus.Done:
		col = okCol
	}
	return tagStyle.Render(" · ") + lipgloss.NewStyle().Foreground(col).Render(words)
}

// statusRoot is the record that stands for the program: the root, else
// the first.
func statusRoot(recs []termstatus.Report) termstatus.Report {
	for _, r := range recs {
		if r.ID == "" {
			return r
		}
	}
	if len(recs) > 0 {
		return recs[0]
	}
	return termstatus.Report{}
}

// statusWords says what a program reports, in words: "working 42%
// building", "waiting: permission Apply?", "✗ failed", "✓ done", with
// its app first when it names one. "" with nothing reported.
func statusWords(recs []termstatus.Report) string {
	r := statusRoot(recs)
	var s string
	switch r.State {
	case termstatus.Working:
		s = "working"
		if r.Progress >= 0 {
			s += fmt.Sprintf(" %d%%", r.Progress)
		}
	case termstatus.Blocked:
		s = "waiting"
		if r.Kind != "" {
			s += ": " + string(r.Kind)
		}
	case termstatus.Error:
		s = "✗ failed"
	case termstatus.Done:
		s = "✓ done"
	case termstatus.Idle:
		s = "idle"
	default:
		return ""
	}
	if r.App != "" {
		s = r.App + " " + s
	}
	if r.Msg != "" {
		s += " " + termsafe.Visible(r.Msg)
	}
	return s
}
