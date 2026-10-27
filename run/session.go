package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
)

// Session runs a document's blocks, keeping each env's shell alive from
// one run to the next. Run uses one to run the whole document; an
// interactive front end uses one to run blocks as they are picked.
//
// A Session runs one thing at a time: don't call RunFrom or RunBlock
// while another call is running.
type Session struct {
	d        *doc.Document
	opts     Options
	stack    *env.Stack
	selected map[*doc.Block]bool
	results  []*Result
	lookup   doc.Lookup
	wanted   map[string][]string
}

// NewSession reads d's include blocks and readies its envs. No shell is
// started until a block runs in it. Close the session when done.
func NewSession(d *doc.Document, opts Options) (*Session, error) {
	if opts.Environ == nil {
		opts.Environ = os.Environ()
	}
	if opts.Dir == "" && opts.File != "" {
		opts.Dir = filepath.Dir(opts.File)
	}
	shell := opts.Shell
	if shell == "" {
		shell = Shell(d)
	}
	selected, err := selectBlocks(d, opts.Only)
	if err != nil {
		return nil, err
	}
	s := &Session{
		d:        d,
		opts:     opts,
		selected: selected,
		wanted:   map[string][]string{},
		stack: env.NewStack(env.Config{
			Path: shell,
			Env:  env.Environ(env.Filter(env.Vars(opts.Environ), d.Options.Env.Share)),
			Dir:  opts.Dir,
			Args: opts.Args,
			TTY:  hasTTY(d),
		}),
	}
	// Includes are part of the document, not steps of the run, so they
	// are read whatever runs.
	s.results = Includes(d, opts)
	refs := d.Refs()
	for _, r := range s.results {
		if r.Block.IsTape() && r.Ran {
			refs = append(refs, doc.TextRefs(string(r.Stdout))...)
		}
	}
	for _, ref := range refs {
		if ref.BlockID != "" && !ref.Fact() {
			s.wanted[ref.BlockID] = append(s.wanted[ref.BlockID], ref.Var)
		}
	}
	s.lookup = NewLookup(d, s.results, opts.Environ)
	return s, nil
}

// Results returns one result per block, in order. A block run again gets
// a new result, so hold on to the slice only while nothing runs.
func (s *Session) Results() []*Result { return s.results }

// Close ends the session's shells.
func (s *Session) Close() { s.stack.Close() }

func (s *Session) emit(e Event) {
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(e)
	}
}

// RunFrom runs the blocks from index from to the end, the way Run does:
// it honors skip, run=, Only, confirm and step mode, and stops when a
// block ends its shell or fails with on_error stop. Blocks it runs again
// get new results.
func (s *Session) RunFrom(ctx context.Context, from int) error {
	d, opts := s.d, s.opts
	for i := from; i < len(d.Blocks); i++ {
		b := d.Blocks[i]
		if b.IsInclude() {
			if r := s.results[i]; r.Ran {
				s.emit(Event{Kind: Done, Block: b, Result: r})
			}
			continue
		}
		r := &Result{Block: b}
		if !Runnable(b) || b.Attrs.Has("skip") || !s.selected[b] {
			r.Skipped = true
			s.results[i] = r
			continue
		}
		if c, ok := b.Cond("run"); ok && !c.Eval(LookupAt(s.lookup, d, s.results, b.Span.Start)) {
			r.Skipped = true
			s.results[i] = r
			continue
		}
		if b.Attrs.Has("confirm") || opts.Step || d.Options.Step {
			a, err := s.confirm(b)
			if err != nil || a == Stop {
				s.results[i].Declined = true
				return err
			}
			if a == Skip {
				r.Skipped = true
				s.results[i] = r
				continue
			}
		}
		s.results[i] = r
		s.run(ctx, b, r)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if r.Exited || (r.Failed() && stopOnError(d, b, opts)) {
			break
		}
	}
	return nil
}

// RunBlock runs block i alone and returns its new result. It runs the
// block even if it is marked skip, its run= condition is false or Only
// leaves it out, since running it is what was asked for. A confirm block
// is still confirmed first; if the answer is not Go, the old result
// stays and RunBlock returns it. An include block reads its file again.
// It returns an error if the block can't run at all.
func (s *Session) RunBlock(ctx context.Context, i int) (*Result, error) {
	if i < 0 || i >= len(s.d.Blocks) {
		return nil, fmt.Errorf("no block %d", i)
	}
	b := s.d.Blocks[i]
	r := &Result{Block: b}
	switch {
	case b.IsInclude():
		includeBlock(b, r, s.opts.Dir)
		s.results[i] = r
		s.emit(Event{Kind: Done, Block: b, Result: r})
		return r, nil
	case !Runnable(b):
		return s.results[i], fmt.Errorf("mdsh can't run %s blocks", b.Lang)
	}
	if b.Attrs.Has("confirm") {
		a, err := s.confirm(b)
		if err != nil {
			return s.results[i], err
		}
		if a != Go {
			return s.results[i], nil
		}
	}
	s.results[i] = r
	s.run(ctx, b, r)
	return r, ctx.Err()
}

func (s *Session) confirm(b *doc.Block) (Answer, error) {
	if s.opts.Confirm == nil {
		return Stop, fmt.Errorf("%w: %s", ErrNoConfirm, b.ID)
	}
	return s.opts.Confirm(b), nil
}

func (s *Session) run(ctx context.Context, b *doc.Block, r *Result) {
	s.emit(Event{Kind: Started, Block: b})
	runBlock(ctx, s.stack, s.d, b, r, s.opts, s.emit, s.wanted[b.ID], tapeFor(s.d, s.results, b), LookupAt(s.lookup, s.d, s.results, b.Span.Start))
	s.emit(Event{Kind: Done, Block: b, Result: r})
}
