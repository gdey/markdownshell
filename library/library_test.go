package library

import (
	"os"
	"path/filepath"
	"slices"
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
	l, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	got := l.Files
	if l.Full {
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

func TestDescription(t *testing.T) {
	long := strings.Repeat("x", MaxDesc+10)
	for _, c := range []struct{ src, want string }{
		{"# No front matter\n", ""},
		{"---\ntitle: T\n---\n# Hi\n", ""},
		{"---\ndescription: Deploy the site\n---\n", "Deploy the site"},
		{"---\nsummary: From summary\ndescription: From description\n---\n", "From description"},
		{"---\ndescription: \"\"\nsummary: From summary\n---\n", "From summary"},
		{"---\nabstract: |\n  Two\n  lines\n...\n", "Two lines"},
		{"---\ndescription: [not, text]\n---\n", ""},
		{"---\n---\n", ""},
		{"---\ndescription: " + long + "\n---\n", long[:MaxDesc-1] + "…"},
	} {
		path := filepath.Join(t.TempDir(), "f.md")
		if err := os.WriteFile(path, []byte(c.src), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, got, _ := read(path, int64(len(c.src))); got != c.want {
			t.Errorf("%q: description = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestListCollections(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"top.md":                       "# Top\n",
		"deploys/.mdsh.yaml":           "title: Deploys\ncolor: \"#3b82f6\"\nicon: 🚀\nbackground: banner.png\ntags: [ops, prod]\n",
		"deploys/banner.png":           "png",
		"deploys/release.md":           "---\ntags: [urgent, ops]\n---\n",
		"deploys/canary/canary.md":     "# Canary\n",
		"deploys/canary/db/.mdsh.yaml": "tags: db\n",
		"deploys/canary/db/x.md":       "",
		"broken/.mdsh.yaml":            "- not a mapping\n",
		"broken/b.md":                  "",
		".hidden/.mdsh.yaml":           "title: Hidden\n",
	}
	for name, src := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	l, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, c := range l.Collections {
		rels = append(rels, c.Rel)
	}
	if got := strings.Join(rels, " "); got != "broken deploys deploys/canary/db" {
		t.Fatalf("collections = %s", got)
	}
	if c := l.Collections[0]; c.Error == "" || c.Title != "" {
		t.Errorf("broken: %+v", c)
	}
	d := l.Collections[1]
	if d.Title != "Deploys" || d.Icon != "🚀" || d.Color != "#3b82f6" || filepath.Base(d.Background) != "banner.png" || !slices.Equal(d.Tags, []string{"ops", "prod"}) {
		t.Errorf("deploys: %+v", d)
	}
	tags := map[string]string{}
	for _, f := range l.Files {
		tags[f.Rel] = strings.Join(f.Tags, " ")
	}
	want := map[string]string{
		"top.md":                   "",
		"broken/b.md":              "",
		"deploys/release.md":       "ops prod urgent",
		"deploys/canary/canary.md": "ops prod",
		"deploys/canary/db/x.md":   "ops prod db",
	}
	for rel, w := range want {
		if tags[rel] != w {
			t.Errorf("%s: tags = %q, want %q", rel, tags[rel], w)
		}
	}
}
