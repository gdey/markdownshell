// Package looks reads the desktop app's looks. Each look is a folder in
// frontend/looks with a look.css that paints it and, in TOML between +++
// lines at the top of its first comment, says what it is:
//
//	/* +++
//	id = "witch"
//	name = "Witchcraft"
//	…
//	+++
//
//	   The Witchcraft look (Settings › Look): …
//	*/
//
// Plain's look.css only describes it: its colors are style.css's.
// Everything else that names the
// looks is made from those folders: the Settings › Look menu and its
// options, the stylesheet the pages load (bundle/looks.css), the look
// the app accepts in its settings, the tests every look must pass, the
// website's gallery and the GUI README's pictures.
//
// docs/looks.md says how to add one, and
//
//	go run ./internal/newlook <folder> "<Name>"
//
// starts one.
package looks

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// Look is what a look's look.css says of it, in its front matter.
type Look struct {
	// ID is the look's value for the look setting and the pages'
	// data-look attribute, such as "tty"; its CSS selects
	// :root[data-look="tty"]. Plain's is empty. Lowercase letters and
	// digits, starting with a letter.
	ID string `toml:"id"`
	// Name is what Settings › Look and the docs call it, such as
	// "Teletype".
	Name string `toml:"name"`
	// Order places it in the Look menu and the galleries, lowest first.
	Order int `toml:"order"`
	// Since is the release that brought it, such as "1.4".
	Since string `toml:"since"`
	// Description is a sentence or two for Settings › Look, shown under
	// the menu while the look is chosen.
	Description string `toml:"description"`
	// Tagline finishes the website's "Plain for every day, Teletype
	// for …" sentence: "for fan-fold paper and green phosphor".
	Tagline string `toml:"tagline"`
	// TextSize is the text size, in percent, the look's CSS sets with
	// --text-scale, for the Text size slider. 0 is 100.
	TextSize int `toml:"textSize,omitempty"`
	// Tokens is the prefix of the CSS custom properties the look adds
	// to the plain look's, such as "--tty-". Each must be set in the
	// dark palette as well as the light one.
	Tokens string `toml:"tokens,omitempty"`
	// ContrastGaps excuses the look from TestLookContrast, for the
	// older looks whose gaps the accessibility audit lists. A new look
	// leaves it out and passes.
	ContrastGaps bool `toml:"contrastGaps,omitempty"`
	// Texture, if set, shows the texture checkboxes in Settings › Look
	// while the look is chosen.
	Texture *Texture `toml:"texture,omitempty"`
	// Hero is the look's picture in the website's looks band, from
	// site/static/images/app/. Taken from a release build, so a new
	// look can leave it out until then.
	Hero *Hero `toml:"hero,omitempty"`
	// Screenshots are the look's pictures in the GUI README, from
	// site/static/images/gui/.
	Screenshots []Shot `toml:"screenshots,omitempty"`

	// Dir is the look's folder in frontend/looks.
	Dir string `toml:"-"`
}

// Texture names the texture switches a look draws, by the settings
// they turn off (noSprockets, noRules, noShadows), and the group's
// label. A switch with no name isn't shown.
type Texture struct {
	Label     string `toml:"label"`
	Sprockets string `toml:"sprockets,omitempty"`
	Rules     string `toml:"rules,omitempty"`
	Shadows   string `toml:"shadows,omitempty"`
}

// Hero is a picture, light and dark, for the website.
type Hero struct {
	Light string `toml:"light"`
	Dark  string `toml:"dark,omitempty"`
	Alt   string `toml:"alt"`
}

// Shot is a picture and its alt text.
type Shot struct {
	File string `toml:"file"`
	Alt  string `toml:"alt"`
}

var (
	idPattern     = regexp.MustCompile(`^[a-z][a-z0-9]*$`)
	tokensPattern = regexp.MustCompile(`^--[a-z][a-z0-9]*-$`)
)

// Load reads every look in fsys, a folder of look folders, and returns
// them in order.
func Load(fsys fs.FS) ([]Look, error) {
	files, err := fs.Glob(fsys, "*/look.css")
	if err != nil {
		return nil, err
	}
	var all []Look
	var errs []error
	for _, file := range files {
		b, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, err
		}
		l, err := Parse(b)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
			continue
		}
		l.Dir = path.Dir(file)
		if err := l.check(b); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", file, err))
		}
		all = append(all, l)
	}
	slices.SortStableFunc(all, func(a, b Look) int { return a.Order - b.Order })
	ids, names, orders := map[string]string{}, map[string]string{}, map[int]string{}
	for _, l := range all {
		for _, seen := range []struct {
			what string
			m    map[string]string
			key  string
		}{{"id", ids, l.ID}, {"name", names, l.Name}} {
			if other, ok := seen.m[seen.key]; ok {
				errs = append(errs, fmt.Errorf("%s and %s have the same %s, %q", other, l.Dir, seen.what, seen.key))
			}
			seen.m[seen.key] = l.Dir
		}
		if other, ok := orders[l.Order]; ok {
			errs = append(errs, fmt.Errorf("%s and %s have the same order, %d", other, l.Dir, l.Order))
		}
		orders[l.Order] = l.Dir
	}
	if _, ok := ids[""]; !ok && len(errs) == 0 {
		errs = append(errs, errors.New("no look has the empty id, for Plain"))
	}
	if len(all) > 0 && all[0].ID != "" && len(errs) == 0 {
		errs = append(errs, errors.New("Plain must come first"))
	}
	return all, errors.Join(errs...)
}

func (l Look) check(css []byte) error {
	var errs []error
	need := func(field, v string) {
		if strings.TrimSpace(v) == "" {
			errs = append(errs, fmt.Errorf("no %s", field))
		}
	}
	need("name", l.Name)
	need("since", l.Since)
	need("description", l.Description)
	need("tagline", l.Tagline)
	if l.TextSize != 0 && (l.TextSize < 80 || l.TextSize > 200) {
		errs = append(errs, fmt.Errorf("textSize %d isn't 80 to 200", l.TextSize))
	}
	if t := l.Texture; t != nil {
		need("texture label", t.Label)
		if t.Sprockets == "" && t.Rules == "" && t.Shadows == "" {
			errs = append(errs, errors.New("a texture with no switches"))
		}
	}
	if h := l.Hero; h != nil {
		need("hero light", h.Light)
		need("hero alt", h.Alt)
	}
	for _, s := range l.Screenshots {
		need("screenshot file", s.File)
		need("screenshot alt", s.Alt)
	}
	if l.ID == "" {
		// Plain is style.css; it has nothing of its own to add.
		if strings.TrimSpace(cssComment.ReplaceAllString(string(css), "")) != "" {
			errs = append(errs, errors.New("Plain's look.css has rules; its colors are style.css's"))
		}
		if l.Tokens != "" || l.TextSize != 0 || l.Texture != nil {
			errs = append(errs, errors.New("Plain sets tokens, textSize or texture"))
		}
		return errors.Join(errs...)
	}
	if !idPattern.MatchString(l.ID) {
		errs = append(errs, fmt.Errorf("id %q isn't lowercase letters and digits", l.ID))
	}
	if !tokensPattern.MatchString(l.Tokens) {
		errs = append(errs, fmt.Errorf("tokens %q isn't a prefix such as \"--%s-\"", l.Tokens, l.ID))
	}
	return errors.Join(errs...)
}

var (
	frontMatter = regexp.MustCompile(`(?s)\A/\*\s*\+\+\+\n(.*?)\n\+\+\+\n`)
	cssComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
)

// Parse reads a look.css's front matter: TOML between +++ lines, which
// open the file's first comment. Dir is left empty.
func Parse(css []byte) (Look, error) {
	var l Look
	m := frontMatter.FindSubmatch(css)
	if m == nil {
		return l, errors.New("no front matter: the file must open with /* +++, the look's TOML, and +++")
	}
	md, err := toml.Decode(string(m[1]), &l)
	if err != nil {
		return l, err
	}
	if extra := md.Undecoded(); len(extra) > 0 {
		return l, fmt.Errorf("unknown keys %v", extra)
	}
	return l, nil
}

// Valid says whether id is a look's.
func Valid(all []Look, id string) bool {
	return slices.ContainsFunc(all, func(l Look) bool { return l.ID == id })
}
