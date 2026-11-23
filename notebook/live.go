package notebook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

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
	mu sync.Mutex
	sh *env.Shell // the shell the pane shows; nil until attached
	// env is the env the pane is on: the top level, "", unless
	// SetHistoryEnv moved it.
	env string
	// shells holds every shell the pane has been on that still runs,
	// each with its screen, so moving back to one shows it again.
	shells map[*env.Shell]*liveShell
	// moving is set while SetHistoryEnv moves the pane: what the new
	// shell shows meanwhile goes to the window with its screen so far.
	moving bool
	// queue holds what the shell did, for the goroutine that writes
	// it into the document, which can't be done from the shell's own
	// callbacks.
	queue []liveItem
	wake  chan struct{}
	// unrecorded is set while recording is off: commands still run,
	// but aren't added.
	unrecorded bool
}

// liveShell is a shell the pane has been on.
type liveShell struct {
	env string
	rec *termrec.Recorder // a typed command's screen, as it runs
	// screen is the end of what the shell showed, at most screenKeep
	// bytes, to show again when the pane comes back to it.
	screen []byte
}

// screenKeep is how much of what a shell showed is kept to show again.
const screenKeep = 64 << 10

// keep appends p to the shell's screen, dropping the oldest lines past
// screenKeep.
func (ls *liveShell) keep(p []byte) {
	ls.screen = append(ls.screen, p...)
	if over := len(ls.screen) - screenKeep; over > 0 {
		cut := ls.screen[over:]
		// From a line's start, so no escape is cut in two.
		if i := bytes.IndexByte(cut, '\n'); i >= 0 {
			cut = cut[i+1:]
		}
		ls.screen = append([]byte(nil), cut...)
	}
}

// liveItem is one thing the shell did.
type liveItem struct {
	sh    *env.Shell
	env   string // the env sh runs
	ev    env.Event
	shown termrec.Recording // on a typed command's end
	// unrecorded is whether recording was off as the command started,
	// not when the loop gets to it.
	unrecorded bool
	// current is whether sh was on the pane.
	current bool
}

func newLive() *live {
	return &live{wake: make(chan struct{}, 1), shells: map[*env.Shell]*liveShell{}}
}

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

// HistoryEnv is an env the live pane can be moved to, for the pane's
// env picker.
type HistoryEnv struct {
	// Name is the env, "" for the top level.
	Name string `json:"name"`
	// Current is set for the env the pane is on.
	Current bool `json:"current"`
	// Live is set when an interactive shell runs for it that the pane
	// has been on, which keeps its screen and jobs.
	Live bool `json:"live"`
	// Scripted is set when a scripted shell runs for it, which moving
	// the pane there ends, losing what blocks set there.
	Scripted bool `json:"scripted"`
	// Stopped counts its shell's stopped jobs.
	Stopped int `json:"stopped"`
}

// ErrEnvScripted is SetHistoryEnv's error for an env a scripted shell
// runs, which moving the pane there would end: ask, then call it again
// with restart.
var ErrEnvScripted = errors.New("the env's shell runs scripted: moving History there starts it again, and what blocks set there is lost")

// ErrPaneBusy is SetHistoryEnv's error while a command typed in the
// pane, or a block, runs.
var ErrPaneBusy = errors.New("a command is running: stop it, with ctrl+z or ctrl+c, or let it end, before moving History to another env")

// HistoryEnvs lists the envs the live pane can be on: the top level,
// then each env the document names, each after its parents.
func (n *Notebook) HistoryEnvs() []HistoryEnv {
	n.mu.Lock()
	sess := n.sess
	n.mu.Unlock()
	lv := n.live
	lv.mu.Lock()
	current := lv.env
	byEnv := map[string]*env.Shell{}
	for sh, ls := range lv.shells {
		byEnv[ls.env] = sh
	}
	lv.mu.Unlock()
	var out []HistoryEnv
	for _, name := range sess.Envs() {
		e := HistoryEnv{Name: name, Current: name == current, Scripted: sess.Scripted(name)}
		if sh := byEnv[name]; sh != nil && !sh.Exited() {
			e.Live = true
			for _, j := range sh.Jobs() {
				if j.State == "stopped" {
					e.Stopped++
				}
			}
		}
		out = append(out, e)
	}
	return out
}

// HistoryEnvName is the env the live pane is on, "" for the top level.
func (n *Notebook) HistoryEnvName() string {
	n.live.mu.Lock()
	defer n.live.mu.Unlock()
	return n.live.env
}

// SetHistoryEnv moves the live pane to env name's shell, which runs
// interactively from then on, on a terminal of its own, started if it
// isn't running; commands typed there become History blocks with
// env=name. The shell the pane leaves keeps running, with its jobs and
// screen, for when it comes back. When a scripted shell runs for name,
// which can't become interactive, it is ErrEnvScripted unless restart
// says to end it. It is ErrPaneBusy while anything runs. It sends a
// "shell-env" event, with the env as its Data and the new shell's
// screen so far as its Bytes.
func (n *Notebook) SetHistoryEnv(name string, restart bool) error {
	name = strings.Trim(name, "/")
	n.mu.Lock()
	if !n.history || !n.opts.Run.InteractiveShell {
		n.mu.Unlock()
		return errors.New("History isn't a terminal: turn on Settings › Shell › Interactive shell")
	}
	if n.running {
		n.mu.Unlock()
		return ErrPaneBusy
	}
	sess := n.sess
	n.mu.Unlock()
	known := false
	for _, e := range sess.Envs() {
		known = known || e == name
	}
	if !known {
		return errors.New("no block runs in env " + name)
	}
	lv := n.live
	lv.mu.Lock()
	same := lv.env == name && lv.sh != nil
	lv.mu.Unlock()
	if same {
		return nil
	}
	if name != env.TopLevel {
		if sess.Scripted(name) && !restart {
			return ErrEnvScripted
		}
		sess.SetInteractive(name)
	}
	lv.mu.Lock()
	lv.env, lv.sh, lv.moving = name, nil, true
	lv.mu.Unlock()
	sh, err := n.attachLive(context.Background())
	var screen []byte
	lv.mu.Lock()
	lv.moving = false
	if ls := lv.shells[sh]; ls != nil {
		screen = append(screen, ls.screen...)
	}
	// With lv.mu held, so what the shell shows next comes after.
	n.emit(Event{Kind: "shell-env", Block: -1, Data: name, Bytes: screen})
	lv.mu.Unlock()
	if err != nil {
		return err
	}
	if sh != nil {
		sh.SetSize(n.term.size())
		n.liveJobs(sh)
	}
	return nil
}

// attachLive returns the shell of the live pane, attaching it if it is
// new: the shell of the env the pane is on, started if need be. It is
// nil when the pane isn't live.
func (n *Notebook) attachLive(ctx context.Context) (*env.Shell, error) {
	n.mu.Lock()
	if !n.history || !n.opts.Run.InteractiveShell {
		n.mu.Unlock()
		return nil, nil
	}
	sess := n.sess
	n.mu.Unlock()
	lv := n.live
	lv.mu.Lock()
	name := lv.env
	if sh := lv.sh; sh != nil && lv.shells[sh] != nil {
		lv.mu.Unlock()
		return sh, nil
	}
	lv.mu.Unlock()
	if name != env.TopLevel && !sess.Interactive(name) {
		// The shells were restarted, which forgets the env was moved
		// to. Name it again, unless a block has started it scripted
		// meanwhile, which only SetHistoryEnv may end, having asked.
		if sess.Scripted(name) {
			lv.mu.Lock()
			lv.env, name = env.TopLevel, env.TopLevel
			lv.mu.Unlock()
			n.emit(Event{Kind: "shell-env", Block: -1})
		} else {
			sess.SetInteractive(name)
		}
	}
	sh, err := sess.Shell(ctx, name)
	if err != nil || sh == nil {
		return nil, err
	}
	lv.mu.Lock()
	if lv.env != name {
		// Moved meanwhile.
		lv.mu.Unlock()
		return nil, nil
	}
	lv.sh = sh
	if lv.shells[sh] != nil {
		lv.mu.Unlock()
		return sh, nil
	}
	lv.shells[sh] = &liveShell{env: name}
	lv.mu.Unlock()
	cols, rows := n.term.size()
	sh.SetSize(cols, rows)
	sh.View(liveView{n, sh})
	sh.Watch(func(e env.Event) { n.liveEvent(sh, e) })
	go func() {
		<-sh.Done()
		lv.mu.Lock()
		ls := lv.shells[sh]
		delete(lv.shells, sh)
		shown := lv.sh == sh
		if shown {
			lv.sh = nil
		}
		it := liveItem{sh: sh, ev: env.Event{Kind: "exit"}, shown: shownOn(ls)}
		if ls != nil {
			it.env = ls.env
		}
		it.current = shown
		lv.push(it)
		lv.mu.Unlock()
	}()
	return sh, nil
}

// shownOn is what a shell's running command has shown, if it has one.
func shownOn(ls *liveShell) termrec.Recording {
	if ls == nil || ls.rec == nil {
		return termrec.Recording{}
	}
	return ls.rec.Recording()
}

// liveView takes what the live pane's shell shows.
type liveView struct {
	n  *Notebook
	sh *env.Shell
}

func (v liveView) Write(p []byte) (int, error) {
	lv := v.n.live
	lv.mu.Lock()
	ls := lv.shells[v.sh]
	if ls == nil {
		lv.mu.Unlock()
		return len(p), nil
	}
	if ls.rec != nil {
		ls.rec.Write(p)
	}
	ls.keep(p)
	if lv.sh != v.sh || lv.moving {
		// Not on the pane: kept for when it is again.
		lv.mu.Unlock()
		return len(p), nil
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
	ls := lv.shells[sh]
	if ls == nil || e.Typed {
		// Blocks played from the document run as they always do.
		return
	}
	it := liveItem{sh: sh, env: ls.env, ev: e, unrecorded: lv.unrecorded, current: lv.sh == sh}
	switch e.Kind {
	case "start":
		ls.rec = termrec.NewRecorder(n.term.size())
	case "end":
		if ls.rec != nil {
			it.shown = ls.rec.Recording()
			ls.rec = nil
		}
	}
	lv.push(it)
}

// liveLoop writes what the live pane's shell did into the document,
// until the notebook closes.
func (n *Notebook) liveLoop() {
	lv := n.live
	// cur is the command running at each shell's prompt: its block,
	// or -1 when it isn't one; a shell with none running isn't in it.
	cur := map[*env.Shell]int{}
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
				i, busy := n.liveStart(it.sh, it.env, it.ev.Command, it.unrecorded, it.ev.Resumed)
				cur[it.sh] = i
				if busy {
					n.emit(Event{Kind: "live", Block: -1, Data: "running"})
					// fg, or mdsh bringing back the job a played block
					// stopped, has a stopped job run again: once it
					// holds the terminal, its badge goes.
					sh := it.sh
					time.AfterFunc(liveJobsWait, func() { n.liveJobs(sh) })
				}
				n.emit(Event{Kind: "changed", Block: -1})
			case "end":
				if i, ok := cur[it.sh]; ok {
					n.liveEnd(i, it.ev.Status, it.shown)
					n.emit(Event{Kind: "live", Block: -1})
					n.emit(Event{Kind: "changed", Block: -1})
				}
				delete(cur, it.sh)
			case "exit":
				if i, ok := cur[it.sh]; ok {
					n.liveEnd(i, -1, it.shown)
					n.emit(Event{Kind: "live", Block: -1})
					n.emit(Event{Kind: "changed", Block: -1})
				}
				delete(cur, it.sh)
				if it.current {
					n.emit(Event{Kind: "shell-exit", Block: -1})
				}
			case "line":
				if text, ok := note(it.ev.Command); ok && n.addNote(text) == nil {
					n.emit(Event{Kind: "changed", Block: -1})
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
// stops it to run (see start). It returns the block, or -1, and
// whether the notebook is busy with it, which it isn't for a note.
func (n *Notebook) liveStart(sh *env.Shell, envName, cmd string, unrecorded, resumed bool) (int, bool) {
	if text, ok := note(cmd); ok && !resumed {
		n.addNote(text)
		return -1, false
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
		return -1, true
	}
	var b *doc.Block
	var err error
	if n.reuse {
		if b = history.ReuseIn(n.d, n.lang(), cmd, false, envName); b != nil {
			err = history.KeepRuns(n.d, b)
			n.dirty = true
		}
	}
	if b == nil {
		b, err = n.d.AppendBlock(history.InfoIn(n.lang(), n.nextHistoryID(), false, envName), cmd)
	}
	if err == nil {
		err = n.sync()
	}
	if err != nil {
		n.mu.Unlock()
		n.emit(Event{Kind: "error", Block: -1, Data: "can't add the command to the document: " + err.Error()})
		return -1, true
	}
	n.noteEnd = -1
	// With the lock held, so a run ending meanwhile doesn't rewrite the
	// document under it.
	i := n.index[b]
	started, err := n.sess.TypedStart(i)
	n.mu.Unlock()
	if err != nil {
		return -1, true
	}
	started()
	return i, true
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
	lv := n.live
	lv.mu.Lock()
	ls, shown := lv.shells[sh], lv.sh == sh
	lv.mu.Unlock()
	if !shown || ls == nil {
		// The bar shows the jobs of the shell on the pane.
		return
	}
	jobs := sh.Jobs()
	out := make([]ShellJob, 0, len(jobs))
	fg := sh.Foreground()
	n.mu.Lock()
	for _, j := range jobs {
		if fg != 0 && j.PGID == fg {
			// In the foreground again: it is the command that runs,
			// not a job beside it, though the shell lists it till the
			// next prompt.
			continue
		}
		out = append(out, ShellJob{N: j.N, State: j.State, Command: j.Command, Block: n.blockFor(j.Command, ls.env)})
	}
	n.mu.Unlock()
	data, _ := json.Marshal(out)
	n.emit(Event{Kind: "jobs", Block: -1, Data: string(data)})
}

// liveJobsWait is how long after a typed command starts its jobs are
// sent again, by when fg has given the terminal to the job it resumed.
const liveJobsWait = 200 * time.Millisecond

// blockFor is the index of the last History block of env whose body is
// cmd, with n.mu held, or -1.
func (n *Notebook) blockFor(cmd, envName string) int {
	cmd = strings.TrimSpace(cmd)
	for i := len(n.d.Blocks) - 1; i >= 0; i-- {
		b := n.d.Blocks[i]
		if strings.HasPrefix(b.ID, "history-") && strings.TrimSpace(b.Body) == cmd && strings.Trim(b.Attrs["env"], "/") == envName {
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
