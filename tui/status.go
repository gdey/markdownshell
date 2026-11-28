package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/gdey/markdownshell/run"
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
