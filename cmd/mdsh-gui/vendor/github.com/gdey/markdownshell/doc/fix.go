package doc

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Fix is an edit that makes one of a document's warnings go away, one
// so plain it can be offered at a click, after asking.
type Fix struct {
	// Warning is the warning it fixes, as Warnings has it, without the
	// "line N: " in front.
	Warning string
	// Label names it for a button, such as "Delete the copy".
	Label string
	// Detail says what it does, for asking first.
	Detail string
	// Src is the document's source with the fix made.
	Src []byte
}

// Fixes lists the fixes for d's warnings that have one: an output copy
// for a block that isn't there goes, and a depends= loses an id that
// names no block that runs, and a value in front matter that doesn't
// parse is quoted when a colon in it is why. A warning has at most one
// fix; the front matter's is for FrontMatterErr's text.
func (d *Document) Fixes() []Fix {
	var out []Fix
	if f, ok := d.frontMatterFix(); ok {
		out = append(out, f)
	}
	seen := map[string]bool{}
	for _, c := range d.Copies {
		if b := d.Block(c.For); b != nil && !b.IsInclude() && !b.IsTape() || seen[c.For] {
			continue
		}
		seen[c.For] = true
		out = append(out, d.copyFix(c.For))
	}
	for _, b := range d.Blocks {
		for _, id := range b.Depends() {
			dep := d.Block(id)
			switch {
			case dep == nil || dep.IsInclude() || dep.IsTape():
				out = append(out, d.dependsFix(b, id, unknownDepWarning(b, id), fmt.Sprintf("No block that runs is called %s.", id)))
			case dep.IsInput():
				out = append(out, d.dependsFix(b, id, idleDepWarning(b, dep), fmt.Sprintf("%s is a %s, which never runs.", id, dep.Lang)))
			}
		}
	}
	return out
}

// copyWarning is the warning for an output copy whose for= names no
// block that has output.
func copyWarning(id string) string {
	return fmt.Sprintf("output copy for unknown block %q", id)
}

// copyFix deletes every output copy for id, last first, so the earlier
// ones' spans still hold.
func (d *Document) copyFix(id string) Fix {
	t := *d
	var lines []string
	for _, c := range slices.Backward(d.Copies) {
		if c.For != id {
			continue
		}
		lines = append(lines, strconv.Itoa(lineNo(d.Src, c.Span.Start)))
		e := c.Span.End + leadingBlankLen(t.Src[c.Span.End:])
		t.Src = t.cut(c.Span.Start, e)
	}
	slices.Reverse(lines)
	what, label := "the output copy at line "+lines[0], "Delete the copy"
	if len(lines) > 1 {
		what = fmt.Sprintf("the %d output copies at lines %s and %s", len(lines), strings.Join(lines[:len(lines)-1], ", "), lines[len(lines)-1])
		label = "Delete the copies"
	}
	return Fix{
		Warning: copyWarning(id),
		Label:   label,
		Detail:  fmt.Sprintf("Delete %s. Its for=%s names no block that has output, so it shows nothing.", what, id),
		Src:     t.Src,
	}
}

// dependsFix takes id out of b's depends=, or takes depends= off b when
// id is all it names.
func (d *Document) dependsFix(b *Block, id, warning, why string) Fix {
	ids := slices.DeleteFunc(b.Depends(), func(s string) bool { return s == id })
	in := SplitInfo(b.Info)
	detail := fmt.Sprintf("Take %s out of block %s's depends=. %s", id, d.Label(b), why)
	if len(ids) == 0 {
		in.Del("depends")
		detail = fmt.Sprintf("Take depends=%s off block %s. %s", id, d.Label(b), why)
	} else {
		in.Set("depends", strings.Join(ids, ","), false)
	}
	span := d.InfoSpan(b)
	src := append(append(append([]byte(nil), d.Src[:span.Start]...), in.String()...), d.Src[span.End:]...)
	return Fix{Warning: warning, Label: "Remove from depends=", Detail: detail, Src: src}
}
