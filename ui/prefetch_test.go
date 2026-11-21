package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdey/markdownshell/settings"
)

// The start screen the app opens with takes the listing made as it
// started, once; later ones, or one long after, list the folder afresh.
func TestPrefetchStart(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st, err := settings.Open(filepath.Join(t.TempDir(), "gui.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{dir: dir, settings: st}
	files := func() int {
		t.Helper()
		s, err := app.StartScreen()
		if err != nil {
			t.Fatal(err)
		}
		return len(s.Files)
	}
	write("a.md")
	app.prefetchStart()
	<-app.prefetched.done
	write("b.md")
	if n := files(); n != 1 {
		t.Errorf("first start screen listed %d files, want the 1 prefetched", n)
	}
	if n := files(); n != 2 {
		t.Errorf("second start screen listed %d files, want 2", n)
	}
	app.prefetchStart()
	<-app.prefetched.done
	app.prefetched.made = time.Now().Add(-prefetchFresh)
	write("c.md")
	if n := files(); n != 3 {
		t.Errorf("start screen after a stale prefetch listed %d files, want 3", n)
	}
}
