package doc

import (
	"sort"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Heading is a top-level heading and the section it opens.
type Heading struct {
	Level int
	// Span runs from the start of the heading's line to the next
	// heading of the same or a higher level, or the end of the file.
	Span Span
}

// Headings lists the top-level headings, in order. Headings in a list,
// a block quote, the front matter, or a block's output aren't sections
// and are left out.
func (d *Document) Headings() []Heading {
	body := d.bodyStart()
	root := gfm.Parser().Parse(text.NewReader(d.Src))
	var hs []Heading
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		h, ok := c.(*ast.Heading)
		if !ok {
			continue
		}
		s, ok := nodeStart(c, d.Src)
		if !ok || s < body || d.inside(s) {
			continue
		}
		hs = append(hs, Heading{Level: h.Level, Span: Span{s, len(d.Src)}})
	}
	CloseSections(hs, len(d.Src))
	return hs
}

// CloseSections sets each heading's Span.End to the start of the next
// heading of the same or a higher level, or to end. The headings are in
// order and their Span.Start is set.
func CloseSections(hs []Heading, end int) {
	for i := range hs {
		hs[i].Span.End = end
		for _, n := range hs[i+1:] {
			if n.Level <= hs[i].Level {
				hs[i].Span.End = n.Span.Start
				break
			}
		}
	}
}

// BlocksIn is the blocks that start in span: d.Blocks[from:to].
func (d *Document) BlocksIn(span Span) (from, to int) {
	from = sort.Search(len(d.Blocks), func(i int) bool { return d.Blocks[i].Span.Start >= span.Start })
	to = sort.Search(len(d.Blocks), func(i int) bool { return d.Blocks[i].Span.Start >= span.End })
	return from, max(from, to)
}

// SectionAt is the innermost section holding pos: the last heading
// starting at or before it, since a section runs to the next heading
// that isn't below it. It is false when pos comes before every heading.
func SectionAt(hs []Heading, pos int) (Heading, bool) {
	for i := len(hs) - 1; i >= 0; i-- {
		if hs[i].Span.Start <= pos {
			return hs[i], true
		}
	}
	return Heading{}, false
}
