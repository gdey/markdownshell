package ui

import (
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
