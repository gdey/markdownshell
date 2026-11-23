package run

import (
	"bytes"
	"path/filepath"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/docroot"
)

// Spill is, for a block with out=file, the file its stdout went to. Its
// output fence is an include of the file, showing a window of it.
type Spill struct {
	// File is the file= the output fence names: res://<id>.log, or the
	// path out=file:path gives.
	File string
	// Truncated is set when the output was longer than docroot.MaxRead,
	// so the rest was dropped. Stdout holds what the file does.
	Truncated bool
	// Lines is how many lines the window shows from the top when the
	// fence has no window attributes: the front matter's fold_output, 0
	// for the whole file.
	Lines int
}

// Window is the part of the output the fence shows, given the fence's
// window attributes (see doc.WindowAttrs).
func (s *Spill) Window(r *Result, window doc.Attrs) (Win, doc.Include, error) {
	inc, err := doc.SpillInclude(s.File, window, s.Lines)
	if err != nil {
		return Win{}, inc, err
	}
	if len(r.Stdout) == 0 {
		return Win{}, inc, nil
	}
	w, err := Window(r.Stdout, inc, "")
	return w, inc, err
}

// spillLines is how many lines an out=file window shows by default.
func spillLines(d *doc.Document) int {
	if d.Options.FoldOutput != nil {
		return *d.Options.FoldOutput
	}
	return doc.DefaultFoldOutput
}

// spill writes block b's stdout, r.Stdout, to its out=file file and sets
// r.Spill. The file is written only when what it holds differs, so a run
// that printed the same leaves it alone. A nosave block's output is
// never written to the folder: the window shows from memory.
func spill(d *doc.Document, b *doc.Block, r *Result, opts Options) error {
	s := &Spill{File: SpillFile(b), Lines: spillLines(d)}
	r.Spill = s
	if len(r.Stdout) > docroot.MaxRead {
		r.Stdout = r.Stdout[:docroot.MaxRead]
		if i := bytes.LastIndexByte(r.Stdout, '\n'); i >= 0 {
			r.Stdout = r.Stdout[:i+1]
		}
		s.Truncated = true
	}
	if b.NoSave() {
		return nil
	}
	rel, err := resolveRes(s.File, opts)
	if err != nil {
		return err
	}
	dir := opts.Dir
	if dir == "" && opts.File != "" {
		dir = filepath.Dir(opts.File)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	root := docRootFor(rel, opts)
	if old, err := docroot.ReadFile(root, path, 0); err == nil && bytes.Equal(old, r.Stdout) {
		return nil
	}
	return writeAsset(path, r.Stdout, root)
}

// spillLink is the link to sp's file as an image link would be written:
// relative to opts.LinkDir when set, else to opts.Dir.
func (s *Session) spillLink(sp *Spill) string {
	rel, err := resolveRes(sp.File, s.opts)
	if err != nil || s.opts.LinkDir == "" {
		return rel
	}
	abs, err := filepath.Abs(filepath.Join(s.opts.Dir, filepath.FromSlash(rel)))
	if err != nil {
		return ""
	}
	link, err := filepath.Rel(s.opts.LinkDir, abs)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(link)
}

// refillIncludes reads again the include blocks that show a file block b
// just wrote, wherever they are in the document, so one above b, or
// read before b ran, doesn't show the run before's file; and says so
// with a Done event for each.
func (s *Session) refillIncludes(b *doc.Block) {
	for i, ib := range s.d.Blocks {
		if !ib.IsInclude() || ib.Attrs.Has("skip") {
			continue
		}
		inc, err := ib.Include()
		if err != nil {
			continue
		}
		rel, err := resolveRes(inc.File, s.opts)
		if err != nil || !ownAsset(b, rel, s.opts) {
			continue
		}
		r := s.readInclude(ib)
		s.results[i] = r
		s.emit(Event{Kind: Done, Block: ib, Result: r})
	}
}

// SpillFile is the file= block b's output fence names, for a block with
// out=file: its out=file path, else res://<id>.log.
func SpillFile(b *doc.Block) string {
	if _, arg := outKind(b); arg != "" {
		return arg
	}
	return ResScheme + b.ID + ".log"
}

// SpillFile reads the file block b, which has out=file, wrote its output
// to, under the rules an include's file is read by.
func (s *Session) SpillFile(b *doc.Block) ([]byte, error) {
	return IncludeFile(&doc.Block{Attrs: doc.Attrs{doc.IncludeAttr: "", "file": SpillFile(b)}}, s.opts)
}
