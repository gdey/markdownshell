package run

import (
	"context"
	"fmt"

	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/termrec"
)

// TopShell is the document's top-level shell, started if it isn't
// running. It is nil, with no error, when that shell isn't
// interactive, as only an interactive one is typed at directly.
func (s *Session) TopShell(ctx context.Context) (*env.Shell, error) {
	if err := s.checkFrontMatter(); err != nil {
		return nil, err
	}
	e, err := s.stack.Get(ctx, "", s.d.Options.Env.Share)
	if err != nil {
		return nil, err
	}
	if !e.Shell.Interactive() {
		return nil, nil
	}
	return e.Shell, nil
}

// TypedStart says block i is a command typed at the top-level shell's
// own prompt, which has started: the shell runs it, not the session.
// It is logged; started sends its Started event, which the caller does
// once it no longer holds what OnEvent takes, as the lock that keeps
// the document from changing under TypedStart. TypedEnd ends it.
func (s *Session) TypedStart(i int) (started func(), err error) {
	if i < 0 || i >= len(s.d.Blocks) {
		return nil, fmt.Errorf("no block %d", i)
	}
	b := s.d.Blocks[i]
	r := &Result{Block: b, Ran: true, Running: true, Start: s.now()}
	s.results[i] = r
	s.rotateAssets(b)
	s.logStart(b)
	return func() { s.emit(Event{Kind: Started, Block: b}) }, nil
}

// TypedEnd ends block i, which TypedStart started, with the status the
// shell gave it and what it showed on the terminal. A command that took
// the screen gets the recording, as RunTyped's does, and reports it;
// any other gets the text it showed as its stdout. It is logged; done
// sends its Done event, as TypedStart's started does.
func (s *Session) TypedEnd(i int, status int, shown termrec.Recording) (r *Result, fullScreen bool, done func(), err error) {
	if i < 0 || i >= len(s.d.Blocks) {
		return nil, false, nil, fmt.Errorf("no block %d", i)
	}
	b := s.d.Blocks[i]
	r = s.results[i]
	if r == nil || r.Block != b {
		r = &Result{Block: b, Ran: true, Start: s.now()}
		s.results[i] = r
	}
	r.Running = false
	r.Status = status
	if status < 0 {
		r.Status = 0
	}
	if !shown.FullScreen() {
		r.Stdout = []byte(shown.Text())
	} else {
		fullScreen = true
		r.Image, r.Err = saveRecording(b, shown, s.opts)
	}
	s.logEnd(b, r, r.Start)
	return r, fullScreen, func() { s.emit(Event{Kind: Done, Block: b, Result: r}) }, nil
}
