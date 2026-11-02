package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/notebook"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/settings"
	"github.com/gdey/markdownshell/trust"
)

// App is the service the frontend calls. A call from a tab's page acts on
// that tab; one from a window's tab bar, on that window. Its exported
// methods are all calls the frontend makes; the Host drives it through
// its Control.
type App struct {
	host     Host
	settings *settings.Store
	cwd      string // the folder the program started in
	dir      string // the folder given on the command line, if any
	source   string // the files given to --source, read at startup
	console  *Console
	// trust remembers which files' env.source files may be read into
	// their shells. Nil remembers nothing, so each file asks every time.
	trust *trust.Store
	build Build // the program's, for the About tab
	// lsp is the command that runs the language server for a file, for
	// the Neovim source pane, or nil for none.
	lsp func(file string) []string

	mu        sync.Mutex
	windows   map[uint]*docWindow
	tabs      map[int]*docTab
	tabWindow map[*docTab]*docWindow
	lastTab   int // the last tab id handed out
	dragging  *docTab
	quitting  bool
	// startImages are the collection icons and backgrounds the start
	// screen shows, by real path, which the window may load.
	startImages map[string]bool
	// startFolders are the folders the start screen lists, by rel, with
	// their absolute paths, which the folder settings may edit.
	startFolders map[string]string
	// finderOpens holds the files Finder asks to open (Open With, a
	// double click, a drop on the Dock icon) until the app has started.
	finderOpens openQueue
}

// New makes an App showing its windows with host and keeping its
// settings in store. cwd is the folder the program started in and dir
// the folder named on the command line, if any. source is what the
// files given to --source hold, for every shell to source.
func New(host Host, store *settings.Store, cwd, dir, source string) *App {
	return &App{
		host:      host,
		settings:  store,
		cwd:       cwd,
		dir:       dir,
		source:    source,
		windows:   map[uint]*docWindow{},
		tabs:      map[int]*docTab{},
		tabWindow: map[*docTab]*docWindow{},
		console:   NewConsole(),
	}
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

// TabCalls notes the tab each call from a tab's page came from, for the
// calls that act on a tab.
func TabCalls(next http.Handler) http.Handler {
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
	file := ""
	if t != nil {
		file = t.file()
	}
	w := &docWindow{app: a}
	w.win = a.host.NewWindow(file)
	a.mu.Lock()
	a.windows[w.win.ID()] = w
	a.mu.Unlock()
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
	t := &docTab{app: a, id: a.lastTab, answers: make(chan run.Answer), done: make(chan struct{}), view: defaultView, auditQ: make(chan notebook.AuditEvent, 256)}
	a.tabs[t.id] = t
	a.mu.Unlock()
	go t.watch()
	go t.sendAudit()
	return t
}

// newPageTab makes a tab showing page, a read-only page such as the
// changelog in place of a file, in no window yet.
func (a *App) newPageTab(page string) *docTab {
	t := a.newTab()
	t.page = page
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
	a.host.ShowView(v)
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
	id, ok := a.host.WindowOf(ctx)
	if !ok {
		return nil, errors.New("the call came from no window")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	w := a.windows[id]
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
	id, ok := a.host.Current()
	a.mu.Lock()
	defer a.mu.Unlock()
	if ok {
		if w := a.windows[id]; w != nil {
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
		if t := w.activeTab(); t != nil && t.file() == "" && t.page == "" {
			if err := t.open(abs, ro); err != nil {
				return err
			}
			// A window holding only the start screen is as good as new:
			// it takes the size the file's window had. One with other
			// tabs keeps its size.
			a.mu.Lock()
			alone := len(w.tabs) == 1
			a.mu.Unlock()
			if alone {
				w.win.Fit(abs)
			}
			return nil
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
		slog.Warn("saving settings", "err", err)
	}
	a.host.RecentChanged()
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
	go a.host.Ask(nil, Question{
		Title:   "Unsaved output",
		Message: "These files have output that isn't saved: " + strings.Join(dirty, ", ") + ". Quit anyway?",
		Buttons: []Button{
			{Label: "Quit", Do: func() {
				a.mu.Lock()
				a.quitting = true
				a.mu.Unlock()
				a.host.Quit()
			}},
			{Label: "Cancel", Cancel: true},
		},
	})
	return false
}

// openDialog asks for markdown files, starting in the start folder.
func (a *App) openDialog(win Window) ([]string, error) {
	return a.host.OpenFiles(win, a.startDir())
}

// openFromMenu runs File › Open… for the window in front.
func (a *App) openFromMenu() {
	w := a.current()
	var win Window
	if w != nil {
		win = w.win
	}
	files, err := a.openDialog(win)
	if err != nil {
		a.host.ShowError(win, err)
		return
	}
	for _, f := range files {
		if err := a.openIn(w, f); err != nil {
			a.host.ShowError(win, err)
		}
	}
}

// openQueue holds the files the system asks the app to open until it is
// ready for them. On macOS, Finder's Open With sends the file in an Apple
// Event, not on the command line, and on a cold launch that event comes
// before the app has finished starting.
type openQueue struct {
	mu      sync.Mutex
	ready   bool
	pending []string
}

// add reports whether file can be opened now; if not, it is held for
// start.
func (q *openQueue) add(file string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.ready {
		q.pending = append(q.pending, file)
	}
	return q.ready
}

// start marks the queue ready and returns the files held until now.
func (q *openQueue) start() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ready = true
	files := q.pending
	q.pending = nil
	return files
}

// openFromSystem opens a file the system asked the app to open, such as
// from Finder's Open With, in the window in front, and brings it forward.
// Before the app has started it waits, then opens in the first window.
func (a *App) openFromSystem(file string) {
	if file != "" && a.finderOpens.add(file) {
		a.openSystemFile(file)
	}
}

// openSystemFile opens file in the window in front and brings it forward.
func (a *App) openSystemFile(file string) {
	w := a.current()
	var win Window
	if w != nil {
		win = w.win
	}
	if err := a.openIn(w, file); err != nil {
		a.host.ShowError(win, err)
		return
	}
	if t := a.openTab(file); t != nil {
		a.focusTab(t)
	}
}

// systemOpensReady opens the files the system asked for before the app
// started.
func (a *App) systemOpensReady() {
	for _, f := range a.finderOpens.start() {
		a.openSystemFile(f)
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

// openSettingsTab brings the Settings tab forward at one of its own
// tabs, such as "about", or at the one it was on (General when new) for
// "", opening one if there is none (see showPage).
func (a *App) openSettingsTab(tab string) {
	if t := a.pageTab(settingsPage); t != nil {
		if tab != "" {
			t.emit("settings-tab", tab)
		}
		a.focusTab(t)
		return
	}
	t := a.newPageTab(settingsPage)
	t.at = tab
	a.placePage(t)
}

// settingsPage is the page a Settings tab shows.
const settingsPage = "/settings.html"

// settingsURL is the settings page, opening at tab when it isn't "".
func settingsURL(tab string) string {
	if tab == "" {
		return settingsPage
	}
	return settingsPage + "#" + tab
}
