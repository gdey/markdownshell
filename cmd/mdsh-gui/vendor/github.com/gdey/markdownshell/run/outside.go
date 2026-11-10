package run

import (
	"errors"
	"path/filepath"
	"slices"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/docroot"
)

// Outside lists the files d's include blocks read and its out= files
// write that are outside the document's root, as the document writes
// them, in the order they come. Its result is what opts.AllowOutside
// would allow, so a front end can say so before it allows it: mdsh-gui
// asks once per document and asks again when the list changes. A block
// marked skip is listed too, since an edit can turn it on.
func Outside(d *doc.Document, opts Options) []string {
	if opts.Dir == "" && opts.File != "" {
		opts.Dir = filepath.Dir(opts.File)
	}
	root := docroot.Root(opts.Dir)
	var out []string
	add := func(rel string) {
		if rel == "" || slices.Contains(out, rel) {
			return
		}
		path, err := resolveRes(rel, opts)
		if err != nil {
			// A bad res:// path is an error of its own, not a
			// reach outside.
			return
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.Dir, filepath.FromSlash(path))
		}
		if _, err := docroot.Resolve(root, path); errors.Is(err, docroot.ErrOutside) {
			out = append(out, rel)
		}
	}
	for _, b := range d.Blocks {
		if b.IsInclude() {
			if inc, err := b.Include(); err == nil {
				add(inc.File)
			}
			continue
		}
		switch kind, rel := outKind(b); kind {
		case "image", "screenshot", "recording":
			add(rel)
		}
	}
	return out
}
