package ui

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdey/markdownshell/notebook"
)

// assets serves the frontend, and at notebook.DocPrefix the files beside
// the open documents, such as the images their blocks write.
func (a *App) assets(frontend http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path, ok := strings.CutPrefix(r.URL.Path, notebook.DocPrefix+"/"); ok {
			a.serveDocFile(w, r, filepath.FromSlash("/"+path))
			return
		}
		frontend.ServeHTTP(w, r)
	})
}

// serveDocFile serves the file at path if it is in the folder of an open
// document, or under it, and nothing else.
func (a *App) serveDocFile(w http.ResponseWriter, r *http.Request, path string) {
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil || !a.underDoc(real) {
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
