package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

func TestLinkTarget(t *testing.T) {
	dir := filepath.FromSlash("/notes/proj")
	file := filepath.Join(dir, "doc.md")
	for _, tc := range []struct {
		file, href string
		kind       linkKind
		want       string
		err        bool
	}{
		{file, "https://example.com/a?b=c#d", linkWeb, "https://example.com/a?b=c#d", false},
		{file, "HTTP://example.com", linkWeb, "http://example.com", false},
		{file, "mailto:j@example.com", linkWeb, "mailto:j@example.com", false},
		{file, "//example.com/x", linkWeb, "https://example.com/x", false},
		{file, "other.md", linkDoc, filepath.Join(dir, "other.md"), false},
		{file, "other.md#setup", linkDoc, filepath.Join(dir, "other.md"), false},
		{file, "../up/Read%20Me.MARKDOWN", linkDoc, filepath.FromSlash("/notes/up/Read Me.MARKDOWN"), false},
		{file, "/abs/x.md", linkDoc, filepath.FromSlash("/abs/x.md"), false},
		{file, "file:///abs/x.md", linkDoc, filepath.FromSlash("/abs/x.md"), false},
		{file, "img/plot.png", linkImage, filepath.Join(dir, "img", "plot.png"), false},
		{file, "img/Plot.SVG", linkImage, filepath.Join(dir, "img", "Plot.SVG"), false},
		{file, "setup.command", linkFile, filepath.Join(dir, "setup.command"), false},
		{file, "file:///Applications/Utilities/Terminal.app", linkFile, filepath.FromSlash("/Applications/Utilities/Terminal.app"), false},
		{file, "#heading", 0, "", true},
		{file, "javascript:alert(1)", 0, "", true},
		{"", "other.md", 0, "", true},
		{"", "https://example.com", linkWeb, "https://example.com", false},
	} {
		kind, got, err := linkTarget(tc.file, tc.href)
		if (err != nil) != tc.err || kind != tc.kind || got != tc.want {
			t.Errorf("linkTarget(%q, %q) = %v, %q, %v; want %v, %q, error %v", tc.file, tc.href, kind, got, err, tc.kind, tc.want, tc.err)
		}
	}
}

// TestOpenable checks what a link may open with the system's app, when
// its type is listed: a plain file, not a script, folder or bundle
// (finding 6 of the v1.4.0 security review).
func TestOpenable(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, mode os.FileMode) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	app := filepath.Join(dir, "Evil.app")
	os.MkdirAll(app, 0o755)
	for path, want := range map[string]bool{
		write("notes.pdf", 0o644):      true,
		write("run.pdf", 0o755):        false,
		write("setup.command", 0o755):  false,
		app:                            false,
		filepath.Join(dir, "gone.pdf"): false,
	} {
		if got := openable(path); got != want {
			t.Errorf("openable(%s) = %v, want %v", filepath.Base(path), got, want)
		}
	}
}

func TestOpenTypes(t *testing.T) {
	if got := settings.CleanTypes([]string{" .PDF", "txt", "pdf", "", "a b", "../x", "Png"}); !slicesEqual(got, []string{"pdf", "txt", "png"}) {
		t.Errorf("CleanTypes = %q", got)
	}
	s := settings.Settings{OpenTypes: []string{"pdf"}}
	if !s.OpenType("/a/Report.PDF") || s.OpenType("/a/setup.command") || s.OpenType("/a/pdf") {
		t.Error("OpenType")
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
