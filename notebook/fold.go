package notebook

import (
	"fmt"
	"slices"

	"github.com/gdey/markdownshell/doc"
)

// Fold is a fold made by hand in the rendered view, for SetFolds to
// write to the file.
type Fold struct {
	// Block is the block whose code, or with Output its output, folds,
	// or -1 for a section.
	Block  int  `json:"block"`
	Output bool `json:"output"`
	// Heading is where a section's heading starts in the file: its
	// cell's From.
	Heading int  `json:"heading"`
	On      bool `json:"on"`
}

// foldLine is the line SetFolds puts under a heading to fold its section.
const foldLine = "<!-- mdsh:collapse -->\n"

// SetFolds writes folds to the file, so it opens with them: a block's
// code folds with a bare collapse attribute and its output with a bare
// collapse-output, and a section with an mdsh:collapse line under its
// heading. Unfolding takes them out. Where the file folds by a
// condition, as with collapse={{$?}}!=0 or an mdsh:collapse line with a
// condition in the section, it is left as it is. Undo takes the change
// back.
func (n *Notebook) SetFolds(folds []Fold) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	d := n.d
	type edit struct {
		at   doc.Span
		with string
	}
	var edits []edit
	infos := map[int]string{} // block: its new info string
	var cells []Cell
	on, off := 0, 0
	for _, f := range folds {
		if f.On {
			on++
		} else {
			off++
		}
		if f.Block >= 0 {
			if f.Block >= len(d.Blocks) {
				return fmt.Errorf("no block %d", f.Block)
			}
			b := d.Blocks[f.Block]
			key := "collapse"
			if f.Output {
				if b.IsInclude() {
					continue // an include block can't have collapse-output
				}
				key = "collapse-output"
			}
			info, ok := infos[f.Block]
			if !ok {
				info = b.Info
			}
			infos[f.Block] = doc.SetFlag(info, key, f.On)
			continue
		}
		if cells == nil {
			cells = n.cells()
		}
		i := slices.IndexFunc(cells, func(c Cell) bool { return c.Heading > 0 && c.From == f.Heading })
		if i < 0 {
			return fmt.Errorf("no heading at %d", f.Heading)
		}
		// The section's mdsh:collapse lines are those up to the next
		// heading, of any level, as Cells reads them.
		end := len(d.Src)
		if j := slices.IndexFunc(cells[i+1:], func(c Cell) bool { return c.Heading > 0 }); j >= 0 {
			end = cells[i+1+j].From
		}
		var mine []*doc.Fold
		conditional, folded := false, false
		for _, l := range d.Folds {
			if l.Span.Start >= f.Heading && l.Span.Start < end {
				mine = append(mine, l)
				if l.Cond.Ref != nil {
					conditional = true
				} else if l.Cond.Eval(nil) {
					folded = true
				}
			}
		}
		if conditional || f.On && folded {
			continue
		}
		for _, l := range mine {
			edits = append(edits, edit{l.Span, ""})
		}
		if f.On {
			at, line := cells[i].To, foldLine
			if at > 0 && d.Src[at-1] != '\n' {
				line = "\n" + line
			}
			edits = append(edits, edit{doc.Span{Start: at, End: at}, line})
		}
	}
	for i, info := range infos {
		b := d.Blocks[i]
		if info == b.Info {
			continue
		}
		edits = append(edits, edit{d.InfoSpan(b), info})
	}
	// Splice from the end, so each edit's span still holds. An insertion
	// goes after a removal at the same place.
	slices.SortStableFunc(edits, func(a, b edit) int { return b.at.Start - a.at.Start })
	src := d.Src
	for _, e := range edits {
		src = append(append(append([]byte(nil), src[:e.at.Start]...), e.with...), src[e.at.End:]...)
	}
	what := "change folds"
	switch {
	case on == 0:
		what = "unfold"
	case off == 0:
		what = "fold"
	}
	return n.change(what, "", src, nil)
}
