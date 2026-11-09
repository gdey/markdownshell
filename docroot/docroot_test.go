package docroot

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRoot(t *testing.T) {
	top := tempDir(t)
	t.Setenv("HOME", filepath.Join(top, "home"))
	repo := filepath.Join(top, "repo")
	sub := filepath.Join(repo, "docs", "ops")
	os.MkdirAll(sub, 0o755)
	os.Mkdir(filepath.Join(repo, ".git"), 0o755)
	if got := Root(sub); got != repo {
		t.Errorf("Root in a repo = %s, want %s", got, repo)
	}
	plain := filepath.Join(top, "plain", "a")
	os.MkdirAll(plain, 0o755)
	if got := Root(plain); got != plain {
		t.Errorf("Root outside a repo = %s, want %s", got, plain)
	}
	// A dotfiles repository at home isn't the root.
	home := filepath.Join(top, "home")
	notes := filepath.Join(home, "notes")
	os.MkdirAll(notes, 0o755)
	os.Mkdir(filepath.Join(home, ".git"), 0o755)
	if got := Root(notes); got != notes {
		t.Errorf("Root under a home repo = %s, want %s", got, notes)
	}
}

func TestReadFile(t *testing.T) {
	top := tempDir(t)
	root := filepath.Join(top, "root")
	os.Mkdir(root, 0o755)
	os.WriteFile(filepath.Join(root, "in.txt"), []byte("in"), 0o644)
	os.WriteFile(filepath.Join(top, "out.txt"), []byte("out"), 0o644)
	os.Symlink(filepath.Join(top, "out.txt"), filepath.Join(root, "link.txt"))
	os.Mkdir(filepath.Join(root, "dir"), 0o755)

	if data, err := ReadFile(root, filepath.Join(root, "in.txt"), 0); err != nil || string(data) != "in" {
		t.Errorf("inside = %q, %v", data, err)
	}
	for _, p := range []string{filepath.Join(root, "..", "out.txt"), filepath.Join(root, "link.txt"), filepath.Join(top, "out.txt")} {
		if _, err := ReadFile(root, p, 0); !errors.Is(err, ErrOutside) {
			t.Errorf("ReadFile(%s) = %v, want ErrOutside", p, err)
		}
	}
	if data, err := ReadFile("", filepath.Join(root, "link.txt"), 0); err != nil || string(data) != "out" {
		t.Errorf("no root = %q, %v", data, err)
	}
	if _, err := ReadFile(root, filepath.Join(root, "dir"), 0); !errors.Is(err, ErrNotRegular) {
		t.Errorf("folder = %v, want not a regular file", err)
	}
	if _, err := ReadFile(root, filepath.Join(root, "in.txt"), 1); !errors.Is(err, ErrTooBig) {
		t.Errorf("too big = %v, want ErrTooBig", err)
	}
}

func TestWriteFile(t *testing.T) {
	top := tempDir(t)
	root := filepath.Join(top, "root")
	os.Mkdir(root, 0o755)
	if err := WriteFile(root, filepath.Join(root, "a", "b.svg"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(root, filepath.Join(root, "..", "esc.svg"), []byte("x")); !errors.Is(err, ErrOutside) {
		t.Errorf("../ = %v, want ErrOutside", err)
	}
	// A symlinked folder pointing out is caught.
	os.Symlink(top, filepath.Join(root, "up"))
	if err := WriteFile(root, filepath.Join(root, "up", "esc.svg"), []byte("x")); !errors.Is(err, ErrOutside) {
		t.Errorf("through a symlinked folder = %v, want ErrOutside", err)
	}
	// A symlink at the file pointing out is caught.
	os.WriteFile(filepath.Join(top, "rc"), []byte("keep"), 0o644)
	os.Symlink(filepath.Join(top, "rc"), filepath.Join(root, "rc.svg"))
	if err := WriteFile(root, filepath.Join(root, "rc.svg"), []byte("x")); !errors.Is(err, ErrOutside) {
		t.Errorf("symlink at the file = %v, want ErrOutside", err)
	}
	if data, _ := os.ReadFile(filepath.Join(top, "rc")); string(data) != "keep" {
		t.Errorf("target changed to %q", data)
	}
	if _, err := os.Stat(filepath.Join(top, "esc.svg")); err == nil {
		t.Error("esc.svg was written outside")
	}
}
