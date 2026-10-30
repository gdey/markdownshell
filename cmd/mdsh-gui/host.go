package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/ui"
)

// gui shows a ui.App in Wails windows, with a menu bar and native
// dialogs.
type gui struct {
	app  *application.App
	ctrl ui.Control

	menu       *application.Menu
	viewItems  struct{ split, rendered, outline, code, output *application.MenuItem }
	recentMenu *application.Menu

	mu    sync.Mutex
	named int // windows made so far, for unique names
	prefs *application.WebviewWindow

	started atomic.Bool // the app is running, so menus change on its main thread
}

var _ ui.Host = (*gui)(nil)

// window is a Wails window as a ui.Window.
type window struct{ *application.WebviewWindow }

func (w window) Emit(name string, data any) { w.EmitEvent(name, data) }
func (w window) MoveTo(x, y int)            { w.SetPosition(x, y) }

func (w window) SetTitle(title string) { w.WebviewWindow.SetTitle(title) }
func (w window) Close()                { w.WebviewWindow.Close() }
func (w window) Focus()                { w.WebviewWindow.Focus() }

// attach is win as a Wails window to attach a dialog to, or nil.
func attach(win ui.Window) application.Window {
	if w, ok := win.(window); ok {
		return w.WebviewWindow
	}
	return nil
}

func (g *gui) NewWindow() ui.Window {
	g.mu.Lock()
	g.named++
	name := fmt.Sprintf("doc-%d", g.named)
	g.mu.Unlock()
	opts := application.WebviewWindowOptions{
		Name:           name,
		Title:          "mdsh",
		Width:          960,
		Height:         820,
		URL:            "/window.html",
		EnableFileDrop: true,
	}
	opts.Linux.Menu = g.menu
	win := g.app.Window.NewWithOptions(opts)
	id := win.ID()
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !g.ctrl.Closing(id) {
			e.Cancel()
		}
	})
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		g.ctrl.Dropped(id, e.Context().DroppedFiles())
	})
	win.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) { g.ctrl.Focused(id) })
	return window{win}
}

func (g *gui) WindowOf(ctx context.Context) (uint, bool) {
	win, ok := ctx.Value(application.WindowKey).(application.Window)
	if !ok {
		return 0, false
	}
	return win.ID(), true
}

func (g *gui) Current() (uint, bool) {
	win := g.app.Window.Current()
	if win == nil {
		return 0, false
	}
	return win.ID(), true
}

func (g *gui) EmitAll(name string, data any) { g.app.Event.Emit(name, data) }

// OnMain runs fn on the main thread. Before the app runs there is none
// to send it to, and the caller is the main goroutine, so fn runs there.
func (g *gui) OnMain(fn func()) {
	if !g.started.Load() {
		fn()
		return
	}
	application.InvokeSync(fn)
}

func (g *gui) ShowError(win ui.Window, err error) {
	d := g.app.Dialog.Error().SetTitle("mdsh").SetMessage(err.Error())
	if w := attach(win); w != nil {
		d.AttachToWindow(w)
	}
	d.Show()
}

func (g *gui) Ask(win ui.Window, q ui.Question) {
	d := g.app.Dialog.Question().SetTitle(q.Title).SetMessage(q.Message)
	if w := attach(win); w != nil {
		d.AttachToWindow(w)
	}
	for _, b := range q.Buttons {
		btn := d.AddButton(b.Label)
		if b.Do != nil {
			btn.OnClick(b.Do)
		}
		if b.Default {
			d.SetDefaultButton(btn)
		}
		if b.Cancel {
			d.SetCancelButton(btn)
		}
	}
	d.Show()
}

func (g *gui) OpenFiles(win ui.Window, dir string) ([]string, error) {
	d := g.app.Dialog.OpenFile().
		SetTitle("Open a markdown file").
		AddFilter("Markdown", "*.md;*.markdown").
		SetDirectory(dir)
	if w := attach(win); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForMultipleSelection()
}

func (g *gui) SaveFile(win ui.Window, s ui.SaveFile) (string, error) {
	d := g.app.Dialog.SaveFile().
		SetMessage(s.Message).
		SetDirectory(s.Dir).
		SetFilename(s.Name).
		AddFilter(s.Filter, s.Pattern)
	if w := attach(win); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

func (g *gui) ChooseFolder(ctx context.Context, dir string) (string, error) {
	d := g.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("Choose the start folder").
		SetDirectory(dir)
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

func (g *gui) Reveal(file string) error { return g.app.Env.OpenFileManager(file, true) }

func (g *gui) OpenURL(url string) error   { return g.app.Browser.OpenURL(url) }
func (g *gui) OpenFile(file string) error { return g.app.Browser.OpenFile(file) }

func (g *gui) Quit() { g.app.Quit() }
