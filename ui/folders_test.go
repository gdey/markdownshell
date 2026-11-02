package ui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gdey/markdownshell/collection"
	"github.com/gdey/markdownshell/library"
)

func TestFolderSettings(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.md", "ops/b.md", "ops/rocket.svg"} {
		p := filepath.Join(dir, filepath.FromSlash(n))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("x"), 0o644)
	}
	l, err := library.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	folders, writable := startFolders(dir, l)
	if !slices.Equal(writable, []string{"", "ops"}) {
		t.Fatalf("writable = %q", writable)
	}
	a := &App{startFolders: folders}
	if err := a.SetFolderSettings("ops", collection.Settings{Title: "Ops", Icon: "rocket.svg"}); err != nil {
		t.Fatal(err)
	}
	f, err := a.FolderSettings("ops")
	if err != nil {
		t.Fatal(err)
	}
	if f.Settings.Title != "Ops" || f.Settings.Icon != "rocket.svg" || !slices.Equal(f.Images, []string{"rocket.svg"}) {
		t.Errorf("FolderSettings = %+v", f)
	}
	for _, rel := range []string{"..", "../x", "/etc", "nope"} {
		if err := a.SetFolderSettings(rel, collection.Settings{Title: "x"}); err == nil {
			t.Errorf("SetFolderSettings(%q) worked", rel)
		}
	}
}
