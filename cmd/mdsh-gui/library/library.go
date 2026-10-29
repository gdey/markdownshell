// Package library finds the markdown files the start screen lists.
package library

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
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
	// Mod is when the file last changed.
	Mod time.Time `json:"mod"`
}

// Limits keep a listing of a big folder, such as home, quick.
const (
	MaxDepth = 6       // folders below root
	MaxFiles = 500     // files listed
	MaxSize  = 1 << 20 // bytes read to count blocks
)

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
		files = append(files, File{Path: path, Rel: filepath.ToSlash(rel), Blocks: blocks(path, fi.Size()), Mod: fi.ModTime()})
		return nil
	})
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Rel, b.Rel) })
	return files, full, err
}

func blocks(path string, size int64) int {
	if size > MaxSize {
		return -1
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	d, err := doc.Parse(src)
	if err != nil {
		return -1
	}
	n := 0
	for _, b := range d.Blocks {
		if run.Runnable(b) {
			n++
		}
	}
	return n
}
