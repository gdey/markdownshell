package notebook

import (
	"bytes"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/gdey/markdownshell/doc"
)

// The source pane and Go trade the file's text as line edits rather
// than whole texts, the way a Neovim UI hears nvim_buf_lines_event (see
// docs/design/ui-protocol.md): lines [First, Last) became Text. Each
// change to the file, whoever made it, moves its Version on by one tick,
// and an edit names the version it was made against, so one made
// against an older text is turned down with the line events since,
// which the pane folds in around what was typed before it tries again.
//
// A line here is the text up to and including a newline, or the text
// after the last newline if the file doesn't end with one, so "a\nb" is
// the lines "a\n" and "b", and putting lines back together is joining
// them. Line numbers count from 0.

// Version names a state of the file's text: Doc, which document it is,
// as two tabs, or a file opened again, have their own ticks; and Tick,
// how many changes it has seen.
type Version struct {
	Doc  int64 `json:"doc"`
	Tick int   `json:"tick"`
}

// LineEvent says lines [First, Last) of the text became Text, taking
// the file to Tick. Text is whole lines, so it ends with a newline
// unless it ends the file.
type LineEvent struct {
	Tick  int    `json:"tick"`
	First int    `json:"first"`
	Last  int    `json:"last"`
	Text  string `json:"text"`
}

// LineEdit is a LineEvent without its tick: what the source pane asks
// to change.
type LineEdit struct {
	First int    `json:"first"`
	Last  int    `json:"last"`
	Text  string `json:"text"`
}

// Lines is how the file changed after a version: Version is where it is
// now, and Events take a text at the version asked about there, in
// order. Reset is set when the events since that version are no longer
// kept, or it is another document's: the whole text is to be read
// again.
type Lines struct {
	Version Version     `json:"version"`
	Events  []LineEvent `json:"events"`
	Reset   bool        `json:"reset"`
}

// EditReply is Edit's answer: Applied says the edits were made. When
// they weren't, because the file changed since the version they were
// made against, Events and Reset say how, as Lines does.
type EditReply struct {
	Applied bool        `json:"applied"`
	Version Version     `json:"version"`
	Events  []LineEvent `json:"events"`
	Reset   bool        `json:"reset"`
}

// keepEvents and keepBytes bound the line events a notebook keeps for
// edits made against an older text to catch up with: a run that writes
// megabytes of output is cheaper read whole.
const (
	keepEvents = 512
	keepBytes  = 4 << 20
)

// docs numbers documents for Version.Doc.
var docs atomic.Int64

// lineLog is a notebook's record of the text's changes, as line events.
// seen is the source as of tick: every change goes through a new slice
// (the undo stack relies on that too), so a source that is the same
// slice hasn't changed.
type lineLog struct {
	doc    int64
	tick   int
	seen   []byte
	events []LineEvent
	size   int
}

func newLineLog(src []byte) lineLog {
	return lineLog{doc: docs.Add(1), seen: src}
}

// ErrBadEdit is an edit whose lines aren't in the text, or that overlaps
// the one before it.
var ErrBadEdit = errors.New("edit lines out of order or out of range")

// Version is the text's version now.
func (n *Notebook) Version() Version {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.noteLines()
}

// TextAt is the whole file and its version, for a source pane opening
// on it.
func (n *Notebook) TextAt() (string, Version) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return string(n.d.Src), n.noteLines()
}

// LinesSince is how the text changed after v.
func (n *Notebook) LinesSince(v Version) Lines {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := n.noteLines()
	ev, ok := n.lines.since(v)
	return Lines{Version: now, Events: ev, Reset: !ok}
}

// Edit makes edits, as typed in the source pane against the text at v,
// if the text is still at v. Edits are in order, don't overlap, and
// each names lines of the text at v. Blocks whose code didn't change
// keep their output, as with SetText.
func (n *Notebook) Edit(v Version, edits []LineEdit) (EditReply, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := n.noteLines()
	if n.running {
		return EditReply{Version: now}, ErrBusy
	}
	if v != now {
		ev, ok := n.lines.since(v)
		return EditReply{Version: now, Events: ev, Reset: !ok}, nil
	}
	src, err := applyLines(n.d.Src, edits)
	if err != nil {
		return EditReply{Version: now}, err
	}
	if err := n.setSource(src); err != nil {
		return EditReply{Version: now}, err
	}
	// The events are the edits last first, so each one's lines are
	// those of the text the one before it left.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		n.lines.add(e.First, e.Last, e.Text)
	}
	n.lines.seen = src
	return EditReply{Applied: true, Version: n.noteLines()}, nil
}

// noteLines brings the line log up to the source, with n.mu held, and
// returns the version. A change made some other way than Edit, such as
// a run writing its output, is one event: the lines from the first that
// differs to the last.
func (n *Notebook) noteLines() Version {
	l := &n.lines
	src := n.d.Src
	if sameSlice(src, l.seen) {
		return l.version()
	}
	if !bytes.Equal(src, l.seen) {
		first, last, text := lineDiff(l.seen, src)
		l.add(first, last, text)
	}
	l.seen = src
	return l.version()
}

func (l *lineLog) version() Version { return Version{Doc: l.doc, Tick: l.tick} }

// add notes an event, dropping the oldest once there are too many.
func (l *lineLog) add(first, last int, text string) {
	l.tick++
	l.events = append(l.events, LineEvent{Tick: l.tick, First: first, Last: last, Text: text})
	l.size += len(text)
	drop := 0
	for len(l.events)-drop > keepEvents || (l.size > keepBytes && drop < len(l.events)) {
		l.size -= len(l.events[drop].Text)
		drop++
	}
	if drop > 0 {
		l.events = append([]LineEvent(nil), l.events[drop:]...)
	}
}

// since is the events after v, and whether they are all kept.
func (l *lineLog) since(v Version) ([]LineEvent, bool) {
	if v.Doc != l.doc || v.Tick > l.tick {
		return nil, false
	}
	if v.Tick == l.tick {
		return nil, true
	}
	if len(l.events) == 0 || l.events[0].Tick > v.Tick+1 {
		return nil, false
	}
	return append([]LineEvent(nil), l.events[v.Tick+1-l.events[0].Tick:]...), true
}

// sameSlice says a and b are the same bytes in memory.
func sameSlice(a, b []byte) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

// lineOffset is where line i starts in src, or len(src) for the line
// past the last, and the line from which to look for the next: from is
// line at, which starts at pos.
func lineOffset(src []byte, i, at, pos int) (int, error) {
	for ; at < i; at++ {
		if pos >= len(src) {
			return 0, fmt.Errorf("%w: line %d of %d", ErrBadEdit, i, at)
		}
		nl := bytes.IndexByte(src[pos:], '\n')
		if nl < 0 {
			pos = len(src)
		} else {
			pos += nl + 1
		}
	}
	return pos, nil
}

// applyLines makes edits on src, as Edit says, into a new slice.
func applyLines(src []byte, edits []LineEdit) ([]byte, error) {
	out := make([]byte, 0, len(src)+64)
	line, pos := 0, 0 // the line pos starts, in src
	for _, e := range edits {
		if e.First < line || e.Last < e.First {
			return nil, ErrBadEdit
		}
		from, err := lineOffset(src, e.First, line, pos)
		if err != nil {
			return nil, err
		}
		to, err := lineOffset(src, e.Last, e.First, from)
		if err != nil {
			return nil, err
		}
		if e.Text != "" && e.Text[len(e.Text)-1] != '\n' && to < len(src) {
			return nil, fmt.Errorf("%w: lines %d to %d don't end the file, so must end with a newline", ErrBadEdit, e.First, e.Last)
		}
		if e.Text != "" && from == len(src) && from > 0 && src[from-1] != '\n' {
			return nil, fmt.Errorf("%w: lines put after a last line with no newline would join it", ErrBadEdit)
		}
		out = append(out, src[pos:from]...)
		out = append(out, e.Text...)
		line, pos = e.Last, to
	}
	out = append(out, src[pos:]...)
	return out, nil
}

// lineDiff is the one line event that takes a to b: the lines from the
// first that differs to the last.
func lineDiff(a, b []byte) (first, last int, text string) {
	n := min(len(a), len(b))
	p := 0
	for p < n && a[p] == b[p] {
		p++
	}
	// Back to the start of the line p is in.
	start := bytes.LastIndexByte(a[:p], '\n') + 1
	s := 0
	for s < n-start && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	// The tail kept is whole lines in both: it starts a line, or is
	// nothing.
	lineAt := func(t []byte, p int) bool { return p == 0 || t[p-1] == '\n' }
	for s > 0 && !(lineAt(a, len(a)-s) && lineAt(b, len(b)-s)) {
		s--
	}
	aEnd, bEnd := len(a)-s, len(b)-s
	first = bytes.Count(a[:start], []byte{'\n'})
	last = first + countLines(a[start:aEnd])
	return first, last, string(b[start:bEnd])
}

// countLines is how many lines text is, the last without its newline
// counted.
func countLines(text []byte) int {
	c := bytes.Count(text, []byte{'\n'})
	if len(text) > 0 && text[len(text)-1] != '\n' {
		c++
	}
	return c
}

// BlocksAt lists the blocks in the file, as SourceBlocks does for a
// text, with the version of the text they are in, so the source pane can
// complete depends= without sending the text it has: it maps their lines
// past what was typed since.
func (n *Notebook) BlocksAt() ([]SourceBlock, Version) {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := []SourceBlock{}
	n.eachFence(func(line int, b *doc.Block) {
		if !b.IsInclude() && !b.IsTape() {
			out = append(out, SourceBlock{Line: line, ID: b.ID, Runs: !b.IsInput()})
		}
	})
	return out, n.noteLines()
}

// DependsProblem is a name in the depends= of the block whose fence is
// on Line, from 1, that isn't a block that runs: there is none, or it is
// an input.
type DependsProblem struct {
	Line  int    `json:"line"`
	Name  string `json:"name"`
	Input bool   `json:"input"`
}

// DependsAt is every name in a depends= that isn't a block that runs,
// with the version of the text whose lines it names, for the source
// pane to underline. It is what the pane checked from all the blocks it
// had Go parse out of its text, but is only what is wrong, so checking
// a long file moves a few bytes rather than the file.
func (n *Notebook) DependsAt() ([]DependsProblem, Version) {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := []DependsProblem{}
	n.eachFence(func(line int, b *doc.Block) {
		for _, id := range b.Depends() {
			if n.d.Dep(id) != nil {
				continue
			}
			dep := n.d.Block(id)
			out = append(out, DependsProblem{Line: line, Name: id, Input: dep != nil && dep.IsInput() && !dep.IsInclude() && !dep.IsTape()})
		}
	})
	return out, n.noteLines()
}

// eachFence calls f with each block and the line, from 1, its opening
// fence is on, with n.mu held.
func (n *Notebook) eachFence(f func(line int, b *doc.Block)) {
	src := n.d.Src
	line, pos := 1, 0
	for _, b := range n.d.Blocks {
		if b.Span.Start < pos {
			line, pos = 1, 0
		}
		line += bytes.Count(src[pos:b.Span.Start], []byte{'\n'})
		pos = b.Span.Start
		f(line, b)
	}
}
