// Package stress writes long markdown documents of several shapes, to
// see how mdsh-gui copes with them: the benchmarks in the notebook
// package parse and lay them out, and cmd/mdsh-gui/internal/stress/gui.cjs types into them in the
// real frontend. Command stressdoc writes one to a file.
package stress

import (
	"bytes"
	"fmt"
	"slices"
)

// Shapes are the kinds of document Write makes:
//
//   - runbook: sections of sh blocks, each with saved output, a list and
//     some prose, and every third block depends on the one before. The
//     common case.
//   - prose: mostly paragraphs, lists, tables, quotes and fenced code
//     that doesn't run, with a block now and then.
//   - output: few blocks, each with a long saved output.
//   - inputs: textboxes and choices bound with var=, and blocks that use
//     them and depend on each other in long chains.
//   - flat: blocks without ids and prose, with no headings at all, so
//     nothing folds, no section ends and mdsh names each block by its
//     line.
var Shapes = []string{"runbook", "prose", "output", "inputs", "flat"}

// Write returns a document of shape about lines long: it stops at the
// first whole part that reaches lines. It panics on an unknown shape.
func Write(shape string, lines int) []byte {
	if !slices.Contains(Shapes, shape) {
		panic("stress: no shape " + shape)
	}
	w := &writer{}
	w.line("# Stress test: %s, %d lines", shape, lines)
	w.line("")
	w.line("A generated document for seeing how mdsh copes with long files.")
	w.line("")
	for part := 0; w.n < lines; part++ {
		switch shape {
		case "runbook":
			w.runbook(part)
		case "prose":
			w.prose(part)
		case "output":
			w.output(part)
		case "inputs":
			w.inputs(part)
		case "flat":
			w.flat(part)
		}
	}
	return w.buf.Bytes()
}

type writer struct {
	buf    bytes.Buffer
	n      int // lines written
	blocks int // blocks written, for their ids
}

func (w *writer) line(format string, args ...any) {
	fmt.Fprintf(&w.buf, format, args...)
	w.buf.WriteByte('\n')
	w.n++
}

// block writes an sh block with id, its attrs, its code and, if out is
// more than 0, that many lines of saved output.
func (w *writer) block(attrs string, out int, code ...string) string {
	w.blocks++
	id := fmt.Sprintf("step%d", w.blocks)
	if attrs != "" {
		attrs = " " + attrs
	}
	w.line("```sh {#%s%s}", id, attrs)
	for _, c := range code {
		w.line("%s", c)
	}
	w.line("```")
	w.line("")
	if out > 0 {
		w.line(`<!-- mdsh:output for="%s" -->`, id)
		w.line("```output")
		for i := range out {
			w.line("%s: line %d of the output, with some text to give it width", id, i+1)
		}
		w.line("```")
		w.line("<!-- /mdsh:output -->")
		w.line("")
	}
	return id
}

func (w *writer) paragraph(part int) {
	w.line("Part %d. Some prose describing this step, with **bold**, _italic_,", part)
	w.line("`inline code` and a [link](https://example.com/%d) in it.", part)
	w.line("")
}

func (w *writer) runbook(part int) {
	w.line("## Section %d", part)
	w.line("")
	w.paragraph(part)
	prev := ""
	for b := range 3 {
		attrs := ""
		if b == 2 && prev != "" {
			attrs = "depends=" + prev
		}
		prev = w.block(attrs, 4, fmt.Sprintf("echo step %d", w.blocks+1), "ls -la /tmp | head -3")
	}
	w.line("- a list item")
	w.line("- another item")
	w.line("")
}

func (w *writer) prose(part int) {
	w.line("## Chapter %d", part)
	w.line("")
	for range 3 {
		w.paragraph(part)
	}
	w.line("### Details %d", part)
	w.line("")
	w.line("1. first")
	w.line("2. second")
	w.line("   - nested")
	w.line("")
	w.line("| name | value |")
	w.line("| --- | --- |")
	w.line("| a%d | %d |", part, part)
	w.line("| b%d | %d |", part, part*2)
	w.line("")
	w.line("> A quote, for part %d.", part)
	w.line("")
	w.line("```go")
	w.line("func part%d() int { return %d }", part, part)
	w.line("```")
	w.line("")
	if part%5 == 0 {
		w.block("", 2, fmt.Sprintf("echo part %d", part))
	}
}

func (w *writer) output(part int) {
	w.line("## Long output %d", part)
	w.line("")
	w.paragraph(part)
	w.block("", 500, fmt.Sprintf("seq 500 # part %d", part))
}

func (w *writer) inputs(part int) {
	w.line("## Inputs %d", part)
	w.line("")
	w.paragraph(part)
	w.line("```textbox {#host%d var=HOST%d lines=1}", part, part)
	w.line("host-%d.example.com", part)
	w.line("```")
	w.line("")
	w.line("```choice {#env%d var=ENV%d mode=radio}", part, part)
	w.line("- [x] staging")
	w.line("- [ ] production")
	w.line("```")
	w.line("")
	prev := ""
	if w.blocks > 0 {
		prev = fmt.Sprintf("step%d", w.blocks)
	}
	for range 2 {
		attrs := ""
		if prev != "" {
			attrs = "depends=" + prev
		}
		prev = w.block(attrs, 2, fmt.Sprintf(`echo "$HOST%d $ENV%d"`, part, part))
	}
}

func (w *writer) flat(part int) {
	w.paragraph(part)
	w.line("```sh")
	w.line("echo %d", part)
	w.line("```")
	w.line("")
}
