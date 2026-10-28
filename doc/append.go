package doc

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// AppendBlock adds a fenced block of lang holding body to the end of d,
// after a blank line, and returns it. Earlier blocks keep their spans, so
// results for them stay valid. It fails if the end of d would swallow
// the block, such as an unclosed fence.
func (d *Document) AppendBlock(lang, body string) (*Block, error) {
	if body == "" {
		return nil, errors.New("empty block")
	}
	if body[len(body)-1] != '\n' {
		body += "\n"
	}
	fence := "```"
	for bytes.Contains([]byte(body), []byte(fence)) {
		fence += "`"
	}
	src := bytes.TrimRight(d.Src, "\n")
	src = append(append([]byte(nil), src...), "\n\n"+fence+lang+"\n"+body+fence+"\n"...)
	if len(bytes.TrimSpace(d.Src)) == 0 {
		src = src[2:]
	}
	nd, err := Parse(src)
	if err != nil {
		return nil, err
	}
	if len(nd.Blocks) != len(d.Blocks)+1 {
		return nil, fmt.Errorf("the end of the document doesn't take a new block")
	}
	b := nd.Blocks[len(nd.Blocks)-1]
	if b.Prefix != "" || b.Lang != lang {
		return nil, fmt.Errorf("the end of the document doesn't take a new block")
	}
	d.Src = src
	d.Blocks = append(d.Blocks, b)
	return b, nil
}

// BlockText is b's source, opening fence to closing fence, without the
// list markers and block quote prefixes it is nested in. SetBlock takes
// it back.
func (d *Document) BlockText(b *Block) string {
	src := string(d.Src[b.Span.Start:b.Span.End])
	first, rest, _ := strings.Cut(src, "\n")
	var out strings.Builder
	out.WriteString(first[strings.Index(first, b.Fence):])
	out.WriteByte('\n')
	for _, l := range strings.SplitAfter(rest, "\n") {
		if l == "" {
			continue
		}
		if t, ok := strings.CutPrefix(l, b.Prefix); ok {
			l = t
		} else {
			l = strings.TrimPrefix(l, strings.TrimRight(b.Prefix, " "))
		}
		out.WriteString(l)
	}
	return out.String()
}

// SetBlock replaces b, which must be one of d's blocks, with text, as
// BlockText gives it, and parses d again. The fence, attributes and id
// may change; output saved for b follows a new id. Every *Block in d
// keeps its identity, with its spans moved to match the new source, so
// results that point at them stay valid. It fails if text isn't one
// block of the same kind, or would rename other blocks.
func (d *Document) SetBlock(b *Block, text string) error {
	i := slices.Index(d.Blocks, b)
	if i < 0 {
		return errors.New("not a block of this document")
	}
	if text != "" && text[len(text)-1] != '\n' {
		text += "\n"
	}
	first := string(d.Src[b.Span.Start:b.Span.End])
	first, _, _ = strings.Cut(first, "\n")
	head := first[:strings.Index(first, b.Fence)]
	var lines strings.Builder
	for n, l := range strings.SplitAfter(text, "\n") {
		switch {
		case l == "":
		case n == 0:
			lines.WriteString(head + l)
		default:
			lines.WriteString(b.Prefix + l)
		}
	}
	src := splice(d.Src, b.Span, lines.String())
	nd, err := Parse(src)
	if err != nil {
		return err
	}
	if len(nd.Blocks) != len(d.Blocks) {
		return errors.New("the edit would change the document's blocks; is it one fenced block?")
	}
	nb := nd.Blocks[i]
	if nb.Span != (Span{b.Span.Start, b.Span.Start + lines.Len()}) || nb.BodySpan.End == nb.Span.End {
		return errors.New("the edit would change the document's blocks; is it one closed fenced block?")
	}
	if nb.IsInclude() != b.IsInclude() || nb.IsTape() != b.IsTape() {
		return errors.New("the edit would change what kind of block this is")
	}
	for j, o := range nd.Blocks {
		if j != i && o.ID != d.Blocks[j].ID {
			return fmt.Errorf("the edit would rename #%s to #%s; give the block an id", d.Blocks[j].ID, o.ID)
		}
	}
	if nb.ID != b.ID && b.HasOutput() {
		// The output is now prose: point it at the new id.
		at := Span{nb.Span.End, nb.Span.End + b.Output.End - b.Output.Start}
		re := regexp.MustCompile(`(for=)` + regexp.QuoteMeta(b.ID) + `([\s}])`)
		src = splice(src, at, re.ReplaceAllString(string(src[at.Start:at.End]), "${1}"+nb.ID+"${2}"))
		if nd, err = Parse(src); err != nil {
			return err
		}
	}
	blocks := d.Blocks
	for j, nb := range nd.Blocks {
		*blocks[j] = *nb
	}
	*d = *nd
	d.Blocks = blocks
	return nil
}

// splice returns a copy of src with the bytes at span replaced by s.
func splice(src []byte, span Span, s string) []byte {
	return append(append(append([]byte(nil), src[:span.Start]...), s...), src[span.End:]...)
}
