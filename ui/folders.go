package ui

import (
	"errors"
	"path"
	"path/filepath"
	"slices"

	"github.com/gdey/markdownshell/collection"
	"github.com/gdey/markdownshell/library"
)

// startFolders returns the folders a start screen listing of dir shows,
// by rel, with their absolute paths, and those of them whose
// .mdsh.yaml can be saved, in path order.
func startFolders(dir string, l library.Listing) (map[string]string, []string) {
	folders := map[string]string{}
	add := func(rel string) {
		if _, ok := folders[rel]; !ok {
			folders[rel] = filepath.Join(dir, filepath.FromSlash(rel))
		}
	}
	for _, c := range l.Collections {
		add(c.Rel)
	}
	for _, f := range l.Files {
		if d := path.Dir(f.Rel); d != "." {
			add(d)
		} else {
			add("")
		}
	}
	var writable []string
	for rel, abs := range folders {
		if collection.Writable(abs) {
			writable = append(writable, rel)
		}
	}
	slices.Sort(writable)
	return folders, writable
}

// FolderForm is what the folder settings dialog edits.
type FolderForm struct {
	// Path is the folder's absolute path.
	Path     string              `json:"path"`
	Settings collection.Settings `json:"settings"`
	// Images are image files in the folder, for the icon and background.
	Images []string `json:"images"`
}

// maxImages is how many images FolderSettings offers.
const maxImages = 200

// folder returns the absolute path of rel, a folder the start screen
// lists, when its .mdsh.yaml can be saved.
func (a *App) folder(rel string) (string, error) {
	a.mu.Lock()
	abs, ok := a.startFolders[rel]
	a.mu.Unlock()
	if !ok {
		return "", errors.New("not a folder the start screen lists")
	}
	if !collection.Writable(abs) {
		return "", errors.New(abs + ": read only")
	}
	return abs, nil
}

// FolderSettings reads the .mdsh.yaml of rel, a folder the start screen
// lists, for the folder settings dialog.
func (a *App) FolderSettings(rel string) (FolderForm, error) {
	abs, err := a.folder(rel)
	if err != nil {
		return FolderForm{}, err
	}
	s, err := collection.Read(abs)
	if err != nil {
		return FolderForm{}, err
	}
	return FolderForm{Path: abs, Settings: s, Images: collection.Images(abs, maxImages)}, nil
}

// SetFolderSettings saves s as the .mdsh.yaml of rel, a folder the start
// screen lists. See collection.Write.
func (a *App) SetFolderSettings(rel string, s collection.Settings) error {
	abs, err := a.folder(rel)
	if err != nil {
		return err
	}
	return collection.Write(abs, s)
}
