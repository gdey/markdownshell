// Package collection reads the .mdsh.yaml file that turns a folder of
// runbooks into a collection: a title, description, icon, color,
// background image and tags that front ends show for the folder.
package collection

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// FileName is the name of a folder's collection file.
const FileName = ".mdsh.yaml"

// Limits on what a collection file holds.
const (
	MaxSize  = 64 << 10 // bytes of .mdsh.yaml read
	MaxText  = 300      // runes of title or description kept
	MaxIcon  = 8        // runes of a text icon, enough for one emoji sequence
	MaxTags  = 32       // tags kept
	MaxTagSz = 40       // runes in one tag
)

// Collection is a folder's .mdsh.yaml, checked: paths are absolute and
// inside the folder, the color is safe to put in CSS, and tags are
// normalized. Fields that fail a check are left empty.
type Collection struct {
	// Dir is the folder's absolute path.
	Dir string
	// Title is shown instead of the folder's name.
	Title string
	// Description says what the collection is for.
	Description string
	// Icon is a text icon, such as an emoji. Empty when IconPath is set.
	Icon string
	// IconPath is the absolute path of an icon image.
	IconPath string
	// Color is a CSS color: #hex, rgb(), rgba(), hsl(), hsla() or a name.
	Color string
	// Background is the absolute path of an image shown behind the
	// collection's header.
	Background string
	// Tags are lower case, without a leading #, and unique.
	Tags []string
	// Problems lists what was ignored and why, such as a path that
	// leaves the folder.
	Problems []string
}

// file is .mdsh.yaml as written.
type file struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Icon        string `yaml:"icon"`
	Color       string `yaml:"color"`
	Background  string `yaml:"background"`
	Tags        any    `yaml:"tags"`
}

// Load reads dir's .mdsh.yaml. It returns nil and no error when the
// folder has none, and an error when the file can't be read or isn't a
// YAML mapping; front ends then show the folder as a plain folder.
func Load(dir string) (*Collection, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, FileName)
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", path)
	}
	if fi.Size() > MaxSize {
		return nil, fmt.Errorf("%s: bigger than %d KiB", path, MaxSize>>10)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(dir, src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse checks src, the contents of dir's .mdsh.yaml. Relative paths in
// it are resolved against dir, which must be absolute.
func Parse(dir string, src []byte) (*Collection, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, err
	}
	c := &Collection{Dir: dir}
	if len(doc.Content) == 0 {
		return c, nil // empty file
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("not a mapping of keys to values")
	}
	var f file
	if err := doc.Decode(&f); err != nil {
		// A key of the wrong type, say a list for title, is left empty.
		var te *yaml.TypeError
		if !errors.As(err, &te) {
			return nil, err
		}
		c.Problems = append(c.Problems, te.Errors...)
	}
	c.Title = clip(oneLine(f.Title), MaxText)
	c.Description = clip(oneLine(f.Description), MaxText)
	if icon := strings.TrimSpace(f.Icon); icon != "" {
		if isImage(icon) {
			p, err := inside(dir, icon)
			if err != nil {
				c.Problems = append(c.Problems, "icon: "+err.Error())
			}
			c.IconPath = p
		} else if utf8.RuneCountInString(icon) <= MaxIcon && !strings.ContainsFunc(icon, unicode.IsSpace) {
			c.Icon = icon
		} else {
			c.Problems = append(c.Problems, "icon: not an image file (.svg, .png, .jpg, .gif, .webp) or a short text icon such as an emoji")
		}
	}
	if col := strings.TrimSpace(f.Color); col != "" {
		if ValidColor(col) {
			c.Color = col
		} else {
			c.Problems = append(c.Problems, fmt.Sprintf("color: %q is not a #hex, rgb(), hsl() or named color", col))
		}
	}
	if bg := strings.TrimSpace(f.Background); bg != "" {
		p, err := inside(dir, bg)
		if err != nil {
			c.Problems = append(c.Problems, "background: "+err.Error())
		}
		c.Background = p
	}
	c.Tags = Tags(f.Tags)
	return c, nil
}

// imageExts are the icon and background formats a webview shows.
var imageExts = []string{".svg", ".png", ".jpg", ".jpeg", ".gif", ".webp"}

func isImage(name string) bool {
	return slices.Contains(imageExts, strings.ToLower(filepath.Ext(name)))
}

// inside resolves rel, a forward-slash path relative to dir, and returns
// it if it names a regular file inside dir after following symlinks.
func inside(dir, rel string) (string, error) {
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", fmt.Errorf("%q: must be relative to the folder", rel)
	}
	if !isImage(rel) {
		return "", fmt.Errorf("%q: not an image file (.svg, .png, .jpg, .gif, .webp)", rel)
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	p, err := filepath.EvalSymlinks(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("%q: not found", rel)
	}
	if r, err := filepath.Rel(root, p); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q: outside the folder", rel)
	}
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%q: not a file", rel)
	}
	return p, nil
}

var (
	hexColor  = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	funcColor = regexp.MustCompile(`^(?:rgb|rgba|hsl|hsla)\([0-9.,%/ a-z-]{1,60}\)$`)
	nameColor = regexp.MustCompile(`^[a-zA-Z]{3,24}$`)
)

// ValidColor reports whether s is a color Collection accepts. The check
// is by shape, so it is safe to put s in a style attribute; the browser
// decides whether a name or function is a real color.
func ValidColor(s string) bool {
	return hexColor.MatchString(s) || funcColor.MatchString(s) || nameColor.MatchString(s)
}

// Tags normalizes a tags value from YAML: a list of strings, or one
// string of tags split by commas or spaces, the way Jekyll and Hugo
// front matter allow. Tags are trimmed, lose a leading #, are made
// lower case and are de-duplicated in order; empty or overlong ones are
// dropped.
func Tags(v any) []string {
	var raw []string
	switch v := v.(type) {
	case string:
		raw = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok {
				raw = append(raw, s)
			}
		}
	}
	var tags []string
	for _, t := range raw {
		t = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t == "" || strings.ContainsFunc(t, unicode.IsSpace) || utf8.RuneCountInString(t) > MaxTagSz || slices.Contains(tags, t) {
			continue
		}
		tags = append(tags, t)
		if len(tags) == MaxTags {
			break
		}
	}
	return tags
}

// Merge returns the tags of a, then those of b not in a.
func Merge(a, b []string) []string {
	out := slices.Clone(a)
	for _, t := range b {
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) > n {
		return string([]rune(s)[:n-1]) + "…"
	}
	return s
}
