package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/termrec"
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
	tty      bool // whether the shells have terminals of their own
	jobs     []*bgJob
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
	init, err := sources(d, opts)
	if err != nil {
		return nil, err
	}
	s := &Session{
		d:        d,
		opts:     opts,
		selected: selected,
		stack: env.NewStack(env.Config{
			Path: shell,
			Env:  env.Environ(env.Filter(env.Vars(opts.Environ), d.Options.Env.Share)),
			Dir:  opts.Dir,
			Args: opts.Args,
			TTY:  hasTTY(d) || opts.PTY,
			Init: init,
		}),
		tty: hasTTY(d) || opts.PTY,
	}
	// Includes are part of the document, not steps of the run, so they
	// are read whatever runs.
	s.results = Includes(d, opts)
	s.refs()
	return s, nil
}

// sources reads the files the front matter's env.source names into one
// script, followed by opts.Source.
func sources(d *doc.Document, opts Options) (string, error) {
	base := opts.Dir
	if opts.File != "" {
		base = filepath.Dir(opts.File)
	}
	var scripts []string
	for _, p := range d.Options.Env.Source {
		if rest, ok := strings.CutPrefix(p, "~/"); ok {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("env.source %s: %w", p, err)
			}
			p = filepath.Join(home, rest)
		} else if !filepath.IsAbs(p) && base != "" {
			p = filepath.Join(base, p)
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("env.source: %w", err)
		}
		scripts = append(scripts, string(src))
	}
	return joinScripts(append(scripts, opts.Source)...), nil
}

// joinScripts joins scripts, ending each with a newline.
func joinScripts(scripts ...string) string {
	var b strings.Builder
	for _, s := range scripts {
		b.WriteString(s)
		if s != "" && !strings.HasSuffix(s, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// refs notes the variables other blocks read from each block and readies
// the lookup for them.
func (s *Session) refs() {
	refs := s.d.Refs()
	for _, r := range s.results {
		if r.Block.IsTape() && r.Ran {
			refs = append(refs, doc.TextRefs(string(r.Stdout))...)
		}
	}
	s.wanted = map[string][]string{}
	for _, ref := range refs {
		if ref.BlockID != "" && !ref.Fact() {
			s.wanted[ref.BlockID] = append(s.wanted[ref.BlockID], ref.Var)
		}
	}
	s.lookup = NewLookup(s.d, s.results, s.opts.Environ)
}

// Results returns one result per block, in order. A block run again gets
// a new result, so hold on to the slice only while nothing runs.
func (s *Session) Results() []*Result { return s.results }

// ErrRestart is Sync's answer when the document gained the first tty
// block after the session's shells started without terminals of their
// own.
var ErrRestart = errors.New("the first tty block needs mdsh to start again")

// Sync catches the session up with changes to the document since it
// started: blocks appended, as by doc.AppendBlock, blocks edited in
// place, as by doc.Document.SetBlock, and a new source, as by
// doc.Document.SetSource. A block that kept its identity keeps its
// result, wherever it moved; a new block starts with none. Call it
// while nothing runs.
func (s *Session) Sync() error {
	byBlock := make(map[*doc.Block]*Result, len(s.results))
	for _, r := range s.results {
		byBlock[r.Block] = r
	}
	results := make([]*Result, len(s.d.Blocks))
	for i, b := range s.d.Blocks {
		if r, ok := byBlock[b]; ok {
			results[i] = r
			continue
		}
		results[i] = &Result{Block: b}
		if _, ok := s.selected[b]; !ok {
			s.selected[b] = true
		}
	}
	s.results = results
	s.refs()
	if hasTTY(s.d) && !s.tty {
		return ErrRestart
	}
	return nil
}

// PTY reports whether the session's shells have terminals of their own,
// which RunTyped needs.
func (s *Session) PTY() bool { return s.tty }

// Close ends the session's shells, and any background block still
// running, which it reports as killed.
func (s *Session) Close() {
	for _, j := range s.jobs {
		j.job.Kill()
	}
	s.Reap()
	s.stack.Close()
}

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
		s.Reap()
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
	return s.Wait(ctx)
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
	s.Reap()
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

// RunTyped runs block i on Terminal, as if it had been typed there: what
// is typed in the terminal goes to it, and it shows what it writes. A
// block that switched to the terminal's alternate screen, as vim or top
// do, gets its session as a recording, the way a tty block does, and
// RunTyped reports it as full screen. Any other block gets the text it
// showed as its stdout. The session needs Options.PTY, or a tty block,
// and Options.Terminal.
func (s *Session) RunTyped(ctx context.Context, i int) (r *Result, fullScreen bool, err error) {
	if i < 0 || i >= len(s.d.Blocks) {
		return nil, false, fmt.Errorf("no block %d", i)
	}
	b := s.d.Blocks[i]
	if !Runnable(b) {
		return s.results[i], false, fmt.Errorf("mdsh can't run %s blocks", b.Lang)
	}
	if s.opts.Terminal == nil {
		return s.results[i], false, ErrNoTTY
	}
	s.Reap()
	r = &Result{Block: b, Ran: true}
	s.results[i] = r
	s.emit(Event{Kind: Started, Block: b})
	defer func() { s.emit(Event{Kind: Done, Block: b, Result: r}) }()
	e, err := envFor(ctx, s.stack, s.d, b)
	if err != nil {
		r.Err = err
		return r, false, nil
	}
	t := *s.opts.Terminal
	rec := termrec.NewRecorder(t.Dims())
	giveBack := takeTTY(b, s.opts)
	r.Status, r.Exited, err = e.Shell.RunTerminal(ctx, prelude(b, s.opts)+b.Body, t, rec)
	giveBack()
	if err != nil && !errors.Is(err, env.ErrExited) {
		r.Err = err
	}
	shown := rec.Recording()
	if !shown.FullScreen() {
		r.Stdout = []byte(shown.Text())
		return r, false, ctx.Err()
	}
	img, err := saveRecording(b, shown, s.opts)
	r.Image = img
	if r.Err == nil {
		r.Err = err
	}
	return r, true, ctx.Err()
}

func (s *Session) confirm(b *doc.Block) (Answer, error) {
	if s.opts.Confirm == nil {
		return Stop, fmt.Errorf("%w: %s", ErrNoConfirm, b.ID)
	}
	return s.opts.Confirm(b), nil
}

func (s *Session) run(ctx context.Context, b *doc.Block, r *Result) {
	s.emit(Event{Kind: Started, Block: b})
	if b.Attrs.Has("background") {
		s.background(ctx, b, r)
		return
	}
	runBlock(ctx, s.stack, s.d, b, r, s.opts, s.emit, s.wanted[b.ID], tapeFor(s.d, s.results, b), LookupAt(s.lookup, s.d, s.results, b.Span.Start))
	s.emit(Event{Kind: Done, Block: b, Result: r})
}
