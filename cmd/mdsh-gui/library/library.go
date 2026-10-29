// Package library finds the markdown files the start screen lists.
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
	// Mod is when the file last changed.
	Mod time.Time `json:"mod"`
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

// List finds the .md and .markdown files under root, in path order. It
// leaves out hidden folders, node_modules, vendor and mdsh's .assets
// folders, goes at most MaxDepth folders down and stops at MaxFiles.
// The second result is true when it stopped early.
func List(root string) ([]File, bool, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, false, err
	}
	var files []File
	full := false
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
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" || !d.Type().IsRegular() {
			return nil
		}
		if len(files) == MaxFiles {
			full = true
			return filepath.SkipAll
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		n, desc := read(path, fi.Size())
		files = append(files, File{Path: path, Rel: filepath.ToSlash(rel), Blocks: n, Description: desc, Mod: fi.ModTime()})
		return nil
	})
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Rel, b.Rel) })
	return files, full, err
}

// read returns how many blocks mdsh would run in the file at path, or -1
// if it doesn't parse, and its description.
func read(path string, size int64) (int, string) {
	if size > MaxSize {
		return -1, ""
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return -1, ""
	}
	d, err := doc.Parse(src)
	if err != nil {
		return -1, ""
	}
	n := 0
	for _, b := range d.Blocks {
		if run.Runnable(b) {
			n++
		}
	}
	return n, description(src[d.FrontMatter.Start:d.FrontMatter.End])
}

// description returns the first of DescriptionKeys in the front matter fm,
// both --- lines included, that holds text. Its white space is collapsed
// and it is cut to MaxDesc runes.
func description(fm []byte) string {
	// Drop the opening and closing lines.
	_, body, ok := bytes.Cut(fm, []byte("\n"))
	if !ok {
		return ""
	}
	body = bytes.TrimRight(body, "\r\n")
	if i := bytes.LastIndexByte(body, '\n'); i >= 0 {
		body = body[:i]
	} else {
		body = nil
	}
	var m map[string]any
	if yaml.Unmarshal(body, &m) != nil {
		return ""
	}
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
