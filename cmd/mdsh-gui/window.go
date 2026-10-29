package main

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/run"
)

// watchEvery is how often a window checks its file for changes on disk.
const watchEvery = 2 * time.Second

// docWindow is one window and the file open in it, if any. Each window
// has its own notebook and shells.
type docWindow struct {
	app     *App
	win     *application.WebviewWindow
	answers chan run.Answer
	done    chan struct{} // closed when the window closes

	mu      sync.Mutex
	n       *notebook.Notebook // nil while the start screen shows
	closing bool               // the user chose to close it
	warned  bool               // file-changed was sent for the file on disk
}

// emit sends an event to this window's frontend. Events reach every
// window, so the frontend keeps those its own window sent.
func (dw *docWindow) emit(name string, data any) { dw.win.EmitEvent(name, data) }

func (dw *docWindow) notebook() (*notebook.Notebook, error) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	if dw.n == nil {
		return nil, errNoFile
	}
	return dw.n, nil
}

// file is the open file's path, or "".
func (dw *docWindow) file() string {
	if n, err := dw.notebook(); err == nil {
		return n.File()
	}
	return ""
}

// open opens file in this window, closing the file open in it.
func (dw *docWindow) open(file string, ro notebook.RunOptions) error {
	n, err := notebook.Open(file, notebook.Options{
		Run:     ro,
		OnEvent: func(e notebook.Event) { dw.emit("block", e) },
		Confirm: dw.confirm,
	})
	if err != nil {
		return err
	}
	dw.mu.Lock()
	old := dw.n
	dw.n, dw.warned = n, false
	dw.mu.Unlock()
	if old != nil {
		old.Close()
	}
	dw.title()
	dw.app.opened(n.File())
	dw.emit("opened", n.File())
	return nil
}

// title shows the file's name, marked while it has unsaved output.
func (dw *docWindow) title() {
	n, err := dw.notebook()
	if err != nil {
		dw.win.SetTitle("mdsh")
		return
	}
	t := filepath.Base(n.File()) + " — mdsh"
	if n.Dirty() {
		t = "• " + t
	}
	dw.win.SetTitle(t)
}

// confirm asks the frontend, from the run's goroutine, and waits for
// Answer.
func (dw *docWindow) confirm(block int, question string) run.Answer {
	dw.emit("confirm", map[string]any{"block": block, "question": question})
	select {
	case a := <-dw.answers:
		return a
	case <-dw.done:
		return run.Stop
	}
}

// watch tells the frontend once when the file changes on disk, until
// the file is saved or opened again.
func (dw *docWindow) watch() {
	t := time.NewTicker(watchEvery)
	defer t.Stop()
	for {
		select {
		case <-dw.done:
			return
		case <-t.C:
		}
		dw.mu.Lock()
		n, warned := dw.n, dw.warned
		dw.mu.Unlock()
		if n == nil || warned || !n.Changed() {
			continue
		}
		dw.mu.Lock()
		dw.warned = true
		dw.mu.Unlock()
		dw.emit("file-changed", n.File())
	}
}

// saved notes a save: the file on disk is ours again.
func (dw *docWindow) saved() {
	dw.mu.Lock()
	dw.warned = false
	dw.mu.Unlock()
	dw.title()
	dw.emit("saved", dw.file())
}

// closingHook keeps a window with unsaved output open until the user
// says what to do with it.
func (dw *docWindow) closingHook(e *application.WindowEvent) {
	dw.mu.Lock()
	n, closing := dw.n, dw.closing
	dw.mu.Unlock()
	if !closing && n != nil && n.Dirty() {
		e.Cancel()
		go dw.askToSave(n)
		return
	}
	dw.shutdown()
}

func (dw *docWindow) askToSave(n *notebook.Notebook) {
	d := dw.app.app.Dialog.Question().
		SetTitle("Unsaved output").
		SetMessage(fmt.Sprintf("Save the output in %s before closing?", filepath.Base(n.File()))).
		AttachToWindow(dw.win)
	save := d.AddButton("Save")
	save.OnClick(func() {
		if err := n.Save(); err != nil {
			dw.app.showError(dw.win, err)
			return
		}
		dw.closeNow()
	})
	d.AddButton("Don't Save").OnClick(dw.closeNow)
	d.SetDefaultButton(save)
	d.SetCancelButton(d.AddButton("Cancel"))
	d.Show()
}

// closeNow closes the window without asking.
func (dw *docWindow) closeNow() {
	dw.mu.Lock()
	dw.closing = true
	dw.mu.Unlock()
	dw.win.Close()
}

// shutdown ends the window's shells once it closes.
func (dw *docWindow) shutdown() {
	dw.mu.Lock()
	n := dw.n
	dw.n = nil
	select {
	case <-dw.done:
	default:
		close(dw.done)
	}
	dw.mu.Unlock()
	if n != nil {
		n.Close()
	}
	dw.app.removeWindow(dw)
}

// dropped opens markdown files dropped on the window.
func (dw *docWindow) dropped(e *application.WindowEvent) {
	files := e.Context().DroppedFiles()
	go func() {
		for _, f := range files {
			if ext := filepath.Ext(f); ext != ".md" && ext != ".markdown" {
				continue
			}
			if err := dw.app.openIn(dw, f); err != nil {
				dw.app.showError(dw.win, err)
			}
		}
	}()
}

func (dw *docWindow) hook() {
	dw.win.RegisterHook(events.Common.WindowClosing, dw.closingHook)
	dw.win.OnWindowEvent(events.Common.WindowFilesDropped, dw.dropped)
	go dw.watch()
}
