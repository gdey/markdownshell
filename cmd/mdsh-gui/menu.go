package main

import (
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

// send is a menu item's action: it sends action to the window in front
// as a "menu" event.
func (a *App) send(action string) func(*application.Context) {
	return func(*application.Context) {
		if t := a.currentTab(); t != nil {
			t.emit("menu", action)
		}
	}
}

// inserts are the source view's Insert menu, as the markdown bar has
// them: a label and the frontend's name for the action, sent as
// "insert:" + name. An empty name is a separator.
var inserts = []struct{ label, name string }{
	{"Heading", "heading"},
	{"Bold", "bold"},
	{"Italic", "italic"},
	{"Code", "code"},
	{"Link", "link"},
	{"Image", "image"},
	{"", ""},
	{"Bulleted List", "ul"},
	{"Numbered List", "ol"},
	{"Task List", "task"},
	{"Quote", "quote"},
	{"Table", "table"},
	{"Code Block", "fence"},
	{"", ""},
	{"sh Block", "sh"},
	{"sh Block, Confirm First", "sh-confirm"},
	{"zsh Block", "zsh"},
}

// buildSourceMenu makes the source view's right-click menu: the edit
// commands, and Insert.
func (a *App) buildSourceMenu() {
	m := application.NewContextMenu("source")
	m.AddRole(application.Cut)
	m.AddRole(application.Copy)
	m.AddRole(application.Paste)
	m.AddSeparator()
	ins := m.AddSubmenu("Insert")
	for _, it := range inserts {
		if it.name == "" {
			ins.AddSeparator()
			continue
		}
		ins.Add(it.label).OnClick(a.send("insert:" + it.name))
	}
	m.Update()
}

// buildMenu makes the menu bar. Commands that act on a document are
// sent to the window in front as a "menu" event, so its frontend runs
// them the way its own buttons and keys do.
func (a *App) buildMenu() *application.Menu {
	mac := runtime.GOOS == "darwin"
	send := a.send
	m := application.NewMenu()
	if mac {
		app := m.AddSubmenu("mdsh")
		app.AddRole(application.About)
		app.AddSeparator()
		app.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { a.openSettings() })
		app.AddSeparator()
		app.AddRole(application.Hide)
		app.AddRole(application.HideOthers)
		app.AddRole(application.ShowAll)
		app.AddSeparator()
		app.AddRole(application.Quit)
	}

	file := m.AddSubmenu("File")
	file.Add("New Tab").SetAccelerator("CmdOrCtrl+T").OnClick(func(*application.Context) {
		if w := a.current(); w != nil {
			a.newTabIn(w)
		} else {
			a.newWindow(nil)
		}
	})
	file.Add("New Window").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) { a.newWindow(nil) })
	file.Add("Open…").SetAccelerator("CmdOrCtrl+O").OnClick(func(*application.Context) { go a.openFromMenu() })
	a.recentMenu = file.AddSubmenu("Open Recent")
	file.AddSeparator()
	file.Add("Save").SetAccelerator("CmdOrCtrl+S").OnClick(send("save"))
	file.Add("Save As…").SetAccelerator("CmdOrCtrl+Shift+S").OnClick(send("saveas"))
	file.Add("Create Report…").OnClick(send("report"))
	file.Add("Reload").OnClick(send("reload"))
	file.AddSeparator()
	file.Add("Close Tab").SetAccelerator("CmdOrCtrl+W").OnClick(func(*application.Context) {
		if t := a.currentTab(); t != nil {
			go a.closeTab(t)
		}
	})
	file.Add("Close Window").SetAccelerator("CmdOrCtrl+Shift+W").OnClick(func(*application.Context) {
		if w := a.current(); w != nil {
			w.win.Close()
		}
	})
	if !mac {
		file.AddSeparator()
		file.Add("Quit").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { a.app.Quit() })
	}

	edit := m.AddSubmenu("Edit")
	edit.AddRole(application.Undo)
	edit.AddRole(application.Redo)
	edit.AddSeparator()
	edit.AddRole(application.Cut)
	edit.AddRole(application.Copy)
	edit.AddRole(application.Paste)
	edit.AddRole(application.SelectAll)
	if !mac {
		edit.AddSeparator()
		edit.Add("Preferences…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { a.openSettings() })
	}

	run := m.AddSubmenu("Run")
	run.Add("Run All").SetAccelerator("CmdOrCtrl+R").OnClick(send("runall"))
	run.Add("Stop").SetAccelerator("CmdOrCtrl+.").OnClick(send("stop"))
	run.AddSeparator()
	run.Add("Run Options…").OnClick(send("runoptions"))

	// The check marks show the window in front; each window keeps its
	// own view and says what it is with SetView.
	view := m.AddSubmenu("View")
	a.viewItems.split = view.AddCheckbox("Source View", false).SetAccelerator("CmdOrCtrl+\\").OnClick(send("split"))
	a.viewItems.rendered = view.AddCheckbox("Rendered View", true).SetAccelerator("CmdOrCtrl+Shift+\\").OnClick(send("rendered"))
	a.viewItems.outline = view.AddCheckbox("Outline", false).SetAccelerator("CmdOrCtrl+Shift+O").OnClick(send("outline"))
	view.AddSeparator()
	a.viewItems.code = view.AddCheckbox("Show Code", true).OnClick(send("code"))
	a.viewItems.output = view.AddCheckbox("Show Output", true).OnClick(send("output"))
	view.AddSeparator()
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddRole(application.ResetZoom)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	view.AddSeparator()
	cycle := func(by int) func(*application.Context) {
		return func(*application.Context) {
			if w := a.current(); w != nil {
				a.cycleTab(w, by)
			}
		}
	}
	view.Add("Show Next Tab").SetAccelerator("CmdOrCtrl+Shift+]").OnClick(cycle(1))
	view.Add("Show Previous Tab").SetAccelerator("CmdOrCtrl+Shift+[").OnClick(cycle(-1))

	if mac {
		m.AddRole(application.WindowMenu)
	}
	a.recentItems()
	return m
}

// showView sets the View menu's check marks to v. It is called from
// window events and bound methods, off the main thread, and macOS menus
// may only change on the main thread.
func (a *App) showView(v View) {
	a.onMain(func() { a.checkView(v) })
}

// onMain runs fn on the main thread. Before the app runs there is none
// to send it to, and the caller is the main goroutine, so fn runs there.
func (a *App) onMain(fn func()) {
	if !a.started.Load() {
		fn()
		return
	}
	application.InvokeSync(fn)
}

func (a *App) checkView(v View) {
	items := a.viewItems
	if items.split == nil {
		return
	}
	items.split.SetChecked(v.Split)
	items.rendered.SetChecked(v.Rendered || !v.Split)
	items.outline.SetChecked(v.Outline)
	items.code.SetChecked(v.Code)
	items.output.SetChecked(v.Output)
	if a.menu != nil {
		a.menu.Update()
	}
}

// fillRecent lists the recent files under File › Open Recent, on the
// main thread, as showView does.
func (a *App) fillRecent() {
	a.onMain(a.recentItems)
}

// recentItems fills File › Open Recent. buildMenu calls it before the
// app runs; later calls go through fillRecent.
func (a *App) recentItems() {
	if a.recentMenu == nil {
		return
	}
	a.recentMenu.Clear()
	recent := a.settings.Get().Recent
	for _, f := range recent {
		a.recentMenu.Add(filepath.Base(f)).SetTooltip(f).OnClick(func(*application.Context) {
			go func() {
				if err := a.openIn(a.current(), f); err != nil {
					a.showError(nil, err)
				}
			}()
		})
	}
	if len(recent) == 0 {
		a.recentMenu.Add("No recent files").SetEnabled(false)
	} else {
		a.recentMenu.AddSeparator()
		a.recentMenu.Add("Clear Menu").OnClick(func(*application.Context) {
			_ = a.settings.Update(func(s *settings.Settings) { s.Recent = nil })
			a.fillRecent()
		})
	}
	if a.menu != nil {
		a.menu.Update()
	}
}
