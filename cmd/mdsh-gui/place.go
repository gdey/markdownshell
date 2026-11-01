package main

import (
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

// The sizes windows open at when there is none to remember. Each is cut
// down to fit the screen.
const (
	docWidth       = 1280
	docHeight      = 900
	settingsWidth  = 720
	settingsHeight = 720
)

// keepPlace is whether a window goes back where it was, not just to its
// size. X11 window managers report a window's corner inside its frame
// but place it by its frame's, so it would creep down each time, and
// Wayland doesn't let an app place its windows at all.
const keepPlace = runtime.GOOS != "linux"

// fitShare is how much of a screen's work area a default size may take.
const fitShare = 0.9

// placeDelay is how long a window must stay put after a move or resize
// before where it is gets saved, so a drag saves once.
const placeDelay = 500 * time.Millisecond

// fit cuts w × h down to share of area, so a window that size fits.
// An empty area leaves them as they are.
func fit(w, h int, area application.Rect, share float64) (int, int) {
	if area.IsEmpty() {
		return w, h
	}
	return min(w, int(float64(area.Width)*share)), min(h, int(float64(area.Height)*share))
}

// onScreen is the work area that enough of p's top edge is in to grab
// and move the window by, if any.
func onScreen(p settings.Place, areas []application.Rect) (application.Rect, bool) {
	top := application.Rect{X: p.X, Y: p.Y, Width: p.Width, Height: 30}
	for _, a := range areas {
		if in := top.Intersect(a); in.Width >= 80 && in.Height > 0 {
			return a, true
		}
	}
	return application.Rect{}, false
}

// workAreas are the screens' work areas, the primary's first. Before the
// app runs there may be none.
func (a *App) workAreas() []application.Rect {
	var areas []application.Rect
	for _, s := range a.app.Screen.GetAll() {
		if s.IsPrimary {
			areas = append([]application.Rect{s.WorkArea}, areas...)
		} else {
			areas = append(areas, s.WorkArea)
		}
	}
	return areas
}

// primaryArea is the primary screen's work area, or an empty Rect.
func primaryArea(areas []application.Rect) application.Rect {
	if len(areas) == 0 {
		return application.Rect{}
	}
	return areas[0]
}

// placeFor is where a new window for file opens: where its window was
// last, or the default size, centered, if that isn't remembered or is
// off the screens. move is false when it should be centered.
func (a *App) placeFor(file string, areas []application.Rect) (p settings.Place, move bool) {
	p = settings.Place{File: file, Width: docWidth, Height: docHeight}
	if file != "" {
		if q, ok := a.settings.Get().Place(file); ok && q.Width > 0 && q.Height > 0 {
			if len(areas) == 0 {
				return q, true // the screens are checked once the window is up
			}
			if area, ok := onScreen(q, areas); ok {
				q.Width, q.Height = fit(q.Width, q.Height, area, 1)
				return q, true
			}
			if !keepPlace {
				// Only the size is kept, so a place off the screens
				// doesn't matter.
				q.Width, q.Height = fit(q.Width, q.Height, areas[0], 1)
				return q, true
			}
		}
	}
	p.Width, p.Height = fit(p.Width, p.Height, primaryArea(areas), fitShare)
	return p, false
}

// whenUp runs fn once win's native window is made: now if it is, else
// once its page has loaded.
func whenUp(win *application.WebviewWindow, fn func()) {
	if win.NativeWindow() != nil {
		fn()
		return
	}
	var cancel func()
	cancel = win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		cancel()
		fn()
	})
}

// settle moves win to p and gives it p's size once it is up. The
// window's options size is its content, but the size remembered is its
// frame, so it is set again here to keep the window from growing by its
// title bar each time it opens. A place off the screens, as when a
// display was unplugged, gets the default size, centered.
func (a *App) settle(win *application.WebviewWindow, p settings.Place) {
	whenUp(win, func() {
		areas := a.workAreas()
		area, ok := onScreen(p, areas)
		switch {
		case !keepPlace:
			// Only the size comes back; the window stays where the
			// system put it.
			if s, err := win.GetScreen(); err == nil && s != nil {
				area = s.WorkArea
			}
			win.SetSize(fit(p.Width, p.Height, area, 1))
		case ok || len(areas) == 0:
			win.SetSize(fit(p.Width, p.Height, area, 1))
			win.SetPosition(p.X, p.Y)
		default:
			win.SetSize(fit(docWidth, docHeight, areas[0], fitShare))
			win.Center()
		}
	})
}

// shrinkToFit cuts win down to fitShare of its screen once it is up,
// for a window made before the screens were known.
func (a *App) shrinkToFit(win *application.WebviewWindow) {
	whenUp(win, func() {
		ow, oh := win.Size()
		area := primaryArea(a.workAreas())
		if s, err := win.GetScreen(); err == nil && s != nil {
			area = s.WorkArea
		}
		if w, h := fit(ow, oh, area, fitShare); w != ow || h != oh {
			win.SetSize(w, h)
			win.Center()
		}
	})
}

// resize gives win its frame size w × h once it is up.
func resize(win *application.WebviewWindow, w, h int) {
	whenUp(win, func() { win.SetSize(w, h) })
}

// resizeTo gives win, already up and showing file, the size file's
// window had last, if that is remembered. It stays where it is.
func (a *App) resizeTo(win *application.WebviewWindow, file string) {
	p, ok := a.settings.Get().Place(file)
	if !ok || p.Width <= 0 || p.Height <= 0 || win.IsMaximised() || win.IsFullscreen() {
		return
	}
	area := application.Rect{}
	if s, err := win.GetScreen(); err == nil && s != nil {
		area = s.WorkArea
	}
	win.SetSize(fit(p.Width, p.Height, area, 1))
}

// steady is the state a window's size is saved from: neither
// minimised, maximised nor full screen.
func steady(win *application.WebviewWindow) bool {
	return !win.IsMinimised() && !win.IsMaximised() && !win.IsFullscreen()
}

// savePlace remembers where w is for the file in front of it.
func (w *docWindow) savePlace() {
	t := w.activeTab()
	if t == nil || t.file() == "" || !steady(w.win) {
		return
	}
	x, y := w.win.Position()
	width, height := w.win.Size()
	if width <= 0 || height <= 0 {
		return
	}
	p := settings.Place{File: t.file(), X: x, Y: y, Width: width, Height: height}
	if q, ok := w.app.settings.Get().Place(p.File); ok && q == p {
		return
	}
	if err := w.app.settings.Update(func(s *settings.Settings) { s.Placed(p) }); err != nil {
		w.app.app.Logger.Warn("saving settings", "err", err)
	}
}

// moved saves where w is once it has stayed put a moment.
func (w *docWindow) moved(*application.WindowEvent) {
	w.app.mu.Lock()
	defer w.app.mu.Unlock()
	if w.placeTimer != nil {
		w.placeTimer.Stop()
	}
	w.placeTimer = time.AfterFunc(placeDelay, w.savePlace)
}

// flushPlace saves where w is now, before it closes.
func (w *docWindow) flushPlace() {
	w.app.mu.Lock()
	if w.placeTimer != nil {
		w.placeTimer.Stop()
		w.placeTimer = nil
	}
	w.app.mu.Unlock()
	w.savePlace()
}

// settingsSize is the size the Settings window opens at.
func (a *App) settingsSize() (int, int) {
	if sz := a.settings.Get().SettingsWindow; sz != nil && sz.Width > 0 && sz.Height > 0 {
		return sz.Width, sz.Height
	}
	return settingsWidth, settingsHeight
}

// saveSettingsSize remembers the Settings window's size.
func (a *App) saveSettingsSize(win *application.WebviewWindow) {
	if !steady(win) {
		return
	}
	width, height := win.Size()
	if width <= 0 || height <= 0 {
		return
	}
	if sz := a.settings.Get().SettingsWindow; sz != nil && sz.Width == width && sz.Height == height {
		return
	}
	err := a.settings.Update(func(s *settings.Settings) { s.SettingsWindow = &settings.Size{Width: width, Height: height} })
	if err != nil {
		a.app.Logger.Warn("saving settings", "err", err)
	}
}
