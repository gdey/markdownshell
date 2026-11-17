package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRemoteImage(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	mux := http.NewServeMux()
	mux.HandleFunc("/a.png", func(w http.ResponseWriter, r *http.Request) { w.Write(png) })
	mux.HandleFunc("/x.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`))
	})
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html><script>x()</script></html>")) })
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	mux.HandleFunc("/to-file", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	})
	mux.HandleFunc("/to-png", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/a.png", http.StatusFound) })
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Write(png)
		w.Write(make([]byte, maxRemoteImage))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()

	for _, p := range []string{"/a.png", "/to-png"} {
		got, err := loadRemoteImage(ctx, remoteImageClient, srv.URL+p)
		if err != nil || got != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png) {
			t.Errorf("%s: %q, %v", p, got, err)
		}
	}
	for _, src := range []string{
		srv.URL + "/x.svg", srv.URL + "/page", srv.URL + "/gone", srv.URL + "/to-file", srv.URL + "/big",
		"file:///etc/passwd", "javascript:alert(1)", "data:image/png;base64,AAAA", "plot.png", "//example.com/a.png", "ftp://example.com/a.png",
	} {
		if got, err := loadRemoteImage(ctx, remoteImageClient, src); err == nil || got != "" {
			t.Errorf("%s: %q, %v; want refused", strings.TrimPrefix(src, srv.URL), got, err)
		}
	}
}

func TestLoadImageFile(t *testing.T) {
	dir := t.TempDir()
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	os.WriteFile(filepath.Join(dir, "a.png"), png, 0o644)
	os.WriteFile(filepath.Join(dir, "key"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "x.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0o644)
	if got, err := loadImageFile(filepath.Join(dir, "a.png")); err != nil || !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Errorf("a.png: %q, %v", got, err)
	}
	for _, name := range []string{"key", "x.svg", "missing.png", "."} {
		if got, err := loadImageFile(filepath.Join(dir, name)); err == nil || got != "" {
			t.Errorf("%s: %q, %v; want refused", name, got, err)
		}
	}
}
