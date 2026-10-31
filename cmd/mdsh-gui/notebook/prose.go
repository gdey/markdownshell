package notebook

import (
	"bytes"
	"regexp"
	"slices"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
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
	d := n.d
	l := layout{
		n:      n,
		lookup: run.NewLookup(d, n.results, environ()),
		refs:   linkRefs(d.Src),
		tasks:  n.taskMarks(),
	}
	l.cuts = cuts(d, n.results, l.lookup)
	pieces := render.Pieces(d, n.results, render.Options{Env: environ()})
	pos := max(d.FrontMatter.End, len(d.Shebang))
	for i, b := range d.Blocks {
		l.prose(pos, b.Span.Start)
		pos = b.Output.End
		code, out := pieces[3*i+1], pieces[3*i+2]
		if len(code) == 0 {
			continue
		}
		span := doc.Span{Start: b.Span.Start, End: b.Output.End}
		if b.IsTextbox() {
			t := b.Textbox()
			l.cells = append(l.cells, Cell{Block: -1, Textbox: &TextboxCell{Block: i, ID: b.ID, Text: t.Text, Lines: t.Lines, Placeholder: t.Placeholder, Problem: b.VarTaken}, From: span.Start, To: span.End})
			continue
		}
		if b.IsChoice() {
			c := b.Choice()
			l.cells = append(l.cells, Cell{Block: -1, Choice: &ChoiceCell{Block: i, ID: b.ID, Mode: c.Mode, Options: c.Options, Placeholder: c.Placeholder, Problem: b.VarTaken}, From: span.Start, To: span.End})
			continue
		}
		if !run.Runnable(b) {
			l.cells = append(l.cells, Cell{Block: -1, HTML: n.html(append(append([]byte(nil), code...), out...)), From: span.Start, To: span.End})
			continue
		}
		r := n.results[i]
		l.cells = append(l.cells, Cell{
			Block:       i,
			ID:          b.ID,
			Lang:        b.Lang,
			Body:        b.Body,
			Attrs:       attrsText(b.Info),
			Output:      n.html(out),
			OutputTasks: tasksIn(l.tasks, b.Output),
			Ran:         r.Ran,
			Status:      r.Status,
			Failed:      r.Failed(),
			Running:     n.bg[b],
			From:        span.Start,
			To:          span.End,
		})
	}
	l.prose(pos, len(d.Src))
	pos, line := 0, 1
	for i := range l.cells {
		c := &l.cells[i]
		line += bytes.Count(d.Src[pos:c.From], []byte("\n"))
		pos = c.From
		c.Line = line
	}
	return l.cells
}

// layout is the state Cells keeps while it lays out the prose.
type layout struct {
	n      *Notebook
	lookup doc.Lookup
	cuts   []doc.Span
	refs   []byte
	tasks  []int // taskMarks
	cells  []Cell
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
	starts := []int{from + leadingBlank(src[from:to])}
	root := l.n.md.Parser().Parse(text.NewReader(src[from:to]))
	for c := root.FirstChild(); c != nil; c = c.NextSibling() {
		if s, ok := firstLine(c, src[from:to]); ok {
			starts = append(starts, from+lineStart(src[from:to], s))
		}
	}
	slices.Sort(starts)
	starts = slices.Compact(starts)
	for i, s := range starts {
		e := to
		if i+1 < len(starts) {
			e = starts[i+1]
		}
		e = s + trimBlankLines(src[s:e])
		if e > s {
			l.part(doc.Span{Start: s, End: e})
		}
	}
}

// part adds the prose at span, leaving out what is cut.
func (l *layout) part(span doc.Span) {
	d := l.n.d
	var md bytes.Buffer
	var tasks []int
	kept := keep(span, l.cuts)
	for _, k := range kept {
		tasks = append(tasks, tasksIn(l.tasks, k)...)
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
	if len(l.refs) > 0 {
		md.WriteString("\n\n")
		md.Write(l.refs)
	}
	l.cells = append(l.cells, Cell{Block: -1, HTML: l.n.html(md.Bytes()), Tasks: tasks, From: span.Start, To: span.End})
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
// block at a time still resolves [text][ref] links.
func linkRefs(src []byte) []byte {
	return bytes.Join(linkRef.FindAll(src, -1), []byte("\n"))
}

var linkRef = regexp.MustCompile(`(?m)^ {0,3}\[[^\]\n]+\]:[ \t]*\S.*$`)

// cuts are the parts of d printing leaves out: the lines of mdsh:if
// sections, and the whole of those that are off. Cells start after the
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
	return cuts
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
// view. Blocks whose code didn't change keep their output.
func (n *Notebook) SetText(text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	return n.setSource([]byte(text))
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

// tasksIn are the numbers of the tasks whose marks are in span.
func tasksIn(marks []int, span doc.Span) []int {
	var tasks []int
	for t, p := range marks {
		if p >= span.Start && p < span.End {
			tasks = append(tasks, t)
		}
	}
	return tasks
}
