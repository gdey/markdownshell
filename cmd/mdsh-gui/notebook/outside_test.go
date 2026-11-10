package notebook

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The files Outside lists reach outside the document's folder once
// Options.Outside allows that list, and not before; one added after
// stays in until it is asked about.
func TestOutside(t *testing.T) {
	top := t.TempDir()
	dir := filepath.Join(top, "doc")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"shared.txt", "secret.txt"} {
		if err := os.WriteFile(filepath.Join(top, f), []byte(strings.TrimSuffix(f, ".txt")+" text\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(dir, "doc.md")
	src := "```text {<include file=../shared.txt}\n```\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var asked [][]string
	allow := false
	n, err := Open(file, Options{Outside: func(paths []string) bool {
		asked = append(asked, paths)
		return allow
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if got := n.Outside(); !slices.Equal(got, []string{"../shared.txt"}) {
		t.Errorf("Outside = %q", got)
	}
	if strings.Contains(n.Text(), "shared text") {
		t.Errorf("read before it was allowed:\n%s", n.Text())
	}
	allow = true
	if err := n.Restart(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Text(), "shared text") {
		t.Errorf("not read once allowed:\n%s", n.Text())
	}
	if len(asked) == 0 || !slices.Equal(asked[len(asked)-1], []string{"../shared.txt"}) {
		t.Errorf("asked %q", asked)
	}
	// An include added since is outside the list allowed.
	if ok, err := n.SetText(n.Text(), n.Text()+"\n```text {<include file=../secret.txt}\n```\n"); !ok || err != nil {
		t.Fatalf("SetText = %v, %v", ok, err)
	}
	if strings.Contains(n.Text(), "secret text") {
		t.Errorf("an include added after allowing was read:\n%s", n.Text())
	}
	if got := n.Outside(); !slices.Equal(got, []string{"../shared.txt", "../secret.txt"}) {
		t.Errorf("Outside after the edit = %q", got)
	}
}
