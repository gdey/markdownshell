package collection

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Settings are the keys of a .mdsh.yaml as written, for a form to edit:
// paths stay relative and nothing is checked. See Collection for what
// each key means.
type Settings struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Color       string   `json:"color"`
	Background  string   `json:"background"`
	Tags        []string `json:"tags"`
}

// keys are Settings' keys, in the order Write adds missing ones.
var keys = []string{"title", "description", "icon", "color", "background", "tags"}

func (s Settings) value(key string) string {
	switch key {
	case "title":
		return s.Title
	case "description":
		return s.Description
	case "icon":
		return s.Icon
	case "color":
		return s.Color
	case "background":
		return s.Background
	}
	return ""
}

// Read returns dir's .mdsh.yaml as written. A folder without one reads
// as empty Settings. It fails when the file can't be read or isn't a
// YAML mapping, since Write would then lose what is in it.
func Read(dir string) (Settings, error) {
	doc, err := readDoc(dir)
	if err != nil || doc == nil {
		return Settings{}, err
	}
	var f file
	if err := doc.Decode(&f); err != nil {
		var te *yaml.TypeError
		if !errors.As(err, &te) {
			return Settings{}, err
		}
	}
	return Settings{
		Title:       strings.TrimSpace(f.Title),
		Description: strings.TrimSpace(f.Description),
		Icon:        strings.TrimSpace(f.Icon),
		Color:       strings.TrimSpace(f.Color),
		Background:  strings.TrimSpace(f.Background),
		Tags:        Tags(f.Tags),
	}, nil
}

// readDoc parses dir's .mdsh.yaml, or returns nil when there is none.
func readDoc(dir string) (*yaml.Node, error) {
	path := filepath.Join(dir, FileName)
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", FileName)
	}
	if fi.Size() > MaxSize {
		return nil, fmt.Errorf("%s: bigger than %d KiB", FileName, MaxSize>>10)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: not a mapping of keys to values", FileName)
	}
	return &doc, nil
}

// Write saves s as dir's .mdsh.yaml. Keys s leaves empty are taken out;
// other keys and comments in the file are kept. It saves nothing when
// Parse would ignore part of s, such as an icon that isn't in the
// folder, and says why. When the file would be left with no keys and no
// comments, it is removed, so the folder shows as a plain folder again.
func Write(dir string, s Settings) error {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	doc, err := readDoc(dir)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	m := doc.Content[0]
	tags := Tags(anySlice(s.Tags))
	for _, k := range keys {
		var v *yaml.Node
		if k == "tags" {
			if len(tags) > 0 {
				v = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
				for _, t := range tags {
					v.Content = append(v.Content, str(t))
				}
			}
		} else if val := strings.TrimSpace(s.value(k)); val != "" {
			v = str(val)
		}
		set(m, k, v)
	}

	path := filepath.Join(dir, FileName)
	if len(m.Content) == 0 && doc.HeadComment == "" && doc.FootComment == "" && m.HeadComment == "" && m.FootComment == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	enc.Close()
	out := []byte(b.String())
	if len(out) > MaxSize {
		return fmt.Errorf("%s would be bigger than %d KiB", FileName, MaxSize>>10)
	}
	c, err := Parse(dir, out)
	if err != nil {
		return err
	}
	if len(c.Problems) > 0 {
		return errors.New(strings.Join(c.Problems, "; "))
	}
	return writeFile(path, out)
}

func anySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// str is a string scalar, quoted when YAML would read it as something
// else.
func str(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }

// set puts v under key in the mapping m, keeping the key's place and
// comments, or takes the key out when v is nil.
func set(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value != key {
			continue
		}
		if v == nil {
			m.Content = slices.Delete(m.Content, i, i+2)
			return
		}
		old := m.Content[i+1]
		v.LineComment, v.HeadComment, v.FootComment = old.LineComment, old.HeadComment, old.FootComment
		m.Content[i+1] = v
		return
	}
	if v != nil {
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
	}
}

// writeFile replaces path with data through a temporary file in the
// same folder, so a crash leaves the old file or the new one. It keeps
// the old file's permissions.
func writeFile(path string, data []byte) error {
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), FileName+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Writable reports whether Write can save dir's .mdsh.yaml: the folder
// can be written to and the file, if there is one, is a regular file
// that can be written too.
func Writable(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() || !canWrite(dir) {
		return false
	}
	path := filepath.Join(dir, FileName)
	fi, err = os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	return err == nil && fi.Mode().IsRegular() && canWrite(path)
}

// Images lists up to max image files in dir and the folders below it, as
// forward-slash paths relative to dir, for a form to offer as an icon
// or background. Hidden folders, and those more than two
// below dir, are left out.
func Images(dir string, max int) []string {
	var out []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			if path != dir && (strings.HasPrefix(d.Name(), ".") || strings.Count(rel, string(filepath.Separator)) >= 2) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isImage(d.Name()) || !d.Type().IsRegular() {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		if len(out) == max {
			return filepath.SkipAll
		}
		return nil
	})
	return out
}
