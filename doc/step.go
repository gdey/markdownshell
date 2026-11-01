package doc

import (
	"errors"
	"slices"
)

// ErrNoMove is StepTarget's answer for a move up from the top or down
// from the bottom.
var ErrNoMove = errors.New("the block can't move further that way")

// StepTarget is where b's unit (see Unit) goes to move one step up
// (dir < 0) or down: past the top-level markdown block next to it, or
// with byBlock past the next top-level fenced block. It is ErrNoMove at
// the top or bottom.
func (d *Document) StepTarget(b *Block, dir int, byBlock bool) (int, error) {
	u, err := d.Unit(b)
	if err != nil {
		return 0, err
	}
	bounds := d.Boundaries()
	// tops are the top-level blocks.
	var tops []*Block
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
			if p < u.Start && (!byBlock || slices.ContainsFunc(tops, func(t *Block) bool { return t.Span.Start == p })) {
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
