package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/nvim"
)

// The source pane as an embedded Neovim, for the frontend's nvim.ts.
// Each tab with the pane open has its own Neovim. Its redraws come to the
// page as "nvim-redraw" events, the lines typed in it as "nvim-lines"
// and :w as "nvim-write", and what else the buffer reports (the cursor moving,
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
// the page. A running one is left as it is.
func (a *App) NvimStart(ctx context.Context, cols, rows int, background string) (NvimText, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return NvimText{}, err
	}
	t.mu.Lock()
	running := t.nvim
	t.mu.Unlock()
	text, v := n.TextAt()
	st := NvimText{Text: text, Version: v, Buffer: text}
	if running != nil {
		st.Buffer, st.Seq, err = running.Snapshot()
		return st, err
	}
	s := a.settings.Get()
	f, err := nvim.Find(s.NeovimPath)
	if err != nil {
		return NvimText{}, err
	}
	// The language server is this program, run with --lsp.
	var lspCmd []string
	if exe, err := os.Executable(); err == nil {
		lspCmd = []string{exe, "--lsp", n.File()}
	}
	var e *nvim.Editor
	e, err = nvim.Start(nvim.Options{
		Path:   f.Path,
		Cols:   cols,
		Rows:   rows,
		Name:   "mdsh://" + filepath.Base(n.File()),
		Text:   text,
		Clean:  s.NeovimClean,
		Dark:   background == "dark",
		LSP:    lspCmd,
		Redraw: func(ev [][]any) { t.emitNow("nvim-redraw", ev) },
		Lines:  func(l nvim.Lines) { t.emitNow("nvim-lines", l) },
		Write:  func(seq int) { t.emitNow("nvim-write", seq) },
		Event:  func(name, data string) { t.emitNow("nvim-event", NvimEvent{name, data}) },
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
		return NvimText{}, err
	}
	t.mu.Lock()
	t.nvim = e
	t.mu.Unlock()
	return st, nil
}

// NvimText is what NvimStart answers: the file's text and its version,
// for Edit, and Neovim's text and seq, the count of its changes it sent
// and edits it took. A Neovim that just started has the file's text, at
// seq 0; one already running has its own, typing the file doesn't have
// yet included.
type NvimText struct {
	Text    string           `json:"text"`
	Version notebook.Version `json:"version"`
	Buffer  string           `json:"buffer"`
	Seq     int              `json:"seq"`
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

// NvimApply makes edits on Neovim's text, as the page merged Go's
// changes with what was typed, made against its text as of seq, the
// count of the nvim-lines events the page has had and its own NvimApply
// calls. It reports false, changing nothing, if Neovim has typing the
// page hasn't had: that comes as nvim-lines, and the page tries again.
func (a *App) NvimApply(ctx context.Context, seq int, edits []nvim.LineEdit) (bool, error) {
	e, err := a.editor(ctx)
	if err != nil {
		return false, err
	}
	return e.ApplyLines(seq, edits)
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

// nvimEdit does the Neovim pane's right-click Cut, Copy or Paste in the
// current tab. They act on Neovim's selection: the page, which only
// draws Neovim's grid, has none for the usual edit commands. Paste in
// visual mode replaces the selection, as Neovim's own paste does.
func (a *App) nvimEdit(op string) {
	t := a.currentTab()
	if t == nil {
		return
	}
	t.mu.Lock()
	e := t.nvim
	t.mu.Unlock()
	if e == nil {
		return
	}
	if err := nvimEdit(e, op, a.app.Clipboard.Text, a.app.Clipboard.SetText); err != nil {
		slog.Error("Neovim "+op+": "+err.Error(), "file", t.file())
	}
}

// nvimEdit does op, cut, copy or paste, in e with the clipboard's get
// and set.
func nvimEdit(e *nvim.Editor, op string, get func() (string, bool), set func(string) bool) error {
	if op == "paste" {
		text, ok := get()
		if !ok || text == "" {
			return nil
		}
		return e.Paste(text)
	}
	args, _ := json.Marshal([]bool{op == "cut"})
	out, err := e.Call("selection", string(args))
	if err != nil {
		return err
	}
	var text *string
	if err := json.Unmarshal([]byte(out), &text); err != nil {
		return err
	}
	if text != nil && *text != "" && !set(*text) {
		return errors.New("couldn't set the clipboard")
	}
	return nil
}
