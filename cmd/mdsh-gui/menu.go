package main

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/ui"
)

// send is a menu item's action: it sends action to the window in front
// as a "menu" event.
func (g *gui) send(action string) func(*application.Context) {
	return func(*application.Context) { g.ctrl.Send(action) }
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
func (g *gui) buildSourceMenu() {
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
		ins.Add(it.label).OnClick(g.send("insert:" + it.name))
	}
	m.Update()
}

// buildMenu makes the menu bar. Commands that act on a document are
// sent to the window in front as a "menu" event, so its frontend runs
// them the way its own buttons and keys do.
func (g *gui) buildMenu() *application.Menu {
	mac := runtime.GOOS == "darwin"
	send := g.send
	m := application.NewMenu()
	if mac {
		app := m.AddSubmenu("mdsh")
		app.AddRole(application.About)
		app.AddSeparator()
		app.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { g.openSettings() })
		app.AddSeparator()
		app.AddRole(application.Hide)
		app.AddRole(application.HideOthers)
		app.AddRole(application.ShowAll)
		app.AddSeparator()
		app.AddRole(application.Quit)
	}

	file := m.AddSubmenu("File")
	file.Add("New Tab").SetAccelerator("CmdOrCtrl+T").OnClick(func(*application.Context) { g.ctrl.NewTab() })
	file.Add("New Window").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) { g.ctrl.NewWindow() })
	file.Add("Open…").SetAccelerator("CmdOrCtrl+O").OnClick(func(*application.Context) { go g.ctrl.Open() })
	g.recentMenu = file.AddSubmenu("Open Recent")
	file.AddSeparator()
	file.Add("Save").SetAccelerator("CmdOrCtrl+S").OnClick(send("save"))
	file.Add("Save As…").SetAccelerator("CmdOrCtrl+Shift+S").OnClick(send("saveas"))
	file.Add("Create Report…").OnClick(send("report"))
	file.Add("Reload").OnClick(send("reload"))
	file.AddSeparator()
	file.Add("Close Tab").SetAccelerator("CmdOrCtrl+W").OnClick(func(*application.Context) { g.ctrl.CloseTab() })
	file.Add("Close Window").SetAccelerator("CmdOrCtrl+Shift+W").OnClick(func(*application.Context) { g.ctrl.CloseWindow() })
	if !mac {
		file.AddSeparator()
		file.Add("Quit").SetAccelerator("CmdOrCtrl+Q").OnClick(func(*application.Context) { g.app.Quit() })
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
		edit.Add("Preferences…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { g.openSettings() })
	}

	run := m.AddSubmenu("Run")
	run.Add("Run All").SetAccelerator("CmdOrCtrl+R").OnClick(send("runall"))
	run.Add("Stop").SetAccelerator("CmdOrCtrl+.").OnClick(send("stop"))
	run.AddSeparator()
	run.Add("Run Options…").OnClick(send("runoptions"))

	// The check marks show the window in front; each window keeps its
	// own view and says what it is with SetView.
	view := m.AddSubmenu("View")
	g.viewItems.split = view.AddCheckbox("Source View", false).SetAccelerator("CmdOrCtrl+\\").OnClick(send("split"))
	g.viewItems.rendered = view.AddCheckbox("Rendered View", true).SetAccelerator("CmdOrCtrl+Shift+\\").OnClick(send("rendered"))
	g.viewItems.outline = view.AddCheckbox("Outline", false).SetAccelerator("CmdOrCtrl+Shift+O").OnClick(send("outline"))
	view.AddSeparator()
	g.viewItems.code = view.AddCheckbox("Show Code", true).OnClick(send("code"))
	g.viewItems.output = view.AddCheckbox("Show Output", true).OnClick(send("output"))
	view.AddSeparator()
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddRole(application.ResetZoom)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	view.AddSeparator()
	cycle := func(by int) func(*application.Context) {
		return func(*application.Context) { g.ctrl.CycleTab(by) }
	}
	view.Add("Show Next Tab").SetAccelerator("CmdOrCtrl+Shift+]").OnClick(cycle(1))
	view.Add("Show Previous Tab").SetAccelerator("CmdOrCtrl+Shift+[").OnClick(cycle(-1))

	if mac {
		m.AddRole(application.WindowMenu)
	}
	g.recentItems()
	return m
}

// ShowView sets the View menu's check marks to v. It is called from
// window events and bound methods, off the main thread, and macOS menus
// may only change on the main thread.
func (g *gui) ShowView(v ui.View) {
	g.OnMain(func() { g.checkView(v) })
}

func (g *gui) checkView(v ui.View) {
	items := g.viewItems
	if items.split == nil {
		return
	}
	items.split.SetChecked(v.Split)
	items.rendered.SetChecked(v.Rendered || !v.Split)
	items.outline.SetChecked(v.Outline)
	items.code.SetChecked(v.Code)
	items.output.SetChecked(v.Output)
	if g.menu != nil {
		g.menu.Update()
	}
}

// RecentChanged lists the recent files under File › Open Recent, on the
// main thread, as ShowView does.
func (g *gui) RecentChanged() {
	g.OnMain(g.recentItems)
}

// recentItems fills File › Open Recent. buildMenu calls it before the
// app runs; later calls go through RecentChanged.
func (g *gui) recentItems() {
	if g.recentMenu == nil {
		return
	}
	g.recentMenu.Clear()
	recent := g.ctrl.Settings().Get().Recent
	for _, f := range recent {
		g.recentMenu.Add(filepath.Base(f)).SetTooltip(f).OnClick(func(*application.Context) {
			go g.ctrl.OpenFile(f)
		})
	}
	if len(recent) == 0 {
		g.recentMenu.Add("No recent files").SetEnabled(false)
	} else {
		g.recentMenu.AddSeparator()
		g.recentMenu.Add("Clear Menu").OnClick(func(*application.Context) { g.ctrl.ClearRecent() })
	}
	if g.menu != nil {
		g.menu.Update()
	}
}

// openSettings shows the settings window, making it the first time.
func (g *gui) openSettings() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.prefs != nil {
		g.prefs.Show()
		g.prefs.Focus()
		return
	}
	opts := application.WebviewWindowOptions{
		Name:   "settings",
		Title:  "Settings",
		Width:  520,
		Height: 420,
		URL:    "/settings.html",
	}
	g.prefs = g.app.Window.NewWithOptions(opts)
	g.prefs.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		g.mu.Lock()
		g.prefs = nil
		g.mu.Unlock()
	})
}

// buildLinkMenu makes the right-click menu of a link in the rendered
// view, in place of the web view's, whose Open Link would load the link
// over the app's page. The frontend gives the link's href as the
// menu's data.
func (g *gui) buildLinkMenu() {
	m := application.NewContextMenu("link")
	m.Add("Open Link").OnClick(func(c *application.Context) {
		if err := g.ctrl.FollowLink(menuHref(c)); err != nil {
			g.ctrl.ShowError(err)
		}
	})
	m.Add("Copy Link").OnClick(func(c *application.Context) {
		g.app.Clipboard.SetText(g.ctrl.LinkTarget(menuHref(c)))
	})
	m.Update()
}

// menuHref is the href the frontend gave a link's menu, as a quoted CSS
// string.
func menuHref(c *application.Context) string {
	s := strings.TrimSpace(c.ContextMenuData())
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return s
}
