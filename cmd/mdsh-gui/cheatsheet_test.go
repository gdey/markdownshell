package main

import (
	"strings"
	"testing"
)

func TestCheatsheetHTML(t *testing.T) {
	md := "---\ntitle: Cheat sheet\nweight: 60\n---\n\nIntro.\n\n## Fence attributes\n\n| Attribute | Does |\n| --- | --- |\n| `skip` | Don't run |\n\n<script>alert(1)</script>\n"
	got, err := cheatsheetHTML([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<p>Intro.</p>`, `<h2 id="fence-attributes">Fence attributes</h2>`, `<td><code>skip</code></td>`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	for _, bad := range []string{"title:", "weight", "<hr", "<script>"} {
		if strings.Contains(got, bad) {
			t.Errorf("has %s:\n%s", bad, got)
		}
	}
	// Without front matter the page is rendered as it is.
	if got, err := cheatsheetHTML([]byte("## Keys\n")); err != nil || !strings.Contains(got, "<h2") {
		t.Errorf("no front matter: %q, %v", got, err)
	}
}

// The cheat sheet built in renders, with its sections.
func TestCheatsheetBuiltIn(t *testing.T) {
	html, err := renderCheatsheet()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="command-line"`, `id="fence-attributes"`, `id="front-matter"`, `id="mdsh-gui"`, "<table>"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(html, "<hr") {
		t.Error("the front matter shows as a rule")
	}
}

func TestCheatsheetTab(t *testing.T) {
	tab := &docTab{id: 5, page: cheatsheetPage}
	if got := tab.info(); got.Title != "Cheat sheet" || got.URL != "mdsh://doc/docs/cheatsheet.md" || got.Page != cheatsheetPage || got.File != "" {
		t.Errorf("info = %+v", got)
	}
}
