package render

import (
	"bytes"
	"cmp"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/doc"
)

// Clear returns d's source with every run's output taken out, as if no
// block had ever run: the output after each block goes, each output
// copy is left empty so a later run can fill it, and placeholders that
// hold a block's value ({{#id/...}}, {{$?}}) lose their stored value.
// Includes keep what they read, since that is the file's text, not a
// run's output, and a copy that names no block is left as it is.
func Clear(d *doc.Document) []byte {
	src := d.Src
	type edit struct {
		span doc.Span
		text string
	}
	var edits []edit
	for _, b := range d.Blocks {
		if b.IsInclude() || !b.HasOutput() {
			continue
		}
		edits = append(edits, edit{b.Output, ""})
	}
	inEdit := func(pos int) bool {
		return slices.ContainsFunc(edits, func(e edit) bool { return pos >= e.span.Start && pos < e.span.End })
	}
	for _, c := range d.Copies {
		// A copy of an include or tape, or of no block, isn't a run's.
		i := slices.IndexFunc(d.Blocks, func(b *doc.Block) bool { return b.ID == c.For })
		if i < 0 || d.Blocks[i].IsInclude() || d.Blocks[i].IsTape() || inEdit(c.Span.Start) {
			continue
		}
		var out string
		switch {
		case c.Region:
			out = doc.RegionStart(c.For) + "\n" + doc.RegionEnd + "\n"
		case c.Stream == "stderr":
			out = Fence("output", "for="+attrValue(c.For)+" stream=stderr", "")
		default:
			out = Fence("output", "for="+attrValue(c.For), "")
		}
		out = indent(out, c.Prefix)
		if c.Lead != "" {
			out = c.Lead + strings.TrimPrefix(out, c.Prefix)
		}
		if !bytes.HasSuffix(src[c.Span.Start:c.Span.End], []byte("\n")) {
			out = strings.TrimSuffix(out, "\n")
		}
		edits = append(edits, edit{c.Span, out})
	}
	for _, p := range d.Placeholders {
		if !p.HasValue || (p.BlockID == "" && !p.Fact()) || inEdit(p.Span.Start) {
			continue
		}
		edits = append(edits, edit{p.Span, p.Ref.String()})
	}
	slices.SortFunc(edits, func(a, b edit) int { return cmp.Compare(a.span.Start, b.span.Start) })

	var buf bytes.Buffer
	pos := 0
	for _, e := range edits {
		if e.span.Start < pos {
			continue // inside an edit already made
		}
		buf.Write(src[pos:e.span.Start])
		buf.WriteString(e.text)
		pos = e.span.End
	}
	buf.Write(src[pos:])
	return buf.Bytes()
}
