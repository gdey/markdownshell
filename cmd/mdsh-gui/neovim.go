package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/nvim"
)

// The source pane as an embedded Neovim, for the frontend's nvim.ts.
// Each tab with the pane open has its own Neovim. Its redraws come to the
// page as "nvim-redraw" events, its edits as "nvim-text" and :w as
// "nvim-write", and what else the buffer reports (the cursor moving,
// :RunBlock, …) as "nvim-event"; "nvim-exit" says it quit.

// NeovimInfo is what the Settings window shows about Neovim.
type NeovimInfo struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	// Error is why Neovim can't be used, or "".
	Error string `json:"error"`
}

// Neovim says which nvim the source pane would run.
func (a *App) Neovim() NeovimInfo {
	f, err := nvim.Find(a.settings.Get().NeovimPath)
	info := NeovimInfo{Path: f.Path}
	if f.Version != (nvim.Version{}) {
		info.Version = f.Version.String()
	}
	if err != nil {
		info.Error = err.Error()
	}
	return info
}

// NvimStart starts the tab's Neovim with a grid of cols by rows, editing
// the file's text, with 'background' set to "light" or "dark" to match
// the page. It returns the text Neovim started with, the base for
// SetText. A running one is left as it is.
func (a *App) NvimStart(ctx context.Context, cols, rows int, background string) (string, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	running := t.nvim != nil
	t.mu.Unlock()
	text := n.Text()
	if running {
		return text, nil
	}
	s := a.settings.Get()
	f, err := nvim.Find(s.NeovimPath)
	if err != nil {
		return "", err
	}
	var e *nvim.Editor
	e, err = nvim.Start(nvim.Options{
		Path:    f.Path,
		Cols:    cols,
		Rows:    rows,
		Name:    "mdsh://" + filepath.Base(n.File()),
		Text:    text,
		Clean:   s.NeovimClean,
		Dark:    background == "dark",
		Redraw:  func(ev [][]any) { t.emitNow("nvim-redraw", ev) },
		Changed: func(text string) { t.emitNow("nvim-text", text) },
		Write:   func(text string) { t.emitNow("nvim-write", text) },
		Event:   func(name, data string) { t.emitNow("nvim-event", NvimEvent{name, data}) },
		Exit: func(err error) {
			t.mu.Lock()
			if t.nvim == e {
				t.nvim = nil
			}
			t.mu.Unlock()
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			t.emitNow("nvim-exit", msg)
		},
	})
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	t.nvim = e
	t.mu.Unlock()
	return text, nil
}

var errNoNvim = errors.New("Neovim isn't running")

// editor is the tab's Neovim.
func (a *App) editor(ctx context.Context) (*nvim.Editor, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.nvim == nil {
		return nil, errNoNvim
	}
	return t.nvim, nil
}

// NvimInput types keys, in Neovim's notation.
func (a *App) NvimInput(ctx context.Context, keys string) error {
	e, err := a.editor(ctx)
	if err != nil {
		return err
	}
	return e.Input(keys)
}

// NvimMouse passes on a mouse event at a grid cell.
func (a *App) NvimMouse(ctx context.Context, button, action, mod string, row, col int) error {
	e, err := a.editor(ctx)
	if err != nil {
		return err
	}
	return e.Mouse(button, action, mod, row, col)
}

// NvimPaste pastes text.
func (a *App) NvimPaste(ctx context.Context, text string) error {
	e, err := a.editor(ctx)
	if err != nil {
		return err
	}
	return e.Paste(text)
}

// NvimResize resizes the grid. Before Neovim runs there is nothing to
// resize: NvimStart takes the size.
func (a *App) NvimResize(ctx context.Context, cols, rows int) error {
	e, err := a.editor(ctx)
	if errors.Is(err, errNoNvim) {
		return nil
	}
	if err != nil {
		return err
	}
	return e.Resize(cols, rows)
}

// NvimSetText replaces Neovim's text, as after a run writes output and
// the page merged it with what was typed. Before Neovim runs there is
// nothing to do: NvimStart takes the text.
func (a *App) NvimSetText(ctx context.Context, text string) error {
	e, err := a.editor(ctx)
	if errors.Is(err, errNoNvim) {
		return nil
	}
	if err != nil {
		return err
	}
	return e.SetText(text)
}

// NvimApply gives Neovim text, Go's text merged with what was typed, as
// edits to the text Neovim last had, so typing elsewhere in the moment
// since isn't lost. If an edit's lines changed, the whole text is set.
func (a *App) NvimApply(ctx context.Context, text string, edits []nvim.Edit) error {
	e, err := a.editor(ctx)
	if errors.Is(err, errNoNvim) {
		return nil
	}
	if err != nil {
		return err
	}
	return e.ApplyEdits(text, edits)
}

// NvimEvent is something the Neovim buffer reports: its name, such as
// "cursor" or "run", and its data as JSON.
type NvimEvent struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

// NvimCall calls fn in mdsh.lua with args, a JSON array, and returns its
// result as JSON: the cursor, setting the run buttons or folds, and so on.
func (a *App) NvimCall(ctx context.Context, fn, args string) (string, error) {
	e, err := a.editor(ctx)
	if err != nil {
		return "", err
	}
	return e.Call(fn, args)
}

// NvimSaved marks Neovim's buffer unchanged after a save.
func (a *App) NvimSaved(ctx context.Context) error {
	e, err := a.editor(ctx)
	if err != nil {
		return err
	}
	return e.Saved()
}

// NvimStop quits the tab's Neovim, as the source pane closes.
func (a *App) NvimStop(ctx context.Context) {
	t, err := a.tab(ctx)
	if err == nil {
		t.stopNvim()
	}
}

// stopNvim quits the tab's Neovim, if it has one.
func (t *docTab) stopNvim() {
	t.mu.Lock()
	e := t.nvim
	t.nvim = nil
	t.mu.Unlock()
	if e != nil {
		go e.Close()
	}
}

// emitNow sends an event to the tab's page without keeping it for a
// page that catches up on a run.
func (t *docTab) emitNow(name string, data any) {
	if w := t.window(); w != nil {
		w.win.EmitEvent(name, tabEvent{t.id, data})
	}
}
