package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/library"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/report"
	"github.com/gdey/markdownshell/run"
)

// This file holds the methods the frontend calls.

// Doc is what the frontend shows about the open file.
type Doc struct {
	File    string `json:"file"`
	Dirty   bool   `json:"dirty"`
	Changed bool   `json:"changed"`
	Running bool   `json:"running"` // a run is going on
	// Fresh is set the first time Doc is asked for after a file opens
	// in the tab, but not after it is reloaded, so the page can show
	// the view the settings open files in.
	Fresh bool                `json:"fresh"`
	Run   notebook.RunOptions `json:"run"`
	// Args is Run.Args as one line, quoted as a shell would need.
	Args string `json:"args"`
}

// Doc describes the open file. File is "" while the start screen shows.
func (a *App) Doc(ctx context.Context) (Doc, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return Doc{}, err
	}
	n, err := t.notebook()
	if err != nil {
		return Doc{}, nil
	}
	t.mu.Lock()
	running, fresh := t.running, t.fresh
	t.fresh = false
	t.mu.Unlock()
	ro := n.RunOptions()
	return Doc{File: n.File(), Dirty: n.Dirty(), Changed: n.Changed(), Running: running, Fresh: fresh, Run: ro, Args: notebook.JoinArgs(ro.Args)}, nil
}

// Live is what the run going on in the tab has sent so far, for a page
// that starts partway through it.
func (a *App) Live(ctx context.Context) ([]liveEvent, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.live), nil
}

// Cells is the document laid out for drawing.
func (a *App) Cells(ctx context.Context) ([]notebook.Cell, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return nil, err
	}
	return n.Cells(), nil
}

// RunBlock runs block i alone.
func (a *App) RunBlock(ctx context.Context, i int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = t.run(func() error { return n.RunBlock(i) })
	a.afterRun(t, n)
	return err
}

// RunFrom runs block i and the blocks after it.
func (a *App) RunFrom(ctx context.Context, i int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = t.run(func() error { return n.RunFrom(i) })
	a.afterRun(t, n)
	return err
}

// afterRun marks the title and saves if autosave is on. It doesn't
// save over a file that changed on disk; the banner asks about that.
func (a *App) afterRun(t *docTab, n *notebook.Notebook) {
	if a.settings.Get().Autosave && n.Dirty() && !n.Changed() {
		if err := n.Save(); err == nil {
			t.saved()
			return
		}
	}
	t.changed()
}

// Stop stops the running block, answering stop if a block is waiting
// to be confirmed.
func (a *App) Stop(ctx context.Context) {
	t, err := a.tab(ctx)
	if err != nil {
		return
	}
	select {
	case t.answers <- run.Stop:
	default:
	}
	if n, err := t.notebook(); err == nil {
		n.Stop()
	}
}

// Answer replies to the question a confirm event asked: "run", "skip"
// or "stop".
func (a *App) Answer(ctx context.Context, reply string) error {
	t, err := a.tab(ctx)
	if err != nil {
		return err
	}
	ans, ok := map[string]run.Answer{"run": run.Go, "skip": run.Skip, "stop": run.Stop}[reply]
	if !ok {
		return fmt.Errorf("unknown answer %q", reply)
	}
	select {
	case t.answers <- ans:
		return nil
	default:
		return errors.New("nothing is waiting for an answer")
	}
}

// Open asks for markdown files and opens them: the first in this tab if
// it shows the start screen, the rest in new tabs.
func (a *App) Open(ctx context.Context) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	files, err := a.openDialog(w.win)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := a.openIn(w, f); err != nil {
			return err
		}
	}
	return nil
}

// NewFile asks where to make a new markdown file and opens it: in this
// tab if it shows the start screen, else in a new tab.
func (a *App) NewFile(ctx context.Context) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	return a.newFile(w)
}

// OpenPath opens file, from the start screen.
func (a *App) OpenPath(ctx context.Context, file string) error {
	t, err := a.tab(ctx)
	if err != nil {
		return err
	}
	if open := a.openTab(file); open != nil {
		a.focusTab(open)
		return nil
	}
	return t.open(file, notebook.RunOptions{Shell: a.settings.Get().Shell})
}

// Save writes the output into the file.
func (a *App) Save(ctx context.Context) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if err := n.Save(); err != nil {
		return err
	}
	t.saved()
	return nil
}

// SaveAs asks where to save the file with its output, and from then on
// the tab shows the new file. It returns false if the user cancels.
func (a *App) SaveAs(ctx context.Context) (bool, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return false, err
	}
	w := t.window()
	if w == nil {
		return false, errNoFile
	}
	file, err := a.app.Dialog.SaveFile().
		SetMessage("Save a copy with its output").
		SetDirectory(filepath.Dir(n.File())).
		SetFilename(filepath.Base(n.File())).
		AddFilter("Markdown", "*.md;*.markdown").
		AttachToWindow(w.win).
		PromptForSingleSelection()
	if err != nil || file == "" {
		return false, err
	}
	if err := n.SaveAs(file); err != nil {
		return false, err
	}
	t.saved()
	a.opened(n.File())
	return true, nil
}

// Reload reads the file from disk again, dropping output not saved.
func (a *App) Reload(ctx context.Context) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	return t.open(n.File(), n.RunOptions())
}

// SetRunOptions sets the arguments (one line, split as a shell would),
// --only, --step and --stop-on-error for this file.
func (a *App) SetRunOptions(ctx context.Context, args, only string, step, stopOnError bool) error {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	list, err := notebook.SplitArgs(args)
	if err != nil {
		return fmt.Errorf("arguments: %w", err)
	}
	ro := n.RunOptions()
	ro.Args, ro.Only, ro.Step, ro.StopOnError = list, only, step, stopOnError
	return n.SetRunOptions(ro)
}

// Start is what the start screen lists.
type Start struct {
	Dir    string         `json:"dir"`
	Home   string         `json:"home"` // for showing paths as ~/…
	Files  []library.File `json:"files"`
	Full   bool           `json:"full"` // more files than listed
	Recent []string       `json:"recent"`
}

// StartScreen lists the markdown files under the start folder, and the
// recent files that still exist.
func (a *App) StartScreen() (Start, error) {
	dir := a.startDir()
	files, full, err := library.List(dir)
	if err != nil {
		return Start{}, err
	}
	var recent []string
	for _, f := range a.settings.Get().Recent {
		if _, err := os.Stat(f); err == nil {
			recent = append(recent, f)
		}
	}
	home, _ := os.UserHomeDir()
	return Start{Dir: dir, Home: home, Files: files, Full: full, Recent: recent}, nil
}

// Settings are the app-wide settings.
func (a *App) Settings() settings.Settings { return a.settings.Get() }

// SetSettings saves the settings the Settings window edits: autosave,
// Vim mode, theme, shell, the view files open in and the start folder. Open windows hear of it with a
// "settings" event.
func (a *App) SetSettings(s settings.Settings) error {
	err := a.settings.Update(func(cur *settings.Settings) {
		cur.Autosave, cur.Vim, cur.Shell, cur.StartFolder = s.Autosave, s.Vim, s.Shell, s.StartFolder
		cur.Theme = ""
		if s.Theme == "light" || s.Theme == "dark" {
			cur.Theme = s.Theme
		}
		cur.View = ""
		if s.View == "rendered" || s.View == "source" || s.View == "both" {
			cur.View = s.View
		}
	})
	if err == nil {
		a.app.Event.Emit("settings", a.settings.Get())
	}
	return err
}

// ChooseFolder asks for a folder and returns it, or "" if none was
// picked.
func (a *App) ChooseFolder(ctx context.Context) (string, error) {
	d := a.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("Choose the start folder").
		SetDirectory(a.startDir())
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// NewWindow opens a window with the start screen.
func (a *App) NewWindow() { a.newWindow(nil) }

// BlockAttrs describes the block attributes, for completion, hover help
// and the attribute form.
func (a *App) BlockAttrs() []doc.AttrSpec { return doc.BlockAttrs }

// Names are the block ids and envs in the file, for completion.
func (a *App) Names(ctx context.Context) (notebook.Names, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Names{}, err
	}
	return n.Names(), nil
}

// CheckInfo checks the attributes in a fence's info string.
func (a *App) CheckInfo(info string) []notebook.Problem { return notebook.CheckInfo(info) }

// Text is the whole file, for the split view's source pane.
func (a *App) Text(ctx context.Context) (string, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	return n.Text(), nil
}

// SetText replaces the whole file, as typed in the source pane.
func (a *App) SetText(ctx context.Context, text string) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetText(text)
	t.changed()
	return err
}

// View is what a tab shows, for the View menu's check marks.
type View struct {
	Split    bool `json:"split"`    // the source pane
	Rendered bool `json:"rendered"` // the rendered view
	Outline  bool `json:"outline"`
	Code     bool `json:"code"`
	Output   bool `json:"output"`
}

// defaultView is what a new tab shows until its page says.
var defaultView = View{Rendered: true, Code: true, Output: true}

// SetView notes what the calling tab shows.
func (a *App) SetView(ctx context.Context, v View) {
	t, err := a.tab(ctx)
	if err != nil {
		return
	}
	t.mu.Lock()
	t.view = v
	t.mu.Unlock()
	if a.currentTab() == t {
		a.showView(v)
	}
}

// ReportStart is what the Report dialog starts with.
type ReportStart struct {
	Ran   int `json:"ran"`   // runnable blocks that ran in this tab
	Total int `json:"total"` // runnable blocks
	// Dest is where a report goes unless another file is picked: beside
	// the document, named after it, without an extension.
	Dest string `json:"dest"`
	Home string `json:"home"` // for showing paths as ~/…
}

// ReportStart reads what the Report dialog shows when it opens.
func (a *App) ReportStart(ctx context.Context) (ReportStart, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return ReportStart{}, err
	}
	ran, total := n.Ran()
	file := n.File()
	home, _ := os.UserHomeDir()
	return ReportStart{Ran: ran, Total: total, Dest: strings.TrimSuffix(file, filepath.Ext(file)), Home: home}, nil
}

// ChooseReport asks where to save a report in format, starting at dest,
// and returns the file, or "" if none was picked.
func (a *App) ChooseReport(ctx context.Context, format report.Format, dest string) (string, error) {
	t, _, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	w := t.window()
	if w == nil {
		return "", errNoFile
	}
	name := map[report.Format]string{report.PDF: "PDF", report.HTML: "HTML", report.Markdown: "Markdown"}[format]
	return a.app.Dialog.SaveFile().
		SetMessage("Save the report").
		SetDirectory(filepath.Dir(dest)).
		SetFilename(filepath.Base(dest)).
		AddFilter(name, "*"+format.Ext()).
		AttachToWindow(w.win).
		PromptForSingleSelection()
}

// ReportCommand is the mdsh command that makes the report r describes.
func (a *App) ReportCommand(ctx context.Context, r notebook.Report) (string, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	return n.ReportCommand(r, home), nil
}

// CreateReport writes the report r describes.
func (a *App) CreateReport(ctx context.Context, r notebook.Report) error {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	return n.WriteReport(r)
}

// Reveal shows file in the file manager.
func (a *App) Reveal(file string) error {
	return a.app.Env.OpenFileManager(file, true)
}

// Type sends keys typed in the terminal pane to the tty block running in
// it.
func (a *App) Type(ctx context.Context, keys string) {
	if _, n, err := a.notebook(ctx); err == nil {
		n.Type(keys)
	}
}

// SetTerminalSize sets the terminal pane's size in cells, which the
// window works out from its width.
func (a *App) SetTerminalSize(ctx context.Context, cols, rows int) {
	if _, n, err := a.notebook(ctx); err == nil {
		n.SetTerminalSize(cols, rows)
	}
}

// Tabs are the calling window's tabs, for its tab bar.
func (a *App) Tabs(ctx context.Context) (Tabs, error) {
	w, err := a.window(ctx)
	if err != nil {
		return Tabs{}, err
	}
	return w.state(), nil
}

// NewTab opens a tab with the start screen in the calling window, after
// the tab in front.
func (a *App) NewTab(ctx context.Context) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	a.newTabIn(w)
	return nil
}

// SelectTab brings tab id to the front of its window.
func (a *App) SelectTab(id int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	a.selectTab(t)
	return nil
}

// CloseTab closes tab id, asking first if it has unsaved output.
func (a *App) CloseTab(id int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	go a.closeTab(t)
	return nil
}

// DragTab notes that tab id is being dragged, so the window it is
// dropped on can take it.
func (a *App) DragTab(id int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.dragging = t
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", true)
	return nil
}

// DropTab puts the tab being dragged in the calling window, at index i
// of its tab bar. It does nothing if no tab is being dragged.
func (a *App) DropTab(ctx context.Context, i int) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	a.mu.Lock()
	t := a.dragging
	a.dragging = nil
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", false)
	if t != nil {
		a.moveTab(t, w, i)
		w.win.Focus()
	}
	return nil
}

// EndDrag ends dragging tab id. If no window took it and it was let go
// outside its window, at x, y on the screen, it gets a window of its own
// there.
func (a *App) EndDrag(id int, outside bool, x, y int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	a.mu.Lock()
	dropped := a.dragging != t
	a.dragging = nil
	w := a.tabWindow[t]
	alone := w != nil && len(w.tabs) == 1
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", false)
	if dropped || !outside || alone {
		return nil
	}
	a.detach(t, x, y)
	return nil
}

// DetachTab gives the tab being dragged a window of its own, at x, y on
// the screen, if it shares its window. It does nothing if no tab is being
// dragged.
func (a *App) DetachTab(x, y int) error {
	a.mu.Lock()
	t := a.dragging
	a.dragging = nil
	w := a.tabWindow[t]
	alone := w == nil || len(w.tabs) == 1
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", false)
	if t != nil && !alone {
		a.detach(t, x, y)
	}
	return nil
}

// detach moves t to a new window with its tab bar at x, y.
func (a *App) detach(t *docTab, x, y int) {
	nw := a.newWindow(t)
	nw.win.SetPosition(max(0, x-120), max(0, y-16))
	nw.win.Focus()
}
