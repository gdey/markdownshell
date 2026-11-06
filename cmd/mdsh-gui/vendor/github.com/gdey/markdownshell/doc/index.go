package doc

import (
	"fmt"
	"slices"
	"sort"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Index answers questions about d that each look over the whole
// document: where blocks and sections can move to, what uses a block,
// and how depends= limits a move. Asking d itself scans the document
// every time; an Index works each answer out once, so a caller asking
// for every block, as laying out a document's cells does, takes time in
// proportion to the document rather than its square. An Index holds
// while d doesn't change.
type Index struct {
	d *Document

	parsed   bool
	bounds   []int
	headings []Heading

	pos  map[*Block]int    // each block's index in d.Blocks
	byID map[string]*Block // the first block with each id, as Block

	uses map[string][]use // by the id used

	edges       bool
	deps, rdeps [][]edge // by block index: its depends=, and those on it
}

// NewIndex indexes d.
func (d *Document) NewIndex() *Index {
	x := &Index{d: d, pos: make(map[*Block]int, len(d.Blocks)), byID: make(map[string]*Block, len(d.Blocks))}
	for i, b := range d.Blocks {
		x.pos[b] = i
		if _, ok := x.byID[b.ID]; !ok {
			x.byID[b.ID] = b
		}
	}
	return x
}

// parse finds the top-level markdown blocks' starts and the headings,
// with one parse of the document.
func (x *Index) parse() {
	if x.parsed {
		return
	}
	x.parsed = true
	d := x.d
	body := d.bodyStart()
	root := gfm.Parser().Parse(text.NewReader(d.Src))
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		s, ok := nodeStart(c, d.Src)
		if !ok || s < body || d.inside(s) {
			continue
		}
		x.bounds = append(x.bounds, s)
		if h, ok := c.(*ast.Heading); ok {
			x.headings = append(x.headings, Heading{Level: h.Level, Span: Span{s, len(d.Src)}})
		}
	}
	x.bounds = append(x.bounds, len(d.Src))
	slices.Sort(x.bounds)
	x.bounds = slices.Compact(x.bounds)
	CloseSections(x.headings, len(d.Src))
}

// Boundaries is d.Boundaries.
func (x *Index) Boundaries() []int {
	x.parse()
	return x.bounds
}

// IsBoundary reports whether pos is one of Boundaries.
func (x *Index) IsBoundary(pos int) bool {
	x.parse()
	_, ok := slices.BinarySearch(x.bounds, pos)
	return ok
}

// Headings is d.Headings.
func (x *Index) Headings() []Heading {
	x.parse()
	return x.headings
}

// Section is d.Section.
func (x *Index) Section(start int) (Span, int, error) {
	x.parse()
	i := sort.Search(len(x.headings), func(i int) bool { return x.headings[i].Span.Start >= start })
	if i == len(x.headings) || x.headings[i].Span.Start != start {
		return Span{}, 0, fmt.Errorf("no heading there")
	}
	return x.headings[i].Span, x.headings[i].Level, nil
}

// Unit is d.Unit.
func (x *Index) Unit(b *Block) (Span, error) {
	i, ok := x.pos[b]
	if !ok {
		return Span{}, fmt.Errorf("not a block of this document")
	}
	if b.Prefix != "" || !x.IsBoundary(b.Span.Start) {
		return Span{}, fmt.Errorf("the block %s is in a list, a quote or an output, and only top-level blocks move", x.d.Label(b))
	}
	return x.d.unitFrom(i), nil
}

// DeleteBlockers is d.DeleteBlockers.
func (x *Index) DeleteBlockers(b *Block) []string {
	u, err := x.Unit(b)
	if err != nil {
		return nil
	}
	return x.users(b.ID, u)
}

// A use is something that uses a block's id: where it is, or -1 when it
// counts wherever it is, and what to call it, worked out when asked.
type use struct {
	pos  int
	what func() string
}

// users is d.users.
func (x *Index) users(id string, span Span) []string {
	if x.uses == nil {
		x.findUses()
	}
	var out []string
	for _, u := range x.uses[id] {
		if u.pos >= span.Start && u.pos < span.End {
			continue
		}
		if s := u.what(); !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// findUses lists what uses each id, in the order users names them:
// placeholders, then the blocks in order, each with its depends=, its
// conditions and, for a tape, the block it is for and those its text
// reads, then mdsh:if sections and mdsh:collapse lines.
func (x *Index) findUses() {
	d := x.d
	x.uses = map[string][]use{}
	add := func(id string, pos int, what func() string) { x.uses[id] = append(x.uses[id], use{pos, what}) }
	for _, p := range d.Placeholders {
		add(p.BlockID, p.Span.Start, p.Ref.String)
	}
	for _, o := range d.Blocks {
		pos := o.Span.Start
		for _, id := range o.Depends() {
			add(id, pos, func() string { return d.name(o) + "'s depends=" })
		}
		for _, a := range condAttrs {
			if c, ok := o.Cond(a); ok && c.Ref != nil {
				add(c.Ref.BlockID, pos, func() string { return fmt.Sprintf("%s's %s=", d.name(o), a) })
			}
		}
		if !o.IsTape() {
			continue
		}
		tape := func() string { return fmt.Sprintf("the tape at line %d", lineNo(d.Src, pos)) }
		add(o.Attrs["for"], pos, tape)
		if !o.IsInclude() {
			for _, r := range TextRefs(o.Body) {
				add(r.BlockID, pos, tape)
			}
		}
	}
	for _, sec := range d.Sections {
		if sec.Cond.Ref != nil {
			add(sec.Cond.Ref.BlockID, -1, func() string { return fmt.Sprintf("the mdsh:if at line %d", lineNo(d.Src, sec.If.Start)) })
		}
	}
	for _, f := range d.Folds {
		if f.Cond.Ref != nil {
			add(f.Cond.Ref.BlockID, -1, func() string { return fmt.Sprintf("the mdsh:collapse at line %d", lineNo(d.Src, f.Span.Start)) })
		}
	}
}

// Dep is d.Dep.
func (x *Index) Dep(id string) *Block {
	b := x.byID[id]
	if b == nil || b.IsInclude() || b.IsTape() || b.IsInput() {
		return nil
	}
	return b
}

// An edge is a depends= that counts: block b names dep, a block above
// it that runs. k is the name's place in b's depends=.
type edge struct{ b, k, dep int }

// findEdges finds the depends= edges, by block.
func (x *Index) findEdges() {
	if x.edges {
		return
	}
	x.edges = true
	d := x.d
	x.deps = make([][]edge, len(d.Blocks))
	x.rdeps = make([][]edge, len(d.Blocks))
	for i, b := range d.Blocks {
		for k, id := range b.Depends() {
			dep := x.Dep(id)
			if dep == nil || dep == b || dep.Span.Start >= b.Span.Start {
				continue
			}
			e := edge{i, k, x.pos[dep]}
			x.deps[i] = append(x.deps[i], e)
			x.rdeps[e.dep] = append(x.rdeps[e.dep], e)
		}
	}
}

// DependsLimits is d.DependsLimits. It looks only at the edges of the
// blocks in span.
func (x *Index) DependsLimits(span Span) MoveLimits {
	x.findEdges()
	d := x.d
	l := MoveLimits{After: -1, Before: len(d.Src)}
	from, to := d.BlocksIn(span)
	in := func(i int) bool { return i >= from && i < to }
	// After: the lowest block above span that a block in it depends on,
	// the first such depends= in the document's order when two name it.
	for i := from; i < to; i++ {
		for _, e := range x.deps[i] {
			if dep := d.Blocks[e.dep]; !in(e.dep) && dep.Span.Start > l.After {
				l.After, l.AfterDep, l.AfterOn = dep.Span.Start, d.Blocks[i], dep
			}
		}
	}
	// Before: the highest block below span that depends on one in it,
	// with its first such depends=.
	best := edge{b: -1}
	for i := from; i < to; i++ {
		for _, e := range x.rdeps[i] {
			if in(e.b) {
				continue
			}
			if best.b < 0 || e.b < best.b || e.b == best.b && e.k < best.k {
				best = e
			}
		}
	}
	if best.b >= 0 {
		l.Before, l.BeforeDep, l.BeforeOn = d.Blocks[best.b].Span.Start, d.Blocks[best.b], d.Blocks[best.dep]
	}
	return l
}
