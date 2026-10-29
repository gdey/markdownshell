package notebook

import (
	"errors"
	"fmt"
	"slices"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// Move is a block move, planned or done.
type Move struct {
	// Base is the file the move applies to, and Text the file after it.
	// Base can differ from the file before the move was asked for, when
	// blocks had to be given ids first (see PlanMove).
	Base string `json:"base"`
	Text string `json:"text"`
	// Block is the block's index after the move, and Line the line its
	// fence starts on, counting from 1. FromLine is the line it started
	// on before, in Base.
	Block    int `json:"block"`
	Line     int `json:"line"`
	FromLine int `json:"fromLine"`
	// Warnings are what the move broke or changed that depends on block
	// order, such as a condition that now reads a block below it.
	Warnings []string `json:"warnings,omitempty"`
	// Note is a softer word, for a block that ran: it ran in the old
	// order.
	Note string `json:"note,omitempty"`
}

// ErrNoMove is returned for a move up from the top or down from the
// bottom.
var ErrNoMove = errors.New("the block can't move further that way")

// MoveBlock moves top-level block i, with its output and tape, up
// (dir < 0) or down past the cell next to it, or with byBlock past the
// next block, prose and all. It is a rendered-view edit, so Undo takes
// it back. Blocks without ids get them first, as saving would give them,
// so the move renames nothing.
func (n *Notebook) MoveBlock(i, dir int, byBlock bool) (Move, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	m, err := n.planMove(i, dir, byBlock)
	if err != nil {
		return m, err
	}
	return m, n.change("move block", "", []byte(m.Text), nil)
}

// PlanMove works out MoveBlock's move without making it, for the source
// view, which makes it as an edit of its own so its undo takes it back.
// If blocks had to be given ids first, that is done, and Base says what
// the file is now.
func (n *Notebook) PlanMove(i, dir int, byBlock bool) (Move, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.planMove(i, dir, byBlock)
}

func (n *Notebook) planMove(i, dir int, byBlock bool) (Move, error) {
	if n.running || len(n.bg) > 0 {
		return Move{}, ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) {
		return Move{}, fmt.Errorf("no block %d", i)
	}
	m, err := n.tryMove(i, dir, byBlock)
	if err != nil && !errors.Is(err, ErrNoMove) && n.unnamed() {
		// Give blocks ids, as saving would, and try again.
		if _, nerr := run.NameBlocks(n.d, n.results, run.Options{File: n.file}); nerr != nil {
			return Move{}, nerr
		}
		n.dirty = true
		if serr := n.sync(); serr != nil {
			return Move{}, serr
		}
		m, err = n.tryMove(i, dir, byBlock)
	}
	if err != nil {
		return Move{}, err
	}
	if r := n.results[i]; r != nil && r.Ran {
		m.Note = "It ran in the old order; run it again to check."
	}
	return m, nil
}

// unnamed reports whether a block that runs has no id of its own.
func (n *Notebook) unnamed() bool {
	return slices.ContainsFunc(n.d.Blocks, func(b *doc.Block) bool { return run.Runnable(b) && !b.Named() })
}

// tryMove plans the move on a copy of the document.
func (n *Notebook) tryMove(i, dir int, byBlock bool) (Move, error) {
	d, err := doc.Parse(n.d.Src)
	if err != nil {
		return Move{}, err
	}
	b := d.Blocks[i]
	from := lineNo(d.Src, b.Span.Start)
	at, err := moveTarget(d, b, dir, byBlock)
	if err != nil {
		return Move{}, err
	}
	before := d.OrderProblems(run.Runnable)
	order, err := d.MoveBlock(b, at)
	if err != nil {
		return Move{}, err
	}
	m := Move{Base: string(n.d.Src), Text: string(d.Src), Block: slices.Index(order, i), Line: lineNo(d.Src, b.Span.Start), FromLine: from}
	for _, p := range doc.NewProblems(before, d.OrderProblems(run.Runnable)) {
		m.Warnings = append(m.Warnings, p.Text)
	}
	return m, nil
}

// moveTarget is where b's unit goes to move one step up (dir < 0) or
// down: past the cell next to it, or with byBlock past the next
// top-level block.
func moveTarget(d *doc.Document, b *doc.Block, dir int, byBlock bool) (int, error) {
	u, err := d.Unit(b)
	if err != nil {
		return 0, err
	}
	bounds := d.Boundaries()
	// tops are the top-level blocks.
	var tops []*doc.Block
	for _, t := range d.Blocks {
		if t.Prefix == "" && slices.Contains(bounds, t.Span.Start) && t.Span.Start != u.Start {
			if t.Span.Start > u.Start && t.Span.Start < u.End {
				continue // b's tape
			}
			tops = append(tops, t)
		}
	}
	after := func(pos int) (int, bool) { // the first boundary at or after pos
		k, _ := slices.BinarySearch(bounds, pos)
		return bounds[min(k, len(bounds)-1)], k < len(bounds)
	}
	end, _ := after(u.End)
	if dir < 0 {
		at := -1
		for _, p := range bounds {
			if p < u.Start && (!byBlock || slices.ContainsFunc(tops, func(t *doc.Block) bool { return t.Span.Start == p })) {
				at = p
			}
		}
		if at < 0 {
			return 0, ErrNoMove
		}
		return at, nil
	}
	if end >= len(d.Src) {
		return 0, ErrNoMove
	}
	if byBlock {
		for _, t := range tops {
			if t.Span.Start >= end {
				tu, err := d.Unit(t)
				if err != nil {
					return 0, err
				}
				at, _ := after(tu.End)
				return at, nil
			}
		}
		return 0, ErrNoMove
	}
	at, _ := after(end + 1)
	return at, nil
}

// BlockAt is the top-level block whose fence, body, output or tape is on
// line (counting from 1), for moving it from the source view, or -1.
func (n *Notebook) BlockAt(line int) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	d := n.d
	for i := len(d.Blocks) - 1; i >= 0; i-- {
		b := d.Blocks[i]
		if lineNo(d.Src, b.Span.Start) > line {
			continue
		}
		if b.Prefix != "" {
			return -1
		}
		if b.IsTape() && i > 0 {
			// A tape right after its block moves with it.
			if u, err := d.Unit(d.Blocks[i-1]); err == nil && u.End >= b.Span.End {
				i, b = i-1, d.Blocks[i-1]
			}
		}
		u, err := d.Unit(b)
		if err != nil || line > lineNo(d.Src, max(u.End-1, u.Start)) {
			return -1
		}
		return i
	}
	return -1
}
