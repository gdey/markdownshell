package notebook

import (
	"bytes"
	"log/slog"
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

// Names depends= ignores stay when the dependencies are edited, and a
// textbox can't be made one.
func TestSetDependsKeepsUnknown(t *testing.T) {
	file := write(t, "```sh {#a}\n```\n\n```textbox {#name}\n```\n\n```sh {#b depends=\"nope, a\"}\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.SetDepends(2, nil); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "{#b depends=nope}") {
		t.Errorf("after remove:\n%s", text)
	}
	if err := n.SetDepends(2, []int{0}); err != nil {
		t.Fatal(err)
	}
	if text := string(n.d.Src); !strings.Contains(text, "{#b depends=a,nope}") {
		t.Errorf("after add:\n%s", text)
	}
	if err := n.SetDepends(2, []int{1}); err == nil || !strings.Contains(err.Error(), "never runs") {
		t.Errorf("textbox: err = %v", err)
	}
}

// A depends= naming no block is logged once, for the console, and again
// only if it comes back after going.
func TestDependsWarningLogged(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)
	file := write(t, "```sh {#a depends=nope}\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if got := strings.Count(buf.String(), `depends= names unknown block \"nope\"`); got != 1 {
		t.Fatalf("logged %d times after open:\n%s", got, buf.String())
	}
	// It says where, for the console to take the source pane there.
	if want := "file=" + file + " line=1"; !strings.Contains(buf.String(), want) {
		t.Errorf("no %q in:\n%s", want, buf.String())
	}
	// An edit that only moves it down a line doesn't log it again.
	if _, err := n.SetText(string(n.d.Src), "# T\n\n"+string(n.d.Src)); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(buf.String(), "unknown block"); got != 1 {
		t.Errorf("logged %d times after an edit:\n%s", got, buf.String())
	}
}
