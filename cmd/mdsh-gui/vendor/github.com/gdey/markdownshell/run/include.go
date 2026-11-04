package run

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gdey/markdownshell/doc"
)

// Includes reads the files d's include blocks name. It returns one result
// per block, like Run, with only the include blocks filled in: Stdout
// holds the selected lines. A relative path is relative to opts.Dir, or
// opts.File's directory; res://name is name in the document's assets
// folder, <doc>.assets.
func Includes(d *doc.Document, opts Options) []*Result {
	if opts.Dir == "" && opts.File != "" {
		opts.Dir = filepath.Dir(opts.File)
	}
	results := make([]*Result, len(d.Blocks))
	for i, b := range d.Blocks {
		results[i] = &Result{Block: b}
		if !b.IsInclude() {
			continue
		}
		if b.Attrs.Has("skip") {
			results[i].Skipped = true
			continue
		}
		includeBlock(b, results[i], opts)
	}
	return results
}

func includeBlock(b *doc.Block, r *Result, opts Options) {
	r.Ran = true
	inc, err := b.Include()
	if err != nil {
		r.Err = err
		return
	}
	path, err := resolveRes(inc.File, opts)
	if err != nil {
		r.Err = fmt.Errorf("include: %w", err)
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(opts.Dir, filepath.FromSlash(path))
	}
	data, err := os.ReadFile(path)
	if pe := (*fs.PathError)(nil); errors.As(err, &pe) {
		err = pe.Err
	}
	if err != nil {
		r.Err = fmt.Errorf("include %s: %w", inc.File, err)
		return
	}
	r.Stdout, r.Err = Lines(data, inc.From, inc.To)
}

// Lines returns lines from through to of data, numbered from 1 and
// inclusive. from 0 is the first line and to 0 the last.
func Lines(data []byte, from, to int) ([]byte, error) {
	from = max(from, 1)
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	if from > n {
		return nil, fmt.Errorf("include: from=%d is past the end of the file (%d lines)", from, n)
	}
	if to == 0 || to > n {
		to = n
	}
	start := 0
	for i := 1; i < from; i++ {
		start += bytes.IndexByte(data[start:], '\n') + 1
	}
	end := start
	for i := from; i <= to && end < len(data); i++ {
		if j := bytes.IndexByte(data[end:], '\n'); j >= 0 {
			end += j + 1
		} else {
			end = len(data)
		}
	}
	return data[start:end], nil
}
