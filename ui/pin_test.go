package ui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gdey/markdownshell/settings"
)

// Pins persist in the settings file, and the start screen lists the
// ones that still exist.
func TestPin(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	for _, f := range []string{a, b} {
		os.WriteFile(f, []byte("# x\n"), 0o644)
	}
	path := filepath.Join(t.TempDir(), "gui.json")
	st, err := settings.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{dir: dir, settings: st}
	if err := app.Pin("a.md", true); err == nil {
		t.Error("Pin took a relative path")
	}
	for _, f := range []string{b, a, b} {
		if err := app.Pin(f, true); err != nil {
			t.Fatal(err)
		}
	}
	os.Remove(a)
	start, err := app.StartScreen()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(start.Pinned, []string{b}) {
		t.Errorf("Pinned = %q, want only %s, which still exists", start.Pinned, b)
	}
	again, err := settings.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Get().Pinned; !slices.Equal(got, []string{b, a}) {
		t.Errorf("saved Pinned = %q", got)
	}
	if err := app.Pin(b, false); err != nil {
		t.Fatal(err)
	}
	if got := st.Get().Pinned; !slices.Equal(got, []string{a}) {
		t.Errorf("after unpinning, Pinned = %q", got)
	}
}
