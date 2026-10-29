package ui

import (
	"path/filepath"
	"testing"
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
		{file, "img/plot.png", linkFile, filepath.Join(dir, "img", "plot.png"), false},
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
