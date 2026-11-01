package notebook

import (
	"fmt"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// SetDepends sets block i's depends= to the blocks deps names, by index
// and in order, or takes the attribute out when deps is empty. It is a
// rendered-view edit, so Undo takes it back. Blocks without ids get
// them first, as saving would give them, since depends= names blocks by
// id. Names already there that aren't a block that runs, which mdsh
// ignores, are kept, after the blocks deps names, so a typo isn't lost.
// A cycle comes back as its parse error and changes nothing, and a
// textbox or a choice, which never runs, is refused.
func (n *Notebook) SetDepends(i int, deps []int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running || len(n.bg) > 0 {
		return ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) {
		return fmt.Errorf("no block %d", i)
	}
	for _, j := range deps {
		if j < 0 || j >= len(n.d.Blocks) {
			return fmt.Errorf("no block %d", j)
		}
		if j == i {
			return fmt.Errorf("block %s can't depend on itself", n.d.Label(n.d.Blocks[i]))
		}
		if d := n.d.Blocks[j]; d.IsInput() {
			return fmt.Errorf("block %s is a %s, which never runs", n.d.Label(d), d.Lang)
		}
	}
	if n.unnamed() {
		if _, err := run.NameBlocks(n.d, n.results, run.Options{File: n.file}); err != nil {
			return err
		}
		n.dirty = true
		if err := n.sync(); err != nil {
			return err
		}
	}
	b := n.d.Blocks[i]
	ids := make([]string, len(deps))
	for k, j := range deps {
		ids[k] = n.d.Blocks[j].ID
	}
	for _, id := range b.Depends() {
		if n.d.Dep(id) == nil {
			ids = append(ids, id)
		}
	}
	info := doc.SplitInfo(b.Info)
	if len(ids) == 0 {
		info.Del("depends")
	} else {
		info.Set("depends", strings.Join(ids, ","), false)
	}
	at := n.d.InfoSpan(b)
	src := append(append(append([]byte(nil), n.d.Src[:at.Start]...), info.String()...), n.d.Src[at.End:]...)
	what := "change dependency"
	switch was := len(b.Depends()); {
	case len(ids) > was:
		what = "add dependency"
	case len(ids) < was:
		what = "remove dependency"
	}
	return n.change(what, "", src, nil)
}
