package ui

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gdey/markdownshell/notebook"
	"github.com/gdey/markdownshell/run"
)

// watchEvery is how often a tab checks its file for changes on disk.
const watchEvery = 2 * time.Second

// docTab is one tab and the file open in it, if any. Each tab has its own
// notebook and shells, which go with it when it moves to another window.
type docTab struct {
	app     *App
	id      int
	answers chan run.Answer
	done    chan struct{} // closed when the tab closes

	mu      sync.Mutex
	n       *notebook.Notebook // nil while the start screen shows
	closing bool               // the user chose to close it
	warned  bool               // file-changed was sent for the file on disk
	fresh   bool               // a file opened and its page hasn't asked about it
	view    View               // what the tab shows, for the View menu
	running bool               // a run started from the tab hasn't returned
	live    []liveEvent        // what the running run has sent so far
}

// liveEvent is an event sent while a run is going, kept so a tab drawn
// again partway through, as after moving to another window, can catch
// up.
type liveEvent struct {
	Name string `json:"name"`
	Data any    `json:"data"`
}

// tabEvent is an event for one tab. The window passes Data on to that
// tab's page only.
type tabEvent struct {
	Tab  int `json:"mdshTab"`
	Data any `json:"data"`
}

// emit sends an event to this tab's page, in whichever window holds it.
func (t *docTab) emit(name string, data any) {
	t.mu.Lock()
	if t.running && name != "menu" {
		t.live = append(t.live, liveEvent{name, data})
	}
	t.mu.Unlock()
	if w := t.window(); w != nil {
		w.win.Emit(name, tabEvent{t.id, data})
	}
}

// window is the window holding the tab, or nil once it has closed.
func (t *docTab) window() *docWindow {
	t.app.mu.Lock()
	defer t.app.mu.Unlock()
	return t.app.tabWindow[t]
}

func (t *docTab) notebook() (*notebook.Notebook, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.n == nil {
		return nil, errNoFile
	}
	return t.n, nil
}

// file is the open file's path, or "".
func (t *docTab) file() string {
	if n, err := t.notebook(); err == nil {
		return n.File()
	}
	return ""
}

// open opens file in this tab, closing the file open in it.
func (t *docTab) open(file string, ro notebook.RunOptions) error {
	n, err := notebook.Open(file, notebook.Options{
		Run:     ro,
		OnEvent: func(e notebook.Event) { t.emit("block", e) },
		Confirm: t.confirm,
		Depend:  t.depend,
	})
	if err != nil {
		return err
	}
	t.mu.Lock()
	old := t.n
	t.n, t.warned = n, false
	t.fresh = old == nil || old.File() != n.File()
	t.mu.Unlock()
	if old != nil {
		old.Close()
	}
	t.changed()
	t.app.opened(n.File())
	t.emit("opened", n.File())
	return nil
}

// Tab is a tab as the tab bar draws it.
type Tab struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	File  string `json:"file"`
	Dirty bool   `json:"dirty"`
}

func (t *docTab) info() Tab {
	n, err := t.notebook()
	if err != nil {
		return Tab{ID: t.id, Title: "Start"}
	}
	return Tab{ID: t.id, Title: filepath.Base(n.File()), File: n.File(), Dirty: n.Dirty()}
}

// changed redraws the tab's window's tab bar and title, after the tab's
// file or its dirty mark changed.
func (t *docTab) changed() {
	if w := t.window(); w != nil {
		w.changed()
	}
}

// confirm asks the frontend, from the run's goroutine, and waits for
// Answer.
func (t *docTab) confirm(block int, question string) run.Answer {
	return t.ask(map[string]any{"block": block, "question": question})
}

// depend asks the frontend whether to run block, which block of
// depends on, first, as confirm does.
func (t *docTab) depend(block, of int, question string) run.Answer {
	return t.ask(map[string]any{"block": block, "of": of, "question": question})
}

// ask sends the frontend a confirm event and waits for Answer.
func (t *docTab) ask(q map[string]any) run.Answer {
	t.emit("confirm", q)
	select {
	case a := <-t.answers:
		t.mu.Lock()
		live := t.live[:0]
		for _, e := range t.live {
			if e.Name != "confirm" {
				live = append(live, e)
			}
		}
		t.live = live
		t.mu.Unlock()
		return a
	case <-t.done:
		return run.Stop
	}
}

// run runs f, a run of the tab's notebook, keeping what it sends for
// Live, and says when it is over with a "ran" event.
func (t *docTab) run(f func() error) error {
	t.mu.Lock()
	t.running, t.live = true, nil
	t.mu.Unlock()
	err := f()
	t.mu.Lock()
	t.running, t.live = false, nil
	t.mu.Unlock()
	t.emit("ran", nil)
	return err
}

// watch tells the frontend once when the file changes on disk, until
// the file is saved or opened again.
func (t *docTab) watch() {
	tick := time.NewTicker(watchEvery)
	defer tick.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-tick.C:
		}
		t.mu.Lock()
		n, warned := t.n, t.warned
		t.mu.Unlock()
		if n == nil || warned || !n.Changed() {
			continue
		}
		t.mu.Lock()
		t.warned = true
		t.mu.Unlock()
		t.emit("file-changed", n.File())
	}
}

// saved notes a save: the file on disk is ours again.
func (t *docTab) saved() {
	t.mu.Lock()
	t.warned = false
	t.mu.Unlock()
	t.changed()
	t.emit("saved", t.file())
}

// unsaved is the notebook if it has output that isn't saved and the
// user hasn't said to close the tab anyway.
func (t *docTab) unsaved() *notebook.Notebook {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing || t.n == nil || !t.n.Dirty() {
		return nil
	}
	return t.n
}

// askToSave asks what to do with the tab's unsaved output before it
// closes, and calls then if it may close.
func (t *docTab) askToSave(n *notebook.Notebook, then func()) {
	w := t.window()
	if w == nil {
		return
	}
	t.app.host.Ask(w.win, Question{
		Title:   "Unsaved output",
		Message: fmt.Sprintf("Save the output in %s before closing?", filepath.Base(n.File())),
		Buttons: []Button{
			{Label: "Save", Default: true, Do: func() {
				if err := n.Save(); err != nil {
					t.app.host.ShowError(w.win, err)
					return
				}
				t.letClose()
				then()
			}},
			{Label: "Don't Save", Do: func() {
				t.letClose()
				then()
			}},
			{Label: "Cancel", Cancel: true},
		},
	})
}

func (t *docTab) letClose() {
	t.mu.Lock()
	t.closing = true
	t.mu.Unlock()
}

// shutdown ends the tab's shells once it closes.
func (t *docTab) shutdown() {
	t.mu.Lock()
	n := t.n
	t.n = nil
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	t.mu.Unlock()
	if n != nil {
		n.Close()
	}
}
