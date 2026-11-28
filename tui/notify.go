package tui

import (
	"context"
	"errors"
	"path/filepath"
	"time"
)

// mdsh -i asks the terminal for a desktop notification (package
// termnotify, Options.Notify) when someone who looked away is wanted: a
// confirm prompt or a question waits for an answer, or a run that took
// notifyAfter or longer ends. Either only once no key has been pressed
// for notifyAfter, so a prompt that comes up right after the key that
// ran its block, or a run watched to its end, notifies nobody.

// notifyAfter is how long a run must take, and how long since the last
// key, before mdsh -i notifies. Tests change it.
var notifyAfter = 10 * time.Second

// notifyTitle is the notification's title: the file's name.
func (m *Model) notifyTitle() string { return filepath.Base(m.opts.Run.File) }

// notifyWaiting notifies that on, a confirm prompt or a question, asks
// msg, once for each.
func (m *Model) notifyWaiting(msg string, on any) {
	if on == m.notifiedWait {
		return
	}
	m.notifiedWait = on
	n := m.opts.Notify
	if on == nil || n == nil || n.Idle() < notifyAfter {
		return
	}
	n.Notify(m.notifyTitle(), msg)
}

// notifyEnded notifies that a long run ended, and how, unless the user
// stopped it.
func (m *Model) notifyEnded(err error) {
	n := m.opts.Notify
	if n == nil || m.runStart.IsZero() || errors.Is(err, context.Canceled) {
		return
	}
	took := time.Since(m.runStart)
	if took < notifyAfter || n.Idle() < notifyAfter {
		return
	}
	var msg string
	switch {
	case err != nil:
		msg = "Stopped: " + err.Error()
	case m.runFailed:
		msg = m.runFailure
	default:
		msg = "Finished in " + runTime(took.Round(time.Second))
	}
	n.Notify(m.notifyTitle(), msg)
}
