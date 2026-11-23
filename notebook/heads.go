package notebook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/weblink"
)

// A long document's cells are laid out top down, a chunk at a time, and
// sent as heads: each cell without what is costly to make or to send,
// its rendered prose, code and output, which is its body (see
// docs/design/ui-protocol.md, stage 2). The page asks for the first
// chunk as a file opens, draws the first screen from it, and asks for
// the rest as it goes, the way a browser lays a page out as it loads.
// It asks for the bodies of the cells near the view only. Cells, which
// lays out and renders everything, is the same layout run to its end.
//
// A head can change after it is sent, as more of the document is laid
// out: a heading's section isn't known until the next heading of its
// level, and the move limits wait for the whole document's parse, which
// the first chunk doesn't wait for. Those changes come back with a later
// chunk as fixes.

// Head is what the page keeps of every cell, to place it, guess its
// height, and list it in the outline and the dependency lines without
// its card: the rest of the cell comes with its body, from Bodies.
type Head struct {
	// Block is the block's index, or an input's, or -1 for prose.
	Block int
	// Kind is HeadProse, HeadBlock, HeadTextbox or HeadChoice.
	Kind int
	// Flags are HeadRan and the rest.
	Flags int
	// Level is a heading's level, and Title its text.
	Level int
	Title string
	// Line, From and To place the cell in the file, as Cell's do.
	Line, From, To int
	// CodeLines and OutputLines are a block's, as Cell's are.
	CodeLines, OutputLines int
	// SectionFrom and SectionTo are a heading's, as Cell's are.
	SectionFrom, SectionTo int
	// ID, Name and Depends are a block's, as Cell's are, but Name is ""
	// when it is the ID.
	ID, Name string
	Depends  []string
	// LastLine is a movable cell's, as Cell's is.
	LastLine int
	// Hash is Cell.Hash.
	Hash string
}

// The kinds of cell a Head is.
const (
	HeadProse = iota
	HeadBlock
	HeadTextbox
	HeadChoice
)

// The flags a Head has, the state of its cell.
const (
	HeadRan = 1 << iota
	HeadFailed
	HeadStale
	HeadRunning
	HeadHidden
	HeadCollapsed
	HeadCollapsedOutput
	HeadSecret
	HeadTerminal
	HeadNoSave
	HeadMovable
)

// headOf is c's head.
func headOf(c *Cell) Head {
	h := Head{Block: c.Block, Level: c.Heading, Title: c.Title, Line: c.Line, From: c.From, To: c.To, CodeLines: c.CodeLines, OutputLines: c.OutputLines, SectionFrom: c.SectionFrom, SectionTo: c.SectionTo, ID: c.ID, Depends: c.Depends, Hash: c.Hash}
	switch {
	case c.Textbox != nil:
		h.Kind, h.Block, h.ID = HeadTextbox, c.Textbox.Block, c.Textbox.ID
	case c.Choice != nil:
		h.Kind, h.Block, h.ID = HeadChoice, c.Choice.Block, c.Choice.ID
	case c.Block >= 0:
		h.Kind = HeadBlock
	}
	if c.Name != c.ID {
		h.Name = c.Name
	}
	if c.Movable {
		h.LastLine = c.LastLine
	}
	for _, f := range []struct {
		on   bool
		flag int
	}{{c.Ran, HeadRan}, {c.Failed, HeadFailed}, {c.Stale, HeadStale}, {c.Running, HeadRunning}, {c.Hidden != "", HeadHidden}, {c.Collapsed, HeadCollapsed}, {c.CollapsedOutput, HeadCollapsedOutput}, {c.Secret, HeadSecret}, {c.Terminal, HeadTerminal}, {c.NoSave, HeadNoSave}, {c.Movable, HeadMovable}} {
		if f.on {
			h.Flags |= f.flag
		}
	}
	return h
}

// HeadList is heads as they go to the page: a 1M-line document has a
// few hundred thousand, so each is a JSON array rather than an object,
//
//	[line, from, to, hash, block, kind, flags, level, codeLines,
//	 outputLines, sectionFrom, sectionTo, id, name, title, depends,
//	 lastLine]
//
// with what is left at its zero at the end left off, block's zero being
// -1. Line and From are counted on from the head before's, the first's
// from 0, To from From, and LastLine from Line, so most of the numbers
// are short.
type HeadList []Head

// MarshalJSON writes l as HeadList says.
func (l HeadList) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('[')
	line, from := 0, 0
	var vals []any
	for k, h := range l {
		if k > 0 {
			b.WriteByte(',')
		}
		var deps any
		if len(h.Depends) > 0 {
			deps = h.Depends
		}
		last := 0
		if h.LastLine > 0 {
			last = h.LastLine - h.Line
		}
		vals = append(vals[:0], h.Line-line, h.From-from, h.To-h.From, h.Hash, h.Block, h.Kind, h.Flags, h.Level, h.CodeLines, h.OutputLines, h.SectionFrom, h.SectionTo, h.ID, h.Name, h.Title, deps, last)
		line, from = h.Line, h.From
		n := len(vals)
		for n > 4 {
			v := vals[n-1]
			if v == nil || v == "" || v == 0 || (n == 5 && v == -1) {
				n--
				continue
			}
			break
		}
		row, err := json.Marshal(vals[:n])
		if err != nil {
			return nil, err
		}
		b.Write(row)
	}
	b.WriteByte(']')
	return b.Bytes(), nil
}

// Heads is a chunk of a layout's heads: Heads are cells [At,
// At+len(Heads)), and Fixes are the heads of cells before At that
// changed after Since, the step the page asked from, of the cells
// FixAt, in order. A fixed cell's body may have changed too. Step is
// where the layout is now, the Since to ask from next. Laid is how much
// of the file, in bytes, is laid out, of Size; Done is set when all of
// it is.
type Heads struct {
	Gen   int64    `json:"gen"`
	At    int      `json:"at"`
	Heads HeadList `json:"heads"`
	FixAt []int    `json:"fixAt,omitempty"`
	Fixes HeadList `json:"fixes,omitempty"`
	Step  int      `json:"step"`
	Laid  int      `json:"laid"`
	Size  int      `json:"size"`
	Done  bool     `json:"done"`
}

// ErrLayoutStale is returned for a layout that is no longer the document's:
// it changed, or another layout was started. The page starts again.
var ErrLayoutStale = errors.New("the document changed since its cells were laid out")

// layouts numbers the layouts, for Heads.Gen.
var layouts atomic.Int64

// cellLayout lays out a document's cells a chunk at a time.
type cellLayout struct {
	layout
	gen     int64
	src     []byte // d.Src when it started; a new slice is a new document
	pieces  [][]byte
	stale   []bool
	secrets []string
	// marks are the task marks found so far, as taskMarks finds them, up
	// to markPos.
	marks   []int
	markPos int
	// blk is the next block to lay out, and pos where the prose above it
	// starts.
	blk, pos int
	// bodies say how to draw each cell's body.
	bodies []bodySrc
	// lineAt and lineN count the cells' lines.
	lineAt, lineN int
	// x is the document's index, and xd the document it indexes, and
	// moved the cells whose move limits it has set. A long document's is
	// made from a parse of its own, away from n.mu, as it takes a parse
	// of the whole file: index brings it.
	x     *doc.Index
	xd    *doc.Document
	index chan indexed
	// indexed is closed once index has the index.
	indexed chan struct{}
	moved   int
	// folds is how many of d.Folds have folded their heading, and
	// headings are the heading cells, in order; open are those whose
	// sections are still open, from the outermost. folded are those an
	// mdsh:collapse line folds, and hidden those with a hidden block in
	// their sections.
	folds    int
	headings []int
	open     []int
	folded   map[int]bool
	hidden   map[int]bool
	// step counts the chunks; changes logs the cells changed after they
	// were laid out, by the step they changed at.
	step    int
	changes []headChange
	done    bool
	// lazyIndex has the layout's last chunk not wait for the index:
	// Relayout's cells keep the move limits they had until it comes, as
	// it takes a parse of the whole file. blocks is how many blocks the
	// document had when the layout started.
	lazyIndex bool
	blocks    int
	// first is where the cells start, after the shebang and front
	// matter, and states the blocks' states once the layout is done,
	// for reuse to tell what an edit changed.
	first  int
	states []blockState
	// reused says reuse laid it out.
	reused bool
}

type headChange struct{ step, cell int }

type indexed struct {
	d *doc.Document
	x *doc.Index
}

// bodySrc is what a cell's body is drawn from: md rendered as HTML, with
// the link reference definitions added if refs is set; or, for block
// block, its code and out, its output piece. A textbox or a choice has
// neither.
type bodySrc struct {
	md    []byte
	refs  bool
	block int
	out   []byte
}

// newCellLayout starts laying out n's cells. The caller holds n.mu.
func (n *Notebook) newCellLayout() *cellLayout {
	d := n.d
	n.newParses()
	l := &cellLayout{
		layout: layout{
			n:      n,
			lookup: run.NewLookup(d, n.results, environ()),
			refs:   linkRefs(d.Src),
		},
		gen:     layouts.Add(1),
		src:     d.Src,
		blocks:  len(d.Blocks),
		pieces:  render.Pieces(d, n.results, render.Options{Env: environ(), PlainIncludes: true}),
		stale:   run.Stale(d, n.results),
		secrets: n.sess.SecretVars(),
		pos:     max(d.FrontMatter.End, len(d.Shebang)),
		lineAt:  0,
		lineN:   1,
		folded:  map[int]bool{},
		hidden:  map[int]bool{},
	}
	l.markPos, l.first = l.pos, l.pos
	l.cuts = cuts(d, n.results, l.lookup)
	l.index = make(chan indexed, 1)
	l.indexed = make(chan struct{})
	go func(src []byte) {
		defer close(l.indexed)
		// The source is never changed in place, so it can be parsed
		// again here while the document goes on.
		d, err := doc.Parse(src)
		if err != nil {
			close(l.index)
			return
		}
		x := d.NewIndex()
		x.Boundaries()
		l.index <- indexed{d, x}
	}(d.Src)
	return l
}

// waitIndex has l use the index, waiting for it if wait is set.
func (l *cellLayout) waitIndex(wait bool) {
	if l.x != nil {
		return
	}
	var r indexed
	var ok bool
	if wait {
		r, ok = <-l.index
	} else {
		select {
		case r, ok = <-l.index:
		default:
			return
		}
	}
	if !ok {
		// It parsed under n.mu, so it can't fail here; if it did, the
		// document is indexed under the lock.
		r = indexed{l.n.d, l.n.d.NewIndex()}
	}
	l.x, l.xd = r.x, r.d
}

// current says l is still the layout of n's document.
func (l *cellLayout) current() bool {
	d := l.n.d
	return len(d.Src) == len(l.src) && (len(d.Src) == 0 || &d.Src[0] == &l.src[0])
}

// addMarks adds the task marks in the source up to pos, as taskMarks
// would, for the prose up to there and the output of the blocks before.
func (l *cellLayout) addMarks(from, to int) {
	if from < to {
		md := l.n.d.Src[from:to]
		var marks []int
		if l.parsed != nil && l.parsedFrom == from && l.parsedTo == to {
			marks = l.parsed.marks
		} else {
			marks = l.n.parseProse(md).marks
		}
		for _, p := range marks {
			l.marks = append(l.marks, from+p)
		}
	}
}

// chunk lays out at least want more cells, or the rest of the document.
// The caller holds n.mu.
func (l *cellLayout) chunk(want int) {
	if l.done {
		return
	}
	l.step++
	d := l.n.d
	first := len(l.cells)
	// Chunks go back without the move limits until the index is made;
	// they come as fixes then.
	l.waitIndex(false)
	for l.blk < len(d.Blocks) && len(l.cells)-first < want {
		i := l.blk
		b := d.Blocks[i]
		l.blk++
		l.proseCells(l.pos, b.Span.Start)
		l.pos = b.Output.End
		// The marks are found in the order taskMarks finds them, the
		// prose's from its parse for the layout where that is the same.
		l.addMarks(l.markPos, b.Span.Start)
		l.addMarks(b.Output.Start, b.Output.End)
		l.markPos = max(l.markPos, b.Output.End)
		l.tasks = l.marks
		l.fillTasks()
		l.blockCell(i, b)
	}
	if l.blk == len(d.Blocks) && len(l.cells)-first < want {
		l.proseCells(l.pos, len(d.Src))
		l.addMarks(l.markPos, len(d.Src))
		l.markPos = len(d.Src)
		l.tasks = l.marks
		l.fillTasks()
		l.pos = len(d.Src)
		l.done = true
		l.states = l.blockStates()
	}
	if l.done && !l.lazyIndex {
		l.waitIndex(true)
	}
	l.lines(first)
	l.foldHeadings(first)
	// The sections still open run to the end of the file, as far as
	// is known: a heading laid out later may close them.
	for _, h := range l.open {
		l.closeSection(h, len(d.Src), first)
	}
	l.moves(first)
}

// proseCells lays out the prose in src[from:to], keeping each cell's
// body to draw later. fillTasks numbers their tasks, once the marks up
// to to are found.
func (l *cellLayout) proseCells(from, to int) {
	at := len(l.cells)
	l.prose(from, to)
	for len(l.bodies) < len(l.cells) {
		l.bodies = append(l.bodies, bodySrc{md: l.pending[len(l.bodies)-at], refs: true, block: -1})
	}
	l.pending = l.pending[:0]
}

// fillTasks numbers the tasks of the prose cells proseCells last laid
// out.
func (l *cellLayout) fillTasks() {
	at := len(l.cells) - len(l.kept)
	for k, kept := range l.kept {
		c := &l.cells[at+k]
		for _, s := range kept {
			c.Tasks = append(c.Tasks, tasksIn(l.tasks, s)...)
		}
	}
	l.kept = l.kept[:0]
}

// blockCell lays out block i, b, as Cells draws it, if it is drawn.
func (l *cellLayout) blockCell(i int, b *doc.Block) {
	n, d := l.n, l.n.d
	code, out := l.pieces[3*i+1], l.pieces[3*i+2]
	span := doc.Span{Start: b.Span.Start, End: b.Output.End}
	add := func(c Cell, body bodySrc) {
		l.cells = append(l.cells, c)
		l.bodies = append(l.bodies, body)
	}
	// Printing leaves out a block whose show= is false or whose mdsh:if
	// section is off, but Run all still runs it, so a block that runs,
	// or a tape that types into one, is drawn anyway, marked hidden.
	// Anything else that printing leaves out does nothing and isn't
	// drawn.
	var hidden string
	if len(code) == 0 {
		switch {
		case !run.Runnable(b) && !b.IsTape():
			return
		case inCut(b.Span.Start, l.cuts):
			hidden = "Left out when printed: it is in an mdsh:if section that is off. Run all runs it unless its run= says otherwise."
		default:
			hidden = "Left out when printed: its show= condition is false. Run all runs it unless its run= says otherwise."
		}
		if b.IsTape() {
			hidden = strings.Replace(hidden, "Run all runs it unless its run= says otherwise.", "It still types into its block when that runs.", 1)
			// A copy, as every body's md is, so a layout reuse makes
			// doesn't hold the source it was made from.
			add(Cell{Block: -1, Hidden: hidden, From: span.Start, To: span.End}, bodySrc{md: bytes.Clone(d.Src[b.Span.Start:b.Span.End]), block: -1})
			return
		}
	}
	if b.IsTextbox() {
		t := b.Textbox()
		add(Cell{Block: -1, Textbox: &TextboxCell{Block: i, ID: b.ID, Text: t.Text, Lines: t.Lines, Fit: t.Fit, Placeholder: t.Placeholder, Problem: b.VarTaken, NoSave: b.NoSave()}, From: span.Start, To: span.End}, bodySrc{block: -1})
		return
	}
	if b.IsChoice() {
		c := b.Choice()
		add(Cell{Block: -1, Choice: &ChoiceCell{Block: i, ID: b.ID, Mode: c.Mode, Options: c.Options, Placeholder: c.Placeholder, Problem: b.VarTaken, NoSave: b.NoSave()}, From: span.Start, To: span.End}, bodySrc{block: -1})
		return
	}
	if !run.Runnable(b) {
		c := Cell{Block: -1, From: span.Start, To: span.End}
		if inc, err := b.Include(); err == nil && b.IsInclude() {
			if inc.From > 1 {
				c.FirstLine = inc.From
			}
			c.Include = &IncludeCell{Block: i, File: inc.File, From: inc.From, To: inc.To, Lang: b.Lang}
		}
		add(c, bodySrc{md: append(append([]byte(nil), code...), out...), block: -1})
		return
	}
	r := n.results[i]
	after := run.LookupAt(l.lookup, d, n.results, b.Output.End)
	outLine := 0
	if b.Output.End > b.Span.End {
		outLine = l.lineNo(b.Span.End + leadingBlank(d.Src[b.Span.End:b.Output.End]))
	}
	add(Cell{
		Collapsed:       hidden == "" && holds(b, "collapse", after),
		Hidden:          hidden,
		CollapsedOutput: foldsOutput(b, d.Options, after, out),
		longOutput:      d.Options.FoldsOutput(shownLines(out)),
		OutputLine:      outLine,
		Block:           i,
		ID:              b.ID,
		Name:            l.name(b),
		Lang:            b.Lang,
		Attrs:           attrsText(b.Info),
		Depends:         b.Depends(),
		At:              outputAt(b, r, out),
		OutputTasks:     tasksIn(l.tasks, b.CurrentOutput()),
		Links:           b.Attrs.Has(weblink.Attr),
		Secret:          b.Attrs.Has(secretAttr) || run.NamesAny(b.Body, l.secrets),
		SecretCode:      b.Attrs.Has(secretAttr),
		Ran:             r.Ran,
		Status:          r.Status,
		Failed:          r.Failed(),
		Stale:           l.stale[i],
		NoSave:          b.NoSave(),
		Running:         n.bg[b],
		Terminal:        n.bg[b] && n.bgTerms[i] != nil,
		Ms:              r.Duration.Milliseconds(),
		TimedOut:        r.TimedOut,
		Timeout:         timeoutMs(b),
		CodeLines:       textLines(b.Body),
		OutputLines:     textLines(string(out)),
		From:            span.Start,
		To:              span.End,
	}, bodySrc{block: i, out: out})
}

// foldsOutput is whether b's output, out, opens folded: as its
// collapse-output= says, else when it is longer than the front matter's
// fold_output (doc.DefaultFoldOutput lines unless it says), so a long
// output stays open only once someone unfolds it, which writes
// collapse-output=false (SetFolds).
func foldsOutput(b *doc.Block, opts doc.Options, lookup doc.Lookup, out []byte) bool {
	if c, ok := b.Cond("collapse-output"); ok {
		return c.Eval(lookup)
	}
	return opts.FoldsOutput(shownLines(out))
}

// shownLines is how many lines an output, as printed, shows: its
// fences' lines, and the lines of what isn't fenced, such as a table,
// that aren't blank, without the fences' own opening and closing lines.
func shownLines(out []byte) int {
	n := 0
	fence := "" // the open fence's marker
	for _, l := range strings.Split(string(out), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case fence == "" && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")):
			fence = t[:len(t)-len(strings.TrimLeft(t, t[:1]))]
		case fence != "" && strings.HasPrefix(t, fence) && strings.Trim(t, t[:1]) == "":
			fence = ""
		case fence != "" || t != "":
			n++
		}
	}
	return n
}

// textLines is how many lines s has, not counting the newline that ends
// it.
func textLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}

// lines numbers cells [first:] and sets their hashes.
func (l *cellLayout) lines(first int) {
	src := l.n.d.Src
	for i := first; i < len(l.cells); i++ {
		c := &l.cells[i]
		l.lineN += bytes.Count(src[l.lineAt:c.From], []byte("\n"))
		l.lineAt = c.From
		c.Line = l.lineN
		c.DropAfter, c.DropBefore = -1, -1
		c.Hash = l.hash(i)
		if c.Heading > 0 {
			// A section runs to the next heading of the same or a
			// higher level.
			for len(l.open) > 0 && l.cells[l.open[len(l.open)-1]].Heading >= c.Heading {
				l.closeSection(l.open[len(l.open)-1], c.From, first)
				l.open = l.open[:len(l.open)-1]
			}
			l.open = append(l.open, i)
			l.headings = append(l.headings, i)
		}
		if c.Hidden != "" {
			for _, h := range l.open {
				if !l.hidden[h] {
					l.hidden[h] = true
					l.fold(h, first)
				}
			}
		}
	}
}

// foldHeadings folds the headings above the mdsh:collapse lines laid out
// so far: a line folds the section of the heading above it.
func (l *cellLayout) foldHeadings(first int) {
	n, d := l.n, l.n.d
	for ; l.folds < len(d.Folds); l.folds++ {
		f := d.Folds[l.folds]
		if !l.done && f.Span.Start >= l.pos {
			break
		}
		if !f.Cond.Eval(run.LookupAt(l.lookup, d, n.results, f.Span.Start)) {
			continue
		}
		k := sort.Search(len(l.headings), func(k int) bool { return l.cells[l.headings[k]].From > f.Span.Start }) - 1
		if k < 0 {
			continue
		}
		i := l.headings[k]
		l.folded[i] = true
		l.fold(i, first)
	}
}

// closeSection sets heading cell h's section to run to end: the blocks
// in it, if one of them runs.
func (l *cellLayout) closeSection(h, end, first int) {
	d := l.n.d
	c := &l.cells[h]
	from, to := d.BlocksIn(doc.Span{Start: c.From, End: end})
	if !slices.ContainsFunc(d.Blocks[from:to], run.Runnable) {
		from, to = 0, 0
	}
	if c.SectionFrom != from || c.SectionTo != to {
		c.SectionFrom, c.SectionTo = from, to
		l.changed(h, first)
	}
}

// fold sets heading cell h's Collapsed: folded by an mdsh:collapse line
// in its section, unless a hidden block is in it too, as a hidden
// block's own collapse is ignored, so every block Run all runs is on
// screen when the file opens.
func (l *cellLayout) fold(h, first int) {
	c := &l.cells[h]
	if want := l.folded[h] && !l.hidden[h]; c.Collapsed != want {
		c.Collapsed = want
		l.changed(h, first)
	}
}

// changed notes cell i changed, if it was laid out before first.
func (l *cellLayout) changed(i, first int) {
	if i < first {
		l.changes = append(l.changes, headChange{l.step, i})
	}
}

// moves sets the move limits of the cells that don't have them yet, if
// the document's index is made.
func (l *cellLayout) moves(first int) {
	if l.x == nil {
		return
	}
	d, x := l.xd, l.x
	for ; l.moved < len(l.cells); l.moved++ {
		i := l.moved
		c := &l.cells[i]
		was := moveOf(c)
		c.Drop, c.Movable, c.LastLine, c.History, c.Undeletable = false, false, 0, false, ""
		c.DropAfter, c.DropBefore = -1, -1
		c.Drop = x.IsBoundary(c.From)
		if c.Heading > 0 && c.Drop {
			if span, _, err := x.Section(c.From); err == nil {
				c.dropLimits(x, span)
			}
		}
		bi := c.Block
		if c.Textbox != nil {
			bi = c.Textbox.Block
		} else if c.Choice != nil {
			bi = c.Choice.Block
		} else if bi < 0 && c.Heading == 0 {
			// A fence that doesn't run, drawn as it reads, moves too.
			// A tape moves with its block, not on its own.
			bi = blockAt(d, c.From)
			if bi > 0 {
				if end, ok := unitEnd(d, bi-1, x); ok && end > c.From {
					bi = -1
				}
			}
		}
		if bi >= 0 {
			if end, ok := unitEnd(d, bi, x); ok {
				last := max(end-1, c.From)
				c.Movable, c.LastLine = true, c.Line+bytes.Count(d.Src[c.From:last], []byte("\n"))
				c.History = doc.IsHistoryID(d.Blocks[bi].ID)
				c.dropLimits(x, doc.Span{Start: d.Blocks[bi].Span.Start, End: end})
				if users := x.DeleteBlockers(d.Blocks[bi]); len(users) > 0 {
					c.Undeletable = "Can't delete: " + strings.Join(users, ", ") + " still use it. Take those out first."
				}
			}
		}
		if moveOf(c) != was {
			l.changed(i, first)
		}
	}
}

// moveLimits are a cell's move limits, which moves sets.
type moveLimits struct {
	drop, movable, history  bool
	lastLine, after, before int
	undeletable             string
}

func moveOf(c *Cell) moveLimits {
	return moveLimits{c.Drop, c.Movable, c.History, c.LastLine, c.DropAfter, c.DropBefore, c.Undeletable}
}

// carryMoves gives c, a cell of a layout whose index isn't made yet,
// the move limits of was, the cell of the layout before that it is, as
// far as can be told without the index: the edit, which took the text
// from at on line on, moved what was after it by lines and bytes. The
// index sets them as they are once it comes.
func carryMoves(c, was *Cell, at, line, lines, bytes int) {
	c.Drop, c.Movable, c.History, c.Undeletable = was.Drop, was.Movable, was.History, was.Undeletable
	c.LastLine, c.DropAfter, c.DropBefore = was.LastLine, was.DropAfter, was.DropBefore
	if c.LastLine > 0 && c.LastLine >= line {
		c.LastLine += lines
	}
	if c.DropAfter > at {
		c.DropAfter += bytes
	}
	if c.DropBefore > at {
		c.DropBefore += bytes
	}
}

// hash is a hash of what cell i's body is drawn from: cells whose
// hashes are alike have bodies alike.
func (l *cellLayout) hash(i int) string {
	h := fnv.New64a()
	s := l.bodies[i]
	c := &l.cells[i]
	// What else a card draws that its head doesn't say.
	fmt.Fprintf(h, "%v %v %q\x00", c.Tasks, c.OutputTasks, c.Hidden)
	switch {
	case c.Textbox != nil || c.Choice != nil:
		// Not its block's index, which a block added above changes
		// without changing the card.
		var in []byte
		if t := c.Textbox; t != nil {
			k := *t
			k.Block = 0
			in, _ = json.Marshal(k)
		} else {
			k := *c.Choice
			k.Block = 0
			in, _ = json.Marshal(k)
		}
		h.Write(in)
	case s.block >= 0:
		b := l.n.d.Blocks[s.block]
		r := l.n.results[s.block]
		fmt.Fprintf(h, "block %q %d %d\x00", b.Info, len(b.Body), len(s.out))
		h.Write([]byte(b.Body))
		h.Write(s.out)
		for _, e := range b.History {
			fmt.Fprintf(h, "\x00%d %s %d %t %t\x00", e.K, e.At, e.Exit, e.Exited, e.Region)
			h.Write(l.n.d.Src[e.Span.Start:e.Span.End])
		}
		if b.IsHTTP() && r != nil && r.Ran {
			fmt.Fprintf(h, "\x00http %t\x00", r.HTTP != nil)
			h.Write(r.Stdout)
			if r.HTTP != nil {
				for _, c := range r.HTTP.Checks {
					fmt.Fprintf(h, "\x00%t %s %s", c.OK, c.Text, c.Got)
				}
			}
		}
	default:
		fmt.Fprintf(h, "md %t\x00", s.refs)
		h.Write(s.md)
		if s.refs {
			h.Write([]byte{0})
			h.Write(l.refs)
		}
	}
	return strconv.FormatUint(h.Sum64(), 36)
}

// body is cell i, with its body drawn.
func (l *cellLayout) body(i int) Cell {
	n, s := l.n, l.bodies[i]
	c := l.cells[i]
	switch {
	case s.block >= 0:
		b := n.d.Blocks[s.block]
		c.Body, c.Output, c.Earlier, c.HTTP = b.Body, n.html(s.out), n.earlier(b), httpCell(b, n.results[s.block], s.out)
	case s.md == nil:
	case !s.refs:
		c.HTML = n.html(s.md)
	default:
		c.HTML = n.proseHTML(c.Hash, s.md, l.refs)
	}
	return c
}

// proseHTML is md, with refs after it, rendered, from the cache of
// prose rendered by hash where it is there. Prose with an image isn't
// kept, as whether the image shows depends on the files beside the
// document, not only on its text.
func (n *Notebook) proseHTML(hash string, md, refs []byte) string {
	if n.proseFile != n.file {
		n.prose, n.proseFile = nil, n.file
	}
	if html, ok := n.prose[hash]; ok {
		return html
	}
	src := md
	if len(refs) > 0 {
		src = make([]byte, 0, len(md)+2+len(refs))
		src = append(append(append(src, md...), "\n\n"...), refs...)
	}
	html := n.html(src)
	if bytes.Contains(md, []byte("![")) || bytes.Contains(bytes.ToLower(md), []byte("<img")) {
		return html
	}
	if n.prose == nil || len(n.prose) >= proseCache {
		n.prose = map[string]string{}
	}
	n.prose[hash] = html
	return html
}

// proseCache is the most rendered prose cells the notebook keeps.
const proseCache = 20_000

// all lays out the rest of the document, and draws every body, for
// Cells.
func (l *cellLayout) all() []Cell {
	if l.x == nil {
		l.x, l.xd = l.n.d.NewIndex(), l.n.d
	}
	for !l.done {
		l.chunk(1 << 30)
	}
	for i := range l.cells {
		l.cells[i] = l.body(i)
	}
	return l.cells
}

// Attach starts laying the document's cells out afresh, as a page
// opens it or catches up with a change, and returns the first want of
// their heads.
func (n *Notebook) Attach(want int) Heads {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.lay = n.newCellLayout()
	return n.lay.next(0, 0, want)
}

// MoreHeads returns the heads of layout gen from cell at on, at least
// want of them if there are that many, laying more out as it needs to,
// and the fixes to the heads before at since step since. It returns
// ErrLayoutStale when gen is no longer the document's layout.
func (n *Notebook) MoreHeads(gen int64, at, since, want int) (Heads, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	l := n.lay
	if l == nil || l.gen != gen || !l.current() {
		return Heads{}, ErrLayoutStale
	}
	if l.done && l.x == nil {
		// The index Relayout didn't wait for is waited for without
		// n.mu, so typing goes on while the file is parsed.
		n.mu.Unlock()
		<-l.indexed
		n.mu.Lock()
		if n.lay != l || !l.current() {
			return Heads{}, ErrLayoutStale
		}
	}
	if at > len(l.cells) {
		return Heads{}, fmt.Errorf("cell %d isn't laid out", at)
	}
	return l.next(at, since, want), nil
}

// next lays out cells up to at+want and returns them from at.
func (l *cellLayout) next(at, since, want int) Heads {
	if len(l.cells) < at+want {
		l.chunk(at + want - len(l.cells))
	}
	if l.done && l.x == nil {
		// Relayout left the move limits as they were; they come now,
		// as fixes where they changed.
		l.waitIndex(true)
		l.step++
		l.moved = 0
		l.moves(len(l.cells))
	}
	end := min(len(l.cells), at+want)
	h := Heads{Gen: l.gen, At: at, Heads: make(HeadList, 0, end-at), Step: l.step, Laid: l.pos, Size: len(l.n.d.Src), Done: l.done && end == len(l.cells)}
	for i := at; i < end; i++ {
		h.Heads = append(h.Heads, headOf(&l.cells[i]))
	}
	k := sort.Search(len(l.changes), func(k int) bool { return l.changes[k].step > since })
	var fixed []int
	for _, c := range l.changes[k:] {
		if c.cell < at {
			fixed = append(fixed, c.cell)
		}
	}
	slices.Sort(fixed)
	for _, i := range slices.Compact(fixed) {
		h.FixAt = append(h.FixAt, i)
		h.Fixes = append(h.Fixes, headOf(&l.cells[i]))
	}
	return h
}

// Bodies is cells, of layout gen, whole, with their bodies drawn. It
// returns ErrLayoutStale when gen is no longer the document's layout.
func (n *Notebook) Bodies(gen int64, cells []int) ([]Cell, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	l := n.lay
	if l == nil || l.gen != gen || !l.current() {
		return nil, ErrLayoutStale
	}
	out := make([]Cell, len(cells))
	for k, i := range cells {
		if i < 0 || i >= len(l.cells) {
			return nil, fmt.Errorf("cell %d isn't laid out", i)
		}
		out[k] = l.body(i)
	}
	return out, nil
}

// Splice is how the cells of layout Gen differ from those of layout
// From, the page's, after an edit: cells [At, At+Remove) of From's are
// replaced by Heads, and the cells after them are From's moved, in the
// file, by Line lines and Pos bytes, and in the document's blocks by
// Block, as the edit added or took them away. Fixes are the heads of the
// cells outside Heads, by their index in Gen, FixAt, that changed
// otherwise, as a heading's section does when a block goes into it.
// Reset is set when From isn't the layout Go had, or wasn't laid out
// to its end: Heads are then every cell's, for the page to start again
// from.
type Splice struct {
	Gen    int64    `json:"gen"`
	Reset  bool     `json:"reset,omitempty"`
	At     int      `json:"at"`
	Remove int      `json:"remove"`
	Heads  HeadList `json:"heads"`
	Line   int      `json:"line"`
	Pos    int      `json:"pos"`
	Block  int      `json:"block"`
	FixAt  []int    `json:"fixAt,omitempty"`
	Fixes  HeadList `json:"fixes,omitempty"`
	Step   int      `json:"step"`
	Size   int      `json:"size"`
	// Moving is set while the move limits are still those of From's
	// cells: MoreHeads from Step brings the fixes once they are known.
	Moving bool `json:"moving,omitempty"`
}

// Relayout lays the document out afresh, as Attach does, after an edit
// to the document of layout from, the one whose heads the page has, and
// returns how the cells changed, so a keystroke in a long file sends
// the page the few cells it changed, not every head.
func (n *Notebook) Relayout(from int64) Splice {
	n.mu.Lock()
	defer n.mu.Unlock()
	old := n.lay
	n.lay = n.newCellLayout()
	l := n.lay
	reuse := old != nil && old.gen == from && old.done
	// The index's parse of the whole file, for the move limits, is most
	// of a long file's layout: the cells keep their old limits until it
	// comes, when the page asks for them with MoreHeads.
	l.lazyIndex = reuse
	s := Splice{Gen: l.gen, Size: len(n.d.Src)}
	if reuse && l.reuse(old, &s) {
		l.reused, s.Moving = true, l.x == nil
		return s
	}
	for !l.done {
		l.chunk(1 << 30)
	}
	s.Moving = l.x == nil
	if !reuse {
		s.Reset = true
		s.Heads = make(HeadList, 0, len(l.cells))
		for i := range l.cells {
			s.Heads = append(s.Heads, headOf(&l.cells[i]))
		}
		return s
	}
	was, now := old.cells, l.cells
	head := 0
	for head < len(was) && head < len(now) && sameCell(&was[head], &now[head]) {
		head++
	}
	tail := 0
	for tail < len(was)-head && tail < len(now)-head && sameCell(&was[len(was)-1-tail], &now[len(now)-1-tail]) {
		tail++
	}
	s.At, s.Remove = head, len(was)-head-tail
	s.Line = bytes.Count(n.d.Src, []byte{'\n'}) - bytes.Count(old.src, []byte{'\n'})
	s.Pos = len(n.d.Src) - len(old.src)
	s.Block = l.blocks - old.blocks
	if l.x == nil {
		at, line := len(old.src), 1<<62
		if head < len(was) {
			at, line = was[head].From, was[head].Line
		}
		for i := range head {
			carryMoves(&now[i], &was[i], at, line, s.Line, s.Pos)
		}
		// A cell typed in takes the limits of the one it replaces.
		for k := 0; k < len(now)-head-tail && k < s.Remove; k++ {
			carryMoves(&now[head+k], &was[head+k], at, line, s.Line, s.Pos)
		}
		for k := tail; k > 0; k-- {
			c, w := &now[len(now)-k], &was[len(was)-k]
			carryMoves(c, w, at, line, s.Line, s.Pos)
			if w.LastLine > 0 {
				// It is after the edit, all of it.
				c.LastLine = w.LastLine + s.Line
			}
		}
	}
	s.Step = l.step
	s.Heads = make(HeadList, 0, len(now)-head-tail)
	for i := head; i < len(now)-tail; i++ {
		s.Heads = append(s.Heads, headOf(&now[i]))
	}
	fix := func(i int, want Head) {
		if h := headOf(&now[i]); !sameHeadAs(h, want) {
			s.FixAt = append(s.FixAt, i)
			s.Fixes = append(s.Fixes, h)
		}
	}
	for i := range head {
		fix(i, headOf(&was[i]))
	}
	for k := tail; k > 0; k-- {
		fix(len(now)-k, s.moved(headOf(&was[len(was)-k])))
	}
	return s
}

// sameCell says a and b are the same cell, as far as lining two layouts
// up goes: alike in what they draw and their state, wherever they are.
func sameCell(a, b *Cell) bool {
	x, y := headOf(a), headOf(b)
	// Whether a cell moves is left out, as it can be unknown yet.
	return x.Hash != "" && x.Hash == y.Hash && x.Kind == y.Kind && x.Flags&^HeadMovable == y.Flags&^HeadMovable && x.Level == y.Level && x.Title == y.Title
}

// moved is h, a head of a cell after the edit, moved as s says.
func (s Splice) moved(h Head) Head {
	h.Line += s.Line
	h.From += s.Pos
	h.To += s.Pos
	if h.LastLine > 0 {
		h.LastLine += s.Line
	}
	if h.Block >= 0 {
		h.Block += s.Block
	}
	if h.SectionTo > 0 {
		h.SectionFrom += s.Block
		h.SectionTo += s.Block
	}
	return h
}

// sameHeadAs says heads a and b are alike in every field.
func sameHeadAs(a, b Head) bool {
	return a.Block == b.Block && a.Kind == b.Kind && a.Flags == b.Flags && a.Level == b.Level && a.Title == b.Title &&
		a.Line == b.Line && a.From == b.From && a.To == b.To && a.CodeLines == b.CodeLines && a.OutputLines == b.OutputLines &&
		a.SectionFrom == b.SectionFrom && a.SectionTo == b.SectionTo && a.ID == b.ID && a.Name == b.Name &&
		slices.Equal(a.Depends, b.Depends) && a.LastLine == b.LastLine && a.Hash == b.Hash
}
