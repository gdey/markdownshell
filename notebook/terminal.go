package notebook

import (
	"sync"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
)

// terminal is the pane blocks marked tty run in. The window draws it;
// keys come in through Type and what the block shows goes out as "tty"
// events.
type terminal struct {
	keys    chan []byte
	resized chan struct{}

	mu         sync.Mutex
	cols, rows int
	block      int  // the block using it, Unrecorded, or -1
	history    bool // the block was typed in the History pane
	// background is set on the terminal of a background block, whose
	// events say so in their Data.
	background bool
}

func newTerminal() *terminal {
	return &terminal{
		keys:    make(chan []byte, 64),
		resized: make(chan struct{}, 1),
		cols:    80,
		rows:    24,
		block:   -1,
	}
}

// env is the terminal as run sees it, sending what blocks show to emit.
func (t *terminal) env(emit func(Event)) *env.Terminal {
	return &env.Terminal{
		Keys:    t.keys,
		Out:     termOut{t, emit},
		Size:    t.size,
		Resized: t.resized,
	}
}

func (t *terminal) size() (cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cols, t.rows
}

type termOut struct {
	t    *terminal
	emit func(Event)
}

func (o termOut) Write(p []byte) (int, error) {
	o.t.mu.Lock()
	i, data := o.t.block, ""
	if o.t.background {
		data = "background"
	}
	o.t.mu.Unlock()
	o.emit(Event{Kind: "tty", Block: i, Data: data, Bytes: append([]byte(nil), p...)})
	return len(p), nil
}

// Unrecorded is the block a command typed in the History pane with
// recording off runs as, in terminal events: it is in no card.
const Unrecorded = -2

// take marks block i as using the terminal, emitting "terminal" so the
// window opens the pane, and "terminal-end" when the block gives it back.
// A block typed in the History pane says so in the event's Data, since
// it shows there rather than in its card.
func (t *terminal) take(i int, emit func(Event)) (giveBack func()) {
	t.mu.Lock()
	t.block = i
	pane := ""
	if t.history {
		pane = "history"
	}
	t.mu.Unlock()
	// Keys typed before the block started aren't for it.
	for len(t.keys) > 0 {
		<-t.keys
	}
	emit(Event{Kind: "terminal", Block: i, Data: pane})
	return func() {
		t.mu.Lock()
		t.block = -1
		t.mu.Unlock()
		emit(Event{Kind: "terminal-end", Block: i})
	}
}

// Type sends what was typed in the terminal pane to the block running
// in it. It is dropped if no block is.
func (n *Notebook) Type(keys string) {
	t := n.term
	t.mu.Lock()
	running := t.block != -1
	t.mu.Unlock()
	if !running || keys == "" {
		return
	}
	select {
	case t.keys <- []byte(keys):
	default:
	}
}

// TerminalSize is the terminal size the front matter asks for, in
// cells, with 0 for either it leaves to the window.
func (n *Notebook) TerminalSize() (cols, rows int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	t := n.d.Options.Terminal
	return t.Cols, t.Rows
}

// FrontMatterError is why the front matter doesn't parse, or "" when it
// does. Until it does, the page says so and no block runs.
func (n *Notebook) FrontMatterError() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.d.FrontMatterErr == nil {
		return ""
	}
	return n.d.FrontMatterErr.Error()
}

// FrontMatterFix says what FixWarning does with FrontMatterError as the
// warning, or is the zero Fix when mdsh has no fix for the front matter.
func (n *Notebook) FrontMatterFix() Fix {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.d.FrontMatterErr == nil {
		return Fix{}
	}
	f, err := n.fixFor(n.d.FrontMatterErr.Error())
	if err != nil {
		return Fix{}
	}
	return Fix{Label: f.Label, Detail: f.Detail}
}

// EnvBanner is the front matter's note that the file brings in more
// than the base set from the environment, or "" (see
// doc.Options.ShareBanner).
func (n *Notebook) EnvBanner() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.d.Options.ShareBanner()
}

// SetTerminalSize sets the terminal pane's size in cells. The next tty
// block records at this size, and a running one follows it.
func (n *Notebook) SetTerminalSize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	n.term.setSize(cols, rows)
}

// SetBackgroundTerminalSize sets the size, in cells, of the terminals
// in the cards of background tty blocks, which the window works out
// from the cards' width; the blocks running on them follow it.
func (n *Notebook) SetBackgroundTerminalSize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	n.mu.Lock()
	n.bgCols, n.bgRows = cols, rows
	terms := make([]*terminal, 0, len(n.bgTerms))
	for _, t := range n.bgTerms {
		terms = append(terms, t)
	}
	n.mu.Unlock()
	for _, t := range terms {
		t.setSize(cols, rows)
	}
}

// setSize sizes t, telling the block running on it if that changed.
func (t *terminal) setSize(cols, rows int) {
	t.mu.Lock()
	changed := t.cols != cols || t.rows != rows
	t.cols, t.rows = cols, rows
	t.mu.Unlock()
	if changed {
		t.resize()
	}
}

// takeTTY is run's TakeTTY.
func (n *Notebook) takeTTY(b *doc.Block) func() {
	n.mu.Lock()
	i, ok := n.index[b]
	n.mu.Unlock()
	if !ok {
		i = Unrecorded
	}
	return n.term.take(i, n.emit)
}

func (n *Notebook) emit(e Event) {
	if n.opts.OnEvent != nil {
		n.opts.OnEvent(e)
	}
}

// backgroundTerminal is run's BackgroundTerminal: a terminal of its own
// for background block b, as wide as the pane, drawn in its card. Its
// events say Data "background", since it stays open while other blocks
// run, and the pane, too, may open.
func (n *Notebook) backgroundTerminal(b *doc.Block) (*env.Terminal, func()) {
	n.mu.Lock()
	i, ok := n.index[b]
	if !ok {
		i = Unrecorded
	}
	t := newTerminal()
	if n.bgCols > 0 {
		t.cols, t.rows = n.bgCols, n.bgRows
	} else {
		t.cols, t.rows = n.term.size()
	}
	t.block, t.background = i, true
	if ok {
		n.bgTerms[i] = t
	}
	n.mu.Unlock()
	n.emit(Event{Kind: "terminal", Block: i, Data: "background"})
	return t.env(n.emit), func() {
		n.mu.Lock()
		if n.bgTerms[i] == t {
			delete(n.bgTerms, i)
		}
		n.mu.Unlock()
		n.emit(Event{Kind: "terminal-end", Block: i, Data: "background"})
	}
}

// TypeBackground sends what was typed in block i's card to the block,
// which runs in the background on a terminal of its own. It is dropped
// if the block isn't running so.
func (n *Notebook) TypeBackground(i int, keys string) {
	n.mu.Lock()
	t := n.bgTerms[i]
	n.mu.Unlock()
	if t == nil || keys == "" {
		return
	}
	select {
	case t.keys <- []byte(keys):
	default:
	}
}

// RedrawBackground has the program on background block i's terminal
// draw its screen again, as a page that shows the card afresh needs:
// the terminal shrinks by a row and grows back, which full-screen
// programs such as vim answer by drawing everything.
func (n *Notebook) RedrawBackground(i int) {
	n.mu.Lock()
	t := n.bgTerms[i]
	n.mu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	rows := t.rows
	t.rows = max(rows-1, 1)
	t.mu.Unlock()
	t.resize()
	time.Sleep(50 * time.Millisecond)
	t.mu.Lock()
	t.rows = rows
	t.mu.Unlock()
	t.resize()
}

// resize tells the block running on t that its size changed.
func (t *terminal) resize() {
	select {
	case t.resized <- struct{}{}:
	default:
	}
}
