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
// window (the root) or the History pane, with "tty " before it for the
// Teletype look's.
func palette(sel string) string {
	key := ""
	switch {
	case sel == ":root", sel == `:root[data-look="tty"]`:
		key = "light"
	case strings.Contains(sel, `data-scheme="dark"`):
		key = "dark"
	case strings.Contains(sel, `data-history-scheme="light"`) && strings.HasSuffix(sel, "#history"):
		key = "light pane"
	case strings.Contains(sel, `data-history-scheme="dark"`) && strings.HasSuffix(sel, "#history"):
		key = "dark pane"
	default:
		return ""
	}
	if strings.Contains(sel, `data-look="tty"`) {
		key = "tty " + key
	}
	return key
}

// The Teletype look must set every token the plain look does, in the
// light and in the dark, so a token added to one can't be forgotten in
// the other and show the plain color through the paper. And each
// palette must be the same for the window as for the History pane.
func TestTeletypeTokens(t *testing.T) {
	plain := cssVars(t, "frontend/style.css", palette)
	tty := cssVars(t, "frontend/teletype.css", palette)
	if len(plain["light"]) < 40 || len(plain["dark"]) < 40 {
		t.Fatalf("found %d light and %d dark tokens in style.css; is the parse right?", len(plain["light"]), len(plain["dark"]))
	}
	for _, scheme := range []string{"light", "dark"} {
		for _, v := range slices.Sorted(maps.Keys(plain[scheme])) {
			if !tty["tty "+scheme][v] {
				t.Errorf("Teletype %s does not set %s", scheme, v)
			}
		}
		// The pane's palette is the window's minus the faces and the
		// text size, which the pane inherits.
		for look, sets := range map[string]map[string]map[string]bool{"plain": plain, "Teletype": tty} {
			prefix := ""
			if look == "Teletype" {
				prefix = "tty "
			}
			for v := range sets[prefix+scheme] {
				if strings.HasPrefix(v, "--font-") || v == "--text-scale" || v == "--term-bg" || v == "--term-fg" {
					continue
				}
				if !sets[prefix+scheme+" pane"][v] {
					t.Errorf("%s %s does not set %s for the History pane", look, scheme, v)
				}
			}
		}
	}
	for v := range tty["tty light"] {
		if strings.HasPrefix(v, "--tty-") && !tty["tty dark"][v] {
			t.Errorf("phosphor does not set %s", v)
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
	edited(&cur, settings.Settings{Look: "bogus", Theme: "bogus", LineSpacing: "huge", Accent: "red", HistoryPane: "sideways"})
	if cur.Look != "" || cur.Theme != "" || cur.LineSpacing != "" || cur.Accent != "" || cur.HistoryPane != "" || cur.TextSize != 0 {
		t.Errorf("values the window can't send kept: %+v", cur)
	}
}
