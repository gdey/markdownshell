package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/history"
	"github.com/gdey/markdownshell/termrec"
	"github.com/gdey/markdownshell/termstatus"
)

// With the interactive shell (--interactive-shell), the History pane is
// live, as in mdsh-gui: a terminal on the document's shell, which draws
// its own prompt and reads every key, so vi, ctrl+z, fg and jobs work
// as in any terminal. What is typed at its prompt still goes into the
// file, from the shell's marks: a comment as a note, any other command
// as a #history-N block with what it showed, a full-screen one as a
// recording. ctrl+g gives the keys back to the file.

// liveTerm is the live pane's state.
type liveTerm struct {
	sh *env.Shell // nil until attached, and once the shell ends
	// p draws what the shell shows, and takes the keys typed for it.
	p *pane
	// env is the env whose shell the pane is on, "" for the top level
	// (see moveHistory).
	env string
	// parked holds the shells of the other envs the pane has been on,
	// by env, each with its screen, for when it comes back.
	parked map[string]parkedShell
	// drawn is every live pane's drawn, so one wait covers them all.
	drawn chan struct{}

	mu    sync.Mutex
	rec   *termrec.Recorder // what a typed command shows, while it runs
	recSh *env.Shell        // the shell rec is for
	// queue holds what the shell did, in order, for Update; the shell's
	// callbacks can't wait on it.
	queue []liveMsg
	wake  chan struct{}

	// The rest belong to Update.
	busy bool // a command typed at the prompt runs
	cur  int  // its block, or -1
	jobs []env.ShellJob
	// recs is what the program the shell runs reports about itself
	// (OSC 7501), for the pane's bar.
	recs []termstatus.Report
	// ends are typed commands' ends that wait for a run of blocks,
	// which has the session.
	ends []liveMsg
}

// liveMsg is something the live pane's shell did.
type liveMsg struct {
	sh    *env.Shell
	ev    env.Event
	shown termrec.Recording // on a typed command's end
}

// liveDrawnMsg says the live pane's screen changed.
type liveDrawnMsg struct{ m *Model }

// parkedShell is a shell the pane has left, with its screen.
type parkedShell struct {
	sh *env.Shell
	p  *pane
}

func newLiveTerm() *liveTerm {
	lt := &liveTerm{wake: make(chan struct{}, 1), cur: -1, parked: map[string]parkedShell{}, drawn: make(chan struct{}, 1)}
	lt.p = lt.newPane()
	return lt
}

// newPane is a pane for a live shell.
func (lt *liveTerm) newPane() *pane {
	p := newPane()
	p.drawn = lt.drawn
	return p
}

// isLive reports whether the History pane is live: open, with the
// interactive shell.
func (m *Model) isLive() bool {
	return m.hist.open && m.opts.Run.InteractiveShell && m.sess.InteractiveShell()
}

// attachLive attaches the live pane to the shell of the env it is on,
// the top level unless moveHistory moved it, starting it if need be,
// so what it shows, such as its prompt, draws in the pane.
func (m *Model) attachLive() error {
	lt := m.lterm
	if lt.env != env.TopLevel && !m.sess.Interactive(lt.env) {
		// The shells were started again, which forgets the env the pane
		// was moved to. Name it again, unless a block has started it
		// scripted meanwhile, which only moveHistory may end.
		if m.sess.Scripted(lt.env) {
			lt.env = env.TopLevel
		} else {
			m.sess.SetInteractive(lt.env)
		}
	}
	sh, err := m.sess.Shell(context.Background(), lt.env)
	if err != nil || sh == nil {
		return err
	}
	if lt.sh == sh {
		return nil
	}
	lt.sh = sh
	p := lt.p
	p.take(-3)
	// The emulator's answers, such as where its cursor is, and the keys
	// typed in the pane go to the shell.
	go func(keys chan []byte) {
		for {
			select {
			case b := <-keys:
				sh.Type(b)
			case <-sh.Done():
				return
			case <-m.done:
				return
			}
		}
	}(p.keys)
	m.fit()
	sh.View(liveView{m, sh, p})
	sh.Watch(func(e env.Event) { m.liveEvent(sh, p, e) })
	go func() {
		<-sh.Done()
		lt.push(liveMsg{sh: sh, ev: env.Event{Kind: "exit"}})
	}()
	return nil
}

// liveView takes what the live pane's shell shows.
type liveView struct {
	m  *Model
	sh *env.Shell
	p  *pane // its own, kept while the pane is on another env
}

func (v liveView) Write(b []byte) (int, error) {
	lt := v.m.lterm
	lt.mu.Lock()
	if lt.rec != nil && lt.recSh == v.sh {
		lt.rec.Write(b)
	}
	lt.mu.Unlock()
	return paneOut{v.p}.Write(b)
}

var _ io.Writer = liveView{}

// liveEvent takes what the shell did, with its lock held: it keeps a
// typed command's screen and queues the rest for Update.
func (m *Model) liveEvent(sh *env.Shell, p *pane, e env.Event) {
	if e.Typed {
		// Blocks played from the file run as they always do.
		return
	}
	lt := m.lterm
	it := liveMsg{sh: sh, ev: e}
	lt.mu.Lock()
	switch e.Kind {
	case "start":
		lt.rec, lt.recSh = termrec.NewRecorder(p.size()), sh
	case "end":
		if lt.rec != nil && lt.recSh == sh {
			it.shown = lt.rec.Recording()
			lt.rec = nil
		}
	}
	lt.mu.Unlock()
	lt.push(it)
}

func (lt *liveTerm) push(it liveMsg) {
	lt.mu.Lock()
	lt.queue = append(lt.queue, it)
	lt.mu.Unlock()
	select {
	case lt.wake <- struct{}{}:
	default:
	}
}

// watchLive passes what the live pane's shell did to Update, in order,
// and says when its screen changed.
func (m *Model) watchLive() tea.Msg {
	lt := m.lterm
	for {
		lt.mu.Lock()
		if len(lt.queue) > 0 {
			it := lt.queue[0]
			lt.queue = lt.queue[1:]
			lt.mu.Unlock()
			return it
		}
		lt.mu.Unlock()
		select {
		case <-lt.wake:
		case <-lt.drawn:
			return liveDrawnMsg{m}
		case <-m.done:
			return nil
		}
	}
}

// onLive handles what the live pane's shell did.
func (m *Model) onLive(msg liveMsg) tea.Cmd {
	lt := m.lterm
	if msg.sh != lt.sh {
		if msg.ev.Kind == "exit" {
			// A shell the pane left has ended: its env starts a new
			// one when the pane comes back.
			for name, pk := range lt.parked {
				if pk.sh == msg.sh {
					delete(lt.parked, name)
				}
			}
		}
		return nil
	}
	switch msg.ev.Kind {
	case "start":
		m.liveStart(msg)
		if lt.busy {
			// fg, or mdsh bringing back the job a played block
			// stopped, has a stopped job run again: once it holds the
			// terminal, the rule says so.
			sh := msg.sh
			time.AfterFunc(200*time.Millisecond, func() { lt.push(liveMsg{sh: sh, ev: env.Event{Kind: "state"}}) })
		}
	case "end":
		if lt.busy {
			m.liveEnd(msg)
		}
	case "exit":
		if lt.busy {
			m.liveEnd(msg)
		}
		if msg.sh == lt.sh {
			lt.sh, lt.jobs = nil, nil
			lt.p.release()
			m.histLog(helpStyle.Render("(the shell ended: type to start a new one)"))
		}
	case "line":
		if text, ok := history.Note(msg.ev.Command); ok {
			m.addNote(text)
		}
	case "state":
		m.liveJobs(msg.sh)
	case "status":
		// What the command typed at the prompt reports about itself.
		lt.recs = msg.ev.Records
		m.forwardStatus(lt.p, m.liveStatusKey(), lt.recs)
	}
	m.refresh()
	return nil
}

// liveStart takes a command typed at the shell's prompt, which has
// started: a note becomes prose; anything else becomes a History block,
// unless recording is off, or it is mdsh bringing back the job a played
// block stopped.
func (m *Model) liveStart(msg liveMsg) {
	lt := m.lterm
	cmd := msg.ev.Command
	if text, ok := history.Note(cmd); ok && !msg.ev.Resumed {
		m.addNote(text)
		return
	}
	lt.busy, lt.cur = true, -1
	if msg.ev.Resumed || strings.TrimSpace(cmd) == "" || m.hist.unrecorded {
		return
	}
	lang := history.Lang(m.shellPath())
	var b *doc.Block
	var err error
	if m.opts.HistoryReuse {
		if b = history.ReuseIn(m.d, lang, cmd, false, lt.env); b != nil {
			id := b.ID
			if err = history.KeepRuns(m.d, b); err == nil {
				err = m.syncSession()
			}
			if err == nil {
				m.foldRuns()
				b = m.d.Block(id)
			}
		}
	}
	if b == nil {
		b, err = m.d.AppendBlock(history.InfoIn(lang, history.NextID(m.d), false, lt.env), cmd)
	}
	if err == nil {
		err = m.sess.Sync()
	}
	if err != nil {
		m.histLog(stderrStyle.Render("can't add the command to the file: " + err.Error()))
		return
	}
	m.hist.noteEnd = -1
	m.results = append(m.results, m.sess.Results()[len(m.results):]...)
	m.layout()
	m.dirty = true
	i := m.index[b]
	if started, err := m.sess.TypedStart(i); err == nil {
		started()
		lt.cur = i
		m.moveTo(i)
	}
}

// liveEnd ends the command liveStart started. While blocks run, as one
// played while the command held the shell does, its result waits for
// the run, which has the session.
func (m *Model) liveEnd(msg liveMsg) {
	lt := m.lterm
	cur := lt.cur
	lt.busy, lt.cur = false, -1
	if cur < 0 {
		return
	}
	msg.ev.Status = statusOf(msg)
	msg.ev.Command = fmt.Sprint(cur)
	if m.running {
		lt.ends = append(lt.ends, msg)
		return
	}
	m.finishLive(cur, msg)
}

func statusOf(msg liveMsg) int {
	if msg.ev.Kind == "exit" {
		return -1
	}
	return msg.ev.Status
}

// finishLive gives block i, a command typed at the prompt, its result,
// and tty if it took the screen.
func (m *Model) finishLive(i int, msg liveMsg) {
	if i >= len(m.d.Blocks) {
		return
	}
	r, full, done, err := m.sess.TypedEnd(i, msg.ev.Status, msg.shown)
	if err != nil || r == nil {
		return
	}
	done()
	m.results[i] = r
	b := r.Block
	if full && !b.Attrs.Has("tty") {
		text := b.Fence + strings.TrimSuffix(b.Info, "}") + " tty}\n" + b.Body + b.Fence + "\n"
		if err := m.d.SetBlock(b, text); err != nil {
			m.histLog(stderrStyle.Render(err.Error()))
		}
		m.sess.Sync()
		m.layout()
	}
	m.dirty = true
	if m.opts.AutoSave {
		m.save(false)
	}
}

// finishLiveEnds gives the typed commands that ended during a run their
// results, now that it is over.
func (m *Model) finishLiveEnds() {
	lt := m.lterm
	ends := lt.ends
	lt.ends = nil
	for _, e := range ends {
		var i int
		fmt.Sscan(e.ev.Command, &i)
		m.finishLive(i, e)
	}
}

// liveJobs reads the shell's jobs, for the pane's rule. A job brought
// back to the foreground is the command that runs, not one beside it.
func (m *Model) liveJobs(sh *env.Shell) {
	lt := m.lterm
	if sh != lt.sh {
		return
	}
	fg := sh.Foreground()
	lt.jobs = lt.jobs[:0]
	for _, j := range sh.Jobs() {
		if fg != 0 && j.PGID == fg {
			continue
		}
		lt.jobs = append(lt.jobs, j)
	}
}

// jobsText names the shell's jobs, a stopped one marked, for the rule.
func (lt *liveTerm) jobsText() string {
	var parts []string
	for _, j := range lt.jobs {
		label := j.Command
		if j.N > 0 {
			label = fmt.Sprintf("%%%d %s", j.N, j.Command)
		}
		parts = append(parts, label+" ("+j.State+")")
	}
	return strings.Join(parts, " · ")
}

// liveKey sends k to the shell, starting a new one if it ended; ctrl+g
// gives the keys back to the file.
func (m *Model) liveKey(k tea.KeyMsg) tea.Cmd {
	if k.String() == "ctrl+g" {
		m.hist.focus = false
		if m.hist.full {
			m.hist.full = false
			m.resize(m.width, m.height)
		}
		return nil
	}
	if m.lterm.sh == nil {
		if err := m.attachLive(); err != nil {
			m.status = "can't start the shell: " + err.Error()
			return nil
		}
	}
	m.lterm.p.typeKey(k)
	return nil
}
