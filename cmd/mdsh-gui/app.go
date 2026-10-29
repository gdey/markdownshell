package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
	"github.com/gdey/markdownshell/run"
)

// App is the service the frontend calls. A call acts on the window it
// came from.
type App struct {
	app      *application.App
	settings *settings.Store
	cwd      string // the folder mdsh-gui started in
	dir      string // the folder given on the command line, if any

	menu       *application.Menu
	viewItems  struct{ split, rendered, outline, code, output *application.MenuItem }
	recentMenu *application.Menu

	mu       sync.Mutex
	windows  map[uint]*docWindow
	named    int // windows made so far, for unique names
	prefs    *application.WebviewWindow
	quitting bool

	started atomic.Bool // the app is running, so menus change on its main thread
}

// errNoFile is returned by calls that need a file before one is open.
var errNoFile = errors.New("no file is open")

// newWindow opens a window showing the start screen.
func (a *App) newWindow() *docWindow {
	a.mu.Lock()
	a.named++
	name := fmt.Sprintf("doc-%d", a.named)
	a.mu.Unlock()
	opts := application.WebviewWindowOptions{
		Name:           name,
		Title:          "mdsh",
		Width:          960,
		Height:         820,
		URL:            "/",
		EnableFileDrop: true,
	}
	opts.Linux.Menu = a.menu
	dw := &docWindow{app: a, answers: make(chan run.Answer), done: make(chan struct{})}
	dw.win = a.app.Window.NewWithOptions(opts)
	a.mu.Lock()
	a.windows[dw.win.ID()] = dw
	a.mu.Unlock()
	dw.hook()
	return dw
}

func (a *App) removeWindow(dw *docWindow) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.windows, dw.win.ID())
}

// window is the window a frontend call came from.
func (a *App) window(ctx context.Context) (*docWindow, error) {
	w, ok := ctx.Value(application.WindowKey).(application.Window)
	if !ok {
		return nil, errors.New("the call came from no window")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	dw := a.windows[w.ID()]
	if dw == nil {
		return nil, errors.New("the call came from a window with no document")
	}
	return dw, nil
}

func (a *App) notebook(ctx context.Context) (*docWindow, *notebook.Notebook, error) {
	dw, err := a.window(ctx)
	if err != nil {
		return nil, nil, err
	}
	n, err := dw.notebook()
	return dw, n, err
}

// current is the document window in front, for menu commands, or nil.
func (a *App) current() *docWindow {
	w := a.app.Window.Current()
	a.mu.Lock()
	defer a.mu.Unlock()
	if w != nil {
		if dw := a.windows[w.ID()]; dw != nil {
			return dw
		}
	}
	if len(a.windows) == 1 {
		for _, dw := range a.windows {
			return dw
		}
	}
	return nil
}

// openIn opens file in dw if dw shows the start screen, else in a new
// window. A file already open in a window brings that window forward.
func (a *App) openIn(dw *docWindow, file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	a.mu.Lock()
	var open *docWindow
	for _, w := range a.windows {
		if w.file() == abs {
			open = w
		}
	}
	a.mu.Unlock()
	if open != nil {
		open.win.Focus()
		return nil
	}
	ro := notebook.RunOptions{Shell: a.settings.Get().Shell}
	if dw != nil && dw.file() == "" {
		return dw.open(abs, ro)
	}
	nw := a.newWindow()
	if err := nw.open(abs, ro); err != nil {
		nw.closeNow()
		return err
	}
	return nil
}

// opened records file in Open Recent.
func (a *App) opened(file string) {
	if err := a.settings.Update(func(s *settings.Settings) { s.Opened(file) }); err != nil {
		a.app.Logger.Warn("saving settings", "err", err)
	}
	a.fillRecent()
}

func (a *App) showError(win application.Window, err error) {
	d := a.app.Dialog.Error().SetTitle("mdsh").SetMessage(err.Error())
	if win != nil {
		d.AttachToWindow(win)
	}
	d.Show()
}

// shouldQuit asks before quitting with unsaved output in any window.
func (a *App) shouldQuit() bool {
	a.mu.Lock()
	quitting := a.quitting
	var dirty []string
	for _, dw := range a.windows {
		if n, err := dw.notebook(); err == nil && n.Dirty() {
			dirty = append(dirty, filepath.Base(n.File()))
		}
	}
	a.mu.Unlock()
	if quitting || len(dirty) == 0 {
		return true
	}
	go func() {
		d := a.app.Dialog.Question().
			SetTitle("Unsaved output").
			SetMessage("These files have output that isn't saved: " + strings.Join(dirty, ", ") + ". Quit anyway?")
		quit := d.AddButton("Quit")
		quit.OnClick(func() {
			a.mu.Lock()
			a.quitting = true
			windows := make([]*docWindow, 0, len(a.windows))
			for _, dw := range a.windows {
				windows = append(windows, dw)
			}
			a.mu.Unlock()
			for _, dw := range windows {
				dw.mu.Lock()
				dw.closing = true
				dw.mu.Unlock()
			}
			a.app.Quit()
		})
		d.SetCancelButton(d.AddButton("Cancel"))
		d.Show()
	}()
	return false
}

// openDialog asks for markdown files, starting in the start folder.
func (a *App) openDialog(win application.Window) ([]string, error) {
	d := a.app.Dialog.OpenFile().
		SetTitle("Open a markdown file").
		AddFilter("Markdown", "*.md;*.markdown").
		SetDirectory(a.startDir())
	if win != nil {
		d.AttachToWindow(win)
	}
	return d.PromptForMultipleSelection()
}

// openFromMenu runs File › Open… for the window in front.
func (a *App) openFromMenu() {
	dw := a.current()
	var win application.Window
	if dw != nil {
		win = dw.win
	}
	files, err := a.openDialog(win)
	if err != nil {
		a.showError(win, err)
		return
	}
	for _, f := range files {
		if err := a.openIn(dw, f); err != nil {
			a.showError(win, err)
		}
	}
}

func (a *App) startDir() string {
	if a.dir != "" {
		return a.dir
	}
	home, _ := os.UserHomeDir()
	docs := ""
	if runtime.GOOS == "darwin" {
		docs = filepath.Join(home, "Documents")
	}
	return a.settings.Get().StartDir(a.cwd, home, docs)
}

// openSettings shows the settings window, making it the first time.
func (a *App) openSettings() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prefs != nil {
		a.prefs.Show()
		a.prefs.Focus()
		return
	}
	opts := application.WebviewWindowOptions{
		Name:   "settings",
		Title:  "Settings",
		Width:  520,
		Height: 420,
		URL:    "/settings.html",
	}
	a.prefs = a.app.Window.NewWithOptions(opts)
	a.prefs.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.mu.Lock()
		a.prefs = nil
		a.mu.Unlock()
	})
}
