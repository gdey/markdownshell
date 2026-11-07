package doc

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrRenumber is returned by DeleteBlock when taking the block out would
// renumber a block after it whose block-N id something uses. Naming the
// blocks first (see NameBlocks) fixes it.
var ErrRenumber = errors.New("deleting the block would renumber the blocks after it")

// DeleteBlock takes b's unit (see Unit) out of the document, the block
// with its attached output and its tape, and parses d again. The gap it
// leaves closes up to one blank line. It fails, leaving d as it was, if
// anything else in the document uses b's id, such as a depends=, a
// condition or a placeholder, since the file wouldn't parse without it.
// It returns the problems it leaves that don't stop the file parsing,
// such as an output copy of b somewhere else.
//
// Every other *Block keeps its identity, with its spans moved to match,
// so results indexed by block can follow. Ids don't change: a delete
// that would renumber a block that is referred to or that has output
// fails with ErrRenumber.
func (d *Document) DeleteBlock(b *Block) (warnings []string, err error) {
	u, err := d.Unit(b)
	if err != nil {
		return nil, err
	}
	if users := d.users(b.ID, u); len(users) > 0 {
		return nil, fmt.Errorf("can't delete %s: %s still use it; take those out first", d.name(b), strings.Join(users, ", "))
	}
	s, e := u.Start, u.End
	e += leadingBlankLen(d.Src[e:])
	rest := d.cut(s, e)

	var kept []int
	for i, ob := range d.Blocks {
		if ob.Span.Start < s || ob.Span.Start >= e {
			kept = append(kept, i)
		}
	}
	nd, err := Parse(rest)
	if err != nil && slices.ContainsFunc(d.Blocks, func(o *Block) bool { return o.Span.Start >= e && !o.Named() }) {
		// It may be a block-N id that now names nothing, or the
		// wrong block.
		return nil, ErrRenumber
	}
	if err != nil {
		return nil, fmt.Errorf("the document doesn't parse without %s: %w", d.name(b), err)
	}
	if err := d.sameKept(nd, kept); err != nil {
		return nil, err
	}
	for _, w := range nd.Warnings {
		if !slices.Contains(d.Warnings, w) {
			warnings = append(warnings, w)
		}
	}
	if n := countCopies(d, b.ID); n > 0 {
		warnings = slices.DeleteFunc(warnings, func(w string) bool { return strings.HasSuffix(w, copyWarning(b.ID)) })
		what := "an output copy"
		if n > 1 {
			what = fmt.Sprintf("%d output copies", n)
		}
		warnings = append(warnings, fmt.Sprintf("%s of %s stay in the file, and show nothing now.", what, d.name(b)))
	}
	blocks := make([]*Block, len(kept))
	for j, i := range kept {
		blocks[j] = d.Blocks[i]
		*blocks[j] = *nd.Blocks[j]
	}
	*d = *nd
	d.Blocks = blocks
	return warnings, nil
}

// cut returns the source without s to e, whole lines with the blank
// lines after them, closing the gap to one blank line.
func (d *Document) cut(s, e int) []byte {
	src := d.Src
	rest := append(append([]byte(nil), src[:s]...), src[e:]...)
	body := d.bodyStart()
	switch {
	case s == len(rest):
		// The end of the file: keep one newline.
		cut := s - trimBlankTail(rest[body:s])
		rest = rest[:cut]
		if cut > body {
			rest = append(rest, '\n')
		}
	case s > body && !endsBlank(rest[:s]):
		// Keep what was on each side apart, as the cut lines kept them.
		rest = slices.Insert(rest, s, '\n')
	}
	return rest
}

// DeleteBlockers names what stops DeleteBlock deleting b: what else in
// the document uses its id, such as a depends= or a placeholder. It is
// nil when b can go, or when b isn't a top-level block.
func (d *Document) DeleteBlockers(b *Block) []string {
	return d.NewIndex().DeleteBlockers(b)
}

// users names what outside span uses block id: blocks that depend on it
// or read it in a condition, tapes for it, placeholders, and sections or
// folds whose condition reads it.
func (d *Document) users(id string, span Span) []string {
	return d.NewIndex().users(id, span)
}

// countCopies is how many output copies in d are of block id.
func countCopies(d *Document, id string) int {
	n := 0
	for _, c := range d.Copies {
		if c.For == id {
			n++
		}
	}
	return n
}

// sameKept checks that nd holds d's blocks at kept, in order, unchanged,
// with their output and their ids where anything uses them.
func (d *Document) sameKept(nd *Document, kept []int) error {
	if len(nd.Blocks) != len(kept) {
		return errors.New("deleting the block would change the document's other blocks")
	}
	used := map[string]bool{}
	for _, r := range d.Refs() {
		used[r.BlockID] = true
	}
	for _, c := range d.Copies {
		used[c.For] = true
	}
	for _, b := range d.Blocks {
		if b.IsTape() {
			used[b.Attrs["for"]] = true
		}
		for _, id := range b.Depends() {
			used[id] = true
		}
	}
	for j, i := range kept {
		ob, nb := d.Blocks[i], nd.Blocks[j]
		if ob.Info != nb.Info || ob.Body != nb.Body || ob.Prefix != nb.Prefix || ob.Output.End-ob.Output.Start != nb.Output.End-nb.Output.Start {
			return fmt.Errorf("deleting the block would change the block %s or detach its output", d.Label(ob))
		}
		if ob.ID != nb.ID && (ob.Named() || used[ob.ID] || used[nb.ID] || ob.Output.End > ob.Span.End) {
			return ErrRenumber
		}
	}
	return nil
}
