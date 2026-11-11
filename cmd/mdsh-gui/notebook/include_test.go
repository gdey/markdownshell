package notebook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetIncludeRange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("l1\nl2\nl3\nl4\nl5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "doc.md")
	src := "# A\n\n```go {<include file=a.go from=2 to=4}\nl2\nl3\nl4\n```\n\n```sh {#b}\necho b\n```\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	cells := n.Cells()
	var inc *IncludeCell
	for _, c := range cells {
		if c.Include != nil {
			inc = c.Include
		}
	}
	if inc == nil || inc.Block != 0 || inc.File != "a.go" || inc.From != 2 || inc.To != 4 || inc.Lang != "go" {
		t.Fatalf("include cell = %+v", inc)
	}
	f, err := n.IncludeFile(0)
	if err != nil || f.Lines != 5 || f.Text != "l1\nl2\nl3\nl4\nl5\n" {
		t.Fatalf("IncludeFile = %+v, %v", f, err)
	}
	if _, err := n.IncludeFile(1); err == nil {
		t.Error("IncludeFile(1) on a sh block gave no error")
	}
	info := func() string {
		s := n.Text()
		i := strings.Index(s, "```go")
		return s[i : i+strings.Index(s[i:], "\n")]
	}
	set := func(from, to int, want string) {
		t.Helper()
		if err := n.SetIncludeRange(0, from, to); err != nil {
			t.Fatal(err)
		}
		if got := info(); got != want {
			t.Errorf("after SetIncludeRange(%d, %d): %s, want %s", from, to, got, want)
		}
	}
	set(2, 5, "```go {<include file=a.go from=2}") // to at the end comes out
	set(1, 3, "```go {<include file=a.go to=3}")   // from at the start comes out
	set(3, 3, "```go {<include file=a.go to=3 from=3}")
	set(0, 0, "```go {<include file=a.go}")
	set(2, 9, "```go {<include file=a.go from=2}") // past the end is the end
	if err := n.SetIncludeRange(0, 4, 2); err == nil {
		t.Error("from after to gave no error")
	}
	if err := n.SetIncludeRange(0, 6, 0); err == nil {
		t.Error("from past the end gave no error")
	}
	if err := n.SetIncludeRange(1, 1, 2); err == nil {
		t.Error("a sh block gave no error")
	}
	// The included lines follow the range.
	for _, c := range n.Cells() {
		if c.Include != nil && !strings.Contains(c.HTML, "l2\nl3\nl4\nl5") {
			t.Errorf("cell after from=2: %s", c.HTML)
		}
	}
	if what, err := n.Undo(); err != nil || what != "change include range" {
		t.Fatalf("Undo = %q, %v", what, err)
	}
	if got := info(); got != "```go {<include file=a.go}" {
		t.Errorf("after undo: %s", got)
	}
}
