package notebook

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// Deleted is what a block delete did.
type Deleted struct {
	// Name is how the status line names the block, as #id.
	Name string `json:"name"`
	// Warnings are what the delete left behind, such as output copies of
	// the block elsewhere in the file.
	Warnings []string `json:"warnings,omitempty"`
}

// DeleteBlock takes top-level block i out of the file, with its output
// and tape. It is a rendered-view edit, so Undo puts it back. Blocks
// without ids get them first if the delete would renumber them, as
// saving would give them, so nothing that names a block changes what it
// names.
func (n *Notebook) DeleteBlock(i int) (Deleted, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running || len(n.bg) > 0 {
		return Deleted{}, ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) {
		return Deleted{}, fmt.Errorf("no block %d", i)
	}
	del, src, err := n.tryDelete(i)
	if errors.Is(err, doc.ErrRenumber) && n.unnamed() {
		if _, nerr := run.NameBlocks(n.d, n.results, run.Options{File: n.file}); nerr != nil {
			return Deleted{}, nerr
		}
		n.dirty = true
		if serr := n.sync(); serr != nil {
			return Deleted{}, serr
		}
		del, src, err = n.tryDelete(i)
	}
	if err != nil {
		return Deleted{}, err
	}
	return del, n.change("delete block", "", src, nil)
}

// tryDelete plans the delete on a copy of the document.
func (n *Notebook) tryDelete(i int) (Deleted, []byte, error) {
	d, err := doc.Parse(n.d.Src)
	if err != nil {
		return Deleted{}, nil, err
	}
	b := d.Blocks[i]
	del := Deleted{Name: d.Label(b)}
	if b.Named() && !doc.GeneratedID(b.ID) {
		del.Name = "#" + b.ID
	}
	w, err := d.DeleteBlock(b)
	if err != nil {
		return Deleted{}, nil, err
	}
	del.Warnings = w
	return del, d.Src, nil
}

// ClearOutput takes top-level block i's output out of the file, the
// output fences attached to it, and forgets this session's run of it, so
// saving doesn't write the output back. The shell keeps what the run
// did. It is a rendered-view edit, so Undo puts the output back.
func (n *Notebook) ClearOutput(i int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) {
		return fmt.Errorf("no block %d", i)
	}
	b := n.d.Blocks[i]
	if n.bg[b] {
		return ErrBusy
	}
	if b.IsInclude() {
		return errors.New("an include's output is its file's text, so it can't be cleared")
	}
	old := n.results[i]
	if !b.HasOutput() && !old.Ran {
		return nil
	}
	src := append(append([]byte(nil), n.d.Src[:b.Output.Start]...), n.d.Src[b.Output.End:]...)
	edited := !bytes.Equal(src, n.d.Src)
	if err := n.change("clear output", "", src, nil); err != nil {
		return err
	}
	fresh := &run.Result{Block: b}
	n.results[n.index[b]] = fresh
	n.dirty = true
	if edited && len(n.undo) > 0 {
		top := &n.undo[len(n.undo)-1]
		top.res = append(top.res, resChange{b, old, fresh})
	}
	return nil
}
