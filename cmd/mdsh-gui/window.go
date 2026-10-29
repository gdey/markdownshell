package main

import (
	"path/filepath"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// docWindow is one window and its tabs. The tab bar is the window's page;
// each tab is a page of its own inside it.
//
// App.mu guards tabs, active and closing.
type docWindow struct {
	app *App
	win *application.WebviewWindow

	tabs    []*docTab
	active  *docTab
	closing bool // its tabs may close: the window goes once they have
}

// Tabs is what a window's tab bar shows.
type Tabs struct {
	Tabs   []Tab `json:"tabs"`
	Active int   `json:"active"`
}

// tabsLocked lists the window's tabs. App.mu must be held.
func (w *docWindow) tabsLocked() ([]*docTab, *docTab) {
	return slices.Clone(w.tabs), w.active
}

// remove takes t out of the window's tabs and returns where it was. If
// it was in front, the tab that takes its place comes to the front.
// App.mu must be held.
func (w *docWindow) remove(t *docTab) int {
	j := slices.Index(w.tabs, t)
	if j < 0 {
		return j
	}
	w.tabs = slices.Delete(w.tabs, j, j+1)
	if w.active == t {
		w.active = nil
		if len(w.tabs) > 0 {
			w.active = w.tabs[min(j, len(w.tabs)-1)]
		}
	}
	return j
}

// insert puts t at index i, in front. App.mu must be held.
func (w *docWindow) insert(t *docTab, i int) {
	i = max(0, min(i, len(w.tabs)))
	w.tabs = slices.Insert(w.tabs, i, t)
	w.active = t
}

// next is the tab by places from the one in front, going round, or nil
// if there is no other. App.mu must be held.
func (w *docWindow) next(by int) *docTab {
	n := len(w.tabs)
	if n < 2 {
		return nil
	}
	i := slices.Index(w.tabs, w.active)
	return w.tabs[((i+by)%n+n)%n]
}

func (w *docWindow) state() Tabs {
	w.app.mu.Lock()
	tabs, active := w.tabsLocked()
	w.app.mu.Unlock()
	s := Tabs{Tabs: []Tab{}}
	for _, t := range tabs {
		s.Tabs = append(s.Tabs, t.info())
	}
	if active != nil {
		s.Active = active.id
	}
	return s
}

// changed redraws the tab bar, and the title and View menu for the
// active tab.
func (w *docWindow) changed() {
	s := w.state()
	title := "mdsh"
	for _, t := range s.Tabs {
		if t.ID == s.Active && t.File != "" {
			title = t.Title + " — mdsh"
			if t.Dirty {
				title = "• " + title
			}
		}
	}
	w.win.SetTitle(title)
	w.win.EmitEvent("tabs", s)
}

// activeTab is the tab in front, or nil.
func (w *docWindow) activeTab() *docTab {
	w.app.mu.Lock()
	defer w.app.mu.Unlock()
	return w.active
}

// closingHook closes the window's tabs first, asking about unsaved
// output in each, and lets the window close once none are left.
func (w *docWindow) closingHook(e *application.WindowEvent) {
	w.app.mu.Lock()
	closing := w.closing || w.app.quitting
	tabs, _ := w.tabsLocked()
	w.app.mu.Unlock()
	if closing {
		for _, t := range tabs {
			w.app.dropTab(t)
		}
		w.app.removeWindow(w)
		return
	}
	e.Cancel()
	go w.closeTabs(tabs)
}

// closeTabs closes tabs one by one, asking about each with unsaved
// output, and the window after them. A Cancel stops there.
func (w *docWindow) closeTabs(tabs []*docTab) {
	if len(tabs) == 0 {
		w.app.mu.Lock()
		w.closing = true
		w.app.mu.Unlock()
		w.app.onMain(w.win.Close)
		return
	}
	t, rest := tabs[0], tabs[1:]
	if n := t.unsaved(); n != nil {
		w.app.selectTab(t)
		t.askToSave(n, func() { go w.closeTabs(tabs) })
		return
	}
	w.closeTabs(rest)
}

// dropped opens markdown files dropped on the window.
func (w *docWindow) dropped(e *application.WindowEvent) {
	files := e.Context().DroppedFiles()
	go func() {
		for _, f := range files {
			if ext := filepath.Ext(f); ext != ".md" && ext != ".markdown" {
				continue
			}
			if err := w.app.openIn(w, f); err != nil {
				w.app.showError(w.win, err)
			}
		}
	}()
}

func (w *docWindow) hook() {
	w.win.RegisterHook(events.Common.WindowClosing, w.closingHook)
	w.win.OnWindowEvent(events.Common.WindowFilesDropped, w.dropped)
	w.win.OnWindowEvent(events.Common.WindowFocus, func(*application.WindowEvent) {
		if t := w.activeTab(); t != nil {
			t.mu.Lock()
			v := t.view
			t.mu.Unlock()
			w.app.showView(v)
		}
	})
}
