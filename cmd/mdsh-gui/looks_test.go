package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/looks"
)

// lookCSS is a look's stylesheet.
func lookCSS(l looks.Look) string { return filepath.Join("frontend/looks", l.Dir, "look.css") }

// stylesheets are the pages' stylesheets: style.css and every look's.
func stylesheets() []string {
	files := []string{"frontend/style.css"}
	for _, l := range allLooks[1:] {
		files = append(files, lookCSS(l))
	}
	return files
}

// The files made from the looks' front matter must be made again when one
// changes: go generate does it.
func TestLooksGenerated(t *testing.T) {
	ts, err := looks.TS(allLooks)
	if err != nil {
		t.Fatal(err)
	}
	site, err := looks.SiteData(allLooks)
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	want, err := looks.Readme(readme, allLooks)
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string][]byte{
		"frontend/src/looks.gen.ts":  ts,
		"../../site/data/looks.toml": site,
		"README.md":                  want,
	} {
		got, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date with frontend/looks; run go generate", file)
		}
	}
	bundle, err := os.ReadFile("frontend/bundle/looks.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range allLooks[1:] {
		if !bytes.Contains(bundle, []byte("/* frontend/looks/"+l.Dir+"/look.css */")) {
			t.Errorf("frontend/bundle/looks.css has no %s; run go generate", l.Name)
		}
	}
}

var dataLook = regexp.MustCompile(`\[data-look="?(\w+)"?\]`)

// A look paints only itself: every rule in its look.css selects its own
// data-look, so it can't leak into the other looks. @font-face and
// @keyframes are the exceptions, since they select nothing; a look's
// keyframes should still be named for it.
func TestLookCSSScoped(t *testing.T) {
	for _, l := range allLooks[1:] {
		src, err := os.ReadFile(lookCSS(l))
		if err != nil {
			t.Fatal(err)
		}
		css := cssComment.ReplaceAllString(string(src), "")
		css = regexp.MustCompile(`(?s)@(font-face|keyframes[^{]*)\{(?:[^{}]*\{[^{}]*\})*[^{}]*\}`).ReplaceAllString(css, "")
		css = regexp.MustCompile(`@media[^{]*\{`).ReplaceAllString(css, "")
		for _, m := range cssRule.FindAllStringSubmatch(css, -1) {
			for _, sel := range splitSelectors(m[1]) {
				sel = strings.Join(strings.Fields(sel), " ")
				if sel == "" {
					continue
				}
				got := dataLook.FindStringSubmatch(sel)
				if got == nil || got[1] != l.ID {
					t.Errorf("%s: %q doesn't select [data-look=%q]", lookCSS(l), sel, l.ID)
				}
			}
		}
	}
}

var textScale = regexp.MustCompile(`--text-scale:\s*([\d.]+)`)

// The Text size slider shows a look's own size from its front matter, which
// must be the size its CSS sets.
func TestLookTextSize(t *testing.T) {
	for _, l := range allLooks[1:] {
		src, err := os.ReadFile(lookCSS(l))
		if err != nil {
			t.Fatal(err)
		}
		want := l.TextSize
		if want == 0 {
			want = 100
		}
		for _, m := range textScale.FindAllStringSubmatch(string(src), -1) {
			f, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%.0f", f*100); got != strconv.Itoa(want) {
				t.Errorf("%s sets --text-scale %s, but its front matter's textSize is %d", l.Name, m[1], want)
			}
		}
	}
}

// The pictures a look's front matter names must be in the site's images.
func TestLookPictures(t *testing.T) {
	for _, l := range allLooks {
		var files []string
		for _, s := range l.Screenshots {
			files = append(files, "../../site/static/images/gui/"+s.File)
		}
		if h := l.Hero; h != nil {
			files = append(files, "../../site/static/images/app/"+h.Light)
			if h.Dark != "" {
				files = append(files, "../../site/static/images/app/"+h.Dark)
			}
		}
		for _, f := range files {
			if _, err := os.Stat(f); err != nil {
				t.Errorf("%s: %v", l.Name, err)
			}
		}
	}
}

// splitSelectors splits a selector list at its commas, but not those
// inside :is(), :where() or :not().
func splitSelectors(list string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range list {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, list[start:i])
				start = i + 1
			}
		}
	}
	return append(out, list[start:])
}

// go run ./internal/newlook leaves TODOs in a new look's files; a look
// isn't done while any are left.
func TestLooksFinished(t *testing.T) {
	for _, l := range allLooks {
		b, err := os.ReadFile(lookCSS(l))
		if err != nil {
			t.Fatal(err)
		}
		if n := bytes.Count(b, []byte("TODO")); n > 0 {
			t.Errorf("%s has %d TODOs left", lookCSS(l), n)
		}
	}
}
