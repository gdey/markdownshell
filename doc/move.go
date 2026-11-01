package doc

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var gfm = goldmark.New(goldmark.WithExtensions(extension.GFM))

// Boundaries lists, in order, the byte offsets in Src where a block or a
// section can be moved to: the start of each top-level markdown block
// (a paragraph, heading, list, quote, table or fenced block), and the
// end of the file. Offsets inside a list, a block quote, front matter,
// or a block's attached output or an output copy are not boundaries.
func (d *Document) Boundaries() []int {
	body := d.bodyStart()
	root := gfm.Parser().Parse(text.NewReader(d.Src))
	var out []int
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		s, ok := nodeStart(c, d.Src)
		if !ok || s < body || d.inside(s) {
			continue
		}
		out = append(out, s)
	}
	out = append(out, len(d.Src))
	slices.Sort(out)
	return slices.Compact(out)
}

// inside reports whether pos is strictly inside a block and its output,
// or an output copy, so nothing can go there.
func (d *Document) inside(pos int) bool {
	for _, b := range d.Blocks {
		if pos > b.Span.Start && pos < b.Output.End {
			return true
		}
	}
	for _, c := range d.Copies {
		if pos > c.Span.Start && pos < c.Span.End {
			return true
		}
	}
	return false
}

// bodyStart is where the markdown starts, after the #! line and the
// front matter.
func (d *Document) bodyStart() int {
	pos := 0
	if d.Shebang != "" {
		pos = lineEnd(d.Src, 0)
	}
	return max(pos, d.FrontMatter.End)
}

// nodeStart is the offset of the start of the line node begins on.
func nodeStart(node ast.Node, src []byte) (int, bool) {
	first, ok := -1, false
	ast.Walk(node, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || c.Type() != ast.TypeBlock {
			return ast.WalkContinue, nil
		}
		lines := c.Lines()
		if f, isFence := c.(*ast.FencedCodeBlock); isFence {
			switch {
			case f.Info != nil:
				first, ok = f.Info.Segment.Start, true
			case lines.Len() > 0:
				first, ok = lines.At(0).Start-1, true
			default:
				return ast.WalkContinue, nil
			}
			return ast.WalkStop, nil
		}
		if lines.Len() > 0 {
			first, ok = lines.At(0).Start, true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if !ok {
		return 0, false
	}
	return bytes.LastIndexByte(src[:first], '\n') + 1, true
}

// Unit is what moves with block b: b, its attached output, and its tape
// when the tape comes right after it. It fails for a block that isn't at
// the top level, such as one in a list item or a block quote.
func (d *Document) Unit(b *Block) (Span, error) {
	i := slices.Index(d.Blocks, b)
	if i < 0 {
		return Span{}, errors.New("not a block of this document")
	}
	if b.Prefix != "" || !slices.Contains(d.Boundaries(), b.Span.Start) {
		return Span{}, fmt.Errorf("the block %s is in a list, a quote or an output, and only top-level blocks move", d.Label(b))
	}
	end := b.Output.End
	if i+1 < len(d.Blocks) {
		t := d.Blocks[i+1]
		if t.IsTape() && t.Attrs["for"] == b.ID && blank(d.Src[end:t.Span.Start]) {
			end = t.Output.End
		}
	}
	return Span{b.Span.Start, end}, nil
}

// MoveBlock moves b's unit (see Unit) to at, one of Boundaries, and
// returns the new order of the blocks as MoveSpan does.
func (d *Document) MoveBlock(b *Block, at int) ([]int, error) {
	u, err := d.Unit(b)
	if err != nil {
		return nil, err
	}
	return d.MoveSpan(u, at)
}

// MoveSpan moves the source at span to at, and parses d again. span
// starts at one of Boundaries and ends at one, or at the end of a line
// followed only by blank lines up to one; at is one of Boundaries. The
// moved text gets a blank line on each side, and the gap it leaves
// closes up to one blank line. Moving to where span already is changes
// nothing.
//
// Every *Block keeps its identity, with its spans moved to match. order
// gives the blocks' new order: d.Blocks[j] was d.Blocks[order[j]], so
// results indexed by block can follow. Ids don't change, so name the
// blocks first (see NameBlocks): a move fails, leaving d as it was, if
// it would renumber a block that is referred to or that has output. It
// also fails if it would split an mdsh:if section.
func (d *Document) MoveSpan(span Span, at int) (order []int, err error) {
	bounds := d.Boundaries()
	src := d.Src
	order = make([]int, len(d.Blocks))
	for i := range order {
		order[i] = i
	}
	e := span.End
	if e < len(src) && e > 0 && src[e-1] != '\n' {
		return nil, errors.New("the span doesn't end at the end of a line")
	}
	e += leadingBlankLen(src[e:])
	if !slices.Contains(bounds, span.Start) || !slices.Contains(bounds, e) || span.Start >= e {
		return nil, errors.New("the span isn't at the top level of the document")
	}
	if !slices.Contains(bounds, at) {
		return nil, errors.New("the spot isn't at the top level of the document")
	}
	s := span.Start
	switch {
	case at == s || at == e:
		return order, nil
	case at > s && at < e:
		return nil, errors.New("can't move a section into itself")
	}
	for _, sec := range d.Sections {
		if (sec.If.Start >= s && sec.If.Start < e) != (sec.End.Start >= s && sec.End.Start < e) {
			return nil, fmt.Errorf("the move would split the mdsh:if section at line %d", lineNo(src, sec.If.Start))
		}
	}

	piece := append([]byte(nil), src[s:s+trimBlankLen(src[s:e])]...)
	if len(piece) > 0 && piece[len(piece)-1] != '\n' {
		piece = append(piece, '\n')
	}
	body := d.bodyStart()
	rest := append(append([]byte(nil), src[:s]...), src[e:]...)
	to := at
	if at > e {
		to -= e - s
	}
	switch {
	case s == len(rest):
		// The end of the file: keep one newline.
		cut := s - trimBlankTail(rest[body:s])
		rest = rest[:cut]
		rest = append(rest, '\n')
		if cut == body {
			rest = rest[:cut]
		}
	case s > body && !endsBlank(rest[:s]):
		rest = slices.Insert(rest, s, '\n')
		if to > s {
			to++
		}
	}
	var ins []byte
	if to > body && !endsBlank(rest[:to]) {
		if rest[to-1] != '\n' {
			ins = append(ins, '\n')
		}
		ins = append(ins, '\n')
	}
	ins = append(ins, piece...)
	if to < len(rest) {
		ins = append(ins, '\n')
	}
	out := slices.Insert(rest, to, ins...)

	// The blocks in span go between the others at to.
	var moved, others []int
	for i, b := range d.Blocks {
		if b.Span.Start >= s && b.Span.Start < e {
			moved = append(moved, i)
		} else {
			others = append(others, i)
		}
	}
	k := 0
	for k < len(others) && d.Blocks[others[k]].Span.Start < at {
		k++
	}
	order = slices.Concat(others[:k], moved, others[k:])

	nd, err := Parse(out)
	if err != nil {
		return nil, fmt.Errorf("the moved document doesn't parse: %w", err)
	}
	if err := d.sameBlocks(nd, order); err != nil {
		return nil, err
	}
	blocks := make([]*Block, len(order))
	for j, i := range order {
		blocks[j] = d.Blocks[i]
		*blocks[j] = *nd.Blocks[j]
	}
	*d = *nd
	d.Blocks = blocks
	return order, nil
}

// sameBlocks checks that nd holds d's blocks in order, unchanged, with
// their output, and the same copies, sections and folds.
func (d *Document) sameBlocks(nd *Document, order []int) error {
	if len(nd.Blocks) != len(d.Blocks) {
		return errors.New("the move would change the document's blocks")
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
	for j, i := range order {
		ob, nb := d.Blocks[i], nd.Blocks[j]
		if ob.Info != nb.Info || ob.Body != nb.Body || ob.Prefix != nb.Prefix || ob.Output.End-ob.Output.Start != nb.Output.End-nb.Output.Start {
			return fmt.Errorf("the move would change the block %s or detach its output; name the document's blocks first", d.Label(ob))
		}
		if ob.ID != nb.ID && (ob.Named() || used[ob.ID] || used[nb.ID]) {
			return fmt.Errorf("the move would renumber the block %s from %s to %s; name the document's blocks first", d.Label(ob), ob.ID, nb.ID)
		}
	}
	if len(nd.Copies) != len(d.Copies) || len(nd.Sections) != len(d.Sections) || len(nd.Folds) != len(d.Folds) || len(nd.Placeholders) != len(d.Placeholders) {
		return errors.New("the move would change the document's output copies, sections or placeholders")
	}
	return nil
}

// leadingBlankLen is the length of the blank lines at the start of src.
func leadingBlankLen(src []byte) int {
	n := 0
	for n < len(src) {
		end := bytes.IndexByte(src[n:], '\n')
		if end < 0 {
			if blank(src[n:]) {
				return len(src)
			}
			break
		}
		if !blank(src[n : n+end]) {
			break
		}
		n += end + 1
	}
	return n
}

// trimBlankLen is the length of src less the blank lines at its end,
// keeping the last line's newline.
func trimBlankLen(src []byte) int {
	end := len(src)
	for end > 0 {
		s := end
		if src[s-1] == '\n' {
			s--
		}
		start := bytes.LastIndexByte(src[:s], '\n') + 1
		if !blank(src[start:end]) {
			break
		}
		end = start
	}
	return end
}

// trimBlankTail is the length of the blank lines and trailing newline at
// the end of src.
func trimBlankTail(src []byte) int {
	return len(src) - len(bytes.TrimRight(src, " \t\r\n"))
}

// endsBlank reports whether src is empty or ends with a blank line.
func endsBlank(src []byte) bool {
	if len(src) == 0 {
		return true
	}
	if src[len(src)-1] != '\n' {
		return false
	}
	s := src[:len(src)-1]
	start := bytes.LastIndexByte(s, '\n') + 1
	return start == 0 && blank(s) || start > 0 && blank(s[start:])
}
