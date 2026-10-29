package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestList(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"README.md":                  "# Hi\n",
		"runbooks/deploy.md":         "```sh\necho one\n```\n\n```sh\necho two\n```\n\n```go\nnot run\n```\n",
		"runbooks/notes.markdown":    "```zsh\necho three\n```\n",
		"runbooks/notes.txt":         "not markdown",
		".git/HEAD.md":               "hidden",
		"node_modules/pkg/README.md": "skipped",
		"deploy.assets/out.md":       "mdsh's own",
		"a/b/c/d/e/f/g/too-deep.md":  "too deep",
		"a/b/c/d/e/f/deep-enough.md": "deep enough",
	}
	for name, src := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, full, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if full {
		t.Errorf("List says it stopped early")
	}
	var names []string
	for _, f := range got {
		names = append(names, f.Rel)
		if f.Path != filepath.Join(root, filepath.FromSlash(f.Rel)) {
			t.Errorf("Path %s doesn't match Rel %s", f.Path, f.Rel)
		}
	}
	if got, want := strings.Join(names, " "), "README.md a/b/c/d/e/f/deep-enough.md runbooks/deploy.md runbooks/notes.markdown"; got != want {
		t.Errorf("files = %s\nwant    %s", got, want)
	}
	blocks := map[string]int{}
	for _, f := range got {
		blocks[f.Rel] = f.Blocks
	}
	if blocks["runbooks/deploy.md"] != 2 || blocks["README.md"] != 0 || blocks["runbooks/notes.markdown"] != 1 {
		t.Errorf("blocks = %v", blocks)
	}
}
