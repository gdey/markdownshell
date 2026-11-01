package notebook

import (
	"strings"
	"testing"
)

func TestDeleteBlock(t *testing.T) {
	src := "# T\n\n" +
		"```sh {#a}\necho a\n```\n\n" +
		"```sh {#b}\necho b\n```\n\n" +
		"```output {for=b}\nold\n```\n\n" +
		"End.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	before := n.Text()
	del, err := n.DeleteBlock(1)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(before, "```sh {#b}\necho b\n```\n\n```output {for=b}\nold\n```\n\n", "", 1); n.Text() != want || want == before || del.Name != "#b" || len(del.Warnings) != 0 {
		t.Errorf("after deleting b (%+v):\n%s", del, n.Text())
	}
	if len(n.results) != 1 || !n.results[0].Ran {
		t.Error("a lost its result")
	}
	if what, err := n.Undo(); err != nil || what != "delete block" || n.Text() != before {
		t.Errorf("undo: %q, %v:\n%s", what, err, n.Text())
	}
	if !n.results[0].Ran {
		t.Error("a lost its result on undo")
	}
	// A block another reads stays.
	n2, err := Open(write(t, "```sh {#a}\necho a\n```\n\n```sh {#b depends=a}\necho b\n```\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n2.Close()
	if _, err := n2.DeleteBlock(0); err == nil || !strings.Contains(err.Error(), "depends=") {
		t.Errorf("deleting a used block: %v", err)
	}
}

func TestDeleteNamesBlocks(t *testing.T) {
	src := "```sh\necho one\n```\n\n" +
		"```sh\necho two\n```\n\n" +
		"It said {{#block-2/?}}.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if _, err := n.DeleteBlock(0); err != nil {
		t.Fatal(err)
	}
	if text := n.Text(); strings.Contains(text, "echo one") || strings.Contains(text, "block-2/") || !strings.Contains(text, "```sh {#block-") {
		t.Errorf("text:\n%s", text)
	}
}

func TestClearOutput(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\n" +
		"```output {for=a}\nold\n```\n\n" +
		"```sh {#b}\necho b\n```\n\nEnd.\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(1); err != nil {
		t.Fatal(err)
	}
	ran := n.Text()
	if !strings.Contains(ran, "```output {for=b") {
		t.Fatalf("b's run isn't in the file:\n%s", ran)
	}
	// Saved output only.
	if err := n.ClearOutput(0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(n.Text(), "old") || !strings.Contains(n.Text(), "```sh {#a}\necho a\n```\n\n```sh {#b}") {
		t.Errorf("after clearing a:\n%s", n.Text())
	}
	// A block that ran this session: saving mustn't write it back.
	if err := n.ClearOutput(1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(n.Text(), "for=b") || n.results[1].Ran {
		t.Errorf("after clearing b (ran %v):\n%s", n.results[1].Ran, n.Text())
	}
	if err := n.fold(); err != nil || strings.Contains(n.Text(), "for=b") {
		t.Errorf("folding the results wrote b's output back (%v):\n%s", err, n.Text())
	}
	// Undo brings b's run back, then a's saved output.
	if what, err := n.Undo(); err != nil || what != "clear output" || !n.results[1].Ran || !strings.Contains(n.Text(), "for=b") {
		t.Errorf("undo b: %q, %v, ran %v", what, err, n.results[1].Ran)
	}
	if _, err := n.Undo(); err != nil || n.Text() != ran {
		t.Errorf("undo a: %v:\n%s", err, n.Text())
	}
	if _, err := n.Redo(); err != nil || strings.Contains(n.Text(), "old") {
		t.Errorf("redo a: %v", err)
	}
}
