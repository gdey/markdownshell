package doc

import (
	"errors"
	"fmt"
	"strings"
)

// ErrDependsOrder is what a move fails with, wrapped, when it would take
// a block above one its depends= names.
var ErrDependsOrder = errors.New("a block would go above one it depends on")

type dependsOrderError struct{ msg string }

func (e *dependsOrderError) Error() string { return e.msg }
func (e *dependsOrderError) Unwrap() error { return ErrDependsOrder }

// Depends returns the ids of the blocks b's depends= names, in order,
// or nil if it has none.
func (b *Block) Depends() []string { return b.Attrs.List("depends") }

// Dep is the block id names in a depends=, or nil if no block can be
// depended on by that id: there is none, or it is an include, a tape or
// an input, which never runs. A depends= naming one is ignored.
func (d *Document) Dep(id string) *Block {
	b := d.Block(id)
	if b == nil || b.IsInclude() || b.IsTape() || b.IsInput() {
		return nil
	}
	return b
}

// checkDepends checks every block's depends=. An id that names no
// block, or an include, a tape or an input, is a warning, and ignored,
// so a typo doesn't stop the document opening. No block may depend on
// itself, directly or through others.
func checkDepends(d *Document) error {
	for _, b := range d.Blocks {
		for _, id := range b.Depends() {
			dep := d.Block(id)
			switch {
			case dep == nil || dep.IsInclude() || dep.IsTape():
				d.Warnings = append(d.Warnings, fmt.Sprintf("line %d: block %s: depends= names unknown block %q; it is ignored", lineNo(d.Src, b.Span.Start), b.ID, id))
			case dep.IsInput():
				d.Warnings = append(d.Warnings, fmt.Sprintf("line %d: block %s: depends= names %s, a %s, which never runs; it is ignored", lineNo(d.Src, b.Span.Start), b.ID, id, dep.Lang))
			}
		}
	}
	// Walk the graph from each block, depth first; a block met again
	// while it is still on the path closes a cycle.
	const (
		unseen = iota
		onPath
		done
	)
	state := map[*Block]int{}
	var path []string
	var visit func(b *Block) error
	visit = func(b *Block) error {
		switch state[b] {
		case done:
			return nil
		case onPath:
			i := 0
			for path[i] != b.ID {
				i++
			}
			cycle := append(path[i:len(path):len(path)], b.ID)
			return fmt.Errorf("line %d: block %s: depends= makes a cycle: %s", lineNo(d.Src, b.Span.Start), b.ID, strings.Join(cycle, " → "))
		}
		state[b] = onPath
		path = append(path, b.ID)
		for _, id := range b.Depends() {
			if dep := d.Dep(id); dep != nil {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		path = path[:len(path)-1]
		state[b] = done
		return nil
	}
	for _, b := range d.Blocks {
		if err := visit(b); err != nil {
			return err
		}
	}
	return nil
}

// MoveLimits say where a block's unit or a heading's section can move
// without taking a block above one it depends on: to an offset at with
// After < at <= Before.
type MoveLimits struct {
	// After is the bottommost block outside the span that a block in it
	// depends on, or -1; Before the topmost block outside it that
	// depends on a block in it, or len(Src).
	After, Before int
	// The depends= that set them, each as the dependent and the block
	// it depends on, or nil.
	AfterDep, AfterOn   *Block
	BeforeDep, BeforeOn *Block
}

// DependsLimits works out span's MoveLimits. A depends= already out of
// order doesn't limit the move.
func (d *Document) DependsLimits(span Span) MoveLimits {
	l := MoveLimits{After: -1, Before: len(d.Src)}
	in := func(b *Block) bool { return b.Span.Start >= span.Start && b.Span.Start < span.End }
	for _, b := range d.Blocks {
		for _, id := range b.Depends() {
			dep := d.Dep(id)
			if dep == nil || dep == b || dep.Span.Start >= b.Span.Start || in(b) == in(dep) {
				continue
			}
			if in(dep) && b.Span.Start < l.Before {
				l.Before, l.BeforeDep, l.BeforeOn = b.Span.Start, b, dep
			}
			if in(b) && dep.Span.Start > l.After {
				l.After, l.AfterDep, l.AfterOn = dep.Span.Start, b, dep
			}
		}
	}
	return l
}

// CheckDependsMove fails if moving span to at, one of Boundaries,
// would take a block above one it depends on (see DependsLimits).
func (d *Document) CheckDependsMove(span Span, at int) error {
	l := d.DependsLimits(span)
	switch {
	case at > l.Before:
		return &dependsOrderError{fmt.Sprintf("%s depends on %s, so %s can't go below it", d.name(l.BeforeDep), d.name(l.BeforeOn), d.name(l.BeforeOn))}
	case at <= l.After:
		return &dependsOrderError{fmt.Sprintf("%s depends on %s, so %s can't go above it", d.name(l.AfterDep), d.name(l.AfterOn), d.name(l.AfterDep))}
	}
	return nil
}
