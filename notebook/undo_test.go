package notebook

import (
	"errors"
	"strings"
	"testing"
)

func TestUndo(t *testing.T) {
	src := "- [ ] one\n\n" +
		"```textbox {#host}\n```\n\n" +
		"```choice {#env mode=radio}\n- [ ] staging\n- [x] prod\n```\n\n" +
		"```sh {#a out=markdown}\necho '- [ ] output'\n```\n"
	n, err := Open(write(t, src), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	undo := func(want string) {
		t.Helper()
		if what, err := n.Undo(); err != nil || what != want {
			t.Errorf("Undo = %q, %v; want %q", what, err, want)
		}
	}
	redo := func(want string) {
		t.Helper()
		if what, err := n.Redo(); err != nil || what != want {
			t.Errorf("Redo = %q, %v; want %q", what, err, want)
		}
	}
	has := func(s string, want bool) {
		t.Helper()
		if strings.Contains(n.Text(), s) != want {
			t.Errorf("source has %q = %v, want %v:\n%s", s, !want, want, n.Text())
		}
	}

	undo("")
	if err := n.SetTask(0, true); err != nil {
		t.Fatal(err)
	}
	// Typing in a textbox is one edit, however often it saves.
	for _, s := range []string{"d", "db", "db1"} {
		if err := n.SetTextbox(0, s); err != nil {
			t.Fatal(err)
		}
	}
	if err := n.SetChoice(1, []bool{true, false}); err != nil {
		t.Fatal(err)
	}
	has("- [x] staging", true)
	undo("pick choice")
	has("- [x] prod", true)
	undo("edit textbox")
	has("db", false)
	redo("edit textbox")
	has("db1\n", true)
	undo("edit textbox")
	undo("check task")
	has("- [ ] one", true)
	undo("")
	if n.Text() != src {
		t.Errorf("after undoing everything:\n%s", n.Text())
	}
	redo("check task")
	has("- [x] one", true)

	// A new edit drops what could be redone.
	if err := n.SetTask(0, false); err != nil {
		t.Fatal(err)
	}
	redo("")

	// A task in output that ran this session changes the run's output
	// too, which saving writes, and undo puts that back as well.
	if err := n.RunBlock(2); err != nil {
		t.Fatal(err)
	}
	if err := n.SetTask(1, true); err != nil {
		t.Fatal(err)
	}
	if out := string(n.results[2].Stdout); out != "- [x] output\n" {
		t.Errorf("stdout = %q", out)
	}
	undo("check task")
	if out := string(n.results[2].Stdout); out != "- [ ] output\n" {
		t.Errorf("stdout after undo = %q", out)
	}

	// An edit the rendered view didn't make, such as typing in the
	// source view, can't be undone from it, so neither can those before.
	redo("check task")
	if _, err := n.SetText(n.Text(), n.Text()+"\nMore.\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Undo(); !errors.Is(err, ErrStale) {
		t.Errorf("Undo after a source edit: %v", err)
	}
	undo("")
}
