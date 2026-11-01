// Package library finds the markdown files a start screen lists: the
// TUI's and mdsh-gui's.
package library

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gdey/markdownshell/collection"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
	"gopkg.in/yaml.v3"
)

// File is a markdown file under the listed folder.
type File struct {
	// Path is the file's absolute path.
	Path string `json:"path"`
	// Rel is Path relative to the listed folder, with forward slashes.
	Rel string `json:"rel"`
	// Blocks is how many blocks mdsh would run, or -1 if the file
	// doesn't parse.
	Blocks int `json:"blocks"`
	// Description is the file's front matter description, if it has
	// one. See DescriptionKeys.
	Description string `json:"description,omitempty"`
	// Tags are the file's front matter tags plus those of the
	// collections it is in, from the listed folder down. See
	// collection.Tags.
	Tags []string `json:"tags,omitempty"`
	// Mod is when the file last changed.
	Mod time.Time `json:"mod"`
}

// Collection is a listed folder with a .mdsh.yaml file. See package
// collection.
type Collection struct {
	// Rel is the folder relative to the listed folder, with forward
	// slashes; "" is the listed folder itself.
	Rel         string   `json:"rel"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Color       string   `json:"color,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// IconPath and Background are absolute paths of images inside the
	// folder; IconURL and BackgroundURL are where the window loads
	// them, filled in by whoever serves them.
	IconPath      string `json:"-"`
	Background    string `json:"-"`
	IconURL       string `json:"iconURL,omitempty"`
	BackgroundURL string `json:"backgroundURL,omitempty"`
	// Problems are the parts of .mdsh.yaml that were ignored, and why.
	Problems []string `json:"problems,omitempty"`
	// Error, if set, says why .mdsh.yaml was ignored as a whole; the
	// folder shows as a plain folder.
	Error string `json:"error,omitempty"`
}

// Listing is what List finds.
type Listing struct {
	// Files are in path order.
	Files []File
	// Collections are the folders with a .mdsh.yaml, in path order.
	Collections []Collection
	// Full is true when List stopped at MaxFiles.
	Full bool
}

// Limits keep a listing of a big folder, such as home, quick.
const (
	MaxDepth = 6       // folders below root
	MaxFiles = 500     // files listed
	MaxSize  = 1 << 20 // bytes read to count blocks
	MaxDesc  = 300     // runes of a description kept
)

// DescriptionKeys are the front matter keys read for a description, first
// match wins. description is what Jekyll, Hugo, Docusaurus, MkDocs,
// Obsidian Publish, Astro and Quarto use; summary is Hugo's; abstract is
// Pandoc's and Quarto's.
var DescriptionKeys = []string{"description", "summary", "abstract"}

// skipDir is true for folders that hold no runbooks worth listing.
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || strings.HasSuffix(name, ".assets")
}

// List finds the .md and .markdown files under root, in path order, and
// the collections among its folders. It leaves out hidden folders,
// node_modules, vendor and mdsh's .assets folders, goes at most MaxDepth
// folders down and stops at MaxFiles.
func List(root string) (Listing, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Listing{}, err
	}
	var l Listing
	// tags holds each visited folder's inherited tags: its own
	// collection's plus those of the collections above it, up to root.
	tags := map[string][]string{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil // an unreadable folder below root is left out
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if path != root && (skipDir(d.Name()) || strings.Count(rel, string(filepath.Separator)) >= MaxDepth) {
				return filepath.SkipDir
			}
			inherited := tags[filepath.Dir(path)] // nil for root
			tags[path] = inherited
			c, err := collection.Load(path)
			if rel == "." {
				rel = ""
			}
			switch {
			case err != nil:
				l.Collections = append(l.Collections, Collection{Rel: filepath.ToSlash(rel), Error: err.Error()})
			case c != nil:
				tags[path] = collection.Merge(inherited, c.Tags)
				l.Collections = append(l.Collections, Collection{
					Rel: filepath.ToSlash(rel), Title: c.Title, Description: c.Description,
					Icon: c.Icon, Color: c.Color, Tags: c.Tags,
					IconPath: c.IconPath, Background: c.Background, Problems: c.Problems,
				})
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" || !d.Type().IsRegular() {
			return nil
		}
		if len(l.Files) == MaxFiles {
			l.Full = true
			return filepath.SkipAll
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		n, desc, own := read(path, fi.Size())
		l.Files = append(l.Files, File{
			Path: path, Rel: filepath.ToSlash(rel), Blocks: n, Description: desc,
			Tags: collection.Merge(tags[filepath.Dir(path)], own), Mod: fi.ModTime(),
		})
		return nil
	})
	slices.SortFunc(l.Files, func(a, b File) int { return strings.Compare(a.Rel, b.Rel) })
	slices.SortFunc(l.Collections, func(a, b Collection) int { return strings.Compare(a.Rel, b.Rel) })
	return l, err
}

// read returns how many blocks mdsh would run in the file at path, or -1
// if it doesn't parse, its description and its front matter tags.
func read(path string, size int64) (int, string, []string) {
	if size > MaxSize {
		return -1, "", nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return -1, "", nil
	}
	d, err := doc.Parse(src)
	if err != nil {
		return -1, "", nil
	}
	n := 0
	for _, b := range d.Blocks {
		if run.Runnable(b) {
			n++
		}
	}
	fm := frontMatter(src[d.FrontMatter.Start:d.FrontMatter.End])
	return n, description(fm), collection.Tags(fm["tags"])
}

// frontMatter parses the front matter fm, both --- lines included.
func frontMatter(fm []byte) map[string]any {
	// Drop the opening and closing lines.
	_, body, ok := bytes.Cut(fm, []byte("\n"))
	if !ok {
		return nil
	}
	body = bytes.TrimRight(body, "\r\n")
	if i := bytes.LastIndexByte(body, '\n'); i >= 0 {
		body = body[:i]
	} else {
		body = nil
	}
	var m map[string]any
	if yaml.Unmarshal(body, &m) != nil {
		return nil
	}
	return m
}

// description returns the first of DescriptionKeys in the front matter m
// that holds text. Its white space is collapsed and it is cut to MaxDesc
// runes.
func description(m map[string]any) string {
	for _, k := range DescriptionKeys {
		s, _ := m[k].(string)
		if s = strings.Join(strings.Fields(s), " "); s == "" {
			continue
		}
		if utf8.RuneCountInString(s) > MaxDesc {
			s = string([]rune(s)[:MaxDesc-1]) + "…"
		}
		return s
	}
	return ""
}
