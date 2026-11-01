package notebook

import (
	"bytes"
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
	m, err := n.plan(stepMove(i, dir, byBlock), i)
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
	return n.plan(stepMove(i, dir, byBlock), i)
}

// MoveTo moves what starts at from, a cell's From, to at, one of the
// spots doc.Document.Boundaries lists (a cell's From where its Drop is
// set, or -1 for the end of the file), as dragging does: a top-level
// block with its output and tape, or a heading's whole section, which
// only goes before a heading of its level or higher, or to the end.
// Block is -1 for a section. Undo takes it back.
func (n *Notebook) MoveTo(from, at int) (Move, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running || len(n.bg) > 0 {
		return Move{}, ErrBusy
	}
	// Offsets change if blocks get ids first, so the move is kept as
	// the block or heading and the boundary, by number.
	if at < 0 {
		at = len(n.d.Src)
	}
	bounds := n.d.Boundaries()
	b := slices.Index(bounds, at)
	if b < 0 {
		return Move{}, errors.New("that isn't a spot a block or section can go")
	}
	i := slices.IndexFunc(n.d.Blocks, func(b *doc.Block) bool { return b.Span.Start == from })
	h := slices.IndexFunc(n.d.Headings(), func(h doc.Heading) bool { return h.Span.Start == from })
	if i < 0 && h < 0 {
		return Move{}, errors.New("only a top-level block or a heading's section moves")
	}
	what := "move block"
	fn := func(d *doc.Document) ([]int, int, int, error) {
		blk := d.Blocks[i]
		line := lineNo(d.Src, blk.Span.Start)
		order, err := d.MoveBlock(blk, d.Boundaries()[b])
		return order, line, lineNo(d.Src, blk.Span.Start), err
	}
	if i < 0 {
		what = "move section"
		fn = func(d *doc.Document) ([]int, int, int, error) {
			from, at := d.Headings()[h].Span.Start, d.Boundaries()[b]
			span, _, err := d.Section(from)
			if err != nil {
				return nil, 0, 0, err
			}
			piece := bytes.TrimRight(d.Src[span.Start:span.End], " \t\r\n")
			line := lineNo(d.Src, from)
			order, err := d.MoveSection(from, at)
			if err != nil {
				return nil, 0, 0, err
			}
			lo := at - 2
			if at > span.Start {
				lo -= span.End - span.Start
			}
			lo = max(lo, 0)
			k := bytes.Index(d.Src[lo:], piece)
			return order, line, lineNo(d.Src, lo+max(k, 0)), nil
		}
	}
	m, err := n.plan(fn, i)
	if err != nil {
		return m, err
	}
	if m.Text == m.Base {
		return m, nil
	}
	return m, n.change(what, "", []byte(m.Text), nil)
}

// moveFn makes a move on a copy of the document, returning the blocks'
// new order and the line the moved text starts on before and after.
type moveFn func(d *doc.Document) (order []int, from, to int, err error)

// stepMove is MoveBlock's move.
func stepMove(i, dir int, byBlock bool) moveFn {
	return func(d *doc.Document) ([]int, int, int, error) {
		if i < 0 || i >= len(d.Blocks) {
			return nil, 0, 0, fmt.Errorf("no block %d", i)
		}
		b := d.Blocks[i]
		line := lineNo(d.Src, b.Span.Start)
		at, err := moveTarget(d, b, dir, byBlock)
		if err != nil {
			return nil, 0, 0, err
		}
		order, err := d.MoveBlock(b, at)
		return order, line, lineNo(d.Src, b.Span.Start), err
	}
}

// plan plans a move, giving blocks ids first if it needs them. block is
// the moved block's index, or -1 for a section.
func (n *Notebook) plan(fn moveFn, block int) (Move, error) {
	if n.running || len(n.bg) > 0 {
		return Move{}, ErrBusy
	}
	m, order, err := n.tryMove(fn, block)
	if err != nil && !errors.Is(err, ErrNoMove) && !errors.Is(err, doc.ErrDependsOrder) && n.unnamed() {
		// Give blocks ids, as saving would, and try again.
		if _, nerr := run.NameBlocks(n.d, n.results, run.Options{File: n.file}); nerr != nil {
			return Move{}, nerr
		}
		n.dirty = true
		if serr := n.sync(); serr != nil {
			return Move{}, serr
		}
		m, order, err = n.tryMove(fn, block)
	}
	if err != nil {
		return Move{}, err
	}
	// The blocks that moved are those whose place changed against the
	// others; one that ran ran in the old order.
	for j, i := range order {
		if r := n.results[i]; r != nil && r.Ran && (i == block || block < 0 && movedPast(order, j)) {
			m.Note = "It ran in the old order; run it again to check."
			if block < 0 {
				m.Note = "Blocks in it ran in the old order; run them again to check."
			}
			break
		}
	}
	return m, nil
}

// movedPast reports whether the block at j in order is out of its old
// order with some other block.
func movedPast(order []int, j int) bool {
	for k, i := range order {
		if (k < j) != (i < order[j]) && k != j {
			return true
		}
	}
	return false
}

// unnamed reports whether a block that runs has no id of its own.
func (n *Notebook) unnamed() bool {
	return slices.ContainsFunc(n.d.Blocks, func(b *doc.Block) bool { return run.Runnable(b) && !b.Named() })
}

// tryMove plans a move on a copy of the document.
func (n *Notebook) tryMove(fn moveFn, block int) (Move, []int, error) {
	d, err := doc.Parse(n.d.Src)
	if err != nil {
		return Move{}, nil, err
	}
	before := d.OrderProblems(run.Runnable)
	order, from, to, err := fn(d)
	if err != nil {
		return Move{}, nil, err
	}
	m := Move{Base: string(n.d.Src), Text: string(d.Src), Block: -1, Line: to, FromLine: from}
	if block >= 0 {
		m.Block = slices.Index(order, block)
	}
	for _, p := range doc.NewProblems(before, d.OrderProblems(run.Runnable)) {
		m.Warnings = append(m.Warnings, p.Text)
	}
	return m, order, nil
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
