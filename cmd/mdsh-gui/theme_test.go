package main

import (
	"fmt"
	"maps"
	"math"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/settings"
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
// --bp-*, --gum-*, --witch-*) must be set in its dark palette as well as its light one.
func TestLookTokens(t *testing.T) {
	plain := cssVars(t, "../../web/style.css", palette)
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
		{"Teletype", "tty", "../../web/teletype.css", "--tty-"},
		{"Blueprint", "blueprint", "../../web/blueprint.css", "--bp-"},
		{"Bubble Gum", "gum", "../../web/bubblegum.css", "--gum-"},
		{"Witchcraft", "witch", "../../web/witchcraft.css", "--witch-"},
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
	cur := settings.Settings{Recent: []string{"/a.md"}, Pinned: []string{"/p.md"}, LastFolder: "/a"}
	edited(&cur, settings.Settings{Look: "tty", Theme: "dark", NoSpellCheck: true, View: "both", Recent: nil})
	if cur.Look != "tty" || cur.Theme != "dark" || !cur.NoSpellCheck || cur.View != "both" {
		t.Errorf("edited = %+v", cur)
	}
	if len(cur.Recent) != 1 || len(cur.Pinned) != 1 || cur.LastFolder != "/a" {
		t.Errorf("edited changed what the window doesn't edit: %+v", cur)
	}
	edited(&cur, settings.Settings{Look: "tty", TextSize: 300, UIFont: "Inter", CodeFont: `x"; } body {`, LineSpacing: "relaxed", NoRules: true, Accent: "#AABBCC", HistoryPane: "invert", HistoryFocus: "cursor"})
	if cur.TextSize != 150 || cur.UIFont != "Inter" || cur.CodeFont != "" || cur.LineSpacing != "relaxed" || !cur.NoRules || cur.Accent != "#aabbcc" || cur.HistoryPane != "invert" || cur.HistoryFocus != "cursor" {
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
	edited(&cur, settings.Settings{Look: "gum"})
	if cur.Look != "gum" {
		t.Errorf("gum look dropped: %q", cur.Look)
	}
	edited(&cur, settings.Settings{Look: "witch"})
	if cur.Look != "witch" {
		t.Errorf("witch look dropped: %q", cur.Look)
	}
	edited(&cur, settings.Settings{Look: "bogus", Theme: "bogus", LineSpacing: "huge", Accent: "red", HistoryPane: "sideways", HistoryFocus: "ring"})
	if cur.Look != "" || cur.Theme != "" || cur.LineSpacing != "" || cur.Accent != "" || cur.HistoryPane != "" || cur.HistoryFocus != "" || cur.TextSize != 0 {
		t.Errorf("values the window can't send kept: %+v", cur)
	}
}

// cssValues returns the custom properties each palette sets in file,
// with their values, keyed as cssVars keys them.
func cssValues(t *testing.T, file string) map[string]map[string]string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	sets := map[string]map[string]string{}
	for _, m := range cssRule.FindAllStringSubmatch(cssComment.ReplaceAllString(string(src), ""), -1) {
		for _, sel := range strings.Split(m[1], ",") {
			key := palette(strings.Join(strings.Fields(sel), " "))
			if key == "" {
				continue
			}
			if sets[key] == nil {
				sets[key] = map[string]string{}
			}
			for _, d := range cssDecl.FindAllStringSubmatch(m[2], -1) {
				sets[key][d[1]] = strings.TrimSpace(d[2])
			}
		}
	}
	return sets
}

// contrast is the WCAG contrast ratio of two #rrggbb colors.
func contrast(a, b string) float64 {
	lum := func(hex string) float64 {
		var c [3]float64
		for i := range c {
			var v int
			fmt.Sscanf(hex[1+2*i:3+2*i], "%02x", &v)
			f := float64(v) / 255
			if f <= 0.03928 {
				c[i] = f / 12.92
			} else {
				c[i] = math.Pow((f+0.055)/1.055, 2.4)
			}
		}
		return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]
	}
	x, y := lum(a), lum(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// The interface guidelines (docs/hig.md) ask 4.5:1 of text on its
// surface and 3:1 of a control's edge, in light and dark, in every
// look. The text pairs: the text and the muted text on the window,
// the text on code, the button text on the accent and on the failure
// red, the link on the window. The edge pair: the accent, which
// draws the focus ring and the selected card's outline, on the window.
// Blueprint and Teletype have known gaps, listed in the accessibility
// audit; the plain look, Bubble Gum and Witchcraft must pass.
func TestLookContrast(t *testing.T) {
	text := [][2]string{{"--fg", "--bg"}, {"--muted", "--bg"}, {"--fg", "--code-bg"}, {"--muted", "--code-bg"}, {"--accent-fg", "--accent"}, {"--bad-fg", "--bad"}, {"--link", "--bg"}}
	edge := [][2]string{{"--accent", "--bg"}}
	for _, look := range []struct{ name, file, prefix string }{
		{"plain", "../../web/style.css", ""},
		{"Bubble Gum", "../../web/bubblegum.css", "gum "},
		{"Witchcraft", "../../web/witchcraft.css", "witch "},
	} {
		sets := cssValues(t, look.file)
		for _, scheme := range []string{"light", "dark"} {
			p := sets[look.prefix+scheme]
			check := func(pairs [][2]string, min float64) {
				for _, pair := range pairs {
					fg, bg := p[pair[0]], p[pair[1]]
					if len(fg) != 7 || len(bg) != 7 || fg[0] != '#' || bg[0] != '#' {
						t.Errorf("%s %s: %s is %q and %s is %q; the check needs #rrggbb", look.name, scheme, pair[0], fg, pair[1], bg)
						continue
					}
					if r := contrast(fg, bg); r < min {
						t.Errorf("%s %s: %s %s on %s %s is %.2f:1, under %.1f:1", look.name, scheme, pair[0], fg, pair[1], bg, r, min)
					}
				}
			}
			check(text, 4.5)
			check(edge, 3)
		}
	}
}
