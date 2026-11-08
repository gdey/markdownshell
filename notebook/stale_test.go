package notebook

import (
	"fmt"
	"slices"
	"testing"

	"github.com/gdey/markdownshell/run"
)

// TestClearOutputStale checks that clearing a block's output makes the
// blocks that depend on it stale, and that running a block after asks
// about the cleared one and the stale ones, but not the rest.
func TestClearOutputStale(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\n```sh {#b}\necho b\n```\n\n```sh {#c depends=a}\necho c\n```\n\n```sh {#d depends=a,b,c}\necho d\n```\n\n```sh {#e depends=a,d}\necho e\n```\n"
	var asked []string
	n, err := Open(write(t, src), Options{Depend: func(b, of int, _ string) run.Answer {
		asked = append(asked, fmt.Sprint(b, "<", of))
		return run.Go
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	for i := range 5 {
		if err := n.RunBlock(i); err != nil {
			t.Fatal(err)
		}
	}
	asked = nil
	if err := n.ClearOutput(0); err != nil {
		t.Fatal(err)
	}
	if got, want := staleOf(n), []bool{false, false, true, true, true}; !slices.Equal(got, want) {
		t.Errorf("stale after clearing a: %v, want %v", got, want)
	}
	if err := n.RunBlock(4); err != nil {
		t.Fatal(err)
	}
	if want := []string{"0<4", "2<3", "3<4"}; !slices.Equal(asked, want) {
		t.Errorf("asked %q, want %q", asked, want)
	}
	if got := staleOf(n); slices.Contains(got, true) {
		t.Errorf("stale after running e: %v", got)
	}
}

// staleOf is each block card's Stale, by block.
func staleOf(n *Notebook) []bool {
	var stale []bool
	for _, c := range n.Cells() {
		if c.Block >= 0 {
			stale = append(stale, c.Stale)
		}
	}
	return stale
}
