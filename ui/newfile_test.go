package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdey/markdownshell/notebook"
)

func TestCreateFile(t *testing.T) {
	a := &App{tabs: map[int]*docTab{}}
	dir := t.TempDir()
	for _, tc := range []struct{ name, file, want string }{
		{"notes.md", "notes.md", "# notes\n"},
		{"plan", "plan.md", "# plan\n"},
		{"Run book.markdown", "Run book.markdown", "# Run book\n"},
	} {
		got, err := a.createFile(filepath.Join(dir, tc.name), "")
		if err != nil {
			t.Fatalf("createFile(%q): %v", tc.name, err)
		}
		if want := filepath.Join(dir, tc.file); got != want {
			t.Errorf("createFile(%q) = %q; want %q", tc.name, got, want)
		}
		src, err := os.ReadFile(got)
		if err != nil {
			t.Fatal(err)
		}
		if string(src) != tc.want {
			t.Errorf("createFile(%q) wrote %q; want %q", tc.name, src, tc.want)
		}
	}

	// A template is copied as it is.
	tmpl := filepath.Join(t.TempDir(), "runbook.md")
	if err := os.WriteFile(tmpl, []byte("# Runbook\n\n```sh {#check}\nuptime\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := a.createFile(filepath.Join(dir, "deploy.md"), tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if src, _ := os.ReadFile(got); string(src) != "# Runbook\n\n```sh {#check}\nuptime\n```\n" {
		t.Errorf("from a template: %q", src)
	}
	if _, err := a.createFile(filepath.Join(dir, "x.md"), filepath.Join(dir, "missing.md")); err == nil {
		t.Error("createFile from a missing template made a file")
	}

	// A file open in a tab isn't replaced.
	open := filepath.Join(dir, "notes.md")
	n, err := notebook.Open(open, notebook.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	a.tabs[1] = &docTab{app: a, id: 1, n: n}
	if _, err := a.createFile(open, ""); err == nil {
		t.Error("createFile replaced a file open in a tab")
	}
}
