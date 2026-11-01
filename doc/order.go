package doc

import (
	"fmt"
	"slices"
	"strings"
)

// OrderProblem is something in a document that depends on the order of
// its blocks, and might be wrong in this order. Key names it, so the
// problems before and after a move can be compared (see NewProblems).
type OrderProblem struct {
	Key  string
	Text string
}

// OrderProblems lists what in d depends on block order and may be wrong:
//
//   - a condition, placeholder or tape that reads a block below it, which
//     hasn't run yet when it is read;
//   - a condition or placeholder such as {{$?}} that reads the runnable
//     block above it, naming that block, since a move changes which it is;
//   - each runnable block or input inside an mdsh:if section, naming the
//     sections, since a move can take it in or out of one;
//   - each var= bound by more than one input, naming the one that wins.
//
// Some of these are only facts, not mistakes, so show the problems a
// change added, with NewProblems. runnable says which blocks run.
func (d *Document) OrderProblems(runnable func(*Block) bool) []OrderProblem {
	var out []OrderProblem
	add := func(key, text string) { out = append(out, OrderProblem{key, text}) }
	pos := map[*Block]int{}
	for i, b := range d.Blocks {
		pos[b] = i
	}
	// above is the runnable block before pos, or nil.
	above := func(at int) *Block {
		var p *Block
		for _, b := range d.Blocks {
			if b.Span.End > at {
				break
			}
			if runnable(b) {
				p = b
			}
		}
		return p
	}
	// read checks a ref read at offset at, by what (a description).
	read := func(r Ref, at int, what, key string) {
		switch {
		case r.BlockID != "":
			if b := d.Block(r.BlockID); b != nil && b.Span.Start > at {
				add("later\x00"+key+"\x00"+r.String(), fmt.Sprintf("%s reads %s, which comes after it, so it hasn't run yet", what, r))
			}
		case r.Fact():
			p := above(at)
			id, name := "", "no block, since none runs above it"
			if p != nil {
				id, name = p.ID, d.name(p)
			}
			add("above\x00"+key+"\x00"+r.String()+"\x00"+id, fmt.Sprintf("%s reads %s from the block above it, which is now %s", what, r, name))
		}
	}

	for _, b := range d.Blocks {
		for _, a := range condAttrs {
			if c, ok := b.Cond(a); ok && c.Ref != nil {
				read(*c.Ref, b.Span.Start, fmt.Sprintf("%s=%s on %s", a, c.Text, d.name(b)), b.ID+"\x00"+a)
			}
		}
		if b.IsTape() && !b.IsInclude() {
			if t := d.Block(b.Attrs["for"]); t != nil {
				for _, r := range TextRefs(b.Body) {
					read(r, t.Span.Start, "the tape for "+d.name(t), b.Attrs["for"]+"\x00tape")
				}
			}
		}
	}
	for _, p := range d.Placeholders {
		read(p.Ref, p.Span.Start, fmt.Sprintf("%s at line %d", p.Ref, lineNo(d.Src, p.Span.Start)), "text")
	}
	for _, s := range d.Sections {
		if s.Cond.Ref != nil {
			read(*s.Cond.Ref, s.If.Start, fmt.Sprintf("the mdsh:if %s at line %d", s.Cond.Text, lineNo(d.Src, s.If.Start)), "if\x00"+s.Cond.Text)
		}
	}
	for _, f := range d.Folds {
		if f.Cond.Ref != nil {
			read(*f.Cond.Ref, f.Span.Start, fmt.Sprintf("the mdsh:collapse %s at line %d", f.Cond.Text, lineNo(d.Src, f.Span.Start)), "fold\x00"+f.Cond.Text)
		}
	}

	for _, b := range d.Blocks {
		if !runnable(b) && !b.IsInput() {
			continue
		}
		var conds []string
		for _, s := range d.Sections {
			if s.If.Start < b.Span.Start && b.Span.Start < s.End.Start {
				conds = append(conds, s.Cond.Text)
			}
		}
		if len(conds) > 0 {
			add("section\x00"+b.ID+"\x00"+strings.Join(conds, "\x00"), fmt.Sprintf("%s is inside mdsh:if %s, so it is left out when that is false", d.name(b), strings.Join(conds, " and ")))
		} else {
			add("section\x00"+b.ID, fmt.Sprintf("%s is outside every mdsh:if section", d.name(b)))
		}
	}

	binders := map[string][]*Block{}
	var vars []string
	for _, b := range d.Blocks {
		if v := b.Attrs["var"]; v != "" && b.IsInput() {
			if binders[v] == nil {
				vars = append(vars, v)
			}
			binders[v] = append(binders[v], b)
		}
	}
	for _, v := range vars {
		if bs := binders[v]; len(bs) > 1 {
			add("var\x00"+v+"\x00"+bs[0].ID, fmt.Sprintf("%s now sets var=%s, since it is the first input that binds it", d.name(bs[0]), v))
		}
	}
	return out
}

// NewProblems returns the problems in after that weren't in before.
func NewProblems(before, after []OrderProblem) []OrderProblem {
	var out []OrderProblem
	for _, p := range after {
		if !slices.ContainsFunc(before, func(q OrderProblem) bool { return q.Key == p.Key }) {
			out = append(out, p)
		}
	}
	return out
}

// name is how messages name b: #id, or the block at line N.
func (d *Document) name(b *Block) string {
	if l := d.Label(b); !strings.HasPrefix(l, "at line ") {
		return "#" + l
	}
	return "the block at line " + strings.TrimPrefix(d.Label(b), "at line ")
}
