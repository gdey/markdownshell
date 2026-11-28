package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	kind, msg, on := m.waitingOn()
	m.notifyWaiting(msg, on)
	st := m.opts.Status
	if st == nil {
		return
	}
	if on == nil {
		st.Unblocked()
		return
	}
	st.Blocked(kind, msg)
}

// waitingOn is what the file waits on, as statusAsking reports it: the
// kind of answer, what is asked, and the prompt or question itself; on
// is nil when nothing is asked.
func (m *Model) waitingOn() (kind termstatus.Kind, msg string, on any) {
	if p := m.prompt; p != nil {
		switch {
		case p.of != nil && p.why != "":
			msg = "Run " + m.d.Label(p.block) + " first, for " + m.d.Label(p.of) + "? It is switched off"
		case p.of != nil:
			msg = "Run " + m.d.Label(p.block) + " first, for " + m.d.Label(p.of) + "?"
		default:
			msg = "Run " + m.d.Label(p.block) + "?"
		}
		return termstatus.Permission, msg, p
	}
	if a := m.asking(); a != nil {
		kind = termstatus.Question
		if a.q.Secret {
			kind = termstatus.Auth
		}
		return kind, m.d.Label(a.q.Block) + ": " + a.q.Text, a
	}
	return "", "", nil
}

// statusStarted tells the terminal a run started, before its first
// block does.
func (m *Model) statusStarted() {
	m.runFailed = false
	m.runStart = time.Now()
	m.opts.Status.Working(termstatus.NoProgress, "")
}

// statusEvent follows the run's events: each block starting, and what
// a block says with mdsh ctl status.
func (m *Model) statusEvent(e run.Event) {
	if e.Kind == run.Done && e.Result.Failed() {
		// For the notification too, so kept without a tracker.
		m.runFailed = true
		m.runFailure = fmt.Sprintf("%s failed, exit %d", m.d.Label(e.Block), e.Result.ExitStatus())
	}
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
