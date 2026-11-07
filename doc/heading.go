package doc

import (
	"sort"
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
	return d.NewIndex().Headings()
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
