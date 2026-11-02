package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// starter is what a new file holds: a heading named for the file.
func starter(file string) []byte {
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	return []byte("# " + title + "\n")
}

// createFile writes a new markdown file at file, adding .md if it has no
// extension, and returns its path. It holds a copy of template, or a
// heading if template is "". A file open in a tab is left alone, as
// replacing it would leave that tab showing what is no longer on disk.
func (a *App) createFile(file, template string) (string, error) {
	if filepath.Ext(file) == "" {
		file += ".md"
	}
	if a.openTab(file) != nil {
		return "", fmt.Errorf("%s is open in a tab; close it first", filepath.Base(file))
	}
	src := starter(file)
	if template != "" {
		var err error
		if src, err = os.ReadFile(template); err != nil {
			return "", err
		}
	}
	return file, os.WriteFile(file, src, 0o644)
}

// newFile asks where to make a new markdown file, makes it from
// template and opens it in w, as openIn does. It does nothing if the
// user cancels.
func (a *App) newFile(w *docWindow, template string) error {
	var win Window
	if w != nil {
		win = w.win
	}
	file, err := a.host.SaveFile(win, SaveFile{
		Message: "Create a new markdown file",
		Button:  "Create",
		Dir:     a.startDir(),
		Name:    "Untitled.md",
		Filter:  "Markdown",
		Pattern: "*.md;*.markdown",
	})
	if err != nil || file == "" {
		return err
	}
	if file, err = a.createFile(file, template); err != nil {
		return err
	}
	return a.openIn(w, file)
}

// newFromMenu runs File › New File…: the tab in front asks which
// template to start from. With no window open there is no page to ask
// in, so it makes a blank file.
func (a *App) newFromMenu() {
	if t := a.currentTab(); t != nil {
		t.emit("menu", "newfile")
		return
	}
	if err := a.newFile(nil, ""); err != nil {
		a.host.ShowError(nil, err)
	}
}
