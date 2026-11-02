package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDir(t *testing.T) {
	home := "/home/j"
	for _, tc := range []struct{ folder, want string }{
		{"", "/home/j/.mdsh/sessions"},
		{"  ", "/home/j/.mdsh/sessions"},
		{"~", "/home/j"},
		{"~/notes/sessions", "/home/j/notes/sessions"},
		{"/var/sessions", "/var/sessions"},
	} {
		if got := Dir(tc.folder, home); got != filepath.FromSlash(tc.want) {
			t.Errorf("Dir(%q) = %q; want %q", tc.folder, got, tc.want)
		}
	}
}

func TestToday(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions") // made as needed
	day := time.Date(2026, 11, 2, 9, 30, 0, 0, time.Local)

	file, made, err := Today(dir, "", day)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "2026-11-02.md"); file != want || !made {
		t.Fatalf("Today = %q, %v; want %q, true", file, made, want)
	}
	if src, _ := os.ReadFile(file); string(src) != "# 2026-11-02\n" {
		t.Errorf("new session holds %q", src)
	}

	// Later the same day it is the same file, as it was left.
	if err := os.WriteFile(file, []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, made, err := Today(dir, "", day.Add(8*time.Hour))
	if err != nil || again != file || made {
		t.Errorf("Today again = %q, %v, %v; want %q, false", again, made, err, file)
	}
	if src, _ := os.ReadFile(file); string(src) != "# Notes\n" {
		t.Errorf("Today changed the day's session: %q", src)
	}

	// A template is copied as it is.
	tmpl := filepath.Join(t.TempDir(), "session.md")
	if err := os.WriteFile(tmpl, []byte("# Session\n\n```sh\nuptime\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	next, made, err := Today(dir, tmpl, day.AddDate(0, 0, 1))
	if err != nil || !made || filepath.Base(next) != "2026-11-03.md" {
		t.Fatalf("Today from a template = %q, %v, %v", next, made, err)
	}
	if src, _ := os.ReadFile(next); string(src) != "# Session\n\n```sh\nuptime\n```\n" {
		t.Errorf("from a template: %q", src)
	}
	if _, _, err := Today(dir, filepath.Join(dir, "missing.md"), day.AddDate(0, 0, 2)); err == nil {
		t.Error("Today from a missing template made a file")
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-11-04.md")); err == nil {
		t.Error("a missing template left a file behind")
	}
}

func TestLast(t *testing.T) {
	if got, err := Last(filepath.Join(t.TempDir(), "missing")); got != "" || err != nil {
		t.Errorf("Last of a missing folder = %q, %v", got, err)
	}
	dir := t.TempDir()
	if got, err := Last(dir); got != "" || err != nil {
		t.Errorf("Last of an empty folder = %q, %v", got, err)
	}
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	write := func(name string, at time.Time) string {
		t.Helper()
		f := filepath.Join(dir, name)
		if err := os.WriteFile(f, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(f, at, at); err != nil {
			t.Fatal(err)
		}
		return f
	}
	write("2026-10-30.md", base.Add(-time.Hour))
	edited := write("2026-10-31.md", base.Add(time.Hour)) // written to after the 1st
	write("2026-11-01.md", base)
	write("notes.txt", base.Add(2*time.Hour))
	if err := os.Mkdir(filepath.Join(dir, "old.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := Last(dir); got != edited || err != nil {
		t.Errorf("Last = %q, %v; want %q", got, err, edited)
	}
	// Changed at the same time, the later date wins.
	tie := write("2026-11-02.md", base.Add(time.Hour))
	if got, _ := Last(dir); got != tie {
		t.Errorf("Last of a tie = %q; want %q", got, tie)
	}
}
