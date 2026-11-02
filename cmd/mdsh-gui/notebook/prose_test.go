package notebook

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

const proseSrc = `---
mdsh:
  shell: sh
---
Title
=====

- one

- two
---

| a | b |
| - | - |
| 1 | 2 |

<!-- mdsh:if {{$MDSH_GUI_NEVER_SET}} -->
Hidden.
<!-- mdsh:end -->
Say [hi][ref] to {{#greet/env(NAME):"you"}}.

` + "```sh {#greet}\nNAME=world\n```\n" + `
Left over:
` + "```output {for=gone}\nold\n```\n\n- ~~~\n  plain\n  ~~~\n" + `
[ref]: https://example.com
`

func TestProseCells(t *testing.T) {
	n, err := Open(write(t, proseSrc), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	src := n.Text()
	var got []string
	for _, c := range n.Cells() {
		got = append(got, src[c.From:c.To])
	}
	want := []string{
		"Title\n=====\n",
		// A thematic break has no lines of its own in goldmark, so it
		// goes with the block above.
		"- one\n\n- two\n---\n",
		"| a | b |\n| - | - |\n| 1 | 2 |\n",
		"Say [hi][ref] to {{#greet/env(NAME):\"you\"}}.\n",
		"```sh {#greet}\nNAME=world\n```\n",
		// Fences mdsh doesn't run start at their opening line.
		"Left over:\n",
		"```output {for=gone}\nold\n```\n",
		"- ~~~\n  plain\n  ~~~\n",
		"[ref]: https://example.com\n",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("cells:\n%q\nwant:\n%q", got, want)
	}
	cells := n.Cells()
	if c := cells[0]; c.Line != 5 {
		t.Errorf("first cell: line %d", c.Line)
	}
	if h := cells[3].HTML; !strings.Contains(h, `<a href="https://example.com">hi</a>`) || !strings.Contains(h, "to you.") {
		t.Errorf("a link to a reference elsewhere, and the placeholder, should be filled in: %q", h)
	}
}

func TestSetText(t *testing.T) {
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunRange(0, 4); err != nil {
		t.Fatal(err)
	}
	// fail moves above hello, and hello is edited.
	text := "```sh {#fail}\n(exit 3)\n```\n\n```sh {#hello}\necho bye\n```\n"
	ran := n.Text()
	// Typed over the file as it was before the run, the text would drop
	// the run's output, so it is turned down.
	if ok, err := n.SetText(src, text); ok || err != nil || n.Text() != ran {
		t.Fatalf("a stale base: ok = %v, err = %v, text %q", ok, err, n.Text())
	}
	if ok, err := n.SetText(ran, text); !ok || err != nil {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	cells := n.Cells()
	if len(cells) != 2 || cells[0].ID != "fail" || !cells[0].Failed || cells[1].Ran {
		t.Errorf("cells = %+v", cells)
	}
	if err := n.RunBlock(1); err != nil {
		t.Fatal(err)
	}
	if c := n.Cells()[1]; !strings.Contains(c.Output, "bye") {
		t.Errorf("edited block output = %q", c.Output)
	}
	if !strings.Contains(n.Text(), "```output {for=hello exit=0}\nbye\n```") || !n.Cells()[1].Ran {
		t.Errorf("after a run, the text should hold the output and results stay:\n%s", n.Text())
	}
	if !n.Dirty() {
		t.Error("output not saved yet, but not dirty")
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	text = n.Text()
	if _, err := n.SetText(text, "---\nmdsh: [\n---\n"); err == nil || n.Text() != text {
		t.Errorf("a bad text: err = %v, text %q", err, n.Text())
	}
}

func TestCollapsed(t *testing.T) {
	src := "# A\n<!-- mdsh:collapse -->\n\n" +
		"```sh {#a collapse collapse-output={{$?}}==0}\necho hi\n```\n\n```output {for=a}\nhi\n```\n\n" +
		"## B\n\n```choice {#d var=details}\n- [ ] yes\n- [x] no\n```\n\n" +
		"## C\ntext\n<!-- mdsh:collapse {{$MDSH_VAR_details}}!=no -->\n\n" +
		"```sh {#b}\necho b\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var got []string
	for _, c := range n.Cells() {
		got = append(got, strings.TrimSpace(n.Text()[c.From:min(c.To, c.From+6)]))
		switch {
		case c.Heading == 1:
			if !c.Collapsed {
				t.Errorf("# A isn't collapsed")
			}
		case c.Heading == 2 && strings.Contains(c.HTML, ">B<"):
			if c.Collapsed {
				t.Errorf("## B is collapsed")
			}
		case c.Heading == 2:
			if c.Collapsed {
				t.Errorf("## C is collapsed, though details is no")
			}
		case c.Block == 0:
			// The block hasn't run, so {{$?}} reads as empty.
			if !c.Collapsed || c.CollapsedOutput || c.OutputLine != 8 {
				t.Errorf("block a: collapsed %v, output %v, output line %d", c.Collapsed, c.CollapsedOutput, c.OutputLine)
			}
		case c.Block == 2:
			if c.Collapsed || c.OutputLine != 0 {
				t.Errorf("block b: collapsed %v, output line %d", c.Collapsed, c.OutputLine)
			}
		}
		if strings.Contains(c.HTML, "mdsh:collapse") {
			t.Errorf("cell shows the collapse line: %q", c.HTML)
		}
	}
	if len(got) != 7 {
		t.Errorf("cells = %q", got)
	}
	// Picking yes folds C.
	if err := n.SetChoice(1, []bool{true, false}); err != nil {
		t.Fatal(err)
	}
	for _, c := range n.Cells() {
		if c.Heading == 2 && strings.Contains(c.HTML, ">C<") && !c.Collapsed {
			t.Errorf("## C isn't collapsed after picking yes")
		}
	}
}

func TestCellsSections(t *testing.T) {
	src := "Intro.\n\n```sh {#pre}\necho pre\n```\n\n# Setup\n\n```sh {#a}\necho a\n```\n\n## Notes\n\nNo blocks here.\n\n## Check\n\n```sh {#hidden show=false}\necho hidden\n```\n\n# Deploy\n\n```sh {#d}\necho d\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var got []string
	for _, c := range n.Cells() {
		if c.Heading > 0 {
			got = append(got, fmt.Sprintf("%d:%d-%d", c.Line, c.SectionFrom, c.SectionTo))
		}
	}
	// Setup holds its subsections; Notes has no block; Check's block is
	// hidden but still runs.
	if want := []string{"7:1-3", "13:0-0", "17:2-3", "23:3-4"}; !slices.Equal(got, want) {
		t.Errorf("sections = %v, want %v", got, want)
	}
	if err := n.RunRange(1, 3); err != nil {
		t.Fatal(err)
	}
	var ran []string
	for i, r := range n.results {
		if r.Ran {
			ran = append(ran, n.d.Blocks[i].ID)
		}
	}
	if want := []string{"a", "hidden"}; !slices.Equal(ran, want) {
		t.Errorf("ran %v, want %v", ran, want)
	}
}

// Every block Run all runs is drawn, even one printing leaves out, so a
// document can't run code its reader never saw (GUI security review
// v1.2.1, finding 2).
func TestHiddenBlocksDrawn(t *testing.T) {
	file := write(t, "# Setup\n\n```sh {#hello}\necho hello\n```\n\n"+
		"```sh {#evil show=false collapse}\necho evil\n```\n\n"+
		"<!-- mdsh:if false -->\n## Off\n\n```sh {#evil2}\necho evil2\n```\n\n```text\nnot code\n```\n<!-- mdsh:end -->\n\n"+
		"```sh {#term tty}\nbash\n```\n\n```tape {for=term show=false}\nType \"echo typed\" Enter\n```\n\n"+
		"```text {show=false}\nnot code either\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	got := map[string]Cell{}
	var tape *Cell
	for _, c := range n.Cells() {
		if c.ID != "" {
			got[c.ID] = c
		}
		if strings.Contains(c.HTML, "not code") || strings.Contains(c.HTML, "Off") {
			t.Errorf("drawn: %q", c.HTML)
		}
		if strings.Contains(c.HTML, "echo typed") {
			tape = &c
		}
	}
	if c := got["hello"]; c.Hidden != "" {
		t.Errorf("hello hidden: %q", c.Hidden)
	}
	if c, ok := got["evil"]; !ok || !strings.Contains(c.Hidden, "show=") || c.Collapsed || c.Body != "echo evil\n" {
		t.Errorf("evil: %+v", c)
	}
	if c, ok := got["evil2"]; !ok || !strings.Contains(c.Hidden, "mdsh:if") {
		t.Errorf("evil2: %+v", c)
	}
	if tape == nil || !strings.Contains(tape.Hidden, "types into") {
		t.Errorf("tape: %+v", tape)
	}
	// What is drawn is what runs.
	if err := n.RunRange(0, 4); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"hello", "evil", "evil2"} {
		if !got[id].Ran && !ranCell(n.Cells(), id) {
			t.Errorf("%s didn't run", id)
		}
	}
}

// A section the file folds opens unfolded when it holds a hidden block.
func TestHiddenBlockUnfoldsSection(t *testing.T) {
	file := write(t, "# Appendix\n<!-- mdsh:collapse -->\n\n```sh {#evil show=false}\necho evil\n```\n\n"+
		"# Notes\n<!-- mdsh:collapse -->\n\n```sh {#ok}\necho ok\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var folded []string
	for _, c := range n.Cells() {
		if c.Heading > 0 && c.Collapsed {
			folded = append(folded, strings.TrimSpace(c.HTML))
		}
	}
	if len(folded) != 1 || !strings.Contains(folded[0], "Notes") {
		t.Errorf("folded headings: %q, want only Notes", folded)
	}
}

func ranCell(cells []Cell, id string) bool {
	for _, c := range cells {
		if c.ID == id {
			return c.Ran
		}
	}
	return false
}
