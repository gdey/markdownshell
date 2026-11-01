package doc

import (
	"fmt"
	"strings"
)

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
