package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	tty      bool // whether the shells have terminals of their own
	jobs     []*bgJob
}

// NewSession reads d's include blocks and readies its envs. No shell is
// started until a block runs in it. Close the session when done.
func NewSession(d *doc.Document, opts Options) (*Session, error) {
	if opts.Environ == nil {
		opts.Environ = os.Environ()
	}
	opts.Environ = WithMode(opts.Environ, mode(opts.Mode))
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
			TTY:  hasTTY(d),
			Init: init,
		}),
		tty: hasTTY(d),
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

// SetFile renames the document the session runs: blocks run from now on
// see file as MDSH_FILE and save their assets in <file>.assets. The
// shells and their working directory stay as they are.
func (s *Session) SetFile(file string) { s.opts.File = file }

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
// result, wherever it moved; a new block starts with none, except that
// a new include block is read. Call it
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
		if b.IsInclude() {
			results[i] = s.readInclude(b)
		}
	}
	s.results = results
	s.refs()
	if hasTTY(s.d) && !s.tty {
		return ErrRestart
	}
	return nil
}

// ReadIncludes reads every include block's file again, as after a run
// whose blocks may have written them. Call it while nothing runs.
func (s *Session) ReadIncludes() {
	for i, b := range s.d.Blocks {
		if b.IsInclude() {
			s.results[i] = s.readInclude(b)
		}
	}
}

// readInclude reads include block b's file, or skips it with skip.
func (s *Session) readInclude(b *doc.Block) *Result {
	r := &Result{Block: b}
	if b.Attrs.Has("skip") {
		r.Skipped = true
		return r
	}
	includeBlock(b, r, s.opts)
	return r
}

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
// it honors skip, run= (once included), Only, confirm, depends= and step mode, and stops when a
// block ends its shell or fails with on_error stop. Blocks it runs again
// get new results.
func (s *Session) RunFrom(ctx context.Context, from int) error {
	return s.RunRange(ctx, from, len(s.d.Blocks))
}

// RunRange runs blocks from up to but not including to, as RunFrom
// runs them: confirm, run=, skip, depends= and on_error all apply, and
// a block's dependencies run first even when they are outside the
// range. mdsh-gui uses it to run a section.
func (s *Session) RunRange(ctx context.Context, from, to int) error {
	d, opts := s.d, s.opts
	// ran holds the blocks already run as dependencies on the way, so
	// they don't run twice.
	ran := map[*doc.Block]bool{}
	for i := max(from, 0); i < min(to, len(d.Blocks)); i++ {
		s.Reap()
		b := d.Blocks[i]
		if ran[b] {
			continue
		}
		if b.IsInclude() {
			// Read it again: a block above may have just written it.
			r := s.readInclude(b)
			s.results[i] = r
			if r.Ran {
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
		if c, ok := b.Cond("run"); ok && (c.Once && s.hasOutput(i) || !c.Eval(LookupAt(s.lookup, d, s.results, b.Span.Start))) {
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
		if err := s.runDepends(ctx, b, ran); err != nil {
			var de *depError
			switch {
			case errors.Is(err, errStopped):
				s.results[i].Declined = true
				return s.Wait(ctx)
			case errors.As(err, &de):
				// As if the dependency had failed in place: the
				// block doesn't run, and on_error says whether the
				// run goes on.
				r.Skipped = true
				s.results[i] = r
				if stopOnError(d, de.dep, opts) {
					return s.Wait(ctx)
				}
				continue
			}
			if ctx.Err() == nil {
				s.results[i].Declined = true
			}
			return err
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

// hasOutput reports whether block i has output, for run=once: saved
// after it in the file, or from running earlier in this session.
func (s *Session) hasOutput(i int) bool {
	r := s.results[i]
	return s.d.Blocks[i].HasOutput() || r != nil && r.Ran
}

// RunBlock runs block i alone and returns its new result. It runs the
// block even if it is marked skip, its run= condition is false or Only
// leaves it out, since running it is what was asked for. A confirm block
// is still confirmed first; if the answer is not Go, the old result
// stays and RunBlock returns it. Then the blocks its depends= names are
// asked about and run, as Options.Depend says; a Stop there keeps the old
// result too, and a failed dependency returns ErrDependency. An include
// block reads its file again.
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
		includeBlock(b, r, s.opts)
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
	if err := s.runDepends(ctx, b, nil); err != nil {
		if errors.Is(err, errStopped) {
			err = nil
		}
		return s.results[i], err
	}
	s.results[i] = r
	s.run(ctx, b, r)
	return r, ctx.Err()
}

// errStopped is runDepends' error when the answer for a dependency was
// Stop.
var errStopped = errors.New("stopped")

// depError is runDepends' error when a dependency failed.
type depError struct {
	dep   *doc.Block
	label string // the block that didn't run
}

func (e *depError) Error() string { return fmt.Sprintf("%s didn't run: %s", e.label, ErrDependency) }
func (e *depError) Unwrap() error { return ErrDependency }

// runDepends readies b to run: in the order b's depends= lists them,
// each block that hasn't run in this session, or failed when it did, is
// asked about with Depend, and run if the answer is Go, after its own
// dependencies. A dependency the answer skips is left as it is. It
// returns errStopped if an answer was Stop, a *depError if a dependency
// failed or ended its shell, and ctx's error if ctx ended. ran, if not
// nil, gets each block it runs.
func (s *Session) runDepends(ctx context.Context, b *doc.Block, ran map[*doc.Block]bool) error {
	for _, id := range b.Depends() {
		i := slices.IndexFunc(s.d.Blocks, func(x *doc.Block) bool { return x.ID == id })
		if i < 0 {
			return fmt.Errorf("%s depends on unknown block %q", s.d.Label(b), id)
		}
		dep := s.d.Blocks[i]
		if r := s.results[i]; r != nil && r.Ran && !r.Failed() {
			continue
		}
		if !Runnable(dep) {
			return fmt.Errorf("%s depends on %s, but mdsh can't run %s blocks", s.d.Label(b), s.d.Label(dep), dep.Lang)
		}
		a, err := s.depend(dep, b)
		if err != nil {
			return err
		}
		switch a {
		case Stop:
			return errStopped
		case Skip:
			continue
		}
		if err := s.runDepends(ctx, dep, ran); err != nil {
			return err
		}
		s.Reap()
		r := &Result{Block: dep}
		s.results[i] = r
		s.run(ctx, dep, r)
		if ran != nil {
			ran[dep] = true
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.Failed() {
			return &depError{dep: dep, label: s.d.Label(b)}
		}
	}
	return nil
}

func (s *Session) depend(dep, b *doc.Block) (Answer, error) {
	if s.opts.Depend == nil {
		return Stop, fmt.Errorf("%w: %s depends on %s, which hasn't run", ErrNoConfirm, s.d.Label(b), s.d.Label(dep))
	}
	return s.opts.Depend(dep, b), nil
}

func (s *Session) confirm(b *doc.Block) (Answer, error) {
	if s.opts.Confirm == nil {
		return Stop, fmt.Errorf("%w: %s", ErrNoConfirm, s.d.Label(b))
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
