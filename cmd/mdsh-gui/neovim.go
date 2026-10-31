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
// "nvim-write"; "nvim-exit" says it quit.

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
// the page. A running one is left as it is.
func (a *App) NvimStart(ctx context.Context, cols, rows int, background string) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	t.mu.Lock()
	running := t.nvim != nil
	t.mu.Unlock()
	if running {
		return nil
	}
	s := a.settings.Get()
	f, err := nvim.Find(s.NeovimPath)
	if err != nil {
		return err
	}
	var e *nvim.Editor
	e, err = nvim.Start(nvim.Options{
		Path:    f.Path,
		Cols:    cols,
		Rows:    rows,
		Name:    "mdsh://" + filepath.Base(n.File()),
		Text:    n.Text(),
		Clean:   s.NeovimClean,
		Dark:    background == "dark",
		Redraw:  func(ev [][]any) { t.emitNow("nvim-redraw", ev) },
		Changed: func(text string) { t.emitNow("nvim-text", text) },
		Write:   func(text string) { t.emitNow("nvim-write", text) },
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
		return err
	}
	t.mu.Lock()
	t.nvim = e
	t.mu.Unlock()
	return nil
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

// NvimLoad gives Neovim the file's text, as after a run writes output.
// Before Neovim runs there is nothing to do: NvimStart takes the text.
func (a *App) NvimLoad(ctx context.Context) error {
	e, err := a.editor(ctx)
	if errors.Is(err, errNoNvim) {
		return nil
	}
	if err != nil {
		return err
	}
	_, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	return e.SetText(n.Text())
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
