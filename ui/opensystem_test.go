package ui

import (
	"path/filepath"
	"slices"
	"testing"
)

// Finder's Open With can arrive before the app starts; the file must
// wait for it, not be dropped.
func TestOpenQueue(t *testing.T) {
	var q openQueue
	if q.add("a.md") || q.add("b.md") {
		t.Fatal("add before start said open now")
	}
	if got := q.start(); !slices.Equal(got, []string{"a.md", "b.md"}) {
		t.Fatalf("start = %v, want the held files", got)
	}
	if !q.add("c.md") {
		t.Fatal("add after start said wait")
	}
	if got := q.start(); len(got) != 0 {
		t.Fatalf("start again = %v, want nothing held", got)
	}
}

// macOS sends the file or folder named on the command line as if Finder
// asked for it. Before start that's dropped, as the app opened it
// already; a folder opened again showed an error sheet that held back
// the first window. After start, asking again brings it forward.
func TestOpenQueueArgs(t *testing.T) {
	dir := t.TempDir()
	var q openQueue
	q.arg(dir)
	q.arg("notes.md")
	abs, err := filepath.Abs("notes.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{dir, abs, "notes.md", "other.md"} {
		if q.add(f) {
			t.Fatalf("add(%q) before start said open now", f)
		}
	}
	if got := q.start(); !slices.Equal(got, []string{"other.md"}) {
		t.Fatalf("start = %v, want only other.md held", got)
	}
	if !q.add(abs) {
		t.Error("add of a command line file after start said wait")
	}
}
