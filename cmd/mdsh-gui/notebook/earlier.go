package notebook

import (
	"bytes"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
)

// A block with history=N keeps its earlier runs' outputs under its
// current output, newest first. The card draws them folded under the
// output, each with its time, its exit status, a copy button and a
// trash; Clear earlier runs takes them all and keeps the current
// output. Both are rendered-view edits, so Undo puts them back.

// EarlierRun is one of a block's earlier runs, as the card draws it.
type EarlierRun struct {
	// K is its place: 1 for the run before the current one, 2 for the
	// one before that, and so on.
	K int `json:"k"`
	// At is when it ran, as its at= has it (RFC 3339, in UTC), or ""
	// for output saved without one.
	At string `json:"at,omitempty"`
	// Exit is its exit status. Failed is set when it isn't 0 or the
	// block ended its shell, and Region when its output is markdown, an
	// image or a recording, which have no exit status of their own.
	Exit   int  `json:"exit"`
	Failed bool `json:"failed,omitempty"`
	Region bool `json:"region,omitempty"`
	// Output is its output, rendered.
	Output string `json:"output"`
}

// earlier is block b's earlier runs, for its cell.
func (n *Notebook) earlier(b *doc.Block) []EarlierRun {
	if len(b.History) == 0 {
		return nil
	}
	runs := make([]EarlierRun, len(b.History))
	for i, e := range b.History {
		runs[i] = EarlierRun{
			K:      e.K,
			At:     e.At,
			Exit:   e.Exit,
			Failed: e.Exit != 0 || e.Exited,
			Region: e.Region,
			Output: n.html(n.d.Src[e.Span.Start:e.Span.End]),
		}
	}
	return runs
}

// ClearEarlierRuns takes top-level block i's earlier runs out of the
// file, and keeps its current output. Undo puts them back. Their image
// files go when the file is saved.
func (n *Notebook) ClearEarlierRuns(i int) error {
	return n.dropEarlier(i, "clear earlier runs", func(doc.Entry) bool { return true })
}

// DeleteEarlierRun takes block i's earlier run k out of the file, and
// numbers the rest again. Undo puts it back.
func (n *Notebook) DeleteEarlierRun(i, k int) error {
	n.mu.Lock()
	found := i >= 0 && i < len(n.d.Blocks) && hasEntry(n.d.Blocks[i], k)
	n.mu.Unlock()
	if !found {
		return fmt.Errorf("block %d has no earlier run %d", i, k)
	}
	return n.dropEarlier(i, "delete earlier run", func(e doc.Entry) bool { return e.K == k })
}

func hasEntry(b *doc.Block, k int) bool {
	for _, e := range b.History {
		if e.K == k {
			return true
		}
	}
	return false
}

func (n *Notebook) dropEarlier(i int, what string, drop func(doc.Entry) bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) {
		return fmt.Errorf("no block %d", i)
	}
	b := n.d.Blocks[i]
	if n.bg[b] {
		return ErrBusy
	}
	// What a run this session wrote is in the source already: fold put
	// it there when the run ended.
	src := render.DropEarlierRuns(n.d.Src, b, drop)
	if err := n.change(what, "", src, nil); err != nil {
		return err
	}
	n.dirty = true
	return nil
}

// warnTrims logs a warning for each block whose earlier runs the next
// write-back trims because its history= keeps fewer than it has: it was
// lowered, or taken off.
func (n *Notebook) warnTrims() {
	for _, b := range n.d.Blocks {
		keep := b.HistoryCap(n.d.Options)
		if len(b.History) <= keep {
			continue
		}
		drop := len(b.History) - keep
		runs := "earlier runs"
		if drop == 1 {
			runs = "earlier run"
		}
		why := "it has no history= now"
		if keep > 0 {
			why = "its history= keeps " + strconv.Itoa(keep)
		}
		line := 1 + bytes.Count(n.d.Src[:b.Span.Start], []byte("\n"))
		slog.Warn(fmt.Sprintf("dropped %d %s of block %s: %s", drop, runs, n.d.Label(b), why), "file", n.file, "line", line)
	}
}
