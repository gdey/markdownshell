package main

import (
	"io/fs"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"

	markdownshell "github.com/gdey/markdownshell"
)

func TestChangelogHTML(t *testing.T) {
	md := "# Changelog\n\nIntro.\n\n## [Unreleased]\n\n## [0.2.0] - 2026-10-01\n\n### Added\n\n- A thing.\n\n  ![shot](docs/changelog/shot.png)\n\n<script>alert(1)</script>\n\n[0.2.0]: https://github.com/gdey/markdownshell/releases/tag/v0.2.0\n"
	got, err := changelogHTML([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<h1 id="changelog">Changelog</h1>`, `<h2 id="020---2026-10-01"><a href="https://github.com/gdey/markdownshell/releases/tag/v0.2.0">0.2.0</a> - 2026-10-01</h2>`, `<img src="/changelog/shot.png" alt="shot">`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	for _, bad := range []string{"Unreleased", "<script>", "docs/changelog"} {
		if strings.Contains(got, bad) {
			t.Errorf("has %s:\n%s", bad, got)
		}
	}
}

// The changelog built in renders, and every screenshot it shows is
// built in too, where its link points.
func TestChangelogImages(t *testing.T) {
	html, err := renderChangelog()
	if err != nil {
		t.Fatal(err)
	}
	imgs := regexp.MustCompile(`<img src="([^"]*)"`).FindAllStringSubmatch(html, -1)
	if len(imgs) == 0 {
		t.Fatal("the changelog shows no screenshots")
	}
	for _, m := range imgs {
		name, ok := strings.CutPrefix(m[1], changelogImages)
		if !ok {
			t.Errorf("%s isn't a built-in screenshot", m[1])
			continue
		}
		rec := httptest.NewRecorder()
		serveChangelogImage(rec, httptest.NewRequest("GET", m[1], nil), name)
		if want := imageTypes[path.Ext(name)]; rec.Code != 200 || want == "" || rec.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %s", m[1], rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	shots, _ := fs.Glob(markdownshell.ChangelogImages, "docs/changelog/*.png")
	if len(shots) == 0 {
		t.Error("no screenshots built in")
	}
	for _, name := range []string{"", "../CHANGELOG.md", "x/y.png", "missing.png", "missing.gif", ".png", "CHANGELOG.md"} {
		rec := httptest.NewRecorder()
		serveChangelogImage(rec, httptest.NewRequest("GET", "/changelog/x", nil), name)
		if rec.Code != 404 {
			t.Errorf("%q: %d, want 404", name, rec.Code)
		}
	}
}

func TestChangelogTab(t *testing.T) {
	tab := &docTab{id: 3, page: changelogPage}
	if got := tab.info(); got.Title != "Changelog" || got.URL != "mdsh://doc/CHANGELOG.md" || got.Page != changelogPage || got.File != "" || !got.ReadOnly {
		t.Errorf("info = %+v", got)
	}
	if got := (&docTab{id: 4}).info(); got.Page != "" || got.Title != "Start" {
		t.Errorf("start screen info = %+v", got)
	}
}
