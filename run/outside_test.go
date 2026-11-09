package run_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gdey/markdownshell/docroot"
	"github.com/gdey/markdownshell/run"
)

// Includes and out= files stay inside the document's folder, unless
// AllowOutside (security review v1.4.0, findings 3 and 4).
func TestOutside(t *testing.T) {
	top := t.TempDir()
	dir := filepath.Join(top, "sub")
	os.Mkdir(dir, 0o755)
	file := filepath.Join(dir, "d.md")
	os.WriteFile(filepath.Join(top, "secret"), []byte("key\n"), 0o644)
	src := "```txt {<include file=../secret}\n```\n\n" +
		"```sh {#img out=image:../escaped.svg on_error=continue}\nprintf '<svg/>'\n```\n"

	_, results := runDoc(t, src, run.Options{Dir: dir, File: file})
	if r := results[0]; !errors.Is(r.Err, docroot.ErrOutside) || len(r.Stdout) != 0 {
		t.Errorf("include ../secret = %q, %v; want ErrOutside", r.Stdout, r.Err)
	}
	if r := results[1]; !errors.Is(r.Err, docroot.ErrOutside) {
		t.Errorf("out=image:../escaped.svg: err = %v, want ErrOutside", r.Err)
	}
	if _, err := os.Stat(filepath.Join(top, "escaped.svg")); err == nil {
		t.Error("escaped.svg was written outside the document's folder")
	}

	_, results = runDoc(t, src, run.Options{Dir: dir, File: file, AllowOutside: true})
	if r := results[0]; r.Err != nil || string(r.Stdout) != "key\n" {
		t.Errorf("AllowOutside include = %q, %v", r.Stdout, r.Err)
	}
	if r := results[1]; r.Err != nil || r.Image != "../escaped.svg" {
		t.Errorf("AllowOutside out=image = %+v", r)
	}
}

// A folder in a git repository may include from anywhere in it.
func TestOutsideRepo(t *testing.T) {
	top := t.TempDir()
	os.Mkdir(filepath.Join(top, ".git"), 0o755)
	dir := filepath.Join(top, "docs")
	os.Mkdir(dir, 0o755)
	os.WriteFile(filepath.Join(top, "Makefile"), []byte("all:\n"), 0o644)
	_, results := runDoc(t, "```make {<include file=../Makefile}\n```\n", run.Options{Dir: dir, File: filepath.Join(dir, "d.md")})
	if r := results[0]; r.Err != nil || string(r.Stdout) != "all:\n" {
		t.Errorf("include from the repo = %q, %v", r.Stdout, r.Err)
	}
}
