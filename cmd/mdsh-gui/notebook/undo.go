package notebook

import (
	"bytes"
	"errors"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// undoMax is how many rendered-view edits Undo can take back.
const undoMax = 100

// change is one rendered-view edit, as Undo and Redo see it: the file
// before and after, and what it did to the output of blocks that ran
// this session, which saving writes in place of the file's.
type change struct {
	// What names the edit for the status line, as "check task". Edits
	// with the same key one after another, such as typing in a textbox,
	// are one change.
	what, key     string
	before, after []byte
	outs          []outChange
	res           []resChange
}

// resChange is a block's result swapped for another, as clearing its
// output swaps it for one that never ran.
type resChange struct {
	b             *doc.Block
	before, after *run.Result
}

// outChange is a block's stdout before and after a change.
type outChange struct {
	r             *run.Result
	before, after []byte
}

// ErrStale is returned by Undo and Redo when the file has changed since
// the edit, by a run, the source view or reloading, so taking it back
// would lose that change. Both stacks are emptied.
var ErrStale = errors.New("the file changed since, so that edit can't be undone here")

// change applies src as a rendered-view edit that Undo can take back.
// outs are the changes the edit made to results' output. The caller
// holds n.mu.
func (n *Notebook) change(what, key string, src []byte, outs []outChange) error {
	before := n.d.Src
	if bytes.Equal(src, before) {
		return nil
	}
	if err := n.setSource(src); err != nil {
		return err
	}
	n.redo = nil
	if len(n.undo) > 0 {
		top := &n.undo[len(n.undo)-1]
		if key != "" && top.key == key && bytes.Equal(top.after, before) {
			top.after = n.d.Src
			top.outs = append(top.outs, outs...)
			return nil
		}
	}
	n.undo = append(n.undo, change{what: what, key: key, before: before, after: n.d.Src, outs: outs})
	if len(n.undo) > undoMax {
		n.undo = n.undo[len(n.undo)-undoMax:]
	}
	return nil
}

// Undo takes back the last rendered-view edit, such as checking a task
// or picking a choice, and says what it was. It returns "" when there is
// nothing to undo, and ErrStale when the file changed since.
func (n *Notebook) Undo() (string, error) {
	return n.step(&n.undo, &n.redo, false)
}

// Redo does again what Undo took back.
func (n *Notebook) Redo() (string, error) {
	return n.step(&n.redo, &n.undo, true)
}

// step takes the change on top of from, applies it backwards (or
// forwards, for redo) and puts it on to.
func (n *Notebook) step(from, to *[]change, redo bool) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return "", ErrBusy
	}
	if len(*from) == 0 {
		return "", nil
	}
	c := (*from)[len(*from)-1]
	now, then := c.after, c.before
	if redo {
		now, then = then, now
	}
	if !bytes.Equal(n.d.Src, now) {
		n.undo, n.redo = nil, nil
		return "", ErrStale
	}
	if err := n.setSource(then); err != nil {
		return "", err
	}
	if redo {
		for _, o := range c.outs {
			if bytes.Equal(o.r.Stdout, o.before) {
				o.r.Stdout = o.after
			}
		}
	} else {
		for i := len(c.outs) - 1; i >= 0; i-- {
			if o := c.outs[i]; bytes.Equal(o.r.Stdout, o.after) {
				o.r.Stdout = o.before
			}
		}
	}
	for _, rc := range c.res {
		was, now := rc.after, rc.before
		if redo {
			was, now = now, was
		}
		if i, ok := n.index[rc.b]; ok && n.results[i] == was {
			n.results[i] = now
		}
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, c)
	return c.what, nil
}
