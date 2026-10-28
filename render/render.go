// Package render turns a document and its run results back into markdown.
package render

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// Options control what Markdown prints.
type Options struct {
	// HideCode leaves out runnable blocks. Other code blocks are part of
	// the prose and stay.
	HideCode bool
	// HideOutput leaves out output, old and new.
	HideOutput bool
	// WriteBack renders for saving to the file: the #! line stays,
	// placeholders keep their {{...}} form with the new value inside, and
	// nothing is left out by show=, show-output= or mdsh:if sections.
	// Otherwise the #! line and section markers are dropped, placeholders
	// print as values, and conditions decide what shows.
	WriteBack bool
	// Env is the caller's environment, for {{$VAR}}. Nil means unknown:
	// placeholders keep their stored values and conditions read empty.
	Env []string
}

// Markdown renders d with results spliced in after their blocks. Blocks
// without a result, or that didn't run, keep the output already in the
// file. results may be nil, and is matched to blocks by position.
func Markdown(d *doc.Document, results []*run.Result, opts Options) []byte {
	return bytes.Join(Pieces(d, results, opts), nil)
}

// Pieces renders d like Markdown, cut at its blocks so a run can print
// the document as it goes. For block i, piece 3i is the prose before it,
// 3i+1 the block and 3i+2 its output; the last piece is the prose after
// the last block. results may hold nil for blocks that haven't finished.
func Pieces(d *doc.Document, results []*run.Result, opts Options) [][]byte {
	bufs := make([]bytes.Buffer, 3*len(d.Blocks)+1)
	buf := &bufs[0]
	src := d.Src
	lookup := run.NewLookup(d, results, opts.Env)

	// cuts are the parts of the source that printing leaves out.
	var cuts []doc.Span
	if !opts.WriteBack {
		if d.Shebang != "" {
			cuts = append(cuts, doc.Span{Start: 0, End: lineEnd(src, 0)})
		}
		for _, s := range d.Sections {
			if s.Cond.Eval(run.LookupAt(lookup, d, results, s.If.Start)) {
				cuts = append(cuts, s.If, s.End)
			} else {
				cuts = append(cuts, doc.Span{Start: s.If.Start, End: s.End.End})
			}
		}
	}
	prose := func(from, to int) {
		for _, k := range keep(from, to, cuts) {
			writeProse(buf, d, k.Start, k.End, results, lookup, opts.WriteBack)
		}
	}
	// Display conditions are checked after the block, so {{$?}} is its
	// own status.
	after := func(b *doc.Block) doc.Lookup { return run.LookupAt(lookup, d, results, b.Output.End) }
	shown := func(b *doc.Block, attr string) bool {
		c, ok := b.Cond(attr)
		return opts.WriteBack || !ok || c.Eval(after(b))
	}

	pos := 0
	for i, b := range d.Blocks {
		buf = &bufs[3*i]
		prose(pos, b.Span.Start)
		pos = b.Output.End
		buf = &bufs[3*i+1]
		if inCut(b.Span.Start, cuts) {
			continue
		}

		var r *run.Result
		if i < len(results) {
			r = results[i]
		}
		if b.IsInclude() {
			// Includes are part of the prose, so the hide flags leave
			// them alone.
			if !shown(b, "show") {
				pos = skipBlankLine(src, pos)
				continue
			}
			writeInclude(buf, src, b, r, opts.WriteBack)
			buf = &bufs[3*i+2]
			switch {
			case opts.HideOutput:
			case r != nil && r.Err != nil:
				buf.WriteString(strings.TrimRight(b.Prefix, " ") + "\n")
				buf.WriteString(indent(Output(r, true), b.Prefix))
			case r == nil || !r.Ran:
				buf.Write(src[b.Span.End:b.Output.End])
			}
			continue
		}

		showCode := !(opts.HideCode && run.Runnable(b)) && shown(b, "show")
		showOutput := !opts.HideOutput && shown(b, "show-output")
		if showCode {
			buf.Write(src[b.Span.Start:b.Span.End])
		}
		buf = &bufs[3*i+2]
		if !showOutput {
			if !showCode {
				pos = skipBlankLine(src, pos)
			}
			continue
		}
		if r == nil || !r.Ran {
			if showCode {
				buf.Write(src[b.Span.End:b.Output.End])
			} else if kept := dropGap(src[b.Span.End:b.Output.End], b.Prefix); b.Lead != "" && len(kept) > 0 {
				buf.WriteString(b.Lead)
				buf.Write(bytes.TrimPrefix(kept, []byte(b.Prefix)))
			} else {
				buf.Write(kept)
			}
			continue
		}
		out := Output(r, showStderr(d, b, r, after(b)))
		if out == "" {
			if !showCode {
				pos = skipBlankLine(src, pos)
			}
			continue
		}
		if showCode {
			if !bytes.HasSuffix(src[b.Span.Start:b.Span.End], []byte("\n")) {
				buf.WriteByte('\n')
			}
			buf.WriteString(strings.TrimRight(b.Prefix, " ") + "\n")
			buf.WriteString(indent(out, b.Prefix))
		} else if b.Lead != "" {
			// The output takes the hidden fence's place in its list item.
			buf.WriteString(b.Lead + strings.TrimPrefix(indent(out, b.Prefix), b.Prefix))
		} else {
			buf.WriteString(indent(out, b.Prefix))
		}
	}
	buf = &bufs[len(bufs)-1]
	prose(pos, len(src))
	out := make([][]byte, len(bufs))
	for i := range bufs {
		out[i] = bufs[i].Bytes()
	}
	return out
}

// showStderr decides whether a block's stderr is shown: its show-stderr=
// if set, else the front matter's show_stderr, else only if it failed.
func showStderr(d *doc.Document, b *doc.Block, r *run.Result, lookup doc.Lookup) bool {
	if c, ok := b.Cond("show-stderr"); ok {
		return c.Eval(lookup)
	}
	if d.Options.ShowStderr != nil {
		return *d.Options.ShowStderr
	}
	return r.Failed()
}

// keep returns the parts of [from, to) outside every cut.
func keep(from, to int, cuts []doc.Span) []doc.Span {
	out := []doc.Span{{Start: from, End: to}}
	for _, c := range cuts {
		var next []doc.Span
		for _, k := range out {
			if c.End <= k.Start || c.Start >= k.End {
				next = append(next, k)
				continue
			}
			if k.Start < c.Start {
				next = append(next, doc.Span{Start: k.Start, End: c.Start})
			}
			if c.End < k.End {
				next = append(next, doc.Span{Start: c.End, End: k.End})
			}
		}
		out = next
	}
	return out
}

func inCut(pos int, cuts []doc.Span) bool {
	for _, c := range cuts {
		if pos >= c.Start && pos < c.End {
			return true
		}
	}
	return false
}

func lineEnd(src []byte, pos int) int {
	if i := bytes.IndexByte(src[pos:], '\n'); i >= 0 {
		return pos + i + 1
	}
	return len(src)
}

// writeProse writes src[from:to], filling in the placeholders inside it.
// A placeholder takes its value from lookup, else keeps its stored value.
// {{$?}} is the status of the last block above it that ran.
func writeProse(buf *bytes.Buffer, d *doc.Document, from, to int, results []*run.Result, lookup doc.Lookup, writeBack bool) {
	for _, p := range d.Placeholders {
		if p.Span.Start < from || p.Span.End > to {
			continue
		}
		buf.Write(d.Src[from:p.Span.Start])
		v, ok := run.LookupAt(lookup, d, results, p.Span.Start)(p.Ref)
		if !ok {
			v = p.Value
		}
		if writeBack {
			buf.WriteString(p.Source(v))
		} else {
			buf.WriteString(v)
		}
		from = p.Span.End
	}
	buf.Write(d.Src[from:to])
}

// dropGap drops the blank lines, which may hold only prefix, at the start
// of s.
func dropGap(s []byte, prefix string) []byte {
	p := []byte(strings.TrimRight(prefix, " "))
	for len(s) > 0 {
		end := lineEnd(s, 0)
		if len(bytes.TrimSpace(bytes.TrimPrefix(s[:end], p))) > 0 {
			break
		}
		s = s[end:]
	}
	return s
}

// skipBlankLine skips one blank line at pos, so a hidden block doesn't
// leave a double gap in the prose.
func skipBlankLine(src []byte, pos int) int {
	if bytes.HasPrefix(src[pos:], []byte("\r\n")) {
		return pos + 2
	}
	if bytes.HasPrefix(src[pos:], []byte("\n")) {
		return pos + 1
	}
	return pos
}

// Prose renders d with every runnable block and all output left out.
// results may hold the include blocks' lines, as from run.Includes.
func Prose(d *doc.Document, results []*run.Result, env []string) []byte {
	return Markdown(d, results, Options{HideCode: true, HideOutput: true, Env: env})
}

// Output renders one block's result as the markdown that follows it.
// stderr is included only when withStderr is set; mdsh's own errors
// always are. It returns "" when there is nothing to show.
func Output(r *run.Result, withStderr bool) string {
	var b strings.Builder
	id := r.Block.ID
	stderr := ""
	if withStderr {
		stderr = string(r.Stderr)
	}
	if r.Err != nil {
		if stderr != "" && !strings.HasSuffix(stderr, "\n") {
			stderr += "\n"
		}
		stderr += "mdsh: " + r.Err.Error() + "\n"
	}

	switch run.OutKind(r.Block) {
	case "markdown":
		if len(r.Stdout) > 0 {
			b.WriteString(doc.RegionStart(id) + "\n")
			b.WriteString(withNewline(string(r.Stdout)))
			b.WriteString(doc.RegionEnd + "\n")
		}
	case "image", "screenshot", "recording":
		if r.Image != "" {
			link := r.Image
			// frame= picks the frame a still view, such as a PDF, shows.
			if f := r.Block.Attrs["frame"]; f != "" && run.OutKind(r.Block) == "recording" {
				link += "#frame=" + f
			}
			b.WriteString(doc.RegionStart(id) + "\n")
			fmt.Fprintf(&b, "![%s](%s)\n", id, link)
			b.WriteString(doc.RegionEnd + "\n")
		}
	default:
		if len(r.Stdout) > 0 || r.Status != 0 || r.Exited {
			attrs := fmt.Sprintf("for=%s exit=%d", attrValue(id), r.Status)
			if r.Exited {
				attrs += " exited"
			}
			b.WriteString(Fence("output", attrs, string(r.Stdout)))
		}
	}
	if stderr != "" {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(Fence("output", fmt.Sprintf("for=%s stream=stderr", attrValue(id)), stderr))
	}
	return b.String()
}

// writeInclude writes include block b with the lines r read as its body.
// Printing numbers the lines if b asks for it; write-back stores them as
// they are. Without lines, b is written as it is in src.
func writeInclude(buf *bytes.Buffer, src []byte, b *doc.Block, r *run.Result, writeBack bool) {
	if r == nil || !r.Ran || r.Err != nil {
		buf.Write(src[b.Span.Start:b.Span.End])
		return
	}
	body := withNewline(string(r.Stdout))
	if inc, _ := b.Include(); inc.Numbered && !writeBack {
		body = numberLines(body, max(inc.From, 1))
	}
	open := string(src[b.Span.Start:lineEnd(src, b.Span.Start)])
	closing := string(src[b.BodySpan.End:b.Span.End])
	// A longer fence keeps the lines from closing it.
	if n := fenceLen(b.Fence[0], body); n > len(b.Fence) || closing == "" || !strings.HasSuffix(open, "\n") {
		head := b.Lead
		if head == "" {
			head = b.Prefix
		}
		f := strings.Repeat(b.Fence[:1], max(n, len(b.Fence)))
		open, closing = head+f+b.Info+"\n", b.Prefix+f+"\n"
	}
	buf.WriteString(open)
	buf.WriteString(indent(body, b.Prefix))
	buf.WriteString(closing)
}

// numberLines starts each line of s with its number, counting from first.
func numberLines(s string, first int) string {
	lines := strings.SplitAfter(strings.TrimSuffix(s, "\n"), "\n")
	w := len(strconv.Itoa(first + len(lines) - 1))
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%*d  %s", w, first+i, line)
	}
	return withNewline(b.String())
}

// fenceLen returns the length of a fence of c that no run of c in s can
// close: at least 3, and longer than the longest run.
func fenceLen(c byte, s string) int {
	n, run := 3, 0
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			run = 0
			continue
		}
		if run++; run >= n {
			n = run + 1
		}
	}
	return n
}

// Fence wraps body in a backtick fence long enough that body can't close it.
func Fence(lang, attrs, body string) string {
	n := 3
	run := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '`' {
			run++
			if run >= n {
				n = run + 1
			}
		} else {
			run = 0
		}
	}
	f := strings.Repeat("`", n)
	info := lang
	if attrs != "" {
		info += " {" + attrs + "}"
	}
	return f + info + "\n" + withNewline(body) + f + "\n"
}

// indent starts each line of s with prefix, trimming the prefix's trailing
// spaces on blank lines.
func indent(s, prefix string) string {
	if prefix == "" {
		return s
	}
	var b strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		if line == "" {
			continue
		}
		if line == "\n" || line == "\r\n" {
			b.WriteString(strings.TrimRight(prefix, " "))
		} else {
			b.WriteString(prefix)
		}
		b.WriteString(line)
	}
	return b.String()
}

func withNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func attrValue(s string) string {
	if strings.ContainsAny(s, " \t\"{}") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}
