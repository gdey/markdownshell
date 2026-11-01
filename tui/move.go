package tui

import (
	"bytes"
	"errors"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// undoMax is how many moves u can take back.
const undoMax = 100

// moved is a move u can take back: the file before and after it.
type moved struct {
	before, after []byte
}

// moveBlock moves the block under the cursor, with its output and tape,
// up (dir < 0) or down past the markdown block next to it, as alt+↑/↓
// does in mdsh-gui. Prose stays put. Blocks without ids get them first,
// as saving would, so the move renames nothing. What the move broke that
// depends on block order shows in the footer.
func (m *Model) moveBlock(dir int) {
	i := m.cur()
	switch {
	case i < 0:
		return
	case m.running || len(m.bg) > 0:
		m.status = "a block is running; wait for it to end"
		return
	}
	b := m.d.Blocks[i]
	before := m.d.Src
	problems := m.d.OrderProblems(run.Runnable)
	err := m.step(b, dir)
	if err != nil && !errors.Is(err, doc.ErrNoMove) && !errors.Is(err, doc.ErrDependsOrder) && m.unnamed() {
		// Give blocks ids, as saving would, and try again.
		if _, nerr := run.NameBlocks(m.d, m.results, m.opts.Run); nerr != nil {
			m.status = "move: " + nerr.Error()
			return
		}
		m.dirty = true
		if serr := m.sess.Sync(); serr != nil {
			m.status = "move: " + serr.Error()
			return
		}
		m.results = append([]*run.Result(nil), m.sess.Results()...)
		before = m.d.Src
		problems = m.d.OrderProblems(run.Runnable)
		err = m.step(b, dir)
	}
	if err != nil {
		m.status = "move: " + err.Error()
		return
	}
	m.undo = append(m.undo, moved{before: before, after: m.d.Src})
	if len(m.undo) > undoMax {
		m.undo = m.undo[len(m.undo)-undoMax:]
	}
	m.synced(b)
	m.status = "moved " + m.name(b) + "; u undoes"
	var words []string
	for _, p := range doc.NewProblems(problems, m.d.OrderProblems(run.Runnable)) {
		words = append(words, p.Text)
	}
	if r := m.results[m.index[b]]; r != nil && r.Ran {
		words = append(words, "It ran in the old order; run it again to check.")
	}
	m.notice = strings.Join(words, " ")
}

// step makes the move on the document.
func (m *Model) step(b *doc.Block, dir int) error {
	at, err := m.d.StepTarget(b, dir, false)
	if err != nil {
		return err
	}
	_, err = m.d.MoveBlock(b, at)
	return err
}

// unnamed reports whether a block that runs has no id of its own.
func (m *Model) unnamed() bool {
	for _, b := range m.d.Blocks {
		if run.Runnable(b) && !b.Named() {
			return true
		}
	}
	return false
}

// undoMove takes back the last move. If the file changed since, by an
// edit or a reload, it can't, and forgets the moves.
func (m *Model) undoMove() {
	switch {
	case len(m.undo) == 0:
		m.status = "nothing to undo"
		return
	case m.running || len(m.bg) > 0:
		m.status = "a block is running; wait for it to end"
		return
	}
	c := m.undo[len(m.undo)-1]
	if !bytes.Equal(m.d.Src, c.after) {
		m.undo = nil
		m.status = "the file changed since, so that move can't be undone"
		return
	}
	var b *doc.Block
	if i := m.cur(); i >= 0 {
		b = m.d.Blocks[i]
	}
	if err := m.d.SetSource(c.before); err != nil {
		m.status = "undo: " + err.Error()
		return
	}
	m.undo = m.undo[:len(m.undo)-1]
	m.synced(b)
	m.status = "undid the move"
}

// synced catches the session and screen up with the document after a
// move, keeping the cursor on b.
func (m *Model) synced(b *doc.Block) {
	if err := m.sess.Sync(); err != nil {
		m.status = err.Error()
	}
	m.results = append([]*run.Result(nil), m.sess.Results()...)
	m.index = map[*doc.Block]int{}
	m.layout()
	m.md = map[string]string{}
	m.dirty = true
	if i, ok := m.index[b]; ok && b != nil {
		m.moveTo(i)
	}
	if m.opts.AutoSave {
		m.save(false)
	}
}
