package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
)

// assets serves the frontend, the app icon, and at notebook.DocPrefix the
// files beside the open documents, such as the images their blocks write.
func (a *App) assets(pages http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/appicon.svg" {
			http.ServeFileFS(w, r, frontend, "build/appicon.svg")
			return
		}
		if path, ok := strings.CutPrefix(r.URL.Path, notebook.DocPrefix+"/"); ok {
			a.serveDocFile(w, r, filepath.FromSlash("/"+path))
			return
		}
		pages.ServeHTTP(w, r)
	})
}

// serveDocFile serves the file at path if it is in the folder of an open
// document, or under it, or is an image the start screen shows, and
// nothing else.
func (a *App) serveDocFile(w http.ResponseWriter, r *http.Request, path string) {
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil || !a.underDoc(real) && !a.startImage(real) {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(real)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}

// startImage reports whether path is a collection's icon or background
// on the start screen.
func (a *App) startImage(path string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.startImages[path]
}

// underDoc reports whether path is under the folder of a document open
// in some tab.
func (a *App) underDoc(path string) bool {
	a.mu.Lock()
	tabs := make([]*docTab, 0, len(a.tabs))
	for _, t := range a.tabs {
		tabs = append(tabs, t)
	}
	a.mu.Unlock()
	for _, t := range tabs {
		n, err := t.notebook()
		if err != nil {
			continue
		}
		dir, err := filepath.EvalSymlinks(filepath.Dir(n.File()))
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(dir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
