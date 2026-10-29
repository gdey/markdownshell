// Package doc parses a markdown file into prose and fenced code blocks
// without losing a byte of the original, so output can be spliced back in.
package doc

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Span is a half-open byte range [Start, End) in Document.Src.
type Span struct {
	Start, End int
}

// Block is a fenced code block.
type Block struct {
	ID    string
	Lang  string
	Attrs Attrs
	Body  string
	// Span covers the fence lines, including the closing fence's newline.
	Span Span
	// BodySpan covers the lines between the fences, as they are in Src.
	// It ends at Span.End when the fence is never closed.
	BodySpan Span
	// Fence is the opening fence's run of backticks or tildes, and Info
	// its info string.
	Fence, Info string
	// Output covers any output already attached to this block, starting
	// right after Span. It is empty (Start == End == Span.End) when there
	// is none.
	Output Span
	// Prefix is what starts each line of a block nested in block quotes or
	// list items, such as "> " or "   ". Output written after the block
	// starts each line with it. Empty at the top level.
	Prefix string
	// Lead is the part of the opening fence line that holds list markers
	// when the fence opens on the same line as its list item, such as
	// "- " or "> 1. ". Empty otherwise.
	Lead string

	quote, col int // the container: block quote depth and list content column
	// outMain and outErr are set once Output holds a stdout part (a
	// fence or region) and a stderr fence. Another one is a copy.
	outMain, outErr bool
}

// Document is a parsed markdown file.
type Document struct {
	Src []byte
	// Shebang is the #! line, without its newline. Empty if there is none.
	Shebang string
	// FrontMatter covers the front matter, including both --- lines.
	FrontMatter Span
	Options     Options
	Blocks      []*Block
	// Placeholders are the env placeholders in the prose, in order.
	// Empty when the front matter turns them off.
	Placeholders []*Placeholder
	// Sections are the mdsh:if ... mdsh:end sections, in order of their
	// if lines. They may nest.
	Sections []*Section
	// Copies are the output fences and regions in the prose that aren't
	// right after the block they are for, in order. A run refills them.
	Copies []*Copy
	// Warnings are problems that don't stop the document running, such
	// as a copy for a block that doesn't exist.
	Warnings []string
}

// Copy is a copy of a block's output somewhere else in the document: an
// output fence, or an output region, whose for= block isn't the one
// right above it.
type Copy struct {
	// For is the id of the block whose output it copies.
	For string
	// Stream is "stderr" for a copy of the stderr fence, else "".
	Stream string
	// Region is set for an output region, a copy of out=markdown or
	// image output.
	Region bool
	// Span covers the copy's lines, prefixes included.
	Span Span
	// Prefix and Lead are as for Block.
	Prefix, Lead string
}

// HasOutput reports whether output is already saved after b in the file.
func (b *Block) HasOutput() bool { return b.Output.End > b.Output.Start }

// Block returns the block with the given id, or nil.
func (d *Document) Block(id string) *Block {
	for _, b := range d.Blocks {
		if b.ID == id {
			return b
		}
	}
	return nil
}

// Parse splits src into front matter and fenced blocks. Output fences and
// output regions written by mdsh are attached to the block they follow
// instead of being returned as blocks.
func Parse(src []byte) (*Document, error) {
	d := &Document{Src: src}
	pos := 0
	if bytes.HasPrefix(src, []byte("#!")) {
		end := lineEnd(src, 0)
		d.Shebang = string(bytes.TrimRight(src[:end], "\r\n"))
		pos = end
	}
	fm, opts, err := parseFrontMatter(src, pos)
	if err != nil {
		return nil, err
	}
	d.FrontMatter, d.Options = fm, opts
	if fm.End > pos {
		pos = fm.End
	}

	ids := map[string]bool{}
	includes, tapes := 0, 0
	var last *Block
	var open []*Section
	var lists []int // content columns of the open list items
	quote := 0      // block quote depth of the current line
	for pos < len(src) {
		end := lineEnd(src, pos)
		line := src[pos:end]
		rest, q := stripQuotes(line, -1)
		if q != quote {
			lists, quote = nil, q
		}

		if f, col, lead, ok := scanFence(rest, &lists); ok {
			b, next := readFence(src, pos, end, f, q, col)
			head := line[:len(line)-len(rest)+col]
			b.Prefix = linePrefix(head)
			if lead {
				b.Lead = string(head)
			}
			if last != nil && isOutputFor(b, last.ID) && b.quote == last.quote && b.col == last.col && gap(src, last.Output.End, pos, q) && !last.hasPart(b.Attrs["stream"] == "stderr") {
				last.Output.End = next
				last.addPart(b.Attrs["stream"] == "stderr")
				pos = next
				continue
			}
			if b.Lang == "output" {
				// Output away from its block is a copy, part of the
				// prose. An unclosed one is just prose.
				if b.BodySpan.End < b.Span.End && b.Attrs["for"] != "" {
					d.Copies = append(d.Copies, &Copy{For: b.Attrs["for"], Stream: b.Attrs["stream"], Span: b.Span, Prefix: b.Prefix, Lead: b.Lead})
				}
				last = nil
				pos = next
				continue
			}
			if b.IsInclude() {
				// Includes have no name: give them an id nothing can
				// refer to, and leave block-N numbering alone.
				if b.ID != "" {
					return nil, fmt.Errorf("line %d: an include block has no name; drop #%s", lineNo(src, pos), b.ID)
				}
				includes++
				b.ID = fmt.Sprintf("<include-%d", includes)
				b.Output = Span{next, next}
				d.Blocks = append(d.Blocks, b)
				last = b
				pos = next
				continue
			}
			if b.IsTape() {
				// Tapes are input for a block, not blocks: like
				// includes, they have no name.
				if b.ID != "" {
					return nil, fmt.Errorf("line %d: a tape has no name; drop #%s", lineNo(src, pos), b.ID)
				}
				tapes++
				b.ID = fmt.Sprintf("<tape-%d", tapes)
				b.Output = Span{next, next}
				d.Blocks = append(d.Blocks, b)
				last = b
				pos = next
				continue
			}
			if b.ID == "" {
				b.ID = fmt.Sprintf("block-%d", len(d.Blocks)-includes-tapes+1)
			}
			if ids[b.ID] {
				return nil, fmt.Errorf("line %d: duplicate block id %q", lineNo(src, pos), b.ID)
			}
			ids[b.ID] = true
			b.Output = Span{next, next}
			d.Blocks = append(d.Blocks, b)
			last = b
			pos = next
			continue
		}

		if id, ok := regionStart(rest); ok {
			next := regionEnd(src, end, q)
			if last != nil && id == last.ID && last.quote == q && gap(src, last.Output.End, pos, q) && !last.hasPart(false) {
				last.Output.End = next
				last.addPart(false)
			} else {
				head := line[:len(line)-len(rest)+spaces(rest)]
				c := &Copy{For: id, Region: true, Span: Span{pos, next}, Prefix: linePrefix(head)}
				if c.Prefix != string(head) {
					c.Lead = string(head)
				}
				d.Copies = append(d.Copies, c)
				last = nil
			}
			pos = next
			continue
		}

		if !blank(rest) {
			last = nil
		}
		if text, isIf, isEnd := sectionLine(line); isIf || isEnd {
			if isIf {
				c, err := ParseCond(text)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", lineNo(src, pos), err)
				}
				sec := &Section{If: Span{pos, end}, Cond: c}
				d.Sections = append(d.Sections, sec)
				open = append(open, sec)
			} else {
				if len(open) == 0 {
					return nil, fmt.Errorf("line %d: mdsh:end without mdsh:if", lineNo(src, pos))
				}
				open[len(open)-1].End = Span{pos, end}
				open = open[:len(open)-1]
			}
			pos = end
			continue
		}
		if d.Options.Interpolating() {
			d.Placeholders = scanPlaceholders(src, pos, end, d.Placeholders)
		}
		pos = end
	}
	if len(open) > 0 {
		return nil, fmt.Errorf("line %d: mdsh:if without mdsh:end", lineNo(src, open[len(open)-1].If.Start))
	}
	for _, b := range d.Blocks {
		if b.IsInclude() {
			if _, err := b.Include(); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNo(src, b.Span.Start), err)
			}
		}
		for _, a := range condAttrs {
			if v, ok := b.Attrs[a]; ok {
				if _, err := ParseCond(v); err != nil {
					return nil, fmt.Errorf("line %d: block %s: %s=: %w", lineNo(src, b.Span.Start), b.ID, a, err)
				}
			}
		}
	}
	if err := checkTapes(d); err != nil {
		return nil, err
	}
	for _, p := range d.Placeholders {
		if p.BlockID != "" && !ids[p.BlockID] {
			return nil, fmt.Errorf("line %d: placeholder refers to unknown block %q", lineNo(src, p.Span.Start), p.BlockID)
		}
	}
	for _, c := range d.Copies {
		if b := d.Block(c.For); b == nil || b.IsInclude() || b.IsTape() {
			d.Warnings = append(d.Warnings, fmt.Sprintf("line %d: output copy for unknown block %q", lineNo(src, c.Span.Start), c.For))
		}
	}
	for _, r := range d.Refs() {
		if r.BlockID != "" && !ids[r.BlockID] {
			return nil, fmt.Errorf("%s refers to unknown block %q", r, r.BlockID)
		}
	}
	return d, nil
}

// TapeLang is the language of a tape fence, which types the input for
// the tty block its for= names:
//
//	```tape {for=shell}
//	Type "exit"
//	Enter
//	```
//
// It can be an include block too, to read the tape from a file.
const TapeLang = "tape"

// IsTape reports whether b is a tape for another block.
func (b *Block) IsTape() bool { return b.Lang == TapeLang && b.Attrs.Has("for") }

// Tape returns the tape for block b, or nil.
func (d *Document) Tape(b *Block) *Block {
	for _, t := range d.Blocks {
		if t.IsTape() && t.Attrs["for"] == b.ID {
			return t
		}
	}
	return nil
}

// checkTapes checks each tape is for a tty block, and no block has two.
func checkTapes(d *Document) error {
	seen := map[string]bool{}
	for _, t := range d.Blocks {
		if !t.IsTape() {
			continue
		}
		id := t.Attrs["for"]
		b := d.Block(id)
		switch {
		case b == nil || b.IsTape() || b.IsInclude():
			return fmt.Errorf("line %d: tape for unknown block %q", lineNo(d.Src, t.Span.Start), id)
		case !b.Attrs.Has("tty"):
			return fmt.Errorf("line %d: tape for block %s, which isn't marked tty", lineNo(d.Src, t.Span.Start), id)
		case seen[id]:
			return fmt.Errorf("line %d: block %s has two tapes", lineNo(d.Src, t.Span.Start), id)
		}
		seen[id] = true
	}
	return nil
}

func (b *Block) hasPart(stderr bool) bool {
	if stderr {
		return b.outErr
	}
	return b.outMain
}

func (b *Block) addPart(stderr bool) {
	if stderr {
		b.outErr = true
	} else {
		b.outMain = true
	}
}

func isOutputFor(b *Block, id string) bool {
	return b.Lang == "output" && b.Attrs["for"] == id
}

type fence struct {
	char   byte
	n      int // fence length
	indent int
	info   string
}

// openFence reports whether line opens a fenced code block (CommonMark rules).
func openFence(line []byte) (fence, bool) {
	line = bytes.TrimRight(line, "\r\n")
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 || indent >= len(line) {
		return fence{}, false
	}
	c := line[indent]
	if c != '`' && c != '~' {
		return fence{}, false
	}
	n := 0
	for indent+n < len(line) && line[indent+n] == c {
		n++
	}
	if n < 3 {
		return fence{}, false
	}
	info := bytes.TrimSpace(line[indent+n:])
	if c == '`' && bytes.IndexByte(info, '`') >= 0 {
		return fence{}, false
	}
	return fence{char: c, n: n, indent: indent, info: string(info)}, true
}

func closesFence(line []byte, f fence) bool {
	line = bytes.TrimRight(line, "\r\n")
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 {
		return false
	}
	n := 0
	for indent+n < len(line) && line[indent+n] == f.char {
		n++
	}
	return n >= f.n && len(bytes.TrimSpace(line[indent+n:])) == 0
}

// readFence reads the block whose opening fence spans [start, bodyStart),
// nested in quote block quotes and a list item whose content starts at
// column col. It returns the block and the offset just past its closing
// fence. An unclosed fence runs to the end of its container.
func readFence(src []byte, start, bodyStart int, f fence, quote, col int) (*Block, int) {
	var body bytes.Buffer
	pos := bodyStart
	bodyEnd := -1
	for pos < len(src) {
		end := lineEnd(src, pos)
		line, q := stripQuotes(src[pos:end], quote)
		if q < quote {
			break
		}
		sp := spaces(line)
		if sp < col && !blank(line) {
			break
		}
		line = line[min(sp, col):]
		if closesFence(line, f) {
			bodyEnd = pos
			pos = end
			break
		}
		body.Write(trimIndent(line, f.indent))
		pos = end
	}
	if bodyEnd < 0 {
		bodyEnd = pos
	}
	lang, attrs := ParseInfo(f.info)
	return &Block{
		ID:       attrs["#"],
		Lang:     lang,
		Attrs:    attrs,
		Body:     body.String(),
		Span:     Span{start, pos},
		BodySpan: Span{bodyStart, bodyEnd},
		Fence:    strings.Repeat(string(f.char), f.n),
		Info:     f.info,
		quote:    quote,
		col:      col,
	}, pos
}

// linePrefix turns the container markers that start an opening fence line
// into the prefix of the lines that follow: block quote markers stay and
// list markers become spaces.
func linePrefix(head []byte) string {
	p := []byte(string(head))
	for i, c := range p {
		if c != '>' {
			p[i] = ' '
		}
	}
	return string(p)
}

// stripQuotes strips up to max block quote markers (all of them if max is
// negative) from the start of line. It returns the rest of the line and
// how many markers it stripped.
func stripQuotes(line []byte, max int) ([]byte, int) {
	n := 0
	for max < 0 || n < max {
		i := spaces(line)
		if i > 3 || i >= len(line) || line[i] != '>' {
			break
		}
		i++
		if i < len(line) && line[i] == ' ' {
			i++
		}
		line = line[i:]
		n++
	}
	return line, n
}

// scanFence reports whether line, with its block quote markers stripped,
// opens a fence. lists holds the content columns of the open list items;
// scanFence closes the ones line isn't indented into and opens the ones
// line starts. col is the content column of the list item the fence is in
// (0 outside lists), and lead is set when that item starts on this line.
func scanFence(line []byte, lists *[]int) (f fence, col int, lead bool, ok bool) {
	if blank(line) {
		return fence{}, 0, false, false
	}
	pos := spaces(line)
	for len(*lists) > 0 && (*lists)[len(*lists)-1] > pos {
		*lists = (*lists)[:len(*lists)-1]
	}
	if len(*lists) > 0 {
		col = (*lists)[len(*lists)-1]
	}
	for pos-col <= 3 {
		if f, ok := openFence(line[col:]); ok {
			return f, col, lead, true
		}
		w := listMarker(line[pos:])
		if w == 0 {
			break
		}
		end := pos + w
		sp := spaces(line[end:])
		if sp == 0 || sp > 4 || blank(line[end:]) {
			sp = 1 // empty item, or content that is indented code
		}
		col, lead = end+sp, true
		*lists = append(*lists, col)
		if col >= len(line) {
			break
		}
		pos = col + spaces(line[col:])
	}
	return fence{}, 0, false, false
}

// listMarker returns the width of the list item marker at the start of
// line ("-", "*", "+", "1." or "1)"), or 0 if there is none. A marker must
// be followed by a space or the end of the line.
func listMarker(line []byte) int {
	w := 0
	switch {
	case len(line) > 0 && (line[0] == '-' || line[0] == '*' || line[0] == '+'):
		w = 1
	default:
		for w < len(line) && w < 9 && line[w] >= '0' && line[w] <= '9' {
			w++
		}
		if w == 0 || w >= len(line) || (line[w] != '.' && line[w] != ')') {
			return 0
		}
		w++
	}
	if rest := bytes.TrimRight(line[w:], "\r\n"); len(rest) > 0 && rest[0] != ' ' {
		return 0
	}
	return w
}

// spaces counts the spaces at the start of line.
func spaces(line []byte) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	return n
}

// gap reports whether src[from:to] holds only blank lines inside quote
// block quotes, so output there still belongs to the block above it.
func gap(src []byte, from, to, quote int) bool {
	for from < to {
		end := lineEnd(src, from)
		line, q := stripQuotes(src[from:end], quote)
		if q < quote || !blank(line) {
			return false
		}
		from = end
	}
	return true
}

func trimIndent(line []byte, n int) []byte {
	for i := 0; i < n && len(line) > 0 && line[0] == ' '; i++ {
		line = line[1:]
	}
	return line
}

const (
	regionOpen  = "<!-- mdsh:output for="
	regionClose = "<!-- /mdsh:output -->"
)

// RegionStart returns the line that opens an output region for id.
func RegionStart(id string) string { return regionOpen + strconv.Quote(id) + " -->" }

// RegionEnd is the line that closes an output region.
const RegionEnd = regionClose

func regionStart(line []byte) (string, bool) {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, []byte(regionOpen)) || !bytes.HasSuffix(line, []byte("-->")) {
		return "", false
	}
	q := bytes.TrimSpace(line[len(regionOpen) : len(line)-3])
	id, err := strconv.Unquote(string(q))
	if err != nil {
		return "", false
	}
	return id, true
}

// regionEnd returns the offset just past the closing line of a region whose
// body starts at pos, inside quote block quotes. An unclosed region ends at
// the end of the document.
func regionEnd(src []byte, pos, quote int) int {
	for pos < len(src) {
		end := lineEnd(src, pos)
		line, _ := stripQuotes(src[pos:end], quote)
		if string(bytes.TrimSpace(line)) == regionClose {
			return end
		}
		pos = end
	}
	return pos
}

// lineEnd returns the offset just past the newline of the line at pos.
func lineEnd(src []byte, pos int) int {
	if i := bytes.IndexByte(src[pos:], '\n'); i >= 0 {
		return pos + i + 1
	}
	return len(src)
}

func lineNo(src []byte, pos int) int { return bytes.Count(src[:pos], []byte("\n")) + 1 }

func blank(b []byte) bool { return len(bytes.TrimSpace(b)) == 0 }
