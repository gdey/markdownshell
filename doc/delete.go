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
	src := d.Src
	s, e := u.Start, u.End
	e += leadingBlankLen(src[e:])
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
		// Keep what was on each side apart, as the block kept them.
		rest = slices.Insert(rest, s, '\n')
	}

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
		warnings = slices.DeleteFunc(warnings, func(w string) bool { return strings.Contains(w, fmt.Sprintf("output copy for unknown block %q", b.ID)) })
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

// users names what outside span uses block id: blocks that depend on it
// or read it in a condition, tapes for it, placeholders, and sections or
// folds whose condition reads it.
func (d *Document) users(id string, span Span) []string {
	var out []string
	add := func(s string) {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	in := func(pos int) bool { return pos >= span.Start && pos < span.End }
	for _, p := range d.Placeholders {
		if p.BlockID == id && !in(p.Span.Start) {
			add(p.Ref.String())
		}
	}
	for _, o := range d.Blocks {
		if in(o.Span.Start) {
			continue
		}
		if slices.Contains(o.Depends(), id) {
			add(d.name(o) + "'s depends=")
		}
		for _, a := range condAttrs {
			if c, ok := o.Cond(a); ok && c.Ref != nil && c.Ref.BlockID == id {
				add(fmt.Sprintf("%s's %s=", d.name(o), a))
			}
		}
		if o.IsTape() && o.Attrs["for"] == id {
			add("the tape at line " + fmt.Sprint(lineNo(d.Src, o.Span.Start)))
		}
		if o.IsTape() && !o.IsInclude() {
			for _, r := range TextRefs(o.Body) {
				if r.BlockID == id {
					add(fmt.Sprintf("the tape at line %d", lineNo(d.Src, o.Span.Start)))
				}
			}
		}
	}
	for _, sec := range d.Sections {
		if sec.Cond.Ref != nil && sec.Cond.Ref.BlockID == id {
			add(fmt.Sprintf("the mdsh:if at line %d", lineNo(d.Src, sec.If.Start)))
		}
	}
	for _, f := range d.Folds {
		if f.Cond.Ref != nil && f.Cond.Ref.BlockID == id {
			add(fmt.Sprintf("the mdsh:collapse at line %d", lineNo(d.Src, f.Span.Start)))
		}
	}
	return out
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
