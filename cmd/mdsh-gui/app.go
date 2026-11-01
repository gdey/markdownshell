package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
	"github.com/gdey/markdownshell/run"
)

// App is the service the frontend calls. A call from a tab's page acts on
// that tab; one from a window's tab bar, on that window.
type App struct {
	app      *application.App
	settings *settings.Store
	cwd      string // the folder mdsh-gui started in
	dir      string // the folder given on the command line, if any
	source   string // the files given to --source, read at startup

	menu       *application.Menu
	viewItems  struct{ split, rendered, outline, history, code, output *application.MenuItem }
	recentMenu *application.Menu

	mu        sync.Mutex
	windows   map[uint]*docWindow
	tabs      map[int]*docTab
	tabWindow map[*docTab]*docWindow
	named     int // windows made so far, for unique names
	lastTab   int // the last tab id handed out
	dragging  *docTab
	prefs     *application.WebviewWindow
	quitting  bool

	started atomic.Bool // the app is running, so menus change on its main thread
}

// runOptions are the options a file opens with: the shell and History
// rc file from Settings, and what --source read.
func (a *App) runOptions() notebook.RunOptions {
	s := a.settings.Get()
	return notebook.RunOptions{Shell: s.Shell, Source: a.source, HistoryRC: s.HistoryRC}
}

// errNoFile is returned by calls that need a file before one is open.
var errNoFile = errors.New("no file is open")

// tabKey is the context key for the tab a call came from, which the
// tab's page sends in the x-mdsh-tab header.
type tabKey struct{}

const tabHeader = "x-mdsh-tab"

// tabCalls notes the tab each call from a tab's page came from.
func tabCalls(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if id, err := strconv.Atoi(r.Header.Get(tabHeader)); err == nil {
			r = r.WithContext(context.WithValue(r.Context(), tabKey{}, id))
		}
		next.ServeHTTP(rw, r)
	})
}

// newWindow opens a window holding t, or a new tab showing the start
// screen if t is nil.
func (a *App) newWindow(t *docTab) *docWindow {
	a.mu.Lock()
	a.named++
	name := fmt.Sprintf("doc-%d", a.named)
	a.mu.Unlock()
	opts := application.WebviewWindowOptions{
		Name:           name,
		Title:          "mdsh",
		Width:          960,
		Height:         820,
		URL:            "/window.html",
		EnableFileDrop: true,
	}
	opts.Linux.Menu = a.menu
	w := &docWindow{app: a}
	w.win = a.app.Window.NewWithOptions(opts)
	a.mu.Lock()
	a.windows[w.win.ID()] = w
	a.mu.Unlock()
	w.hook()
	if t == nil {
		t = a.newTab()
	}
	a.moveTab(t, w, 0)
	return w
}

// newTab makes a tab showing the start screen, in no window yet.
func (a *App) newTab() *docTab {
	a.mu.Lock()
	a.lastTab++
	t := &docTab{app: a, id: a.lastTab, answers: make(chan run.Answer), done: make(chan struct{}), view: defaultView}
	a.tabs[t.id] = t
	a.mu.Unlock()
	go t.watch()
	return t
}

// newTabIn opens a tab with the start screen in w, after its tab in
// front.
func (a *App) newTabIn(w *docWindow) *docTab {
	t := a.newTab()
	a.mu.Lock()
	i := slices.Index(w.tabs, w.active) + 1
	a.mu.Unlock()
	a.moveTab(t, w, i)
	return t
}

// cycleTab brings the tab by steps from the one in front of w forward.
func (a *App) cycleTab(w *docWindow, by int) {
	a.mu.Lock()
	t := w.next(by)
	a.mu.Unlock()
	if t != nil {
		a.selectTab(t)
	}
}

// moveTab puts t in w at index i, in front, taking it out of the window
// it was in. A window left with no tabs closes.
func (a *App) moveTab(t *docTab, w *docWindow, i int) {
	a.mu.Lock()
	from := a.tabWindow[t]
	if from != nil {
		if j := from.remove(t); from == w && j < i {
			i--
		}
	}
	w.insert(t, i)
	a.tabWindow[t] = w
	empty := from != nil && from != w && len(from.tabs) == 0
	if empty {
		from.closing = true
	}
	a.mu.Unlock()
	if from != nil && from != w {
		if empty {
			from.win.Close()
		} else {
			from.changed()
		}
	}
	w.changed()
	a.showTabView(t)
}

// selectTab brings t to the front of its window.
func (a *App) selectTab(t *docTab) {
	a.mu.Lock()
	w := a.tabWindow[t]
	if w != nil {
		w.active = t
	}
	a.mu.Unlock()
	if w != nil {
		w.changed()
		a.showTabView(t)
	}
}

func (a *App) showTabView(t *docTab) {
	t.mu.Lock()
	v := t.view
	t.mu.Unlock()
	a.showView(v)
}

// closeTab closes t, asking first if it has unsaved output. Its window
// closes with its last tab.
func (a *App) closeTab(t *docTab) {
	if n := t.unsaved(); n != nil {
		a.selectTab(t)
		t.askToSave(n, func() { a.closeTab(t) })
		return
	}
	a.mu.Lock()
	w := a.tabWindow[t]
	last := w != nil && len(w.tabs) == 1
	a.mu.Unlock()
	if last {
		// The window's closing hook lets it go, as the tab may close.
		w.win.Close()
		return
	}
	a.dropTab(t)
	if w != nil {
		w.changed()
		if t := w.activeTab(); t != nil {
			a.showTabView(t)
		}
	}
}

// dropTab takes t out of its window and ends its shells.
func (a *App) dropTab(t *docTab) {
	a.mu.Lock()
	if w := a.tabWindow[t]; w != nil {
		w.remove(t)
	}
	delete(a.tabWindow, t)
	delete(a.tabs, t.id)
	if a.dragging == t {
		a.dragging = nil
	}
	a.mu.Unlock()
	t.shutdown()
}

func (a *App) removeWindow(w *docWindow) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.windows, w.win.ID())
}

// window is the window a frontend call came from.
func (a *App) window(ctx context.Context) (*docWindow, error) {
	win, ok := ctx.Value(application.WindowKey).(application.Window)
	if !ok {
		return nil, errors.New("the call came from no window")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	w := a.windows[win.ID()]
	if w == nil {
		return nil, errors.New("the call came from a window with no document")
	}
	return w, nil
}

// tab is the tab a frontend call came from.
func (a *App) tab(ctx context.Context) (*docTab, error) {
	id, ok := ctx.Value(tabKey{}).(int)
	if !ok {
		return nil, errors.New("the call came from no tab")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	t := a.tabs[id]
	if t == nil {
		return nil, fmt.Errorf("no tab %d", id)
	}
	return t, nil
}

// tabByID is tab id, for calls from a tab bar about one of its tabs.
func (a *App) tabByID(id int) (*docTab, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t := a.tabs[id]
	if t == nil {
		return nil, fmt.Errorf("no tab %d", id)
	}
	return t, nil
}

func (a *App) notebook(ctx context.Context) (*docTab, *notebook.Notebook, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return nil, nil, err
	}
	n, err := t.notebook()
	return t, n, err
}

// current is the document window in front, for menu commands, or nil.
func (a *App) current() *docWindow {
	win := a.app.Window.Current()
	a.mu.Lock()
	defer a.mu.Unlock()
	if win != nil {
		if w := a.windows[win.ID()]; w != nil {
			return w
		}
	}
	if len(a.windows) == 1 {
		for _, w := range a.windows {
			return w
		}
	}
	return nil
}

// currentTab is the tab in front of the window in front, or nil.
func (a *App) currentTab() *docTab {
	if w := a.current(); w != nil {
		return w.activeTab()
	}
	return nil
}

// openTab is the tab file is open in, or nil.
func (a *App) openTab(file string) *docTab {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil
	}
	a.mu.Lock()
	tabs := make([]*docTab, 0, len(a.tabs))
	for _, t := range a.tabs {
		tabs = append(tabs, t)
	}
	a.mu.Unlock()
	for _, t := range tabs {
		if t.file() == abs {
			return t
		}
	}
	return nil
}

// focusTab brings t and its window to the front.
func (a *App) focusTab(t *docTab) {
	a.selectTab(t)
	if w := t.window(); w != nil {
		w.win.Focus()
	}
}

// openIn opens file in w: in its tab in front if that shows the start
// screen, else in a new tab. With no window it opens a new one. A file
// already open in a tab brings that tab forward.
func (a *App) openIn(w *docWindow, file string) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	if t := a.openTab(abs); t != nil {
		a.focusTab(t)
		return nil
	}
	ro := a.runOptions()
	if w != nil {
		if t := w.activeTab(); t != nil && t.file() == "" {
			return t.open(abs, ro)
		}
		t := a.newTab()
		if err := t.open(abs, ro); err != nil {
			a.dropTab(t)
			return err
		}
		a.mu.Lock()
		i := slices.Index(w.tabs, w.active) + 1
		a.mu.Unlock()
		a.moveTab(t, w, i)
		return nil
	}
	t := a.newTab()
	if err := t.open(abs, ro); err != nil {
		a.dropTab(t)
		return err
	}
	a.newWindow(t)
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
	for _, t := range a.tabs {
		if n, err := t.notebook(); err == nil && n.Dirty() {
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
			a.mu.Unlock()
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
	w := a.current()
	var win application.Window
	if w != nil {
		win = w.win
	}
	files, err := a.openDialog(win)
	if err != nil {
		a.showError(win, err)
		return
	}
	for _, f := range files {
		if err := a.openIn(w, f); err != nil {
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
		Name:      "settings",
		Title:     "Settings",
		Width:     600,
		Height:    520,
		MinWidth:  480,
		MinHeight: 360,
		URL:       "/settings.html",
	}
	a.prefs = a.app.Window.NewWithOptions(opts)
	a.prefs.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		a.mu.Lock()
		a.prefs = nil
		a.mu.Unlock()
	})
}
