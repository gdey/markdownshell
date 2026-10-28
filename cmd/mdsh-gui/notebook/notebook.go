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
	"sync"

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

// Options configure a Notebook.
type Options struct {
	// Args are the positional parameters, $1 and on.
	Args []string
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
	sess, err := run.NewSession(d, run.Options{
		File:    abs,
		LinkDir: filepath.Dir(abs),
		Args:    opts.Args,
		Environ: os.Environ(),
		OnEvent: n.event,
		Confirm: n.confirm,
	})
	if err != nil {
		return nil, err
	}
	n.sess = sess
	n.results = append([]*run.Result(nil), sess.Results()...)
	return n, nil
}

// File is the document's absolute path.
func (n *Notebook) File() string { return n.file }

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
	out := render.Markdown(n.d, n.results, render.Options{WriteBack: true, Env: os.Environ()})
	return os.WriteFile(n.file, out, 0o644)
}

// event runs on the run's goroutine.
func (n *Notebook) event(e run.Event) {
	n.mu.Lock()
	i := n.index[e.Block]
	if e.Kind == run.Done {
		n.results[i] = e.Result
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
