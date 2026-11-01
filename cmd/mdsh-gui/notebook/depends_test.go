package notebook

import (
	"os"
	"strings"
	"testing"
)

func TestSetDepends(t *testing.T) {
	file := write(t, "```sh {#build}\necho built\n```\n\n```sh {#login confirm}\necho in\n```\n\n```sh\necho deploy\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	// Adding names the unnamed block first, so depends= can name blocks.
	if err := n.SetDepends(2, []int{0}); err != nil {
		t.Fatal(err)
	}
	text := string(n.d.Src)
	if !strings.Contains(text, " depends=build}\necho deploy") || !strings.Contains(text, "```sh {#block-") {
		t.Errorf("after add:\n%s", text)
	}
	if err := n.SetDepends(2, []int{0, 1}); err != nil {
		t.Fatal(err)
	}
	if got, want := n.Cells()[2].Depends, []string{"build", "login"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("depends = %q, want %q", got, want)
	}
	// Retargeting keeps the place; other attributes stay as they are.
	if err := n.SetDepends(1, []int{0}); err != nil {
		t.Fatal(err)
	}
	if text = string(n.d.Src); !strings.Contains(text, "{#login confirm depends=build}") {
		t.Errorf("after add to login:\n%s", text)
	}
	// A cycle is refused and changes nothing.
	before := string(n.d.Src)
	if err := n.SetDepends(0, []int{2}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cycle: err = %v", err)
	}
	if string(n.d.Src) != before {
		t.Errorf("a refused change edited the file:\n%s", n.d.Src)
	}
	if err := n.SetDepends(2, []int{2}); err == nil {
		t.Error("a block depending on itself was allowed")
	}
	// Removing the last one takes the attribute out, and Undo brings it back.
	if err := n.SetDepends(1, nil); err != nil {
		t.Fatal(err)
	}
	if text = string(n.d.Src); !strings.Contains(text, "{#login confirm}") {
		t.Errorf("after remove:\n%s", text)
	}
	if what, err := n.Undo(); err != nil || what != "remove dependency" {
		t.Errorf("undo = %q, %v", what, err)
	}
	if text = string(n.d.Src); !strings.Contains(text, "{#login confirm depends=build}") {
		t.Errorf("after undo:\n%s", text)
	}
	if err := n.Save(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(file)
	if !strings.Contains(string(saved), "depends=build,login") {
		t.Errorf("saved:\n%s", saved)
	}
}
