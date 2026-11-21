package notebook

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/history"
	"github.com/gdey/markdownshell/termrec"
)

// live is the History pane as a terminal on an interactive shell: what
// the shell shows goes to the window as "shell" events, keys from the
// window go to the shell, and the commands typed at its prompt become
// History blocks as they run.
type live struct {
	mu  sync.Mutex
	sh  *env.Shell // the shell the pane shows; nil until attached
	rec *termrec.Recorder
	// queue holds what the shell did, for the goroutine that writes
	// it into the document, which can't be done from the shell's own
	// callbacks.
	queue []liveItem
	wake  chan struct{}
	// unrecorded is set while recording is off: commands still run,
	// but aren't added.
	unrecorded bool
}

// liveItem is one thing the shell did.
type liveItem struct {
	sh    *env.Shell
	ev    env.Event
	shown termrec.Recording // on a typed command's end
}

func newLive() *live { return &live{wake: make(chan struct{}, 1)} }

func (lv *live) push(it liveItem) {
	lv.queue = append(lv.queue, it)
	select {
	case lv.wake <- struct{}{}:
	default:
	}
}

// Live reports whether the History pane is a terminal: History is on
// and the top-level shell is interactive. The window then sends the
// pane's keys to TypeShell and draws "shell" events, rather than
// editing lines for Enter.
func (n *Notebook) Live() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.history && n.opts.Run.InteractiveShell && n.sess.InteractiveShell()
}

// SetRecording turns recording on or off for commands typed in the
// live pane: with it off they still run, but aren't added.
func (n *Notebook) SetRecording(on bool) {
	n.live.mu.Lock()
	n.live.unrecorded = !on
	n.live.mu.Unlock()
}

// TypeShell sends keys typed in the live pane to the shell, starting
// it, and attaching the pane to it, if it isn't running.
func (n *Notebook) TypeShell(keys string) error {
	sh, err := n.attachLive(context.Background())
	if err != nil || sh == nil {
		return err
	}
	return sh.Type([]byte(keys))
}

// ShellSize sets the live pane's size, in cells.
func (n *Notebook) ShellSize(cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	n.term.mu.Lock()
	n.term.cols, n.term.rows = cols, rows
	n.term.mu.Unlock()
	n.live.mu.Lock()
	sh := n.live.sh
	n.live.mu.Unlock()
	if sh != nil {
		sh.SetSize(cols, rows)
	}
}

// AttachShell starts the live pane's shell, if it isn't running, and
// attaches the pane to it, so what it shows, such as its first prompt,
// comes as "shell" events. It does nothing unless the pane is live.
func (n *Notebook) AttachShell() error {
	_, err := n.attachLive(context.Background())
	return err
}

// attachLive returns the shell of the live pane, attaching it if it is
// new: the session's top-level shell, started if need be. It is nil
// when the pane isn't live.
func (n *Notebook) attachLive(ctx context.Context) (*env.Shell, error) {
	n.mu.Lock()
	if !n.history || !n.opts.Run.InteractiveShell {
		n.mu.Unlock()
		return nil, nil
	}
	sess := n.sess
	n.mu.Unlock()
	sh, err := sess.TopShell(ctx)
	if err != nil || sh == nil {
		return nil, err
	}
	lv := n.live
	lv.mu.Lock()
	if lv.sh == sh {
		lv.mu.Unlock()
		return sh, nil
	}
	lv.sh, lv.rec = sh, nil
	lv.mu.Unlock()
	cols, rows := n.term.size()
	sh.SetSize(cols, rows)
	sh.View(liveView{n, sh})
	sh.Watch(func(e env.Event) { n.liveEvent(sh, e) })
	go func() {
		<-sh.Done()
		lv.mu.Lock()
		if lv.sh == sh {
			lv.sh, lv.rec = nil, nil
		}
		lv.push(liveItem{sh: sh, ev: env.Event{Kind: "exit"}})
		lv.mu.Unlock()
	}()
	return sh, nil
}

// liveView takes what the live pane's shell shows.
type liveView struct {
	n  *Notebook
	sh *env.Shell
}

func (v liveView) Write(p []byte) (int, error) {
	lv := v.n.live
	lv.mu.Lock()
	if lv.sh != v.sh {
		lv.mu.Unlock()
		return len(p), nil
	}
	if lv.rec != nil {
		lv.rec.Write(p)
	}
	lv.mu.Unlock()
	v.n.emit(Event{Kind: "shell", Block: -1, Bytes: append([]byte(nil), p...)})
	return len(p), nil
}

// liveEvent takes what the shell did, with the shell's lock held: it
// keeps a typed command's screen and queues the rest.
func (n *Notebook) liveEvent(sh *env.Shell, e env.Event) {
	lv := n.live
	lv.mu.Lock()
	defer lv.mu.Unlock()
	if lv.sh != sh || e.Typed {
		// Blocks played from the document run as they always do.
		return
	}
	it := liveItem{sh: sh, ev: e}
	switch e.Kind {
	case "start":
		lv.rec = termrec.NewRecorder(n.term.size())
	case "end":
		if lv.rec != nil {
			it.shown = lv.rec.Recording()
			lv.rec = nil
		}
	}
	lv.push(it)
}

// liveLoop writes what the live pane's shell did into the document,
// until the notebook closes.
func (n *Notebook) liveLoop() {
	lv := n.live
	// cur is the command running at the prompt: its block, or -1 when
	// it isn't one.
	cur, running := -1, false
	for {
		select {
		case <-n.quit:
			return
		case <-lv.wake:
		}
		for {
			lv.mu.Lock()
			if len(lv.queue) == 0 {
				lv.mu.Unlock()
				break
			}
			it := lv.queue[0]
			lv.queue = lv.queue[1:]
			unrecorded := lv.unrecorded
			lv.mu.Unlock()
			switch it.ev.Kind {
			case "start":
				cur, running = n.liveStart(it.sh, it.ev.Command, unrecorded), true
			case "end":
				if running {
					n.liveEnd(cur, it.ev.Status, it.shown)
				}
				cur, running = -1, false
			case "exit":
				if running {
					n.liveEnd(cur, -1, it.shown)
				}
				cur, running = -1, false
				n.emit(Event{Kind: "shell-exit", Block: -1})
			case "line":
				if text, ok := note(it.ev.Command); ok {
					n.addNote(text)
				}
			case "state":
				n.liveJobs(it.sh)
			}
		}
	}
}

// liveStart takes a command typed at the shell's prompt, which has
// started: a note becomes prose, as sh runs a comment as a command;
// anything else becomes a History block, unless recording is off. The
// notebook is busy until it ends. It returns the block, or -1.
func (n *Notebook) liveStart(sh *env.Shell, cmd string, unrecorded bool) int {
	if text, ok := note(cmd); ok {
		n.addNote(text)
		return -1
	}
	n.mu.Lock()
	n.running = true
	n.cancel = func() { sh.Type([]byte{0x03}) }
	if strings.TrimSpace(cmd) == "" || unrecorded {
		n.noteEnd = -1
		n.mu.Unlock()
		return -1
	}
	var b *doc.Block
	var err error
	if n.reuse {
		if b = history.Reuse(n.d, n.lang(), cmd, false); b != nil {
			err = history.KeepRuns(n.d, b)
			n.dirty = true
		}
	}
	if b == nil {
		b, err = n.d.AppendBlock(history.Info(n.lang(), n.nextHistoryID(), false), cmd)
	}
	if err == nil {
		err = n.sync()
	}
	if err != nil {
		n.mu.Unlock()
		n.emit(Event{Kind: "error", Block: -1, Data: "can't add the command to the document: " + err.Error()})
		return -1
	}
	n.noteEnd = -1
	i, sess := n.index[b], n.sess
	n.mu.Unlock()
	if err := sess.TypedStart(i); err != nil {
		return -1
	}
	return i
}

// liveEnd ends the command liveStart started, with its status and what
// it showed: block i gets its result, and tty if it took the screen.
func (n *Notebook) liveEnd(i, status int, shown termrec.Recording) {
	n.mu.Lock()
	sess := n.sess
	n.mu.Unlock()
	if i >= 0 {
		r, full, _ := sess.TypedEnd(i, status, shown)
		if full && r != nil {
			n.mu.Lock()
			b := r.Block
			if _, ok := n.index[b]; ok && !b.Attrs.Has("tty") {
				text := b.Fence + strings.TrimSuffix(b.Info, "}") + " tty}\n" + b.Body + b.Fence + "\n"
				if err := n.d.SetBlock(b, text); err == nil {
					n.sync()
				}
			}
			n.mu.Unlock()
		}
	}
	n.mu.Lock()
	n.running, n.cancel = false, nil
	n.readIncludes()
	n.fold()
	n.idle.Broadcast()
	n.mu.Unlock()
}

// ShellJob is a job of the live pane's shell, for the window's badges.
type ShellJob struct {
	// N is the job number, as in %1, or 0 when the shell doesn't number
	// its jobs.
	N int `json:"n"`
	// State is "running" or "stopped", or what the shell said.
	State   string `json:"state"`
	Command string `json:"command"`
	// Block is the History block whose command started the job, or -1.
	Block int `json:"block"`
}

// liveJobs sends the shell's jobs as a "jobs" event, its Data a JSON
// list of ShellJob, each with the latest History block whose command
// it is.
func (n *Notebook) liveJobs(sh *env.Shell) {
	jobs := sh.Jobs()
	out := make([]ShellJob, 0, len(jobs))
	n.mu.Lock()
	for _, j := range jobs {
		out = append(out, ShellJob{N: j.N, State: j.State, Command: j.Command, Block: n.blockFor(j.Command)})
	}
	n.mu.Unlock()
	data, _ := json.Marshal(out)
	n.emit(Event{Kind: "jobs", Block: -1, Data: string(data)})
}

// blockFor is the index of the last History block whose body is cmd,
// with n.mu held, or -1.
func (n *Notebook) blockFor(cmd string) int {
	cmd = strings.TrimSpace(cmd)
	for i := len(n.d.Blocks) - 1; i >= 0; i-- {
		b := n.d.Blocks[i]
		if strings.HasPrefix(b.ID, "history-") && strings.TrimSpace(b.Body) == cmd {
			return i
		}
	}
	return -1
}

// Running reports whether a block or a command typed at the live
// pane's prompt is running.
func (n *Notebook) Running() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.running
}
