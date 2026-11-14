package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/docroot"
)

// docCSP is the Content-Security-Policy sent with the files beside the
// open documents, so an .svg or .html among them can load nothing and
// run nothing if the window ever shows one as a page. The pages carry
// their own policy in a meta tag.
const docCSP = "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; sandbox"

// assets serves the frontend, the app icon, the changelog's
// screenshots, and at notebook.DocPrefix the
// files beside the open documents, such as the images their blocks write.
func (a *App) assets(pages http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/appicon.svg" {
			http.ServeFileFS(w, r, frontend, "build/appicon.svg")
			return
		}
		if name, ok := strings.CutPrefix(r.URL.Path, changelogImages); ok {
			serveChangelogImage(w, r, name)
			return
		}
		if path, ok := strings.CutPrefix(r.URL.Path, notebook.DocPrefix+"/"); ok {
			a.serveDocFile(w, r, filepath.FromSlash("/"+path))
			return
		}
		pages.ServeHTTP(w, r)
	})
}

// docTypes are the kinds of file the window loads from beside a
// document: images, audio and video, and terminal recordings, which the
// cast player reads. Nothing else beside a document is served, so the
// route can't hand a page a .env, a key or a database that happens to sit
// in the same folder tree (finding 12 of the v1.4.0 security review).
var docTypes = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".svg": true, ".avif": true, ".apng": true, ".bmp": true, ".ico": true,
	".mp4": true, ".webm": true, ".mov": true, ".m4v": true,
	".mp3": true, ".wav": true, ".ogg": true, ".m4a": true, ".flac": true,
	".cast": true,
}

// docType reports whether path's extension is one of docTypes.
func docType(path string) bool { return docTypes[strings.ToLower(filepath.Ext(path))] }

// serveDocFile serves the file at path if it is an image, media or
// recording (docTypes) in the folder of an open document, or under it,
// or is an image the start screen shows, and nothing else. The type is
// checked on the path the link resolves to, so a link named .png that
// points at a .env isn't served.
func (a *App) serveDocFile(w http.ResponseWriter, r *http.Request, path string) {
	real, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil || !(a.underDoc(real) && docType(real)) && !a.startImage(real) {
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
	w.Header().Set("Content-Security-Policy", docCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
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
// in some tab, and not a hidden file or in a hidden folder below it (see
// docroot.Hidden), so the route can't serve a .cast or image from .git
// or another dot folder.
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
		if rel, err := filepath.Rel(dir, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !docroot.Hidden(rel) {
			return true
		}
	}
	return false
}
