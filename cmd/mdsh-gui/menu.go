package main

import (
	"errors"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/settings"
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

// undo is Edit › Undo's action, or Redo's. A document window decides
// what to undo by where its focus is: an editor's own history, a text
// field's, or the rendered view's edits (see Notebook.Undo). Any other
// window, such as Settings, gets the browser's undo.
func (a *App) undo(action string) func(*application.Context) {
	return func(*application.Context) {
		win := a.app.Window.Current()
		if win == nil {
			return
		}
		a.mu.Lock()
		w := a.windows[win.ID()]
		a.mu.Unlock()
		if w == nil {
			win.ExecJS("document.execCommand(" + strconv.Quote(action) + ")")
			return
		}
		if t := w.activeTab(); t != nil {
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
	{"Textbox", "textbox"},
	{"Choice", "choice"},
}

// buildSourceMenu makes the source view's right-click menus, the
// built-in editor's and the Neovim pane's: the edit commands, and
// Insert. It makes the History transcript's too.
func (a *App) buildSourceMenu() {
	m := application.NewContextMenu("source")
	m.AddRole(application.Cut)
	m.AddRole(application.Copy)
	m.AddRole(application.Paste)
	m.AddSeparator()
	a.addInsertMenu(m)
	m.Update()

	// The Neovim pane draws its text on a canvas, so the page has no
	// selection for the edit roles to act on: its Cut, Copy and Paste
	// go to Neovim's selection instead.
	n := application.NewContextMenu("nvim")
	n.Add("Cut").OnClick(func(*application.Context) { a.nvimEdit("cut") })
	n.Add("Copy").OnClick(func(*application.Context) { a.nvimEdit("copy") })
	n.Add("Paste").OnClick(func(*application.Context) { a.nvimEdit("paste") })
	n.AddSeparator()
	a.addInsertMenu(n)
	n.Update()

	// The History transcript is xterm's, drawn without a page selection
	// for the Copy role to act on, so its menu asks the frontend.
	h := application.NewContextMenu("history")
	h.Add("Copy").OnClick(a.send("historycopy"))
	h.Add("Select All").OnClick(a.send("historyselectall"))
	h.Update()
}

// buildCastMenu makes a recording player's right-click menu, in place
// of the web view's, which has nothing to copy: the player's selection
// is xterm's, not the page's. The frontend gives the player's name as
// the menu's data, and does the copying.
func (a *App) buildCastMenu() {
	m := application.NewContextMenu("cast")
	m.Add("Copy").OnClick(func(c *application.Context) { a.app.Event.Emit("cast-menu", "text:"+menuHref(c)) })
	m.Add("Copy as GIF").OnClick(func(c *application.Context) { a.app.Event.Emit("cast-menu", "gif:"+menuHref(c)) })
	m.Update()
}

// addInsertMenu adds the Insert submenu to a right-click menu.
func (a *App) addInsertMenu(m *application.ContextMenu) {
	ins := m.AddSubmenu("Insert")
	for _, it := range inserts {
		if it.name == "" {
			ins.AddSeparator()
			continue
		}
		ins.Add(it.label).OnClick(a.send("insert:" + it.name))
	}
}

// buildMenu makes the menu bar. Commands that act on a document are
// sent to the window in front as a "menu" event, so its frontend runs
// them the way its own buttons and keys do.
func (a *App) buildMenu() *application.Menu {
	mac := runtime.GOOS == "darwin"
	send := a.send
	m := application.NewMenu()
	if mac {
		a.addAppMenu(m)
	}

	file := m.AddSubmenu("File")
	file.Add("New File…").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) { go a.newFromMenu() })
	file.Add("New Tab").SetAccelerator("CmdOrCtrl+T").OnClick(func(*application.Context) {
		if w := a.current(); w != nil {
			a.newTabIn(w)
		} else {
			a.newWindow(nil)
		}
	})
	file.Add("New Window").SetAccelerator("CmdOrCtrl+Shift+N").OnClick(func(*application.Context) { a.newWindow(nil) })
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
	edit.Add("Undo").SetAccelerator("CmdOrCtrl+Z").OnClick(a.undo("undo"))
	edit.Add("Redo").SetAccelerator("CmdOrCtrl+Shift+Z").OnClick(a.undo("redo"))
	edit.AddSeparator()
	// Moves act on the selected block, or in the source view on the
	// block at the cursor; see the frontend's moveAction.
	edit.Add("Move Block Up").SetAccelerator("Alt+Up").OnClick(send("moveup"))
	edit.Add("Move Block Down").SetAccelerator("Alt+Down").OnClick(send("movedown"))
	edit.Add("Move Block Above Previous Block").SetAccelerator("Alt+Shift+Up").OnClick(send("moveblockup"))
	edit.Add("Move Block Below Next Block").SetAccelerator("Alt+Shift+Down").OnClick(send("moveblockdown"))
	edit.Add("Depends On…").OnClick(send("depends"))
	edit.Add("Front Matter").SetAccelerator("CmdOrCtrl+Shift+M").OnClick(send("frontmatter"))
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
	run.Add("Run Current Block").SetAccelerator("CmdOrCtrl+Enter").OnClick(send("runcurrent"))
	// The section is the selected heading's, or the one the selected
	// block or the source pane's cursor is in; see runCurrentSection.
	run.Add("Run Section").SetAccelerator("CmdOrCtrl+Shift+Enter").OnClick(send("runsection"))
	run.Add("Run All").SetAccelerator("CmdOrCtrl+R").OnClick(send("runall"))
	run.Add("Stop").SetAccelerator("CmdOrCtrl+.").OnClick(send("stop"))
	// It asks first; see the frontend's clearoutputs.
	run.Add("Clear All Outputs…").SetAccelerator("CmdOrCtrl+Alt+K").OnClick(send("clearoutputs"))
	run.AddSeparator()
	run.Add("Run Options…").OnClick(send("runoptions"))

	// The check marks show the window in front; each window keeps its
	// own view and says what it is with SetView.
	view := m.AddSubmenu("View")
	a.viewItems.split = view.AddCheckbox("Source View", false).SetAccelerator("CmdOrCtrl+\\").OnClick(send("split"))
	a.viewItems.rendered = view.AddCheckbox("Rendered View", true).SetAccelerator("CmdOrCtrl+Shift+\\").OnClick(send("rendered"))
	a.viewItems.outline = view.AddCheckbox("Outline", false).SetAccelerator("CmdOrCtrl+Shift+O").OnClick(send("outline"))
	a.viewItems.history = view.AddCheckbox("History", false).SetAccelerator("CmdOrCtrl+Shift+H").OnClick(send("history"))
	a.viewItems.console = view.AddCheckbox("Console", false).SetAccelerator("CmdOrCtrl+Shift+Y").OnClick(send("console"))
	view.AddSeparator()
	a.viewItems.code = view.AddCheckbox("Show Code", true).OnClick(send("code"))
	a.viewItems.output = view.AddCheckbox("Show Output", true).OnClick(send("output"))
	// Folds in the rendered view, and copying them to and from the
	// source view, which keeps its own.
	folding := view.AddSubmenu("Folding")
	folding.Add("Collapse All Blocks").OnClick(send("foldblocks"))
	folding.Add("Collapse All Sections").OnClick(send("foldsections"))
	folding.Add("Expand All").OnClick(send("unfoldall"))
	folding.AddSeparator()
	folding.Add("Copy Folds to Source View").OnClick(send("foldstosource"))
	folding.Add("Copy Folds to Rendered View").OnClick(send("foldstorendered"))
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
	a.addHelpMenu(m)
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
	items.history.SetChecked(v.History)
	items.console.SetChecked(v.Console)
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

// addHelpMenu adds the Help menu, whose Changelog opens the changelog
// built into the app in a read-only tab.
func (a *App) addHelpMenu(m *application.Menu) {
	help := m.AddSubmenu("Help")
	help.Add("Changelog").OnClick(func(*application.Context) { go a.ShowChangelog() })
}

// addAppMenu adds macOS's app menu: About, Settings, Hide and Quit.
func (a *App) addAppMenu(m *application.Menu) {
	app := m.AddSubmenu("mdsh")
	// About opens the Settings window's About tab, which says much
	// more than the stock Cocoa panel the About role shows.
	app.Add("About mdsh").OnClick(func(*application.Context) { a.openSettingsTab("about") })
	app.AddSeparator()
	app.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { a.openSettings() })
	if a.cli != nil && a.cli.bundled != "" {
		a.cliItem = app.Add(cliMenuLabel(a.cli.state())).OnClick(func(*application.Context) { go a.cliFromMenu() })
	}
	app.AddSeparator()
	app.AddRole(application.Hide)
	app.AddRole(application.HideOthers)
	app.AddRole(application.ShowAll)
	app.AddSeparator()
	app.AddRole(application.Quit)
}

// cliMenuLabel is the app menu's item for the command line tool.
func cliMenuLabel(c CLI) string {
	if c.Installed {
		return "Uninstall Command Line Tool…"
	}
	return "Install Command Line Tool…"
}

// updateCLIMenu makes the app menu's item match where the command line
// tool stands.
func (a *App) updateCLIMenu() {
	if a.cliItem == nil {
		return
	}
	label := cliMenuLabel(a.cli.state())
	a.onMain(func() {
		a.cliItem.SetLabel(label)
		if a.menu != nil {
			a.menu.Update()
		}
	})
}

// cliFromMenu installs the command line tool, or uninstalls it, from
// the app menu, and says what happened.
func (a *App) cliFromMenu() {
	var (
		c   CLI
		err error
	)
	if a.cli.state().Installed {
		c, err = a.UninstallCLI()
	} else {
		c, err = a.InstallCLI()
	}
	if err != nil {
		if errors.Is(err, errCanceled) {
			return
		}
		a.showError(nil, err)
		return
	}
	a.app.Event.Emit("cli", c)
	a.app.Dialog.Info().SetTitle("mdsh").SetMessage(cliMessage(c)).Show()
}

// cliMessage says where the command line tool stands, after Install or
// Uninstall.
func cliMessage(c CLI) string {
	switch {
	case c.Linked != "":
		return "mdsh is on your PATH at " + c.Linked + ", which runs this app's copy."
	case c.Installed && !c.InPath:
		return "mdsh is installed at " + c.Link + ", but that folder isn't on your PATH. Add it in your shell's startup file: export PATH=\"" + filepath.Dir(c.Link) + ":$PATH\""
	case c.Installed:
		return "mdsh is installed at " + c.Link + ". Terminals open now find it after rehash, or in a new window."
	default:
		return "mdsh is no longer at " + c.Link + "."
	}
}
