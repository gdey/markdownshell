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

// Cell is one piece of the document as the frontend draws it: prose,
// or a block the user can run.
type Cell struct {
	// Block is the block's index in the document, or -1 for prose.
	Block int `json:"block"`
	// HTML is the prose, rendered. Empty for a block.
	HTML string `json:"html,omitempty"`
	// ID, Lang and Body describe a block.
	ID   string `json:"id,omitempty"`
	Lang string `json:"lang,omitempty"`
	Body string `json:"body,omitempty"`
	// Attrs are the block's attributes other than its id, as written,
	// such as confirm timeout=5m.
	Attrs string `json:"attrs,omitempty"`
	// Output is the block's output, saved or from this session, rendered.
	Output string `json:"output,omitempty"`
	// Ran is true when the block ran this session; Status and Failed say
	// how it went.
	Ran    bool `json:"ran"`
	Status int  `json:"status"`
	Failed bool `json:"failed"`
}

// Event is what OnEvent reports while a block runs.
type Event struct {
	Kind  string `json:"kind"` // started, stdout, stderr or done
	Block int    `json:"block"`
	Data  string `json:"data,omitempty"`
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
}

// Options configure a Notebook.
type Options struct {
	Run RunOptions
	// OnEvent, if set, is called as blocks start, write and finish.
	OnEvent func(Event)
	// Confirm is asked before a block marked confirm runs. If it is nil
	// such blocks don't run.
	Confirm func(block int, question string) run.Answer
}

// Notebook is an open document.
type Notebook struct {
	file string
	opts Options
	md   goldmark.Markdown

	mu      sync.Mutex // guards the fields below
	d       *doc.Document
	sess    *run.Session
	results []*run.Result
	index   map[*doc.Block]int
	cancel  context.CancelFunc
	running bool
	dirty   bool      // output changed since the last save
	stamp   fileStamp // the file as last read or written
}

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
		file:  abs,
		opts:  opts,
		md:    goldmark.New(goldmark.WithExtensions(extension.GFM)),
		d:     d,
		index: map[*doc.Block]int{},
	}
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
		Environ:     os.Environ(),
		OnEvent:     n.event,
		Confirm:     n.confirm,
	})
}

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
	sess, err := n.newSession(ro)
	if err != nil {
		return err
	}
	n.sess.Close()
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

// Close ends the shells blocks ran in, stopping any block that runs.
func (n *Notebook) Close() {
	n.Stop()
	n.sess.Close()
}

// Cells lays the document out for drawing. Blocks mdsh can't run are
// drawn as part of the prose around them, as mdsh -i does. Blocks that
// printing leaves out, such as those in an mdsh:if section that is off,
// are left out here too.
func (n *Notebook) Cells() []Cell {
	n.mu.Lock()
	defer n.mu.Unlock()
	pieces := render.Pieces(n.d, n.results, render.Options{Env: os.Environ()})
	var cells []Cell
	var prose bytes.Buffer
	flush := func() {
		if len(bytes.TrimSpace(prose.Bytes())) > 0 {
			cells = append(cells, Cell{Block: -1, HTML: n.html(prose.Bytes())})
		}
		prose.Reset()
	}
	for i, b := range n.d.Blocks {
		prose.Write(pieces[3*i])
		if !run.Runnable(b) {
			prose.Write(pieces[3*i+1])
			prose.Write(pieces[3*i+2])
			continue
		}
		if len(pieces[3*i+1]) == 0 {
			continue
		}
		flush()
		r := n.results[i]
		cells = append(cells, Cell{
			Block:  i,
			ID:     b.ID,
			Lang:   b.Lang,
			Body:   b.Body,
			Attrs:  attrsText(b.Info),
			Output: n.html(pieces[3*i+2]),
			Ran:    r.Ran,
			Status: r.Status,
			Failed: r.Failed(),
		})
	}
	prose.Write(pieces[len(pieces)-1])
	flush()
	return cells
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

// RunBlock runs block i alone, waiting for it to finish.
func (n *Notebook) RunBlock(i int) error {
	return n.start(func(ctx context.Context) error {
		_, err := n.sess.RunBlock(ctx, i)
		return err
	})
}

// RunFrom runs block i and every block after it, as mdsh does.
func (n *Notebook) RunFrom(i int) error {
	return n.start(func(ctx context.Context) error { return n.sess.RunFrom(ctx, i) })
}

func (n *Notebook) start(f func(context.Context) error) error {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return ErrBusy
	}
	ctx, cancel := context.WithCancel(context.Background())
	n.running, n.cancel = true, cancel
	n.mu.Unlock()

	err := f(ctx)

	n.mu.Lock()
	n.running, n.cancel = false, nil
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
	if err := n.write(abs); err != nil {
		return err
	}
	moved := filepath.Dir(abs) != filepath.Dir(n.file)
	n.file = abs
	if moved {
		sess, err := n.newSession(n.opts.Run)
		if err != nil {
			return err
		}
		n.sess.Close()
		n.sess = sess
	}
	return nil
}

func (n *Notebook) write(file string) error {
	out := render.Markdown(n.d, n.results, render.Options{WriteBack: true, Env: os.Environ()})
	if err := os.WriteFile(file, out, 0o644); err != nil {
		return err
	}
	n.dirty = false
	n.stamp = stampOf(file)
	return nil
}

// event runs on the run's goroutine.
func (n *Notebook) event(e run.Event) {
	n.mu.Lock()
	i := n.index[e.Block]
	if e.Kind == run.Done {
		n.results[i] = e.Result
		n.dirty = true
	}
	n.mu.Unlock()
	if n.opts.OnEvent == nil {
		return
	}
	kind := map[run.EventKind]string{run.Started: "started", run.Stdout: "stdout", run.Stderr: "stderr", run.Done: "done"}[e.Kind]
	n.opts.OnEvent(Event{Kind: kind, Block: i, Data: string(e.Data)})
}

func (n *Notebook) confirm(b *doc.Block) run.Answer {
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
