package notebook

import (
	"sync"

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
	block      int  // the block using it, or -1
	history    bool // the block was typed in the History pane
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
	i := o.t.block
	o.t.mu.Unlock()
	o.emit(Event{Kind: "tty", Block: i, Bytes: append([]byte(nil), p...)})
	return len(p), nil
}

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
	running := t.block >= 0
	t.mu.Unlock()
	if !running || keys == "" {
		return
	}
	select {
	case t.keys <- []byte(keys):
	default:
	}
}

// SetTerminalSize sets the terminal pane's size in cells. The next tty
// block records at this size, and a running one follows it.
func (n *Notebook) SetTerminalSize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	t := n.term
	t.mu.Lock()
	changed := t.cols != cols || t.rows != rows
	t.cols, t.rows = cols, rows
	t.mu.Unlock()
	if changed {
		select {
		case t.resized <- struct{}{}:
		default:
		}
	}
}

// takeTTY is run's TakeTTY.
func (n *Notebook) takeTTY(b *doc.Block) func() {
	n.mu.Lock()
	i := n.index[b]
	n.mu.Unlock()
	return n.term.take(i, n.emit)
}

func (n *Notebook) emit(e Event) {
	if n.opts.OnEvent != nil {
		n.opts.OnEvent(e)
	}
}
