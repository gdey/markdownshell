package main

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssRule    = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	cssDecl    = regexp.MustCompile(`(--[\w-]+)\s*:\s*([^;]+);`)
)

// cssVars returns the custom properties set by the rules in file whose
// selector list has a selector that pick accepts, merged, by what pick
// says the selector is for.
func cssVars(t *testing.T, file string, pick func(sel string) string) map[string]map[string]bool {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	sets := map[string]map[string]bool{}
	for _, m := range cssRule.FindAllStringSubmatch(cssComment.ReplaceAllString(string(src), ""), -1) {
		for _, sel := range strings.Split(m[1], ",") {
			key := pick(strings.Join(strings.Fields(sel), " "))
			if key == "" {
				continue
			}
			if sets[key] == nil {
				sets[key] = map[string]bool{}
			}
			for _, d := range cssDecl.FindAllStringSubmatch(m[2], -1) {
				sets[key][d[1]] = true
			}
		}
	}
	return sets
}

// palette names the palette a selector sets: "light" or "dark", for the
// window (the root) or the History pane, with the look's name and a
// space before it for a look's own, such as "tty light".
func palette(sel string) string {
	look := ""
	if m := lookAttr.FindStringSubmatch(sel); m != nil {
		look = m[1] + " "
	}
	switch {
	case sel == ":root", sel == `:root[data-look="`+strings.TrimSpace(look)+`"]`:
		return look + "light"
	case strings.Contains(sel, `data-scheme="dark"`):
		return look + "dark"
	case strings.Contains(sel, `data-history-scheme="light"`) && strings.HasSuffix(sel, "#history"):
		return look + "light pane"
	case strings.Contains(sel, `data-history-scheme="dark"`) && strings.HasSuffix(sel, "#history"):
		return look + "dark pane"
	}
	return ""
}

var lookAttr = regexp.MustCompile(`data-look="(\w+)"`)

// Each look must set every token the plain look does, in the light and
// in the dark, so a token added to one can't be forgotten in another and
// show the plain color through. Each palette must be the same for the
// window as for the History pane. And a look's own tokens (--tty-*,
// --bp-*) must be set in its dark palette as well as its light one.
func TestLookTokens(t *testing.T) {
	plain := cssVars(t, "frontend/style.css", palette)
	if len(plain["light"]) < 40 || len(plain["dark"]) < 40 {
		t.Fatalf("found %d light and %d dark tokens in style.css; is the parse right?", len(plain["light"]), len(plain["dark"]))
	}
	// The pane's palette is the window's minus the faces and the text
	// size, which the pane inherits.
	paneCovers := func(name string, sets map[string]map[string]bool, prefix string) {
		for _, scheme := range []string{"light", "dark"} {
			for v := range sets[prefix+scheme] {
				if strings.HasPrefix(v, "--font-") || v == "--text-scale" || v == "--term-bg" || v == "--term-fg" {
					continue
				}
				if !sets[prefix+scheme+" pane"][v] {
					t.Errorf("%s %s does not set %s for the History pane", name, scheme, v)
				}
			}
		}
	}
	paneCovers("plain", plain, "")
	for _, look := range []struct{ name, attr, file, own string }{
		{"Teletype", "tty", "frontend/teletype.css", "--tty-"},
		{"Blueprint", "blueprint", "frontend/blueprint.css", "--bp-"},
	} {
		sets := cssVars(t, look.file, palette)
		prefix := look.attr + " "
		for _, scheme := range []string{"light", "dark"} {
			for _, v := range slices.Sorted(maps.Keys(plain[scheme])) {
				if !sets[prefix+scheme][v] {
					t.Errorf("%s %s does not set %s", look.name, scheme, v)
				}
			}
		}
		paneCovers(look.name, sets, prefix)
		for v := range sets[prefix+"light"] {
			if strings.HasPrefix(v, look.own) && !sets[prefix+"dark"][v] {
				t.Errorf("%s dark does not set %s", look.name, v)
			}
		}
	}
}

// The Settings window's edits must all be saved, the look among them,
// or the other windows never hear of a change.
func TestEditedSettings(t *testing.T) {
	cur := settings.Settings{Recent: []string{"/a.md"}, LastFolder: "/a"}
	edited(&cur, settings.Settings{Look: "tty", Theme: "dark", NoSpellCheck: true, View: "both", Recent: nil})
	if cur.Look != "tty" || cur.Theme != "dark" || !cur.NoSpellCheck || cur.View != "both" {
		t.Errorf("edited = %+v", cur)
	}
	if len(cur.Recent) != 1 || cur.LastFolder != "/a" {
		t.Errorf("edited changed what the window doesn't edit: %+v", cur)
	}
	edited(&cur, settings.Settings{Look: "tty", TextSize: 300, UIFont: "Inter", CodeFont: `x"; } body {`, LineSpacing: "relaxed", NoRules: true, Accent: "#AABBCC", HistoryPane: "invert"})
	if cur.TextSize != 150 || cur.UIFont != "Inter" || cur.CodeFont != "" || cur.LineSpacing != "relaxed" || !cur.NoRules || cur.Accent != "#aabbcc" || cur.HistoryPane != "invert" {
		t.Errorf("look options = %+v", cur)
	}
	edited(&cur, settings.Settings{SessionFolder: " ~/notes ", SessionTemplate: "/t/session.md"})
	if cur.SessionFolder != "~/notes" || cur.SessionTemplate != "/t/session.md" {
		t.Errorf("session settings = %q, %q", cur.SessionFolder, cur.SessionTemplate)
	}
	edited(&cur, settings.Settings{Look: "blueprint"})
	if cur.Look != "blueprint" {
		t.Errorf("blueprint look dropped: %q", cur.Look)
	}
	edited(&cur, settings.Settings{Look: "bogus", Theme: "bogus", LineSpacing: "huge", Accent: "red", HistoryPane: "sideways"})
	if cur.Look != "" || cur.Theme != "" || cur.LineSpacing != "" || cur.Accent != "" || cur.HistoryPane != "" || cur.TextSize != 0 {
		t.Errorf("values the window can't send kept: %+v", cur)
	}
}
