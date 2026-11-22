package run

import (
	"context"
	"fmt"
	"strings"

	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/termrec"
)

// TopShell is the document's top-level shell, started if it isn't
// running. It is nil, with no error, when that shell isn't
// interactive, as only an interactive one is typed at directly.
func (s *Session) TopShell(ctx context.Context) (*env.Shell, error) {
	return s.Shell(ctx, env.TopLevel)
}

// Shell is env name's shell, started if it isn't running, as TopShell
// is the top level's: nil, with no error, when it isn't interactive.
// A sub-env runs interactively once SetInteractive names it.
func (s *Session) Shell(ctx context.Context, name string) (*env.Shell, error) {
	if err := s.checkFrontMatter(); err != nil {
		return nil, err
	}
	e, err := s.stack.Get(ctx, name, s.envShare(name))
	if err != nil {
		return nil, err
	}
	if !e.Shell.Interactive() {
		return nil, nil
	}
	return e.Shell, nil
}

// envShare is the share list env name's shell is made with when
// something other than a block starts it: that of the first block that
// runs there, as it would have been made by that block, else the front
// matter's.
func (s *Session) envShare(name string) []string {
	name = strings.Trim(name, "/")
	if name != env.TopLevel {
		for _, b := range s.d.Blocks {
			if strings.Trim(b.Attrs["env"], "/") == name {
				if share := b.Attrs.List("share"); share != nil {
					return share
				}
				break
			}
		}
	}
	return s.d.Options.Env.Share
}

// SetInteractive has env name run interactively from its next start,
// when the top level does, so the History pane can be a terminal on
// it. A scripted shell running for it now is ended, and what blocks
// set there is lost: ask first, with Scripted. It reports whether one
// was ended.
func (s *Session) SetInteractive(name string) bool {
	return s.stack.SetInteractive(name)
}

// Interactive reports whether env name runs, or will run,
// interactively: the top level with Options.InteractiveShell, a
// sub-env once SetInteractive names it.
func (s *Session) Interactive(name string) bool {
	return s.stack.Interactive(name)
}

// Scripted reports whether a scripted shell runs for env name now, so
// SetInteractive would end it.
func (s *Session) Scripted(name string) bool {
	return s.stack.Scripted(name)
}

// Envs lists the envs the document's blocks name, each after its
// parents and in the order the file first names them, with the top
// level, "", first.
func (s *Session) Envs() []string {
	out := []string{env.TopLevel}
	seen := map[string]bool{env.TopLevel: true}
	for _, b := range s.d.Blocks {
		name := strings.Trim(b.Attrs["env"], "/")
		if name == "" {
			continue
		}
		parts := strings.Split(name, "/")
		for i := range parts {
			n := strings.Join(parts[:i+1], "/")
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
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
