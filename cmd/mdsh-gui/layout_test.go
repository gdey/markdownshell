package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var (
	mainTag   = regexp.MustCompile(`<main\b([^>]*)>`)
	idAttr    = regexp.MustCompile(`\bid="([^"]+)"`)
	bodyClass = regexp.MustCompile(`<body\b[^>]*\bclass="([^"]+)"`)
)

// cssProps returns the properties each selector sets in file, with the
// selector's whitespace folded.
func cssProps(t *testing.T, file string) map[string]map[string]bool {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	props := map[string]map[string]bool{}
	for _, m := range cssRule.FindAllStringSubmatch(cssComment.ReplaceAllString(string(src), ""), -1) {
		for _, sel := range strings.Split(m[1], ",") {
			sel = strings.Join(strings.Fields(sel), " ")
			if props[sel] == nil {
				props[sel] = map[string]bool{}
			}
			for _, d := range strings.Split(m[2], ";") {
				if name, _, ok := strings.Cut(d, ":"); ok {
					props[sel][strings.TrimSpace(name)] = true
				}
			}
		}
	}
	return props
}

// The main rule in style.css centres the rendered document (#doc) in a
// column 860px wide. Every other <main>, there for screen readers as a
// landmark, has to undo it, or it shrinks to the rule's padding: the
// window's #frames did, and every tab's page with it.
func TestOtherMainsUndoTheDocumentColumn(t *testing.T) {
	props := cssProps(t, "frontend/style.css")
	for _, p := range []string{"max-width", "margin", "padding"} {
		if !props["main"][p] {
			t.Fatalf("style.css main rule does not set %s; is the parse right?", p)
		}
	}
	for _, page := range []string{"index.html", "window.html", "settings.html"} {
		src, err := os.ReadFile("frontend/" + page)
		if err != nil {
			t.Fatal(err)
		}
		class := ""
		if m := bodyClass.FindSubmatch(src); m != nil {
			class = "body." + string(m[1]) + " "
		}
		for _, m := range mainTag.FindAllSubmatch(src, -1) {
			sel := class + "main"
			if id := idAttr.FindSubmatch(m[1]); id != nil {
				if string(id[1]) == "doc" {
					continue
				}
				sel = "#" + string(id[1])
			}
			for _, p := range []string{"max-width", "margin", "padding"} {
				if !props[sel][p] {
					t.Errorf("%s: <main%s> gets the document column's %s; the %s rule in style.css must set it", page, m[1], p, sel)
				}
			}
		}
	}
}

// Selecting a card must not move its head's buttons. The mousedown on a
// play button selects its card, as WebKit focuses the card rather than
// the button; if selecting then laid the head out again, say by showing
// the trash, the play button slid out from under the pointer, the
// mouseup landed elsewhere, and the first click only selected the card.
func TestSelectingABlockKeepsItsHeadInPlace(t *testing.T) {
	layout := []string{"display", "width", "min-width", "max-width", "padding", "padding-left", "padding-right", "margin", "margin-left", "margin-right", "border", "border-width", "gap", "font-size", "flex", "order", "position"}
	for _, file := range []string{"style.css", "blueprint.css", "teletype.css"} {
		for sel, props := range cssProps(t, "frontend/"+file) {
			if !strings.Contains(sel, ".selected") || !strings.Contains(sel, ".block") || !strings.Contains(sel, ".head") {
				continue
			}
			for _, p := range layout {
				if props[p] {
					t.Errorf("%s: %q sets %s, which moves the head's buttons when a click selects the card; use visibility or colour instead", file, sel, p)
				}
			}
		}
	}
}
