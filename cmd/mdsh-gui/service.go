package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/library"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
	"github.com/gdey/markdownshell/run"
)

// This file holds the methods the frontend calls.

// Doc is what the frontend shows about the open file.
type Doc struct {
	File    string              `json:"file"`
	Dirty   bool                `json:"dirty"`
	Changed bool                `json:"changed"`
	Run     notebook.RunOptions `json:"run"`
	// Args is Run.Args as one line, quoted as a shell would need.
	Args string `json:"args"`
}

// Doc describes the open file. File is "" while the start screen shows.
func (a *App) Doc(ctx context.Context) (Doc, error) {
	dw, err := a.window(ctx)
	if err != nil {
		return Doc{}, err
	}
	n, err := dw.notebook()
	if err != nil {
		return Doc{}, nil
	}
	ro := n.RunOptions()
	return Doc{File: n.File(), Dirty: n.Dirty(), Changed: n.Changed(), Run: ro, Args: notebook.JoinArgs(ro.Args)}, nil
}

// Cells is the document laid out for drawing.
func (a *App) Cells(ctx context.Context) ([]notebook.Cell, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return nil, err
	}
	return n.Cells(), nil
}

// RunBlock runs block i alone.
func (a *App) RunBlock(ctx context.Context, i int) error {
	dw, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.RunBlock(i)
	a.afterRun(dw, n)
	return err
}

// RunFrom runs block i and the blocks after it.
func (a *App) RunFrom(ctx context.Context, i int) error {
	dw, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.RunFrom(i)
	a.afterRun(dw, n)
	return err
}

// afterRun marks the title and saves if autosave is on. It doesn't
// save over a file that changed on disk; the banner asks about that.
func (a *App) afterRun(dw *docWindow, n *notebook.Notebook) {
	if a.settings.Get().Autosave && n.Dirty() && !n.Changed() {
		if err := n.Save(); err == nil {
			dw.saved()
			return
		}
	}
	dw.title()
}

// Stop stops the running block, answering stop if a block is waiting
// to be confirmed.
func (a *App) Stop(ctx context.Context) {
	dw, err := a.window(ctx)
	if err != nil {
		return
	}
	select {
	case dw.answers <- run.Stop:
	default:
	}
	if n, err := dw.notebook(); err == nil {
		n.Stop()
	}
}

// Answer replies to the question a confirm event asked: "run", "skip"
// or "stop".
func (a *App) Answer(ctx context.Context, reply string) error {
	dw, err := a.window(ctx)
	if err != nil {
		return err
	}
	ans, ok := map[string]run.Answer{"run": run.Go, "skip": run.Skip, "stop": run.Stop}[reply]
	if !ok {
		return fmt.Errorf("unknown answer %q", reply)
	}
	select {
	case dw.answers <- ans:
		return nil
	default:
		return errors.New("nothing is waiting for an answer")
	}
}

// Open asks for markdown files and opens them: the first in this
// window if it shows the start screen, the rest in new windows.
func (a *App) Open(ctx context.Context) error {
	dw, err := a.window(ctx)
	if err != nil {
		return err
	}
	files, err := a.openDialog(dw.win)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := a.openIn(dw, f); err != nil {
			return err
		}
	}
	return nil
}

// OpenPath opens file, from the start screen.
func (a *App) OpenPath(ctx context.Context, file string) error {
	dw, err := a.window(ctx)
	if err != nil {
		return err
	}
	return a.openIn(dw, file)
}

// Save writes the output into the file.
func (a *App) Save(ctx context.Context) error {
	dw, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if err := n.Save(); err != nil {
		return err
	}
	dw.saved()
	return nil
}

// SaveAs asks where to save the file with its output, and from then on
// the window shows the new file. It returns false if the user cancels.
func (a *App) SaveAs(ctx context.Context) (bool, error) {
	dw, n, err := a.notebook(ctx)
	if err != nil {
		return false, err
	}
	file, err := a.app.Dialog.SaveFile().
		SetMessage("Save a copy with its output").
		SetDirectory(filepath.Dir(n.File())).
		SetFilename(filepath.Base(n.File())).
		AddFilter("Markdown", "*.md;*.markdown").
		AttachToWindow(dw.win).
		PromptForSingleSelection()
	if err != nil || file == "" {
		return false, err
	}
	if err := n.SaveAs(file); err != nil {
		return false, err
	}
	dw.saved()
	a.opened(n.File())
	return true, nil
}

// Reload reads the file from disk again, dropping output not saved.
func (a *App) Reload(ctx context.Context) error {
	dw, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	return dw.open(n.File(), n.RunOptions())
}

// SetRunOptions sets the arguments (one line, split as a shell would),
// --only, --step and --stop-on-error for this file.
func (a *App) SetRunOptions(ctx context.Context, args, only string, step, stopOnError bool) error {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	list, err := notebook.SplitArgs(args)
	if err != nil {
		return fmt.Errorf("arguments: %w", err)
	}
	ro := n.RunOptions()
	ro.Args, ro.Only, ro.Step, ro.StopOnError = list, only, step, stopOnError
	return n.SetRunOptions(ro)
}

// Start is what the start screen lists.
type Start struct {
	Dir    string         `json:"dir"`
	Home   string         `json:"home"` // for showing paths as ~/…
	Files  []library.File `json:"files"`
	Full   bool           `json:"full"` // more files than listed
	Recent []string       `json:"recent"`
}

// StartScreen lists the markdown files under the start folder, and the
// recent files that still exist.
func (a *App) StartScreen() (Start, error) {
	dir := a.startDir()
	files, full, err := library.List(dir)
	if err != nil {
		return Start{}, err
	}
	var recent []string
	for _, f := range a.settings.Get().Recent {
		if _, err := os.Stat(f); err == nil {
			recent = append(recent, f)
		}
	}
	home, _ := os.UserHomeDir()
	return Start{Dir: dir, Home: home, Files: files, Full: full, Recent: recent}, nil
}

// Settings are the app-wide settings.
func (a *App) Settings() settings.Settings { return a.settings.Get() }

// SetSettings saves the settings the Settings window edits: autosave,
// shell and the start folder.
func (a *App) SetSettings(s settings.Settings) error {
	return a.settings.Update(func(cur *settings.Settings) {
		cur.Autosave, cur.Shell, cur.StartFolder = s.Autosave, s.Shell, s.StartFolder
	})
}

// ChooseFolder asks for a folder and returns it, or "" if none was
// picked.
func (a *App) ChooseFolder(ctx context.Context) (string, error) {
	d := a.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle("Choose the start folder").
		SetDirectory(a.startDir())
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// NewWindow opens a window with the start screen.
func (a *App) NewWindow() { a.newWindow() }
