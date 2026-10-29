package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// starter is what a new file holds: a heading named for the file.
func starter(file string) []byte {
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	return []byte("# " + title + "\n")
}

// createFile writes a new markdown file at file, adding .md if it has no
// extension, and returns its path. A file open in a tab is left alone,
// as replacing it would leave that tab showing what is no longer on
// disk.
func (a *App) createFile(file string) (string, error) {
	if filepath.Ext(file) == "" {
		file += ".md"
	}
	if a.openTab(file) != nil {
		return "", fmt.Errorf("%s is open in a tab; close it first", filepath.Base(file))
	}
	return file, os.WriteFile(file, starter(file), 0o644)
}

// newFile asks where to make a new markdown file, makes it and opens it
// in w, as openIn does. It does nothing if the user cancels.
func (a *App) newFile(w *docWindow) error {
	d := a.app.Dialog.SaveFile().
		SetMessage("Create a new markdown file").
		SetButtonText("Create").
		SetDirectory(a.startDir()).
		SetFilename("Untitled.md").
		CanCreateDirectories(true).
		AddFilter("Markdown", "*.md;*.markdown")
	if w != nil {
		d.AttachToWindow(w.win)
	}
	file, err := d.PromptForSingleSelection()
	if err != nil || file == "" {
		return err
	}
	if file, err = a.createFile(file); err != nil {
		return err
	}
	return a.openIn(w, file)
}

// newFromMenu runs File › New File… for the window in front.
func (a *App) newFromMenu() {
	w := a.current()
	if err := a.newFile(w); err != nil {
		var win application.Window
		if w != nil {
			win = w.win
		}
		a.showError(win, err)
	}
}
