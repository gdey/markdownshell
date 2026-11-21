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
	// unrecorded is whether recording was off as the command started,
	// not when the loop gets to it.
	unrecorded bool
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
	it := liveItem{sh: sh, ev: e, unrecorded: lv.unrecorded}
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
			lv.mu.Unlock()
			switch it.ev.Kind {
			case "start":
				cur, running = n.liveStart(it.sh, it.ev.Command, it.unrecorded, it.ev.Resumed), true
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
// anything else becomes a History block, unless recording is off, or
// it is mdsh bringing back the job a played block stopped. The
// notebook is busy until it ends, though a block played meanwhile
// stops it to run (see start). It returns the block, or -1.
func (n *Notebook) liveStart(sh *env.Shell, cmd string, unrecorded, resumed bool) int {
	if text, ok := note(cmd); ok && !resumed {
		n.addNote(text)
		return -1
	}
	n.mu.Lock()
	n.liveCancel = func() { sh.Type([]byte{0x03}) }
	if !n.playing {
		n.cancel = n.liveCancel
	}
	n.running, n.liveCmd = true, true
	if resumed || strings.TrimSpace(cmd) == "" || unrecorded {
		if !resumed {
			n.noteEnd = -1
		}
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
	// With the lock held, so a run ending meanwhile doesn't rewrite the
	// document under it.
	i := n.index[b]
	started, err := n.sess.TypedStart(i)
	n.mu.Unlock()
	if err != nil {
		return -1
	}
	started()
	return i
}

// liveEnded is a typed command's end that waits for the block played
// meanwhile, which has the session.
type liveEnded struct {
	block, status int
	shown         termrec.Recording
}

// liveEnd ends the command liveStart started, with its status and what
// it showed. When a played block stopped it, as one does to run in the
// shell it held, the block's result waits for that run to end.
func (n *Notebook) liveEnd(i, status int, shown termrec.Recording) {
	n.mu.Lock()
	if n.playing {
		n.liveCmd = false
		n.liveEnds = append(n.liveEnds, liveEnded{i, status, shown})
		n.mu.Unlock()
		return
	}
	done := n.finishLive(liveEnded{i, status, shown})
	n.mu.Unlock()
	// Its Done event gives the notebook the result, which the fold
	// below writes.
	done()
	n.mu.Lock()
	n.running, n.cancel, n.liveCmd = false, nil, false
	n.readIncludes()
	n.fold()
	n.idle.Broadcast()
	n.mu.Unlock()
}

// finishLive gives a typed command's block its result, and tty if it
// took the screen, with n.mu held, so no run starts or rewrites the
// document meanwhile. done sends the block's Done event, once n.mu is
// let go.
func (n *Notebook) finishLive(e liveEnded) (done func()) {
	done = func() {}
	if e.block < 0 {
		return done
	}
	r, full, sent, err := n.sess.TypedEnd(e.block, e.status, e.shown)
	if err != nil {
		return done
	}
	if !full || r == nil {
		return sent
	}
	b := r.Block
	if _, ok := n.index[b]; ok && !b.Attrs.Has("tty") {
		text := b.Fence + strings.TrimSuffix(b.Info, "}") + " tty}\n" + b.Body + b.Fence + "\n"
		if err := n.d.SetBlock(b, text); err == nil {
			n.sync()
		}
	}
	return sent
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

// RunBlockSide runs block i in a side shell, started from the state the
// live pane's shell last reported, so it runs while a command typed at
// that shell's prompt, such as an editor, holds it: see
// run.Session.RunSide. What the block changes isn't kept.
func (n *Notebook) RunBlockSide(i int) error {
	return n.startRun(func(ctx context.Context) error {
		n.mu.Lock()
		if i >= 0 && i < len(n.d.Blocks) {
			n.clicked = n.d.Blocks[i]
		}
		sess := n.sess
		n.mu.Unlock()
		defer func() {
			n.mu.Lock()
			n.clicked = nil
			n.mu.Unlock()
		}()
		_, err := sess.RunSide(ctx, i)
		return err
	}, true)
}
