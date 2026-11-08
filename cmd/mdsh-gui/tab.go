package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gdey/markdownshell/audit"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/nvim"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/writable"
)

// watchEvery is how often a tab checks its file for changes on disk.
const watchEvery = 2 * time.Second

// docTab is one tab and the file open in it, if any. Each tab has its own
// notebook and shells, which go with it when it moves to another window.
type docTab struct {
	app *App
	id  int
	// page is the page the tab shows in place of a document's, such as
	// changelogPage or settingsPage, or "". It is set before the tab goes in a window
	// and never changes. A tab with a page has no file and opens none.
	page string
	// at is the part of page a new tab opens at, such as Settings'
	// "about", or "". It too is set before the tab goes in a window.
	at      string
	answers chan run.Answer
	done    chan struct{} // closed when the tab closes

	mu      sync.Mutex
	n       *notebook.Notebook // nil while the start screen shows
	nDone   chan struct{}      // closed when n is replaced or closed
	closing bool               // the user chose to close it
	warned  bool               // file-changed was sent for the file on disk
	fresh   bool               // a file opened and its page hasn't asked about it
	// openHistory is set when a session opened in the tab and its page
	// hasn't asked about it.
	openHistory bool
	view        View         // what the tab shows, for the View menu
	running     bool         // a run started from the tab hasn't returned
	live        []liveEvent  // what the running run has sent so far
	nvim        *nvim.Editor // the source pane's Neovim, if running
	roRun       string       // the read-only file to run without saving
	// events is the event log of the file open in the tab, which
	// reloading keeps and opening another file starts afresh; auditQ
	// takes its new events to sendAudit.
	events *notebook.EventLog
	auditQ chan notebook.AuditEvent
}

// newEvents starts an event log for a file opening in the tab.
func (t *docTab) newEvents() *notebook.EventLog {
	return notebook.NewEventLog(func(e notebook.AuditEvent) {
		select {
		case t.auditQ <- e:
		default:
			// The page sees the gap in Seq and reads the log again.
		}
	})
}

// sendAudit sends the tab's page each new event, as an "audit" event,
// until the tab closes. It runs on its own goroutine, since events are
// logged with the notebook's lock held.
func (t *docTab) sendAudit() {
	for {
		select {
		case <-t.done:
			return
		case e := <-t.auditQ:
			t.emit("audit", e)
		}
	}
}

// eventLog is the event log of the tab's file, or nil.
func (t *docTab) eventLog() *notebook.EventLog {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.events
}

// askReadOnly reports whether a run of file should first ask for a copy
// to save to: file is read-only and the user hasn't chosen to run it
// without saving.
func (t *docTab) askReadOnly(file string) bool {
	t.mu.Lock()
	skip := t.roRun == file
	t.mu.Unlock()
	return !skip && errors.Is(checkWritable(file), writable.ErrReadOnly)
}

// runReadOnly notes that runs of file go ahead without asking.
func (t *docTab) runReadOnly(file string) {
	t.mu.Lock()
	t.roRun = file
	t.mu.Unlock()
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
	if t.running && name != "menu" && name != "audit" {
		t.live = append(t.live, liveEvent{name, data})
	}
	t.mu.Unlock()
	if w := t.window(); w != nil {
		w.win.EmitEvent(name, tabEvent{t.id, data})
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
	// A run of the notebook this one replaces may still be going, or
	// waiting on a question. Once done closes, its questions are answered
	// stop and what it sends no longer reaches the page, which now shows
	// the new notebook.
	done := make(chan struct{})
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	events := t.eventLog()
	if events == nil || abs != t.file() {
		events = t.newEvents()
	}
	n, err := notebook.Open(file, notebook.Options{
		Run:    ro,
		Events: events,
		OnEvent: func(e notebook.Event) {
			select {
			case <-done:
			default:
				t.emit("block", e)
			}
		},
		Confirm: func(block int, question string) run.Answer {
			return t.confirm(done, block, question)
		},
		Depend: func(block, of int, question string) run.Answer {
			return t.depend(done, block, of, question)
		},
		// Asked once per file, and again when its list changes: the
		// answer is remembered, in the same file mdsh and mdsh -i use.
		Sources: t.app.trust.Ask(abs, func(paths []string) run.Answer {
			return t.ask(done, "sources", map[string]any{"files": paths})
		}),
		// A warning an edit fixed goes stale in the console.
		Resolved: func(file, warning string) { t.app.console.resolve(file, warning) },
	})
	if err != nil {
		return err
	}
	t.mu.Lock()
	old, oldDone := t.n, t.nDone
	t.n, t.nDone, t.warned, t.events = n, done, false, events
	t.fresh = old == nil || old.File() != n.File()
	t.mu.Unlock()
	if oldDone != nil {
		close(oldDone)
	}
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
	// URL is the address of a document the app carries inside it, such
	// as the changelog, for a tab that shows one and has no File. A tab
	// dragged out to another app gives it File, or else URL.
	URL   string `json:"url,omitempty"`
	Dirty bool   `json:"dirty"`
	// Page is the page the tab shows, if not a document's.
	Page string `json:"page,omitempty"`
	// ReadOnly is set for a page that can't be edited, such as the
	// changelog.
	ReadOnly bool `json:"readOnly,omitempty"`
}

func (t *docTab) info() Tab {
	switch t.page {
	case changelogPage:
		return Tab{ID: t.id, Title: "Changelog", URL: changelogURL, Page: t.page, ReadOnly: true}
	case settingsPage:
		return Tab{ID: t.id, Title: "Settings", Page: settingsURL(t.at)}
	case cheatsheetPage:
		return Tab{ID: t.id, Title: "Cheat sheet", URL: cheatsheetURL, Page: t.page, ReadOnly: true}
	}
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
// Answer, or for stale to close: the notebook asking was replaced.
func (t *docTab) confirm(stale <-chan struct{}, block int, question string) run.Answer {
	return t.ask(stale, "confirm", map[string]any{"block": block, "question": question})
}

// depend asks the frontend whether to run block, which block of
// depends on, first, as confirm does.
func (t *docTab) depend(stale <-chan struct{}, block, of int, question string) run.Answer {
	return t.ask(stale, "confirm", map[string]any{"block": block, "of": of, "question": question})
}

// ask sends the frontend a question, as a confirm or sources event
// called name, and waits for Answer. It answers stop itself once the tab
// closes or stale closes.
func (t *docTab) ask(stale <-chan struct{}, name string, q map[string]any) run.Answer {
	select {
	case <-stale:
		return run.Stop
	default:
	}
	t.emit(name, q)
	select {
	case <-stale:
		return run.Stop
	case a := <-t.answers:
		t.mu.Lock()
		live := t.live[:0]
		for _, e := range t.live {
			if e.Name != name {
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
		events := t.events
		t.mu.Unlock()
		events.Event("file.changed", audit.Fields{"file": n.File()})
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
	d := t.app.app.Dialog.Question().
		SetTitle("Unsaved output").
		SetMessage(fmt.Sprintf("Save the output in %s before closing?", filepath.Base(n.File()))).
		AttachToWindow(w.win)
	save := d.AddButton("Save")
	save.OnClick(func() {
		if err := n.Save(); err != nil {
			t.app.showError(w.win, err)
			return
		}
		t.letClose()
		then()
	})
	d.AddButton("Don't Save").OnClick(func() {
		t.letClose()
		then()
	})
	d.SetDefaultButton(save)
	d.SetCancelButton(d.AddButton("Cancel"))
	d.Show()
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
	t.stopNvim()
	if n != nil {
		n.Close()
	}
}
