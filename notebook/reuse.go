package notebook

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// blockState is what a block's cell is laid out from besides the
// source: where the block is, and its run.
type blockState struct {
	start, end                              int
	r                                       *run.Result
	ran, failed, timedOut, running, skipped bool
	stale, bg, term                         bool
	status                                  int
	seq                                     uint64
	dur                                     time.Duration
	at                                      time.Time
}

// blockStates are the states of l's document's blocks.
func (l *cellLayout) blockStates() []blockState {
	n, d := l.n, l.n.d
	states := make([]blockState, len(d.Blocks))
	for i, b := range d.Blocks {
		s := blockState{start: b.Span.Start, end: b.Output.End, stale: l.stale[i], bg: n.bg[b], term: n.bgTerms[i] != nil}
		if i < len(n.results) && n.results[i] != nil {
			r := n.results[i]
			s.r, s.ran, s.failed, s.timedOut, s.running, s.skipped = r, r.Ran, r.Failed(), r.TimedOut, r.Running, r.Skipped
			s.status, s.seq, s.dur, s.at = r.Status, r.Seq, r.Duration, r.Start
		}
		states[i] = s
	}
	return states
}

// reuse lays l out from old, the layout of the document before an edit,
// when the edit was to the prose between two blocks, as typing in prose
// is: the cells before that prose are old's, and those after it old's
// moved, so only that prose is laid out again. Laying out every cell
// was most of the time a keystroke in a long file took. It reports
// whether it could, setting s to how the cells changed; when it can't,
// as when the edit changed a block, a heading's level or the tasks'
// count, l is left to lay out afresh. The cells keep their move limits,
// moved, until the index comes. old's cells become l's.
func (l *cellLayout) reuse(old *cellLayout, s *Splice) bool {
	n, d := l.n, l.n.d
	src, was := d.Src, old.src
	if old.states == nil || len(d.Blocks) != old.blocks || l.first != old.first ||
		!slices.Equal(l.secrets, old.secrets) || !bytes.Equal(l.refs, old.refs) || len(l.cuts) != len(old.cuts) {
		return false
	}
	// The edit took was[p:len(was)-q] and put src[p:len(src)-q] there.
	p := commonPrefix(was, src)
	q := commonSuffix(was[p:], src[p:])
	wasEnd, end := len(was)-q, len(src)-q
	delta := len(src) - len(was)
	if p < l.first {
		return false
	}
	// The prose it is in, gs to ge, between blocks k-1 and k; or the
	// body of block j, k-1, from its fence to the end of its output.
	k := sort.Search(len(d.Blocks), func(i int) bool { return d.Blocks[i].Span.Start >= p })
	gs, ge := l.first, len(src)
	j := -1
	if k > 0 {
		gs = d.Blocks[k-1].Output.End
		if b := d.Blocks[k-1]; p < b.Span.End {
			j, gs = k-1, b.Span.Start
			if p < b.BodySpan.Start || end > b.BodySpan.End {
				return false
			}
		}
	}
	if j < 0 && k < len(d.Blocks) {
		ge = d.Blocks[k].Span.Start
	}
	if j >= 0 {
		ge = d.Blocks[j].Output.End
	}
	if gs > p || end > ge {
		return false
	}
	oldGe := ge - delta
	// The blocks are those before, moved after the edit, and ran as
	// they had; what the cuts leave out is too. An edited block is
	// new, and never ran, as it was.
	states := l.blockStates()
	for i, s := range states {
		w := old.states[i]
		if i >= k {
			w.start += delta
			w.end += delta
		}
		if i == j {
			w.end += delta
			if w.r == nil || s.r == nil || w.ran || w.running || w.skipped || w.bg {
				return false
			}
			w.r = s.r
		} else if !bytes.Equal(l.pieces[3*i+1], old.pieces[3*i+1]) {
			return false
		}
		if s != w || !bytes.Equal(l.pieces[3*i+2], old.pieces[3*i+2]) {
			return false
		}
	}
	for i, c := range l.cuts {
		w := old.cuts[i]
		switch {
		case w.End <= p:
		case w.Start >= wasEnd:
			w.Start += delta
			w.End += delta
		default:
			return false
		}
		if c != w {
			return false
		}
	}
	// The tasks after keep their numbers.
	mk := sort.SearchInts(old.marks, gs)
	mk2 := sort.SearchInts(old.marks, oldGe)
	var marks []int
	if j >= 0 {
		// Those in its output, which is after the edit.
		for _, m := range old.marks[mk:mk2] {
			marks = append(marks, m+delta-gs)
		}
	} else if gs < ge {
		marks = n.parseProse(src[gs:ge]).marks
	}
	if len(marks) != mk2-mk {
		return false
	}
	ca := sort.Search(len(old.cells), func(i int) bool { return old.cells[i].From >= gs })
	cb := sort.Search(len(old.cells), func(i int) bool { return old.cells[i].From >= oldGe })

	// The prose's cells, laid out on their own first, to check them.
	l.marks = make([]int, 0, len(old.marks))
	l.marks = append(l.marks, old.marks[:mk]...)
	for _, m := range marks {
		l.marks = append(l.marks, gs+m)
	}
	l.marks = append(l.marks, old.marks[mk2:]...)
	for i := mk + len(marks); i < len(l.marks); i++ {
		l.marks[i] += delta
	}
	l.tasks = l.marks
	if j >= 0 {
		// Its line, for its name, from the cell above.
		l.at, l.line = 0, 1
		if ca > 0 {
			w := &old.cells[ca-1]
			l.at, l.line = w.From, w.Line
		}
		l.blockCell(j, d.Blocks[j])
	} else {
		l.proseCells(gs, ge)
		l.fillTasks()
	}
	gap, gapBodies := l.cells, l.bodies
	// The headings' sections and folds are the same blocks and
	// collapse lines as before if the headings are the same levels;
	// if not, every heading's are found again once the cells are.
	resection := false
	h := ca
	for i := range gap {
		c := &gap[i]
		if c.Heading == 0 {
			continue
		}
		for h < cb && old.cells[h].Heading == 0 {
			h++
		}
		if h == cb || old.cells[h].Heading != c.Heading {
			resection = true
			break
		}
		w := &old.cells[h]
		c.SectionFrom, c.SectionTo, c.Collapsed = w.SectionFrom, w.SectionTo, w.Collapsed
		h++
	}
	for ; h < cb && !resection; h++ {
		resection = old.cells[h].Heading > 0
	}
	line, at := 1, 0
	if ca > 0 {
		w := &old.cells[ca-1]
		line, at = w.Line, w.From
	}
	for i := range gap {
		c := &gap[i]
		line += bytes.Count(src[at:c.From], []byte{'\n'})
		at = c.From
		c.Line = line
		c.DropAfter, c.DropBefore = -1, -1
		c.Hash = l.hash(i)
	}

	// Old's cells and bodies become l's, moved, as old is done with.
	lines := bytes.Count(src[p:end], []byte{'\n'}) - bytes.Count(was[p:wasEnd], []byte{'\n'})
	editLine := 0
	if ca > 0 {
		w := &old.cells[ca-1]
		editLine = w.Line + bytes.Count(src[w.From:p], []byte{'\n'})
	} else {
		editLine = 1 + bytes.Count(src[:p], []byte{'\n'})
	}
	cells, bodies := old.cells, old.bodies
	old.cells, old.bodies = nil, nil
	s.Line, s.Pos = lines, delta
	fix := func(i int) {
		s.FixAt = append(s.FixAt, i)
		s.Fixes = append(s.Fixes, headOf(&cells[i]))
	}
	for i := range ca {
		c := &cells[i]
		last := c.LastLine
		carryMoves(c, c, p, editLine, lines, delta)
		if c.Movable && c.LastLine != last {
			fix(i)
		}
	}
	// The prose's cells that are as they were, moved, at its two ends
	// stay the page's: only those between are sent.
	same := func(c, w *Cell, moved bool) bool {
		h, g := headOf(c), headOf(w)
		if moved {
			g = s.moved(g)
		}
		h.Flags, h.LastLine = h.Flags&^HeadMovable, 0
		g.Flags, g.LastLine = g.Flags&^HeadMovable, 0
		return sameHeadAs(h, g)
	}
	lo, hi := 0, 0
	for lo < len(gap) && lo < cb-ca && same(&gap[lo], &cells[ca+lo], false) {
		lo++
	}
	for hi < len(gap)-lo && hi < cb-ca-lo && same(&gap[len(gap)-1-hi], &cells[cb-1-hi], true) {
		hi++
	}
	s.At, s.Remove = ca+lo, cb-ca-lo-hi
	for k := range lo {
		c, w := &gap[k], &cells[ca+k]
		carryMoves(c, w, p, editLine, lines, delta)
		if c.Movable && c.LastLine != w.LastLine {
			s.FixAt = append(s.FixAt, ca+k)
			s.Fixes = append(s.Fixes, headOf(c))
		}
	}
	for k := range hi {
		c, w := &gap[len(gap)-1-k], &cells[cb-1-k]
		carryMoves(c, w, p, editLine, lines, delta)
		if w.LastLine > 0 {
			c.LastLine = w.LastLine + lines
		}
	}
	// A cell typed in takes the limits of the one it replaces.
	for k := lo; k < len(gap)-hi && k < cb-ca-hi; k++ {
		carryMoves(&gap[k], &cells[ca+k], p, editLine, lines, delta)
	}
	s.Heads = make(HeadList, 0, len(gap)-lo-hi)
	for k := lo; k < len(gap)-hi; k++ {
		s.Heads = append(s.Heads, headOf(&gap[k]))
	}
	var named []int
	for i := cb; i < len(cells); i++ {
		c := &cells[i]
		c.From += delta
		c.To += delta
		c.Line += lines
		if c.OutputLine > 0 {
			c.OutputLine += lines
		}
		last := c.LastLine
		carryMoves(c, c, p, editLine, lines, delta)
		if last > 0 {
			// It is after the edit, all of it.
			c.LastLine = last + lines
		}
		if c.Block >= 0 && lines != 0 {
			if b := d.Blocks[c.Block]; !b.Named() || doc.GeneratedID(b.ID) {
				c.Name = fmt.Sprintf("line %d", c.Line)
				named = append(named, i)
			}
		}
	}
	for i := range bodies {
		if b := &bodies[i]; b.block >= 0 {
			b.out = l.pieces[3*b.block+2]
		}
	}
	cells = slices.Replace(cells, ca, cb, gap...)
	bodies = slices.Replace(bodies, ca, cb, gapBodies...)
	for _, i := range named {
		fix(i - (cb - ca) + len(gap))
	}
	if resection {
		// The cells sent are as they are now, and the others whose
		// sections changed are fixed.
		sent := s.At + len(s.Heads)
		for _, i := range l.resection(cells) {
			if i < s.At || i >= sent {
				s.FixAt = append(s.FixAt, i)
			}
		}
		for k := range s.Heads {
			s.Heads[k] = headOf(&cells[s.At+k])
		}
		slices.Sort(s.FixAt)
		s.FixAt = slices.Compact(s.FixAt)
		s.Fixes = s.Fixes[:0]
		for _, i := range s.FixAt {
			s.Fixes = append(s.Fixes, headOf(&cells[i]))
		}
	}
	l.cells, l.bodies = cells, bodies
	l.blk, l.pos, l.markPos, l.folds = len(d.Blocks), len(src), len(src), len(d.Folds)
	l.step, l.done, l.states = 1, true, states
	s.Step = l.step
	return true
}

// resection sets the heading cells' sections and folds, as lines,
// foldHeadings and closeSection do as the cells are laid out, and
// returns the cells it changed.
func (l *cellLayout) resection(cells []Cell) []int {
	n, d := l.n, l.n.d
	var heads, open, changed []int
	hidden := map[int]bool{}
	section := func(h, end int) {
		c := &cells[h]
		from, to := d.BlocksIn(doc.Span{Start: c.From, End: end})
		if !slices.ContainsFunc(d.Blocks[from:to], run.Runnable) {
			from, to = 0, 0
		}
		c.SectionFrom, c.SectionTo = from, to
	}
	type was struct {
		from, to  int
		collapsed bool
	}
	before := map[int]was{}
	for i := range cells {
		c := &cells[i]
		if c.Heading > 0 {
			before[i] = was{c.SectionFrom, c.SectionTo, c.Collapsed}
			for len(open) > 0 && cells[open[len(open)-1]].Heading >= c.Heading {
				section(open[len(open)-1], c.From)
				open = open[:len(open)-1]
			}
			open = append(open, i)
			heads = append(heads, i)
		}
		if c.Hidden != "" {
			for _, h := range open {
				hidden[h] = true
			}
		}
	}
	for _, h := range open {
		section(h, len(d.Src))
	}
	folded := map[int]bool{}
	for _, f := range d.Folds {
		if !f.Cond.Eval(run.LookupAt(l.lookup, d, n.results, f.Span.Start)) {
			continue
		}
		if k := sort.Search(len(heads), func(k int) bool { return cells[heads[k]].From > f.Span.Start }) - 1; k >= 0 {
			folded[heads[k]] = true
		}
	}
	for _, h := range heads {
		c := &cells[h]
		c.Collapsed = folded[h] && !hidden[h]
		if (was{c.SectionFrom, c.SectionTo, c.Collapsed}) != before[h] {
			changed = append(changed, h)
		}
	}
	return changed
}

// undo leaves l as newCellLayout made it, for reuse to give up.
func (l *cellLayout) undo() {
	l.cells, l.bodies, l.marks, l.tasks = nil, nil, nil, nil
	l.pending, l.kept, l.parsed = l.pending[:0], l.kept[:0], nil
}

// chunk is how many bytes commonPrefix and commonSuffix compare at a
// time.
const chunk = 4096

// commonPrefix is how many bytes a and b start with alike.
func commonPrefix(a, b []byte) int {
	m := min(len(a), len(b))
	i := 0
	// A chunk at a time, as bytes.Equal is much quicker than a loop.
	for i+chunk <= m && bytes.Equal(a[i:i+chunk], b[i:i+chunk]) {
		i += chunk
	}
	for i < m && a[i] == b[i] {
		i++
	}
	return i
}

// commonSuffix is how many bytes a and b end with alike.
func commonSuffix(a, b []byte) int {
	m := min(len(a), len(b))
	i := 0
	for i+chunk <= m && bytes.Equal(a[len(a)-i-chunk:len(a)-i], b[len(b)-i-chunk:len(b)-i]) {
		i += chunk
	}
	for i < m && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	return i
}
