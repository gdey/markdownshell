// Package doc parses a markdown file into prose and fenced code blocks
// without losing a byte of the original, so output can be spliced back in.
package doc

import (
	"bytes"
	"fmt"
	"strconv"
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
	// Output covers any output already attached to this block, starting
	// right after Span. It is empty (Start == End == Span.End) when there
	// is none.
	Output Span
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
}

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
	var last *Block
	var open []*Section
	for pos < len(src) {
		end := lineEnd(src, pos)
		line := src[pos:end]

		if f, ok := openFence(line); ok {
			b, next := readFence(src, pos, end, f)
			if last != nil && isOutputFor(b, last.ID) && blank(src[last.Output.End:pos]) {
				last.Output.End = next
				pos = next
				continue
			}
			if b.Lang == "output" {
				// An orphaned output fence: leave it as prose.
				pos = next
				continue
			}
			if b.ID == "" {
				b.ID = fmt.Sprintf("block-%d", len(d.Blocks)+1)
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

		if id, ok := regionStart(line); ok {
			next := regionEnd(src, end)
			if last != nil && id == last.ID && blank(src[last.Output.End:pos]) {
				last.Output.End = next
			}
			pos = next
			continue
		}

		if !blank(line) {
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
		for _, a := range condAttrs {
			if v, ok := b.Attrs[a]; ok {
				if _, err := ParseCond(v); err != nil {
					return nil, fmt.Errorf("line %d: block %s: %s=: %w", lineNo(src, b.Span.Start), b.ID, a, err)
				}
			}
		}
	}
	for _, p := range d.Placeholders {
		if p.BlockID != "" && !ids[p.BlockID] {
			return nil, fmt.Errorf("line %d: placeholder refers to unknown block %q", lineNo(src, p.Span.Start), p.BlockID)
		}
	}
	for _, r := range d.Refs() {
		if r.BlockID != "" && !ids[r.BlockID] {
			return nil, fmt.Errorf("%s refers to unknown block %q", r, r.BlockID)
		}
	}
	return d, nil
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

// readFence reads the block whose opening fence spans [start, bodyStart).
// It returns the block and the offset just past its closing fence.
// An unclosed fence runs to the end of the document.
func readFence(src []byte, start, bodyStart int, f fence) (*Block, int) {
	var body bytes.Buffer
	pos := bodyStart
	for pos < len(src) {
		end := lineEnd(src, pos)
		line := src[pos:end]
		if closesFence(line, f) {
			pos = end
			break
		}
		body.Write(trimIndent(line, f.indent))
		pos = end
	}
	lang, attrs := ParseInfo(f.info)
	return &Block{
		ID:    attrs["#"],
		Lang:  lang,
		Attrs: attrs,
		Body:  body.String(),
		Span:  Span{start, pos},
	}, pos
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
// body starts at pos. An unclosed region ends at the end of the document.
func regionEnd(src []byte, pos int) int {
	for pos < len(src) {
		end := lineEnd(src, pos)
		if string(bytes.TrimSpace(src[pos:end])) == regionClose {
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
