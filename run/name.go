package run

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gdey/markdownshell/doc"
)

// NameBlocks gives every runnable block in d that has no id one of its
// own, block-<uuidv7>, written into its fence, so it keeps it when blocks
// move. Call it before saving the document. Its saved output, copies,
// refs and image links follow the new id (see doc.Document.NameBlocks),
// and so do the image links in results. Files saved under the old id in
// <doc>.assets/ are renamed to match. Blocks keep their identity, so a
// session only needs a Sync.
func NameBlocks(d *doc.Document, results []*Result, opts Options) ([]doc.Rename, error) {
	renames, err := d.NameBlocks(Runnable, doc.NewBlockID)
	if err != nil || len(renames) == 0 {
		return renames, err
	}
	old := make(map[string]string, len(renames))
	for _, r := range renames {
		old[r.New] = r.Old
	}
	for _, r := range results {
		if r == nil || r.Image == "" {
			continue
		}
		if o, ok := old[r.Block.ID]; ok {
			re := regexp.MustCompile(`(\.assets/)` + regexp.QuoteMeta(o) + `\.`)
			r.Image = re.ReplaceAllString(r.Image, "${1}"+r.Block.ID+".")
		}
	}
	if opts.Dir == "" && opts.File != "" {
		opts.Dir = filepath.Dir(opts.File)
	}
	dir := filepath.Join(opts.Dir, filepath.FromSlash(filepath.Dir(defaultAsset(&doc.Block{ID: "x"}, "", opts))))
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return renames, nil
	}
	if err != nil {
		return renames, err
	}
	for _, r := range renames {
		for _, f := range files {
			ext, ok := strings.CutPrefix(f.Name(), r.Old)
			if !ok || !strings.HasPrefix(ext, ".") || strings.Contains(ext[1:], ".") {
				continue
			}
			to := filepath.Join(dir, r.New+ext)
			if _, err := os.Lstat(to); err == nil {
				continue
			}
			if err := os.Rename(filepath.Join(dir, f.Name()), to); err != nil {
				return renames, err
			}
		}
	}
	return renames, nil
}
