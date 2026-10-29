package main

import (
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

// buildMenu makes the menu bar. Commands that act on a document are
// sent to the window in front as a "menu" event, so its frontend runs
// them the way its own buttons and keys do.
func (a *App) buildMenu() *application.Menu {
	mac := runtime.GOOS == "darwin"
	send := func(action string) func(*application.Context) {
		return func(*application.Context) {
			if dw := a.current(); dw != nil {
				dw.emit("menu", action)
			}
		}
	}
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
	file.Add("New Window").SetAccelerator("CmdOrCtrl+N").OnClick(func(*application.Context) { a.newWindow() })
	file.Add("Open…").SetAccelerator("CmdOrCtrl+O").OnClick(func(*application.Context) { go a.openFromMenu() })
	a.recentMenu = file.AddSubmenu("Open Recent")
	file.AddSeparator()
	file.Add("Save").SetAccelerator("CmdOrCtrl+S").OnClick(send("save"))
	file.Add("Save As…").SetAccelerator("CmdOrCtrl+Shift+S").OnClick(send("saveas"))
	file.Add("Create Report…").SetEnabled(false) // M5
	file.Add("Reload").OnClick(send("reload"))
	file.AddSeparator()
	file.Add("Close Window").SetAccelerator("CmdOrCtrl+W").OnClick(func(*application.Context) {
		if dw := a.current(); dw != nil {
			dw.win.Close()
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

	view := m.AddSubmenu("View")
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddRole(application.ResetZoom)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)

	if mac {
		m.AddRole(application.WindowMenu)
	}
	a.fillRecent()
	return m
}

// fillRecent lists the recent files under File › Open Recent.
func (a *App) fillRecent() {
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
