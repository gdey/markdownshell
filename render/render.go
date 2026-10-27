// Package render turns a document and its run results back into markdown.
package render

import (
	"bytes"
	"fmt"
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
	var buf bytes.Buffer
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
			writeProse(&buf, d, k.Start, k.End, results, lookup, opts.WriteBack)
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
		prose(pos, b.Span.Start)
		pos = b.Output.End
		if inCut(b.Span.Start, cuts) {
			continue
		}

		showCode := !(opts.HideCode && run.Runnable(b)) && shown(b, "show")
		showOutput := !opts.HideOutput && shown(b, "show-output")
		if showCode {
			buf.Write(src[b.Span.Start:b.Span.End])
		}
		if !showOutput {
			if !showCode {
				pos = skipBlankLine(src, pos)
			}
			continue
		}
		var r *run.Result
		if i < len(results) {
			r = results[i]
		}
		if r == nil || !r.Ran {
			if showCode {
				buf.Write(src[b.Span.End:b.Output.End])
			} else {
				buf.Write(bytes.TrimLeft(src[b.Span.End:b.Output.End], "\r\n"))
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
			buf.WriteByte('\n')
		}
		buf.WriteString(out)
	}
	prose(pos, len(src))
	return buf.Bytes()
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
func Prose(d *doc.Document, env []string) []byte {
	return Markdown(d, nil, Options{HideCode: true, HideOutput: true, Env: env})
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
	case "image", "screenshot":
		if r.Image != "" {
			b.WriteString(doc.RegionStart(id) + "\n")
			fmt.Fprintf(&b, "![%s](%s)\n", id, r.Image)
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
