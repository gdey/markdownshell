package notebook

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// Cells lays the document out for drawing: one cell per top-level
// markdown block of prose, such as a paragraph, heading, list or table,
// and one per fenced block. Blocks mdsh can't run are drawn as prose, as
// mdsh -i does. What printing leaves out, such as the front matter and
// mdsh:if sections that are off, is left out here too.
func (n *Notebook) Cells() []Cell {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.cells()
}

// cells is Cells for a caller that holds n.mu.
func (n *Notebook) cells() []Cell {
	return n.newCellLayout().all()
}

// outputAt is when the run that made block b's output out started: r's,
// when b ran this session, or else the at= the file saved with it.
func outputAt(b *doc.Block, r *run.Result, out []byte) string {
	if len(bytes.TrimSpace(out)) == 0 {
		return ""
	}
	if r.Ran && !r.Start.IsZero() {
		return r.Start.UTC().Format(time.RFC3339)
	}
	return b.CurrentAt
}

// blockAt is the index of the block that starts at pos, or -1.
func blockAt(d *doc.Document, pos int) int {
	i := sort.Search(len(d.Blocks), func(i int) bool { return d.Blocks[i].Span.Start >= pos })
	if i < len(d.Blocks) && d.Blocks[i].Span.Start == pos {
		return i
	}
	return -1
}

// dropLimits sets c's DropAfter and DropBefore from span's
// doc.Document.DependsLimits.
func (c *Cell) dropLimits(x *doc.Index, span doc.Span) {
	l := x.DependsLimits(span)
	c.DropAfter = l.After
	if l.BeforeDep != nil {
		c.DropBefore = l.Before
	}
}

// unitEnd is where the unit that moves with block i ends, as
// doc.Document.Unit has it, given the document's boundaries, and false
// for a block that can't move. It saves Unit's parse for every block.
func unitEnd(d *doc.Document, i int, x *doc.Index) (int, bool) {
	b := d.Blocks[i]
	if b.Prefix != "" || !x.IsBoundary(b.Span.Start) {
		return 0, false
	}
	end := b.Output.End
	if i+1 < len(d.Blocks) {
		t := d.Blocks[i+1]
		if t.IsTape() && t.Attrs["for"] == b.ID && len(bytes.TrimSpace(d.Src[end:t.Span.Start])) == 0 {
			end = t.Output.End
		}
	}
	return end, true
}

// layout is the state Cells keeps while it lays out the prose.
type layout struct {
	n      *Notebook
	lookup doc.Lookup
	cuts   []doc.Span
	refs   []byte
	tasks  []int // taskMarks
	cells  []Cell
	// parsed is the parse of src[parsedFrom:parsedTo], the prose prose
	// last laid out.
	parsed               *proseParse
	parsedFrom, parsedTo int
	// pending is the markdown of the prose cells prose added, in order,
	// for their bodies, refs left off, and kept the spans of the source
	// each draws, for its tasks.
	pending [][]byte
	kept    [][]doc.Span
	// at and line are where lineNo last counted to and its line there.
	at, line int
}

// lineNo is lineNo(src, pos) for the document. The blocks are laid out
// in order, so it counts on from where it last got to rather than from
// the top each time, which in a long document took longer than the rest
// of the layout.
func (l *layout) lineNo(pos int) int {
	src := l.n.d.Src
	if pos < l.at || l.line == 0 {
		l.at, l.line = 0, 1
	}
	l.line += bytes.Count(src[l.at:pos], []byte("\n"))
	l.at = pos
	return l.line
}

// name is how the view names b: as doc.Document.Label does, without
// "at ".
func (l *layout) name(b *doc.Block) string {
	if b.Named() && !doc.GeneratedID(b.ID) {
		return b.ID
	}
	return fmt.Sprintf("line %d", l.lineNo(b.Span.Start))
}

// prose adds a cell for each top-level markdown block in src[from:to].
// Each covers its block's lines up to the next block, less the blank
// lines between them, so the lines goldmark gives no node of their own,
// such as a setext heading's underline or a thematic break, belong to
// the block above.
func (l *layout) prose(from, to int) {
	src := l.n.d.Src
	if from >= to {
		return
	}
	p := l.n.parseProse(src[from:to])
	l.parsed, l.parsedFrom, l.parsedTo = p, from, to
	for i, s := range p.starts {
		s += from
		e := to
		if i+1 < len(p.starts) {
			e = from + p.starts[i+1]
		}
		e = s + trimBlankLines(src[s:e])
		if e > s {
			l.part(doc.Span{Start: s, End: e}, p.headings[s-from])
		}
	}
}

// proseParse is what the layout takes from goldmark's parse of a piece
// of prose, by offset in it: where each of its cells starts, the
// headings among them, and its task marks.
type proseParse struct {
	starts   []int
	headings map[int]heading
	marks    []int
}

// parseProse is md's proseParse, from the notebook's cache of them by
// md where it is there. Each layout parsed the whole document's prose
// again, which, at a few seconds for a long one, was most of the time a
// keystroke took to show; a keystroke changes one piece of it. The cache
// holds the pieces of the last two layouts.
func (n *Notebook) parseProse(md []byte) *proseParse {
	key := proseKey(md)
	if p, ok := n.parses[key]; ok {
		return p
	}
	if p, ok := n.oldParses[key]; ok {
		n.parses[key] = p
		return p
	}
	root := n.md.Parser().Parse(text.NewReader(md))
	p := &proseParse{starts: []int{leadingBlank(md)}, headings: map[int]heading{}, marks: marksOf(root, md)}
	var inside ast.Node // the last block of HTML the block is inside
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		if inside != nil {
			if c == inside {
				inside = nil
			}
			continue
		}
		inside = htmlEnd(c, md)
		if s, ok := firstLine(c, md); ok {
			start := lineStart(md, s)
			p.starts = append(p.starts, start)
			if h, ok := c.(*ast.Heading); ok {
				p.headings[start] = heading{h.Level, html.UnescapeString(string(nodeText(h, md)))}
			}
		}
	}
	slices.Sort(p.starts)
	p.starts = slices.Compact(p.starts)
	if n.parses == nil {
		n.parses = map[[16]byte]*proseParse{}
	}
	n.parses[key] = p
	return p
}

// newParses starts the cache of prose parses afresh for a new layout,
// keeping the last one's, for the pieces that didn't change.
func (n *Notebook) newParses() {
	n.oldParses, n.parses = n.parses, map[[16]byte]*proseParse{}
}

// proseKey is a key for md in the cache of parses: its hash, as holding
// a long document's prose twice over as keys took as much memory again.
func proseKey(md []byte) [16]byte {
	h := fnv.New128a()
	h.Write(md)
	var k [16]byte
	h.Sum(k[:0])
	return k
}

// htmlEnd is the block of HTML that closes the elements node opens,
// when node is a block of HTML that leaves one open, such as
// <details><summary>More</summary> above markdown and a </details> line
// below it, so the markdown between is drawn inside them, as one cell,
// not as cells of their own. It is nil when node opens nothing or what
// it opens isn't closed in src, the prose that was parsed.
func htmlEnd(node ast.Node, src []byte) ast.Node {
	if _, ok := node.(*ast.HTMLBlock); !ok {
		return nil
	}
	open := htmlTags(nil, node, src)
	if len(open) == 0 {
		return nil
	}
	for c := node.NextSibling(); c != nil; c = c.NextSibling() {
		if _, ok := c.(*ast.HTMLBlock); !ok {
			continue
		}
		if open = htmlTags(open, c, src); len(open) == 0 {
			return c
		}
	}
	return nil
}

// htmlTags is open, the elements left open, after the tags of the block
// of HTML node. A closing tag closes the latest element of its name and
// those opened after it; one with nothing to close is passed over.
func htmlTags(open []string, node ast.Node, src []byte) []string {
	var b bytes.Buffer
	lines := node.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		b.Write(seg.Value(src))
	}
	if h := node.(*ast.HTMLBlock); h.HasClosure() {
		seg := h.ClosureLine
		b.Write(seg.Value(src))
	}
	z := html.NewTokenizer(&b)
	for {
		switch z.Next() {
		case html.ErrorToken:
			return open
		case html.StartTagToken:
			name, _ := z.TagName()
			if !voidElements[string(name)] {
				open = append(open, string(name))
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			for i := len(open) - 1; i >= 0; i-- {
				if open[i] == string(name) {
					open = open[:i]
					break
				}
			}
		}
	}
}

// voidElements are the elements HTML never closes.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true,
	"track": true, "wbr": true,
}

// heading is a heading's level and its text.
type heading struct {
	level int
	title string
}

// part adds the prose at span, leaving out what is cut. heading is its
// level if it is a heading.
func (l *layout) part(span doc.Span, heading heading) {
	d := l.n.d
	var md bytes.Buffer
	kept := keep(span, l.cuts)
	for _, k := range kept {
		from := k.Start
		for _, p := range d.Placeholders {
			if p.Span.Start < from || p.Span.End > k.End {
				continue
			}
			md.Write(d.Src[from:p.Span.Start])
			v, ok := run.LookupAt(l.lookup, d, l.n.results, p.Span.Start)(p.Ref)
			if !ok {
				v = p.Value
			}
			md.WriteString(v)
			from = p.Span.End
		}
		md.Write(d.Src[from:k.End])
	}
	if len(bytes.TrimSpace(md.Bytes())) == 0 {
		return
	}
	l.pending = append(l.pending, md.Bytes())
	l.kept = append(l.kept, kept)
	l.cells = append(l.cells, Cell{Block: -1, Heading: heading.level, Title: heading.title, From: span.Start, To: span.End})
}

// holds reports whether b's attr is set and its condition holds.
func holds(b *doc.Block, attr string, lookup doc.Lookup) bool {
	c, ok := b.Cond(attr)
	return ok && c.Eval(lookup)
}

// lineNo is the line pos is on, counting from 1.
func lineNo(src []byte, pos int) int {
	return bytes.Count(src[:pos], []byte("\n")) + 1
}

// firstLine is where the first line of source under node starts, as an
// offset in src, what was parsed. A fence's lines are its body, so its
// opening line is found from its info string or the line above the body.
func firstLine(node ast.Node, src []byte) (int, bool) {
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
				first, ok = lineStart(src, lines.At(0).Start-1), true
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
	return first, ok
}

// linkRefs are src's link reference definitions, so prose drawn one
// block at a time still resolves [text][ref] links: the lines that start
// with at most three spaces and [, and are whole definitions. Looking
// at each line's start first, rather than running a regexp over the
// whole file, keeps it quick on a long document.
func linkRefs(src []byte) []byte {
	var out [][]byte
	for pos := 0; pos < len(src); {
		end := bytes.IndexByte(src[pos:], '\n')
		if end < 0 {
			end = len(src)
		} else {
			end += pos
		}
		line := src[pos:end]
		k := 0
		for k < len(line) && k < 3 && line[k] == ' ' {
			k++
		}
		if k < len(line) && line[k] == '[' && linkRef.Match(line) {
			out = append(out, linkRef.Find(line))
		}
		pos = end + 1
	}
	return bytes.Join(out, []byte("\n"))
}

var linkRef = regexp.MustCompile(`^ {0,3}\[[^\]\n]+\]:[ \t]*\S.*$`)

// cuts are the parts of d printing leaves out: the lines of mdsh:if
// sections, the whole of those that are off, and mdsh:collapse lines. Cells start after the
// shebang and front matter.
func cuts(d *doc.Document, results []*run.Result, lookup doc.Lookup) []doc.Span {
	var cuts []doc.Span
	for _, s := range d.Sections {
		if s.Cond.Eval(run.LookupAt(lookup, d, results, s.If.Start)) {
			cuts = append(cuts, s.If, s.End)
		} else {
			cuts = append(cuts, doc.Span{Start: s.If.Start, End: s.End.End})
		}
	}
	for _, f := range d.Folds {
		cuts = append(cuts, f.Span)
	}
	return cuts
}

// inCut reports whether pos is in one of cuts.
func inCut(pos int, cuts []doc.Span) bool {
	return slices.ContainsFunc(cuts, func(c doc.Span) bool { return pos >= c.Start && pos < c.End })
}

// keep is span less the cuts.
func keep(span doc.Span, cuts []doc.Span) []doc.Span {
	kept := []doc.Span{span}
	for _, c := range cuts {
		var next []doc.Span
		for _, k := range kept {
			if c.End <= k.Start || c.Start >= k.End {
				next = append(next, k)
				continue
			}
			if c.Start > k.Start {
				next = append(next, doc.Span{Start: k.Start, End: c.Start})
			}
			if c.End < k.End {
				next = append(next, doc.Span{Start: c.End, End: k.End})
			}
		}
		kept = next
	}
	return kept
}

// lineStart is the start of the line holding pos.
func lineStart(src []byte, pos int) int {
	return bytes.LastIndexByte(src[:pos], '\n') + 1
}

// leadingBlank is the length of the blank lines at the start of src.
func leadingBlank(src []byte) int {
	n := 0
	for n < len(src) {
		end := bytes.IndexByte(src[n:], '\n')
		if end < 0 || len(bytes.TrimSpace(src[n:n+end])) > 0 {
			break
		}
		n += end + 1
	}
	return n
}

// trimBlankLines is the length of src less the blank lines at its end,
// keeping the last line's newline.
func trimBlankLines(src []byte) int {
	end := len(src)
	for end > 0 {
		s := end
		if src[s-1] == '\n' {
			s--
		}
		start := bytes.LastIndexByte(src[:s], '\n') + 1
		if len(bytes.TrimSpace(src[start:end])) > 0 {
			break
		}
		end = start
	}
	return end
}

// Text is the whole file as the split view shows it.
func (n *Notebook) Text() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return string(n.d.Src)
}

// SetText replaces the whole file with text, as typed in the split
// view, if the file is still base, the text the typing started from,
// and reports whether it did. When something else changed the file
// since, such as a run's output or an include filling its fence, it is
// left alone, so the editor can fold the change into what was typed and
// try again instead of throwing the change away. Blocks whose code
// didn't change keep their output.
func (n *Notebook) SetText(base, text string) (bool, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.busy() {
		return false, ErrBusy
	}
	if string(n.d.Src) != base {
		return false, nil
	}
	return true, n.setSource([]byte(text))
}

func (n *Notebook) setSource(src []byte) error {
	if bytes.Equal(src, n.d.Src) {
		return nil
	}
	if err := n.d.SetSource(src); err != nil {
		return err
	}
	n.dirty = true
	return n.sync()
}

// tasksIn are the numbers of the tasks whose marks are in span. The
// marks are in order.
func tasksIn(marks []int, span doc.Span) []int {
	var tasks []int
	for t := sort.SearchInts(marks, span.Start); t < len(marks) && marks[t] < span.End; t++ {
		tasks = append(tasks, t)
	}
	return tasks
}

// timeoutMs is b's timeout= in milliseconds, or 0 when it has none or
// it doesn't parse.
func timeoutMs(b *doc.Block) int64 {
	t, ok := b.Attrs["timeout"]
	if !ok {
		return 0
	}
	d, err := time.ParseDuration(t)
	if err != nil || d <= 0 {
		return 0
	}
	return d.Milliseconds()
}

// secretAttr marks a block whose code and output hold a secret.
const secretAttr = "secret"
