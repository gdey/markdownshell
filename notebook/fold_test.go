package notebook

import (
	"strings"
	"testing"
)

func TestSetFolds(t *testing.T) {
	src := "# A\n\n```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
		"Setext\n------\ntext\n\n" +
		"## C\n<!-- mdsh:collapse {{$X}}==yes -->\n\n" +
		"```sh {#b collapse={{$?}}!=0}\necho b\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	heading := func(name string) int {
		for _, c := range n.Cells() {
			if c.Heading > 0 && strings.Contains(c.HTML, ">"+name+"<") {
				return c.From
			}
		}
		t.Fatalf("no heading %s", name)
		return 0
	}
	set := func(fs ...Fold) {
		t.Helper()
		if err := n.SetFolds(fs); err != nil {
			t.Fatal(err)
		}
	}
	set(Fold{Block: 0, On: true}, Fold{Block: 0, Output: true, On: true},
		Fold{Block: -1, Heading: heading("A"), On: true},
		Fold{Block: -1, Heading: heading("Setext"), On: true},
		// Conditions are left alone.
		Fold{Block: -1, Heading: heading("C"), On: true},
		Fold{Block: 1, On: false})
	want := "# A\n<!-- mdsh:collapse -->\n\n```sh {#a collapse collapse-output}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
		"Setext\n------\n<!-- mdsh:collapse -->\ntext\n\n" +
		"## C\n<!-- mdsh:collapse {{$X}}==yes -->\n\n" +
		"```sh {#b collapse={{$?}}!=0}\necho b\n```\n"
	if got := n.Text(); got != want {
		t.Fatalf("folded:\n%s\nwant:\n%s", got, want)
	}
	for _, c := range n.Cells() {
		if c.Heading > 0 && !strings.Contains(c.HTML, ">C<") && !c.Collapsed {
			t.Errorf("heading %s isn't collapsed", c.HTML)
		}
		if c.Block == 0 && (!c.Collapsed || !c.CollapsedOutput) {
			t.Errorf("block a: collapsed %v, output %v", c.Collapsed, c.CollapsedOutput)
		}
	}
	// Folding again changes nothing.
	set(Fold{Block: 0, On: true}, Fold{Block: -1, Heading: heading("A"), On: true})
	if n.Text() != want {
		t.Errorf("folding twice: %s", n.Text())
	}
	set(Fold{Block: 0, On: false}, Fold{Block: 0, Output: true, On: false},
		Fold{Block: -1, Heading: heading("A"), On: false},
		Fold{Block: -1, Heading: heading("Setext"), On: false})
	if got := n.Text(); got != src {
		t.Errorf("unfolded:\n%s\nwant:\n%s", got, src)
	}
	// Undo takes an unfold back.
	if what, err := n.Undo(); err != nil || what != "unfold" || n.Text() != want {
		t.Errorf("undo: %q, %v:\n%s", what, err, n.Text())
	}
	if err := n.SetFolds([]Fold{{Block: -1, Heading: 1, On: true}}); err == nil {
		t.Error("a fold at no heading should fail")
	}
}

func TestSetFoldsAtEnd(t *testing.T) {
	n, err := Open(write(t, "text\n\n# End"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	cells := n.Cells()
	if err := n.SetFolds([]Fold{{Block: -1, Heading: cells[len(cells)-1].From, On: true}}); err != nil {
		t.Fatal(err)
	}
	if got := n.Text(); got != "text\n\n# End\n<!-- mdsh:collapse -->\n" {
		t.Errorf("got %q", got)
	}
}

// TestFoldLongOutput: an output longer than fold_output opens folded
// with no attribute, unfolding it writes collapse-output=false, and
// folding it again takes that out.
func TestFoldLongOutput(t *testing.T) {
	src := "---\nmdsh:\n  fold_output: 2\n---\n\n" +
		"```sh {#long}\nseq 3\n```\n\n```output {for=long}\n1\n2\n3\n```\n\n" +
		"```sh {#short}\nseq 2\n```\n\n```output {for=short}\n1\n2\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	folded := func() map[string]bool {
		m := map[string]bool{}
		for _, c := range n.Cells() {
			if c.Block >= 0 {
				m[c.ID] = c.CollapsedOutput
			}
		}
		return m
	}
	if got := folded(); !got["long"] || got["short"] {
		t.Fatalf("folded = %v, want only long", got)
	}
	if err := n.SetFolds([]Fold{{Block: 0, Output: true, On: false}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Text(), "```sh {#long collapse-output=false}\n") {
		t.Fatalf("unfolded:\n%s", n.Text())
	}
	if folded()["long"] {
		t.Error("long is still folded")
	}
	if err := n.SetFolds([]Fold{{Block: 0, Output: true, On: true}}); err != nil {
		t.Fatal(err)
	}
	if n.Text() != src {
		t.Fatalf("folded again:\n%s", n.Text())
	}
	if !folded()["long"] {
		t.Error("long isn't folded again")
	}
}
