// Package notebook is the part of mdsh-gui that doesn't need a window:
// it opens a document, runs its blocks in a run.Session and lays the
// document out as cells for the frontend to draw.
package notebook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

// Cell is one piece of the document as the frontend draws it: a
// top-level block of prose, or a block the user can run.
type Cell struct {
	// Block is the block's index in the document, or -1 for prose.
	Block int `json:"block"`
	// HTML is the prose, rendered. Empty for a block.
	HTML string `json:"html,omitempty"`
	// Tasks number the task list checkboxes in HTML, in order, for
	// SetTask.
	Tasks []int `json:"tasks,omitempty"`
	// Textbox is set for a textbox, which is drawn as one to type in.
	Textbox *TextboxCell `json:"textbox,omitempty"`
	// Choice is set for a choice, which is drawn as a dropdown, radio
	// buttons or checkboxes.
	Choice *ChoiceCell `json:"choice,omitempty"`
	// ID, Lang and Body describe a block.
	ID string `json:"id,omitempty"`
	// Name is how the block is shown: its id, or "line N" when mdsh
	// made the id up; see doc.Document.Label.
	Name string `json:"name,omitempty"`
	Lang string `json:"lang,omitempty"`
	Body string `json:"body,omitempty"`
	// Attrs are the block's attributes other than its id, as written,
	// such as confirm timeout=5m.
	Attrs string `json:"attrs,omitempty"`
	// Depends are the ids of the blocks the block's depends= names, in
	// order, for the lines the rendered view draws between them.
	Depends []string `json:"depends,omitempty"`
	// Output is the block's output, saved or from this session, rendered.
	Output string `json:"output,omitempty"`
	// OutputTasks number the task list checkboxes in Output, as Tasks
	// does for HTML.
	OutputTasks []int `json:"outputTasks,omitempty"`
	// Ran is true when the block ran this session; Status and Failed say
	// how it went.
	Ran    bool `json:"ran"`
	Status int  `json:"status"`
	Failed bool `json:"failed"`
	// Running is true while a background block is still going; its
	// output and status come in when it ends.
	Running bool `json:"running"`
	// Heading is the level of a prose cell that is a heading, 1 to 6;
	// its section runs to the next heading of the same or a higher level.
	Heading int `json:"heading,omitempty"`
	// SectionFrom and SectionTo are the blocks a heading's section
	// holds, its subsections' included: Blocks[SectionFrom:SectionTo],
	// for RunRange. They are both 0 when no block in it runs.
	SectionFrom int `json:"sectionFrom,omitempty"`
	SectionTo   int `json:"sectionTo,omitempty"`
	// Collapsed is true when the view should open with a block's code, or
	// a heading's section, folded: collapse= holds, or an mdsh:collapse
	// line in the section does. CollapsedOutput is collapse-output='s.
	Collapsed       bool `json:"collapsed,omitempty"`
	CollapsedOutput bool `json:"collapsedOutput,omitempty"`
	// OutputLine is the line a block's saved output starts on, counting
	// from 1, or 0 when it has none, for folding it in the source.
	OutputLine int `json:"outputLine,omitempty"`
	// From and To are the cell's bytes in the file, output included.
	From int `json:"from"`
	To   int `json:"to"`
	// Line is the line From is on, counting from 1.
	Line int `json:"line"`
	// Movable is set on a top-level block, which can move up and down
	// the document (see MoveBlock), and LastLine is the last line of the
	// block with its output and tape.
	Movable  bool `json:"movable,omitempty"`
	LastLine int  `json:"lastLine,omitempty"`
}

// Event is what OnEvent reports while a block runs.
type Event struct {
	// Kind is started, stdout, stderr or done; background when a
	// background block starts, which goes on while other blocks run,
	// with done when it ends; or, for a tty block, terminal (it takes
	// the terminal pane), tty (it showed Bytes there) and terminal-end.
	Kind  string `json:"kind"`
	Block int    `json:"block"`
	Data  string `json:"data,omitempty"`
	Bytes []byte `json:"bytes,omitempty"`
}

// RunOptions are the per-file choices mdsh takes as flags.
type RunOptions struct {
	// Args are the positional parameters, $1 and on.
	Args []string `json:"args"`
	// Only runs the named block and the earlier blocks in its env chain,
	// like --only.
	Only string `json:"only"`
	// Step asks before every block, like --step.
	Step bool `json:"step"`
	// StopOnError stops at the first failing block, like --stop-on-error.
	StopOnError bool `json:"stopOnError"`
	// Shell overrides the front matter's shell, like --shell.
	Shell string `json:"shell"`
	// Source runs in every shell as it starts, like the files given to
	// mdsh --source. It isn't sent to the window.
	Source string `json:"-"`
	// HistoryRC is a file, such as ~/.zshrc, the shells read as they
	// start while the History pane is on, so the pane can be used as an
	// everyday shell. It isn't sent to the window.
	HistoryRC string `json:"-"`
}

// Options configure a Notebook.
type Options struct {
	Run RunOptions
	// OnEvent, if set, is called as blocks start, write and finish.
	OnEvent func(Event)
	// Confirm is asked before a block marked confirm runs. If it is nil
	// such blocks don't run.
	Confirm func(block int, question string) run.Answer
	// Depend is asked before running block, which block of depends on
	// and which hasn't run in this session. If it is nil, block of
	// doesn't run.
	Depend func(block, of int, question string) run.Answer
}

// Notebook is an open document.
type Notebook struct {
	file string
	opts Options
	md   goldmark.Markdown
	term *terminal

	mu      sync.Mutex // guards the fields below
	d       *doc.Document
	sess    *run.Session
	results []*run.Result
	index   map[*doc.Block]int
	cancel  context.CancelFunc
	running bool
	clicked *doc.Block // the block RunBlock runs, which needs no confirm
	dirty   bool       // output changed since the last save
	stamp   fileStamp  // the file as last read or written
	// bg are the background blocks still running. While nothing else
	// runs, reaper checks on them every reapEvery; reaping is set while
	// it does, and idle tells those waiting for it that it is done.
	bg      map[*doc.Block]bool
	reaping bool
	// history is set while the History pane is on, and noteEnd is the
	// length of the source when the last note added there ended it, or
	// -1.
	history bool
	noteEnd int
	idle    *sync.Cond
	quit    chan struct{}
	// undo and redo are the rendered view's edits, for Undo and Redo.
	undo, redo []change
}

// reapEvery is how often a notebook checks on background blocks while
// nothing else runs.
const reapEvery = 250 * time.Millisecond

// fileStamp is what Changed compares to notice the file changing on
// disk.
type fileStamp struct {
	mod  time.Time
	size int64
}

func stampOf(file string) fileStamp {
	fi, err := os.Stat(file)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{fi.ModTime(), fi.Size()}
}

// Open reads and parses file and readies its envs.
func Open(file string, opts Options) (*Notebook, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	src, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	d, err := doc.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	n := &Notebook{
		file:    abs,
		opts:    opts,
		d:       d,
		index:   map[*doc.Block]int{},
		term:    newTerminal(),
		bg:      map[*doc.Block]bool{},
		quit:    make(chan struct{}),
		noteEnd: -1,
	}
	n.idle = sync.NewCond(&n.mu)
	n.md = goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(n.imageTransformer(), tableTaskTransformer(), diagramTransformer()), goldmark.WithRendererOptions(diagramRendererOption()))
	for i, b := range d.Blocks {
		n.index[b] = i
	}
	n.stamp = stampOf(abs)
	sess, err := n.newSession(opts.Run)
	if err != nil {
		return nil, err
	}
	n.sess = sess
	n.results = append([]*run.Result(nil), sess.Results()...)
	go n.reaper()
	return n, nil
}

func (n *Notebook) newSession(ro RunOptions) (*run.Session, error) {
	return run.NewSession(n.d, run.Options{
		File:        n.file,
		LinkDir:     filepath.Dir(n.file),
		Args:        ro.Args,
		Only:        ro.Only,
		Step:        ro.Step,
		StopOnError: ro.StopOnError,
		Shell:       ro.Shell,
		Source:      n.source(ro),
		Environ:     n.shellEnv(),
		OnEvent:     n.event,
		Confirm:     n.confirm,
		Depend:      n.depend,
		Terminal:    n.term.env(n.emit),
		TakeTTY:     n.takeTTY,
		Mode:        run.ModeInteractive,
		PTY:         n.history,
	})
}

// environ is the environment blocks run in, and conditions and
// placeholders read: the app's, with $MDSH_MODE interactive.
func environ() []string { return run.WithMode(os.Environ(), run.ModeInteractive) }

// File is the document's absolute path.
func (n *Notebook) File() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.file
}

// RunOptions are the options blocks run with.
func (n *Notebook) RunOptions() RunOptions {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.opts.Run
}

// SetRunOptions changes the options blocks run with. The shells start
// again, since arguments and the shell are fixed when they start, but
// the output shown so far is kept.
func (n *Notebook) SetRunOptions(ro RunOptions) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	n.waitReap()
	sess, err := n.newSession(ro)
	if err != nil {
		return err
	}
	n.closeSession()
	n.sess, n.opts.Run = sess, ro
	return nil
}

// Dirty reports whether output changed since the file was last saved.
func (n *Notebook) Dirty() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.dirty
}

// Changed reports whether the file changed on disk since it was read or
// last saved.
func (n *Notebook) Changed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return stampOf(n.file) != n.stamp
}

// Close ends the shells blocks ran in, stopping any block that runs,
// background blocks included.
func (n *Notebook) Close() {
	n.Stop()
	n.mu.Lock()
	select {
	case <-n.quit:
	default:
		close(n.quit)
	}
	n.waitReap()
	sess := n.sess
	n.mu.Unlock()
	sess.Close()
}

// closeSession ends n.sess, with n.mu held, before it is replaced. A
// background block still running in it is killed and reported through
// event, which takes n.mu, so the session closes on its own goroutine.
func (n *Notebook) closeSession() {
	go n.sess.Close()
}

// reaper checks on background blocks until the notebook closes.
func (n *Notebook) reaper() {
	t := time.NewTicker(reapEvery)
	defer t.Stop()
	for {
		select {
		case <-n.quit:
			return
		case <-t.C:
			n.reap()
		}
	}
}

// reap fills in the results of background blocks that ended, unless a
// run is going, which does that itself before each block.
func (n *Notebook) reap() {
	n.mu.Lock()
	if n.running || n.reaping || len(n.bg) == 0 {
		n.mu.Unlock()
		return
	}
	n.reaping = true
	sess := n.sess
	n.mu.Unlock()

	sess.Reap()

	n.mu.Lock()
	n.reaping = false
	n.idle.Broadcast()
	n.fold()
	n.mu.Unlock()
}

// waitReap waits, with n.mu held, until reap isn't using the session.
func (n *Notebook) waitReap() {
	for n.reaping {
		n.idle.Wait()
	}
}

func attrsText(info string) string {
	in := doc.SplitInfo(info)
	in.Del("#")
	return strings.Join(in.Tokens, " ")
}

func (n *Notebook) html(md []byte) string {
	var buf bytes.Buffer
	if err := n.md.Convert(md, &buf); err != nil {
		return ""
	}
	return buf.String()
}

// ErrBusy is returned when a run is asked for while one is going.
var ErrBusy = errors.New("a block is running")

// RunBlock runs block i alone, waiting for it to finish. Asking for the
// block to run answers its confirm, so it isn't asked again.
func (n *Notebook) RunBlock(i int) error {
	return n.start(func(ctx context.Context) error {
		n.mu.Lock()
		if i >= 0 && i < len(n.d.Blocks) {
			n.clicked = n.d.Blocks[i]
		}
		n.mu.Unlock()
		defer func() {
			n.mu.Lock()
			n.clicked = nil
			n.mu.Unlock()
		}()
		_, err := n.sess.RunBlock(ctx, i)
		return err
	})
}

// RunFrom runs block i and every block after it, as mdsh does.
func (n *Notebook) RunFrom(i int) error {
	return n.start(func(ctx context.Context) error { return n.sess.RunFrom(ctx, i) })
}

// RunRange runs blocks from up to but not including to, as RunFrom
// runs them; a heading's SectionFrom and SectionTo run its section.
func (n *Notebook) RunRange(from, to int) error {
	return n.start(func(ctx context.Context) error { return n.sess.RunRange(ctx, from, to) })
}

func (n *Notebook) start(f func(context.Context) error) error {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return ErrBusy
	}
	n.waitReap()
	ctx, cancel := context.WithCancel(context.Background())
	n.running, n.cancel = true, cancel
	n.mu.Unlock()

	err := f(ctx)

	n.mu.Lock()
	n.running, n.cancel = false, nil
	n.readIncludes()
	if ferr := n.fold(); err == nil {
		err = ferr
	}
	n.mu.Unlock()
	cancel()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// Stop stops the running block, if any.
func (n *Notebook) Stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cancel != nil {
		n.cancel()
	}
}

// ClearOutputs takes every run's output out of the document, as
// render.Clear does, and starts the shells again, so the notebook is as
// if nothing had run: no output, no statuses, no variables. Like an
// edit, it isn't saved until Save.
func (n *Notebook) ClearOutputs() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	n.waitReap()
	if err := n.setSource(render.Clear(n.d)); err != nil {
		return err
	}
	sess, err := n.newSession(n.opts.Run)
	if err != nil {
		return err
	}
	n.closeSession()
	n.sess = sess
	n.results = append([]*run.Result(nil), sess.Results()...)
	return nil
}

// Restart ends the shells, and any background block still running, and
// starts them again, as if nothing had run, keeping the document as it
// is.
func (n *Notebook) Restart() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	n.waitReap()
	sess, err := n.newSession(n.opts.Run)
	if err != nil {
		return err
	}
	n.closeSession()
	n.sess = sess
	n.results = append([]*run.Result(nil), sess.Results()...)
	return nil
}

// Save writes the output of every block that ran into the file.
func (n *Notebook) Save() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	return n.write(n.file)
}

// SaveAs writes the document with its output to file, which becomes the
// notebook's file. If file is in another folder the shells start again
// there, as they would for a file opened from that folder.
func (n *Notebook) SaveAs(file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	n.waitReap()
	if abs != n.file {
		if err := n.moveAssets(abs); err != nil {
			return err
		}
	}
	if err := n.write(abs); err != nil {
		return err
	}
	moved := filepath.Dir(abs) != filepath.Dir(n.file)
	n.file = abs
	if !moved {
		n.sess.SetFile(abs)
		return nil
	}
	sess, err := n.newSession(n.opts.Run)
	if err != nil {
		return err
	}
	n.closeSession()
	n.sess = sess
	return nil
}

// moveAssets readies the output for its new home at file: it copies the
// files in n.file's assets folder (<doc>.assets) to file's, and points
// the output's links into the old folder at the new one. The old folder
// stays, since the old file still links to it.
func (n *Notebook) moveAssets(file string) error {
	from, to := n.file+".assets", file+".assets"
	if err := copyDir(from, to); err != nil {
		return fmt.Errorf("copying %s: %w", filepath.Base(from), err)
	}
	oldLink := filepath.Base(n.file) + ".assets/"
	newLink := filepath.Base(file) + ".assets/"
	for _, r := range n.results {
		if r != nil && strings.HasPrefix(r.Image, oldLink) {
			r.Image = newLink + strings.TrimPrefix(r.Image, oldLink)
		}
	}
	if err := n.fold(); err != nil {
		return err
	}
	// Output saved before this session has no result to change.
	src := n.d.Src
	var out []byte
	last := 0
	for _, b := range n.d.Blocks {
		if !b.HasOutput() {
			continue
		}
		out = append(out, src[last:b.Output.Start]...)
		out = append(out, bytes.ReplaceAll(src[b.Output.Start:b.Output.End], []byte("]("+oldLink), []byte("]("+newLink))...)
		last = b.Output.End
	}
	out = append(out, src[last:]...)
	if bytes.Equal(out, src) {
		return nil
	}
	if err := n.d.SetSource(out); err != nil {
		return err
	}
	return n.sync()
}

// copyDir copies the files in from, and in its folders, to to. It does
// nothing if from doesn't exist.
func copyDir(from, to string) error {
	if _, err := os.Stat(from); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}

func (n *Notebook) write(file string) error {
	renames, err := run.NameBlocks(n.d, n.results, run.Options{File: n.file})
	if err != nil {
		return err
	}
	if len(renames) > 0 {
		if err := n.sync(); err != nil {
			return err
		}
	}
	if err := n.fold(); err != nil {
		return err
	}
	if err := os.WriteFile(file, n.d.Src, 0o644); err != nil {
		return err
	}
	n.dirty = false
	n.stamp = stampOf(file)
	return nil
}

// fold writes the results into the document's source, as saving will, so
// the source pane shows the output as soon as a run ends. No block
// changes, so every result stays.
// readIncludes reads the include blocks' files again, since the run may
// have written them, as a block saving an image an include shows.
func (n *Notebook) readIncludes() {
	n.sess.ReadIncludes()
	for i, r := range n.sess.Results() {
		if r.Block.IsInclude() {
			n.results[i] = r
		}
	}
}

func (n *Notebook) fold() error {
	out := render.Markdown(n.d, n.results, render.Options{WriteBack: true, Env: environ()})
	if bytes.Equal(out, n.d.Src) {
		return nil
	}
	if err := n.d.SetSource(out); err != nil {
		return err
	}
	return n.sync()
}

// event runs on the run's goroutine.
func (n *Notebook) event(e run.Event) {
	n.mu.Lock()
	i, ok := n.index[e.Block]
	if !ok {
		// A block edited away, whose background run a closing session
		// reports as killed.
		delete(n.bg, e.Block)
		n.mu.Unlock()
		return
	}
	background := e.Block.Attrs.Has("background")
	switch {
	case e.Kind == run.Done:
		n.results[i] = e.Result
		n.dirty = true
		delete(n.bg, e.Block)
	case e.Kind == run.Started && background:
		n.bg[e.Block] = true
	}
	n.mu.Unlock()
	if n.opts.OnEvent == nil {
		return
	}
	kind := map[run.EventKind]string{run.Started: "started", run.Stdout: "stdout", run.Stderr: "stderr", run.Done: "done"}[e.Kind]
	if e.Kind == run.Started && background {
		kind = "background"
	}
	n.opts.OnEvent(Event{Kind: kind, Block: i, Data: string(e.Data)})
}

func (n *Notebook) depend(dep, b *doc.Block) run.Answer {
	if n.opts.Depend == nil {
		return run.Stop
	}
	n.mu.Lock()
	i, of := n.index[dep], n.index[b]
	n.mu.Unlock()
	q := fmt.Sprintf("%s depends on this block, which hasn't run yet. Run it first?", n.d.Label(b))
	return n.opts.Depend(i, of, q)
}

func (n *Notebook) confirm(b *doc.Block) run.Answer {
	n.mu.Lock()
	clicked := n.clicked == b
	n.mu.Unlock()
	if clicked {
		return run.Go
	}
	if n.opts.Confirm == nil {
		return run.Stop
	}
	q := b.Attrs["confirm"]
	if q == "" {
		q = "Run this block?"
	}
	n.mu.Lock()
	i := n.index[b]
	n.mu.Unlock()
	return n.opts.Confirm(i, q)
}
