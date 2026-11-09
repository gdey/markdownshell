// Command newlook starts a new look for mdsh-gui: a folder in
// web/looks with a look.css that opens with the look's front
// matter (TOML between +++ lines in its first comment) and sets the
// plain look's colors under the new look's name, so it passes the tests
// from the start. Run it from cmd/mdsh-gui:
//
//	go run ./internal/newlook [-id moss] [-since 1.6] <folder> "<Name>"
//
// The id, the look setting's value and the pages' data-look, is the
// folder's name without dashes unless -id gives one. The release is the
// one after the newest look's unless -since gives one. Then fill in the
// TODOs (the tests fail while any are left), paint it, and run
// go generate. docs/looks.md has the rest.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/looks"
)

const dir = "../../web/looks"

var folderPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func main() {
	log.SetFlags(0)
	id := flag.String("id", "", "the look's id, for settings and data-look (default: the folder without dashes)")
	since := flag.String("since", "", "the release that brings it, such as 1.6 (default: the one after the newest look's)")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), `usage: go run ./internal/newlook [-id id] [-since x.y] <folder> "<Name>"`)
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	if err := newLook(flag.Arg(0), flag.Arg(1), *id, *since); err != nil {
		log.Fatal("newlook: ", err)
	}
}

func newLook(folder, name, id, since string) error {
	if !folderPattern.MatchString(folder) {
		return fmt.Errorf("folder %q: use lowercase letters, digits and dashes", folder)
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("the look needs a name")
	}
	if id == "" {
		id = strings.ReplaceAll(folder, "-", "")
	}
	all, err := looks.Load(os.DirFS(dir))
	if err != nil {
		return err
	}
	order, newest := 0, ""
	for _, l := range all {
		if l.Dir == folder || l.ID == id || strings.EqualFold(l.Name, name) {
			return fmt.Errorf("%s (%q, %s) is already a look", l.Dir, l.ID, l.Name)
		}
		order = max(order, l.Order)
		newest = maxSince(newest, l.Since)
	}
	if since == "" {
		if since, err = nextMinor(newest); err != nil {
			return err
		}
	}
	style, err := os.ReadFile("../../web/style.css")
	if err != nil {
		return err
	}
	css, err := lookCSS(string(style), id, name, order+10, since)
	if err != nil {
		return err
	}
	// What newlook writes must be a look the app can load.
	if _, err := looks.Parse([]byte(css)); err != nil {
		return fmt.Errorf("the new look.css doesn't parse: %w", err)
	}
	path := filepath.Join(dir, folder)
	if err := os.Mkdir(path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(path, "look.css"), []byte(css), 0o644); err != nil {
		return err
	}
	fmt.Printf(`Made %[1]s/look.css for %[2]s (look %[3]q, since %[4]s).

Next:
  1. Fill in its TODOs; the tests fail while any are left.
  2. Paint it: the colors, then the rules for its texture.
  3. Describe it in the front matter at the top: its description,
     tagline and texture switches.
  4. go generate (or go run ./internal/build), then go test ./...
  5. Screenshots, the CHANGELOG entry and the front matter's pictures:
     see docs/looks.md.
`, path, name, id, since)
	return nil
}

// maxSince is the later of two releases, "1.4" and "1.10".
func maxSince(a, b string) string {
	pa, pb := parse(a), parse(b)
	if pa == nil {
		return b
	}
	if pb == nil {
		return a
	}
	for i := range min(len(pa), len(pb)) {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return a
			}
			return b
		}
	}
	return a
}

func parse(v string) []int {
	var out []int
	for _, f := range strings.Split(v, ".") {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// nextMinor is the minor release after v: 1.5 for 1.4.
func nextMinor(v string) (string, error) {
	p := parse(v)
	if len(p) < 2 {
		return "", fmt.Errorf("can't tell the release after %q; give -since", v)
	}
	return fmt.Sprintf("%d.%d", p[0], p[1]+1), nil
}

// The plain look's palettes in style.css: the light, the faces, and the
// dark.
var (
	lightRule = regexp.MustCompile(`(?s)\n?:root,\n:root\[data-history-scheme="light"\] #history \{\n(.*?)\n\}`)
	facesRule = regexp.MustCompile(`(?s)\n:root \{\n(\s*--font-ui.*?)\n\}`)
	darkRule  = regexp.MustCompile(`(?s)\n:root\[data-scheme="dark"\],\n:root\[data-history-scheme="dark"\] #history \{\n(.*?)\n\}`)
	bgToken   = regexp.MustCompile(`--bg:\s*([^;]+);`)
	termRule  = regexp.MustCompile(`\n:root \{ (--term-bg:[^}]*?) \}`)
)

// lookCSS is a new look's stylesheet: style.css's palettes under the
// look's data-look, with one token of its own to show the way.
func lookCSS(style, id, name string, order int, since string) (string, error) {
	var bodies [3]string
	for i, re := range []*regexp.Regexp{lightRule, facesRule, darkRule} {
		m := re.FindStringSubmatch(style)
		if m == nil {
			return "", fmt.Errorf("style.css: no palette matching %s; has its layout changed?", re)
		}
		bodies[i] = m[1]
	}
	// The terminals' colors are set apart in style.css, once for both
	// themes; a look sets them in each palette.
	term := termRule.FindStringSubmatch(style)
	if term == nil {
		return "", errors.New("style.css: no --term-bg and --term-fg; has its layout changed?")
	}
	own := func(body string) string {
		bg := "#ffffff"
		if m := bgToken.FindStringSubmatch(body); m != nil {
			bg = m[1]
		}
		return fmt.Sprintf("%s\n  /* The terminals in output. */\n  %s\n  /* TODO: the look's own tokens, each set in both palettes. */\n  --%s-side: %s;", body, term[1], id, bg)
	}
	var b bytes.Buffer
	look := fmt.Sprintf(`[data-look=%q]`, id)
	fmt.Fprintf(&b, `/* +++
# The look's front matter, in TOML. See docs/looks.md for every key.
# id is the look setting's value and the pages' data-look.
id = %[7]q
name = %[8]q
# Its place in Settings › Look's menu, lowest first, and its release.
order = %[9]d
since = %[10]q
# Shown under Settings › Look's menu while the look is chosen.
description = "TODO: a sentence or two, such as \"%[1]s is … in the light theme, or … when the theme is dark.\""
# Ends "%[1]s for …" on the website's looks band.
tagline = "TODO: for …"
# The prefix of its own tokens, each set in both palettes.
tokens = "--%[2]s-"
# Its text size in percent, if --text-scale below isn't 1.
# textSize = 115

# The texture switches it draws, if any, under its names for them.
# [texture]
# label = "%[1]s paper"
# rules = "Ruled lines"
# shadows = "Shadows under cards"

# The website's picture, light and dark, from a release build.
# [hero]
# light = "hero-%[2]s.png"
# dark = "hero-%[2]s-dark.png"
# alt = "The %[1]s look"

# The GUI README's pictures, from site/static/images/gui.
# [[screenshots]]
# file = "%[2]s-light.png"
# alt = "The deploy runbook in the %[1]s look"
+++

   The %[1]s look (Settings › Look). TODO: say what it is, in the
   light theme and in the dark. It sets the same tokens style.css does,
   plus a few of its own (--%[2]s-*), and adds texture rules. Every rule
   selects %[3]s, so it paints only this look. The tests check
   that every token the plain look sets is set here too, in both
   palettes, that the text and accent keep the interface guidelines'
   contrast, and that nothing here moves the layout. See docs/looks.md.

   The palettes start as the plain look's: change them. */

:root%[3]s,
:root%[3]s[data-history-scheme="light"] #history {
%[4]s
}

/* The look's faces and text size. A text size other than 1 goes in
   the front matter's textSize too, as a percent. */
:root%[3]s {
%[5]s
}

/* The dark theme, for the window or the History pane (see style.css). */
:root%[3]s[data-scheme="dark"],
:root%[3]s[data-history-scheme="dark"] #history {
%[6]s
}

/* TODO: the texture. Paint, not layout: colors, borders, backgrounds,
   shadows and faces. The tab bar and the outline as an example: */
%[3]s #tabbar, %[3]s #outline, %[3]s .tabs { background: var(--%[2]s-side); }
`, name, id, look, own(bodies[0]), bodies[1], own(bodies[2]), id, name, order, since)
	return b.String(), nil
}
