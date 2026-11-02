package notebook

import (
	"encoding/base64"
	"strings"
	"testing"
)

func cellsHTML(t *testing.T, md string) string {
	t.Helper()
	n, err := Open(write(t, md), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var b strings.Builder
	for _, c := range n.Cells() {
		b.WriteString(c.HTML)
	}
	return b.String()
}

func dataURI(svg string) string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg))
}

func TestSVGFence(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>` + "\n"
	got := cellsHTML(t, "# Pic\n\n```svg\n"+svg+"```\n\n- in a list:\n\n  ```svg\n  "+svg+"  ```\n")
	if strings.Count(got, `<figure class="diagram" data-lang="svg">`) != 2 {
		t.Errorf("want two svg figures: %s", got)
	}
	if !strings.Contains(got, `<img alt="svg diagram" src="`+dataURI(svg)+`">`) {
		t.Errorf("svg not drawn as an image: %s", got)
	}
	if !strings.Contains(got, `<code class="language-svg">&lt;svg`) {
		t.Errorf("svg source not kept: %s", got)
	}
}

func TestD2Fence(t *testing.T) {
	got := cellsHTML(t, "```d2\nclient -> api: HTTPS\n```\n")
	for _, want := range []string{
		`<figure class="diagram" data-lang="d2">`,
		`<img class="light" alt="d2 diagram" width="`,
		`<img class="dark" alt="d2 diagram" width="`,
		`<code class="language-d2">client -&gt; api: HTTPS`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "diagram-note") {
		t.Errorf("unexpected note: %s", got)
	}
	// Each theme's picture is d2's SVG, with the label drawn in it.
	l, d, _ := drawD2([]byte("client -> api: HTTPS\n"))
	for _, svg := range [][]byte{l, d} {
		if !strings.Contains(string(svg), "<svg") || !strings.Contains(string(svg), "HTTPS") {
			t.Errorf("not d2's SVG: %.200s", svg)
		}
	}
	if string(l) == string(d) {
		t.Error("light and dark are the same picture")
	}
}

func TestD2Error(t *testing.T) {
	got := cellsHTML(t, "```d2\na ->\n```\n")
	if strings.Contains(got, "<img") || !strings.Contains(got, `<p class="diagram-note">d2: 1:1: connection missing destination</p>`) || !strings.Contains(got, "language-d2") {
		t.Errorf("want the code and d2's error: %s", got)
	}
}

func TestD2Timeout(t *testing.T) {
	old := d2Timeout
	d2Timeout = 0
	defer func() { d2Timeout = old }()
	if _, _, note := drawD2([]byte("slow -> layout\n")); !strings.Contains(note, "too long") {
		t.Errorf("note = %q", note)
	}
}

func TestSVGWidth(t *testing.T) {
	for svg, want := range map[string]string{
		`<?xml version="1.0"?><svg xmlns="x" viewBox="0 0 255.5 455"><svg width="9">`: ` width="256"`,
		`<svg width="120" viewBox="0 0 10 10">`:                                       "",
		`<svg viewBox="-10 -10 40 20">`:                                               ` width="40"`,
		`<svg>`:                                                                       "",
		`not svg`:                                                                     "",
	} {
		if got := svgWidth([]byte(svg)); got != want {
			t.Errorf("svgWidth(%s) = %q, want %q", svg, got, want)
		}
	}
}
