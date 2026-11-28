package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termstatus"
)

// The terminal mdsh runs in is told what the run is doing, with the
// Program Status Protocol (OSC 7501, package termstatus): working, with
// how far through the blocks it is, blocked while a question waits on the
// terminal, and done or error as it exits, which the terminal keeps
// until the user looks. The reports go to /dev/tty, as the questions do,
// so a run whose output is piped or written to the file still reports.
// A terminal that doesn't know the sequence ignores it.

// trackStatus wraps the hooks in opts that say where the run is, so st
// follows along. It returns what to call once the run has ended.
func trackStatus(d *doc.Document, opts *run.Options, st *termstatus.Tracker) (finish func(results []*run.Result, err error)) {
	if st == nil {
		return func([]*run.Result, error) {}
	}
	// Progress is blocks finished over blocks that would run; with
	// --only it is anyone's guess, so none is given.
	total := 0
	if opts.Only == "" {
		for _, b := range d.Blocks {
			if run.Runnable(b) && !b.Attrs.Has("skip") {
				total++
			}
		}
	}
	var done atomic.Int32
	progress := func() int {
		if total == 0 {
			return termstatus.NoProgress
		}
		return int(done.Load()) * 100 / total
	}
	label := func(b *doc.Block) string { return d.Label(b) }

	if confirm := opts.Confirm; confirm != nil {
		opts.Confirm = func(b *doc.Block) run.Answer {
			st.Blocked(termstatus.Permission, "Run "+label(b)+"?")
			defer st.Unblocked()
			return confirm(b)
		}
	}
	if depend := opts.Depend; depend != nil {
		opts.Depend = func(dep, b *doc.Block) run.Answer {
			st.Blocked(termstatus.Permission, "Run "+label(dep)+" first, for "+label(b)+"?")
			defer st.Unblocked()
			return depend(dep, b)
		}
	}
	if dependOff := opts.DependOff; dependOff != nil {
		opts.DependOff = func(dep, b *doc.Block, why string) run.Answer {
			st.Blocked(termstatus.Permission, "Run "+label(dep)+" first, for "+label(b)+"? It is switched off")
			defer st.Unblocked()
			return dependOff(dep, b, why)
		}
	}
	if sources := opts.ApproveSources; sources != nil {
		opts.ApproveSources = func(paths []string) run.Answer {
			st.Blocked(termstatus.Permission, "Read the files env.source names?")
			defer st.Unblocked()
			return sources(paths)
		}
	}
	if ask := opts.Ask; ask != nil {
		opts.Ask = func(ctx context.Context, q run.Question) (string, run.Answered) {
			kind := termstatus.Question
			if q.Secret {
				kind = termstatus.Auth
			}
			st.Blocked(kind, label(q.Block)+": "+q.Text)
			defer st.Unblocked()
			return ask(ctx, q)
		}
	}
	next := opts.OnEvent
	opts.OnEvent = func(e run.Event) {
		switch e.Kind {
		case run.Started:
			st.Working(progress(), label(e.Block))
		case run.Status:
			p := progress()
			if e.Progress.Of > 0 {
				p = e.Progress.Step * 100 / e.Progress.Of
			}
			st.Working(p, label(e.Block)+": "+e.Progress.Text)
		case run.Done:
			done.Add(1)
		}
		if next != nil {
			next(e)
		}
	}
	return func(results []*run.Result, err error) {
		switch {
		case errors.Is(err, context.Canceled):
			// Interrupted: the user is here, and there is no result.
			st.Idle()
			return
		case err != nil:
			st.Error(err.Error())
			return
		}
		ran := 0
		for _, r := range results {
			if r.Failed() {
				st.Error(fmt.Sprintf("%s failed, exit %d", label(r.Block), r.ExitStatus()))
				return
			}
			if r.Ran {
				ran++
			}
		}
		switch ran {
		case 0:
			st.Done("nothing ran")
		case 1:
			st.Done("1 block ran")
		default:
			st.Done(fmt.Sprintf("%d blocks ran", ran))
		}
	}
}
