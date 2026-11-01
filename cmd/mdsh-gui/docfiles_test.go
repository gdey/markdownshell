package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
)

func TestDocFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs")
	for name, data := range map[string]string{
		"docs/doc.md":       "# Doc\n",
		"docs/img/a.svg":    "<svg/>",
		"secret.txt":        "no",
		"docs/img/link.svg": "",
	} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A link out of the folder isn't followed.
	link := filepath.Join(dir, "img", "out.txt")
	if err := os.Symlink(filepath.Join(root, "secret.txt"), link); err != nil {
		t.Fatal(err)
	}
	n, err := notebook.Open(filepath.Join(dir, "doc.md"), notebook.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	a := &App{tabs: map[int]*docTab{1: {n: n}, 2: {}}}
	h := a.assets(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	get := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code
	}
	for path, want := range map[string]int{
		notebook.DocURL(filepath.Join(dir, "img", "a.svg")):           http.StatusOK,
		notebook.DocURL(filepath.Join(root, "secret.txt")):            http.StatusNotFound,
		notebook.DocPrefix + filepath.ToSlash(dir) + "/../secret.txt": http.StatusNotFound,
		notebook.DocURL(link):                              http.StatusNotFound,
		notebook.DocURL(filepath.Join(dir, "img")):         http.StatusNotFound,
		notebook.DocURL(filepath.Join(dir, "missing.png")): http.StatusNotFound,
		"/index.html": http.StatusTeapot,
	} {
		if got := get(path); got != want {
			t.Errorf("GET %s = %d, want %d", path, got, want)
		}
	}
}

func TestStartImages(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"banner.png", "other.png"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("png"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	banner, err := filepath.EvalSymlinks(filepath.Join(dir, "banner.png"))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{tabs: map[int]*docTab{}, startImages: map[string]bool{banner: true}}
	h := a.assets(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	for path, want := range map[string]int{
		notebook.DocURL(filepath.Join(dir, "banner.png")): http.StatusOK,
		notebook.DocURL(filepath.Join(dir, "other.png")):  http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}
