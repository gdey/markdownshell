package main

import (
	"errors"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/ui"
)

// send is a menu item's action: it sends action to the window in front
// as a "menu" event.
func (g *gui) send(action string) func(*application.Context) {
	return func(*application.Context) { g.ctrl.Send(action) }
}

// undo is Edit › Undo's action, or Redo's. A document window decides
// what to undo by where its focus is: an editor's own history, a text
// field's, or the rendered view's edits (see Notebook.Undo); the
// Settings tab, its text field's. Any other window gets the browser's
// undo.
func (g *gui) undo(action string) func(*application.Context) {
	return func(*application.Context) {
		win := g.app.Window.Current()
		if win == nil {
			return
		}
		if !g.ctrl.SendTo(win.ID(), action) {
			win.ExecJS("document.execCommand(" + strconv.Quote(action) + ")")
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
func (g *gui) buildSourceMenu() {
	m := application.NewContextMenu("source")
	m.AddRole(application.Cut)
	m.AddRole(application.Copy)
	m.AddRole(application.Paste)
	m.AddSeparator()
	g.addInsertMenu(m)
	m.Update()

	// The Neovim pane draws its text on a canvas, so the page has no
	// selection for the edit roles to act on: its Cut, Copy and Paste
	// go to Neovim's selection instead.
	n := application.NewContextMenu("nvim")
	n.Add("Cut").OnClick(func(*application.Context) { g.nvimEdit("cut") })
	n.Add("Copy").OnClick(func(*application.Context) { g.nvimEdit("copy") })
	n.Add("Paste").OnClick(func(*application.Context) { g.nvimEdit("paste") })
	n.AddSeparator()
	g.addInsertMenu(n)
	n.Update()

	// The History transcript is xterm's, drawn without a page selection
	// for the Copy role to act on, so its menu asks the frontend.
	h := application.NewContextMenu("history")
	h.Add("Copy").OnClick(g.send("historycopy"))
	h.Add("Select All").OnClick(g.send("historyselectall"))
	h.Update()
}

// buildCastMenu makes a recording player's right-click menu, in place
// of the web view's, which has nothing to copy: the player's selection
// is xterm's, not the page's. The frontend gives the player's name as
// the menu's data, and does the copying.
func (g *gui) buildCastMenu() {
	m := application.NewContextMenu("cast")
	m.Add("Copy").OnClick(func(c *application.Context) { g.app.Event.Emit("cast-menu", "text:"+menuHref(c)) })
	m.Add("Copy as GIF").OnClick(func(c *application.Context) { g.app.Event.Emit("cast-menu", "gif:"+menuHref(c)) })
	m.Update()
}

// nvimEdit does the Neovim pane's Cut, Copy or Paste with the clipboard.
func (g *gui) nvimEdit(op string) {
	g.ctrl.NvimEdit(op, g.app.Clipboard.Text, g.app.Clipboard.SetText)
}

// addInsertMenu adds the Insert submenu to a right-click menu.
func (g *gui) addInsertMenu(m *application.ContextMenu) {
	ins := m.AddSubmenu("Insert")
	for _, it := range inserts {
		if it.name == "" {
			ins.AddSeparator()
			continue
		}
		ins.Add(it.label).OnClick(g.send("insert:" + it.name))
	}
}

// buildMenu makes the menu bar. Commands that act on a document are
// sent to the window in front as a "menu" event, so its frontend runs
// them the way its own buttons and keys do.
func (g *gui) buildMenu() *application.Menu {
	mac := runtime.GOOS == "darwin"
	send := g.send
	m := application.NewMenu()
	if mac {
		g.addAppMenu(m)
	}

	file := m.AddSubmenu("File")
	file.Add("New File…").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) { go g.ctrl.NewFile() })
	file.Add("New Tab").SetAccelerator("CmdOrCtrl+T").OnClick(func(*application.Context) { g.ctrl.NewTab() })
	file.Add("New Window").SetAccelerator("CmdOrCtrl+Shift+N").OnClick(func(*application.Context) { g.ctrl.NewWindow() })
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
	edit.Add("Undo").SetAccelerator("CmdOrCtrl+Z").OnClick(g.undo("undo"))
	edit.Add("Redo").SetAccelerator("CmdOrCtrl+Shift+Z").OnClick(g.undo("redo"))
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
		edit.Add("Preferences…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { g.openSettings() })
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
	g.viewItems.split = view.AddCheckbox("Source View", false).SetAccelerator("CmdOrCtrl+\\").OnClick(send("split"))
	g.viewItems.rendered = view.AddCheckbox("Rendered View", true).SetAccelerator("CmdOrCtrl+Shift+\\").OnClick(send("rendered"))
	g.viewItems.outline = view.AddCheckbox("Outline", false).SetAccelerator("CmdOrCtrl+Shift+O").OnClick(send("outline"))
	g.viewItems.history = view.AddCheckbox("History", false).SetAccelerator("CmdOrCtrl+Shift+H").OnClick(send("history"))
	g.viewItems.console = view.AddCheckbox("Console", false).SetAccelerator("CmdOrCtrl+Shift+Y").OnClick(send("console"))
	view.AddSeparator()
	g.viewItems.code = view.AddCheckbox("Show Code", true).OnClick(send("code"))
	g.viewItems.output = view.AddCheckbox("Show Output", true).OnClick(send("output"))
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
		return func(*application.Context) { g.ctrl.CycleTab(by) }
	}
	view.Add("Show Next Tab").SetAccelerator("CmdOrCtrl+Shift+]").OnClick(cycle(1))
	view.Add("Show Previous Tab").SetAccelerator("CmdOrCtrl+Shift+[").OnClick(cycle(-1))

	if mac {
		m.AddRole(application.WindowMenu)
	}
	g.addHelpMenu(m)
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
	items.history.SetChecked(v.History)
	items.console.SetChecked(v.Console)
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

// addHelpMenu adds the Help menu, whose Cheat Sheet and Changelog open
// the pages built into the app, each in a read-only tab.
func (g *gui) addHelpMenu(m *application.Menu) {
	help := m.AddSubmenu("Help")
	help.Add("Cheat Sheet").OnClick(func(*application.Context) { go g.ctrl.ShowCheatsheet() })
	help.Add("Changelog").OnClick(func(*application.Context) { go g.ctrl.ShowChangelog() })
}

// addAppMenu adds macOS's app menu: About, Settings, Hide and Quit.
func (g *gui) addAppMenu(m *application.Menu) {
	app := m.AddSubmenu("mdsh")
	// About opens Settings' About tab, which says much
	// more than the stock Cocoa panel the About role shows.
	app.Add("About mdsh").OnClick(func(*application.Context) { g.openSettingsTab("about") })
	app.AddSeparator()
	app.Add("Settings…").SetAccelerator("CmdOrCtrl+,").OnClick(func(*application.Context) { g.openSettings() })
	if g.cli != nil && g.cli.bundled != "" {
		g.cliItem = app.Add(cliMenuLabel(g.cli.state())).OnClick(func(*application.Context) { go g.cliFromMenu() })
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
func (g *gui) updateCLIMenu() {
	if g.cliItem == nil {
		return
	}
	label := cliMenuLabel(g.cli.state())
	g.OnMain(func() {
		g.cliItem.SetLabel(label)
		if g.menu != nil {
			g.menu.Update()
		}
	})
}

// cliFromMenu installs the command line tool, or uninstalls it, from
// the app menu, and says what happened.
func (g *gui) cliFromMenu() {
	var (
		c   CLI
		err error
	)
	if g.cli.state().Installed {
		c, err = g.UninstallCLI()
	} else {
		c, err = g.InstallCLI()
	}
	if err != nil {
		if errors.Is(err, errCanceled) {
			return
		}
		g.ShowError(nil, err)
		return
	}
	g.app.Event.Emit("cli", c)
	g.app.Dialog.Info().SetTitle("mdsh").SetMessage(cliMessage(c)).Show()
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

// openSettings brings the Settings tab forward, opening one if there is
// none.
func (g *gui) openSettings() { g.ctrl.OpenSettings("") }

// openSettingsTab brings the Settings tab forward at one of its own
// tabs, such as "about".
func (g *gui) openSettingsTab(tab string) { g.ctrl.OpenSettings(tab) }

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
