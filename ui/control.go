package ui

import (
	"net/http"
	"path/filepath"

	"github.com/gdey/markdownshell/notebook"
	"github.com/gdey/markdownshell/settings"
)

// Control is how a Host drives its App: what its windows tell it, and
// its menu commands. It is apart from App so the frontend can't call it.
type Control struct{ a *App }

// ControlOf is a's Control.
func ControlOf(a *App) Control { return Control{a} }

// Start opens the first window, with file open in it, run with args, or
// with the start screen if file is "".
func (c Control) Start(file string, args []string) error {
	t := c.a.newTab()
	if file != "" {
		ro := notebook.RunOptions{Args: args, Shell: c.a.settings.Get().Shell}
		if err := t.open(file, ro); err != nil {
			return err
		}
	}
	c.a.newWindow(t)
	return nil
}

// Settings are the settings the App keeps.
func (c Control) Settings() *settings.Store { return c.a.settings }

// Assets serves frontend, and at notebook.DocPrefix the files beside the
// open documents, such as the images their blocks write.
func (c Control) Assets(frontend http.Handler) http.Handler { return c.a.assets(frontend) }

// Quitting lets every window close without asking about unsaved
// output, as the program is ending.
func (c Control) Quitting() {
	c.a.mu.Lock()
	c.a.quitting = true
	c.a.mu.Unlock()
}

// ShouldQuit asks before quitting with unsaved output in any window.
func (c Control) ShouldQuit() bool { return c.a.shouldQuit() }

// Closing is told window id is closing, and says whether it may: its
// tabs close first, asking about unsaved output in each, and the window
// closes again once none are left.
func (c Control) Closing(id uint) bool {
	a := c.a
	a.mu.Lock()
	w := a.windows[id]
	if w == nil {
		a.mu.Unlock()
		return true
	}
	closing := w.closing || a.quitting
	tabs, _ := w.tabsLocked()
	a.mu.Unlock()
	if closing {
		for _, t := range tabs {
			a.dropTab(t)
		}
		a.removeWindow(w)
		return true
	}
	go w.closeTabs(tabs)
	return false
}

// Dropped opens the markdown files dropped on window id.
func (c Control) Dropped(id uint, files []string) {
	a := c.a
	a.mu.Lock()
	w := a.windows[id]
	a.mu.Unlock()
	if w == nil {
		return
	}
	go func() {
		for _, f := range files {
			if ext := filepath.Ext(f); ext != ".md" && ext != ".markdown" {
				continue
			}
			if err := a.openIn(w, f); err != nil {
				a.host.ShowError(w.win, err)
			}
		}
	}()
}

// Focused is told window id came to the front.
func (c Control) Focused(id uint) {
	a := c.a
	a.mu.Lock()
	w := a.windows[id]
	a.mu.Unlock()
	if w == nil {
		return
	}
	if t := w.activeTab(); t != nil {
		a.showTabView(t)
	}
}

// Send sends action to the tab in front as a "menu" event, so its page
// runs it the way its own buttons and keys do.
func (c Control) Send(action string) {
	if t := c.a.currentTab(); t != nil {
		t.emit("menu", action)
	}
}

// NewTab opens a tab with the start screen in the window in front, or in
// a new window if there is none.
func (c Control) NewTab() {
	if w := c.a.current(); w != nil {
		c.a.newTabIn(w)
	} else {
		c.a.newWindow(nil)
	}
}

// NewWindow opens a window with the start screen.
func (c Control) NewWindow() Window { return c.a.newWindow(nil).win }

// Open asks for markdown files and opens them in the window in front.
func (c Control) Open() { c.a.openFromMenu() }

// OpenFile opens file in the window in front, telling the user if it
// can't.
func (c Control) OpenFile(file string) {
	if err := c.a.openIn(c.a.current(), file); err != nil {
		c.a.host.ShowError(nil, err)
	}
}

// ClearRecent forgets the recent files.
func (c Control) ClearRecent() {
	_ = c.a.settings.Update(func(s *settings.Settings) { s.Recent = nil })
	c.a.host.RecentChanged()
}

// CloseTab closes the tab in front, asking first if it has unsaved
// output.
func (c Control) CloseTab() {
	if t := c.a.currentTab(); t != nil {
		go c.a.closeTab(t)
	}
}

// CloseWindow closes the window in front.
func (c Control) CloseWindow() {
	if w := c.a.current(); w != nil {
		w.win.Close()
	}
}

// CycleTab brings the tab by places from the one in front forward.
func (c Control) CycleTab(by int) {
	if w := c.a.current(); w != nil {
		c.a.cycleTab(w, by)
	}
}

// FollowLink follows href, a link in the document of the tab in front,
// as OpenLink does.
func (c Control) FollowLink(href string) error {
	t := c.a.currentTab()
	if t == nil {
		return errNoFile
	}
	return c.a.openLink(t, href)
}

// LinkTarget is where href, a link in the document of the tab in front,
// goes: a web address or a file's path, or href itself if it can't
// tell.
func (c Control) LinkTarget(href string) string {
	if t := c.a.currentTab(); t != nil {
		if _, target, err := linkTarget(t.file(), href); err == nil {
			return target
		}
	}
	return href
}

// ShowError tells the user about err, over the window in front.
func (c Control) ShowError(err error) {
	var win Window
	if w := c.a.current(); w != nil {
		win = w.win
	}
	c.a.host.ShowError(win, err)
}
