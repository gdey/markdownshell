package notebook

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// IncludeCell is an include block, as the frontend draws it: a grip
// above and below its lines drags From and To, and a button shows the
// whole file.
type IncludeCell struct {
	// Block is the include's index in the document, for SetIncludeRange
	// and IncludeFile.
	Block int `json:"block"`
	// File is its file= and Lang its language, for highlighting the
	// whole file.
	File string `json:"file"`
	Lang string `json:"lang"`
	// From and To are its from= and to=, as the file holds them: 0 for
	// one it doesn't have, which means the first and the last line.
	From int `json:"from"`
	To   int `json:"to"`
	// Window is how a match= or symbol= include picks its lines, as
	// `match "^FAIL"`, or "" for one that picks them by from= and to=,
	// whose grips then drag them.
	Window string `json:"window,omitempty"`
	// Last is the line in the file its lines end on, from its last
	// read, 0 when that isn't known or nothing matched.
	Last int `json:"last,omitempty"`
	// Warning is why its lines are empty, as no line matching match=,
	// from its last read.
	Warning string `json:"warning,omitempty"`
}

// windowText is how inc picks its lines by content, for
// IncludeCell.Window: "" for from= and to=.
func windowText(inc doc.Include) string {
	var parts []string
	switch {
	case inc.Match != "":
		parts = append(parts, fmt.Sprintf("match %q", inc.Match))
	case inc.Symbol != "":
		parts = append(parts, "symbol "+inc.Symbol)
	case inc.Before == 0 && inc.Lines == 0 && inc.Until == "":
		return ""
	case inc.From > 1:
		parts = append(parts, fmt.Sprintf("from line %d", inc.From))
	}
	if inc.Before > 0 {
		parts = append(parts, fmt.Sprintf("%d before", inc.Before))
	}
	switch {
	case inc.Lines > 0:
		parts = append(parts, fmt.Sprintf("%d lines", inc.Lines))
	case inc.Until != "":
		parts = append(parts, fmt.Sprintf("until %q", inc.Until))
	}
	return strings.Join(parts, ", ")
}

// IncludeFile is the whole of an include's file, for showing it and
// for dragging the include's range over it.
type IncludeFile struct {
	Text string `json:"text"`
	// Lines is how many lines Text has.
	Lines int `json:"lines"`
}

// IncludeFile reads the whole of include block i's file.
func (n *Notebook) IncludeFile(i int) (IncludeFile, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	b, err := n.includeBlock(i)
	if err != nil {
		return IncludeFile{}, err
	}
	data, err := n.sess.IncludeFile(b)
	if err != nil {
		return IncludeFile{}, err
	}
	return IncludeFile{Text: string(data), Lines: run.LineCount(data)}, nil
}

// SetIncludeRange writes from= and to= on include block i, as its grips
// were dragged to. Lines count from 1. from at the first line and to at
// the file's last line, or past it, take the attribute out, so the
// include follows the file as it changes. It is a rendered-view edit, so
// Undo takes it back.
func (n *Notebook) SetIncludeRange(i, from, to int) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.busy() {
		return ErrBusy
	}
	b, err := n.includeBlock(i)
	if err != nil {
		return err
	}
	if inc, err := b.Include(); err == nil && windowText(inc) != "" {
		return fmt.Errorf("its lines are picked by %s, not by from= and to=", windowText(inc))
	}
	if from < 0 || to < 0 {
		return fmt.Errorf("line numbers count from 1")
	}
	if to != 0 && from > to {
		return fmt.Errorf("from=%d is after to=%d", from, to)
	}
	// The file's length bounds the range, when it can be read.
	if data, err := n.sess.IncludeFile(b); err == nil {
		lines := run.LineCount(data)
		if from > lines {
			return fmt.Errorf("from=%d is past the end of the file (%d lines)", from, lines)
		}
		if to >= lines {
			to = 0
		}
	}
	if from <= 1 {
		from = 0
	}
	attr := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	info := doc.SetAttr(doc.SetAttr(b.Info, "from", attr(from)), "to", attr(to))
	if info == b.Info {
		return nil
	}
	at := n.d.InfoSpan(b)
	src := append(append(append([]byte(nil), n.d.Src[:at.Start]...), info...), n.d.Src[at.End:]...)
	return n.change("change include range", "", src, nil)
}

// includeBlock is include block i, for a caller that holds n.mu.
func (n *Notebook) includeBlock(i int) (*doc.Block, error) {
	if i < 0 || i >= len(n.d.Blocks) {
		return nil, fmt.Errorf("no block %d", i)
	}
	b := n.d.Blocks[i]
	if !b.IsInclude() {
		return nil, fmt.Errorf("block %d is not an include", i)
	}
	return b, nil
}
