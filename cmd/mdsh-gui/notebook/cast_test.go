package notebook

import (
	"strings"
	"testing"
)

// Where a recording's player is left is written to its fence, or its tty
// block, as frame=N, and steps one after another undo as one.
func TestSetCastFrame(t *testing.T) {
	cast := `{"version":2,"width":20,"height":2}` + "\n" + `[0.1,"o","a"]` + "\n" + `[1,"o","b"]` + "\n"
	file := write(t, "# Rec\n\n```cast\n"+cast+"```\n\n```sh {#t tty}\necho hi\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.SetCastFrame(-1, cast, 1); err != nil {
		t.Fatal(err)
	}
	if err := n.SetCastFrame(-1, "\n"+cast, 2); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```cast {frame=2}\n") {
		t.Errorf("fence:\n%s", text)
	}
	var html strings.Builder
	for _, c := range n.Cells() {
		html.WriteString(c.HTML)
	}
	if !strings.Contains(html.String(), `#frame=2">`) {
		t.Errorf("the player doesn't open at frame 2: %s", html.String())
	}
	if err := n.SetCastFrame(1, "", 3); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```sh {#t tty frame=3}\n") {
		t.Errorf("tty block:\n%s", text)
	}
	if _, err := n.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Undo(); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```cast\n") {
		t.Errorf("two frame steps didn't undo as one:\n%s", text)
	}
	// 0 is the last frame, which needs no attribute.
	if err := n.SetCastFrame(-1, cast, 2); err != nil {
		t.Fatal(err)
	}
	if err := n.SetCastFrame(-1, cast, 0); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "```cast\n") {
		t.Errorf("frame= kept for the last frame:\n%s", text)
	}
	if err := n.SetCastFrame(-1, "nope", 1); err == nil {
		t.Error("no error for a recording no fence holds")
	}
	if err := n.SetCastFrame(0, "", 1); err == nil {
		t.Error("no error for frame= on a fence that isn't a tty block")
	}
}
