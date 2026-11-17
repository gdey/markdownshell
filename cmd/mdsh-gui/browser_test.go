package main

import (
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/serve"
	"github.com/gdey/markdownshell/settings"
)

// Serve in Browser serves a file on a free loopback port, gives the same
// address when asked again, and stops when the app quits.
func TestBrowsersServe(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.Open(filepath.Join(dir, "gui.json"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(file, []byte("# Doc\n\n```sh\necho hi\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := newBrowsers(serve.Options{Settings: st, Cwd: dir})
	u, err := b.serve(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "http://127.0.0.1:") || !strings.Contains(u, "?token=") {
		t.Errorf("address %q, want a loopback one with a token", u)
	}
	if again, err := b.serve(file); err != nil || again != u {
		t.Errorf("serving it again: %q, %v; want %q", again, err, u)
	}
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET the address: %s", resp.Status)
	}
	b.close(5 * time.Second)
	if _, err := http.Get(u); err == nil {
		t.Error("still serving after close")
	}
	if _, err := b.serve(file); err == nil {
		t.Error("served after close")
	}
}
