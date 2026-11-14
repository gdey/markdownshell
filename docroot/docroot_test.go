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
	// A repository doesn't widen the root: a document in docs/ops gets
	// docs/ops, not the repository.
	repo := filepath.Join(top, "repo")
	sub := filepath.Join(repo, "docs", "ops")
	os.MkdirAll(sub, 0o755)
	os.Mkdir(filepath.Join(repo, ".git"), 0o755)
	if got := Root(sub); got != sub {
		t.Errorf("Root in a repo = %s, want %s", got, sub)
	}
	plain := filepath.Join(top, "plain", "a")
	os.MkdirAll(plain, 0o755)
	if got := Root(plain); got != plain {
		t.Errorf("Root outside a repo = %s, want %s", got, plain)
	}
	// A symlinked folder resolves.
	os.Symlink(plain, filepath.Join(top, "alias"))
	if got := Root(filepath.Join(top, "alias")); got != plain {
		t.Errorf("Root through a symlink = %s, want %s", got, plain)
	}
}

// TestFinding3 is finding 3 of the v1.5.0 security review: a document
// in ~ reached ~/.ssh, and one in a repository reached all of it.
func TestFinding3(t *testing.T) {
	home := tempDir(t)
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("KEY"), 0o600)
	os.WriteFile(filepath.Join(home, ".zshrc"), []byte("rc"), 0o644)
	os.MkdirAll(filepath.Join(home, "Downloads"), 0o755)
	os.WriteFile(filepath.Join(home, "tax.txt"), []byte("TAX"), 0o644)

	root := Root(home)
	if _, err := ReadFile(root, filepath.Join(home, ".ssh", "id_ed25519"), 0); !errors.Is(err, ErrHidden) {
		t.Errorf("~/.ssh/id_ed25519 from ~ = %v, want ErrHidden", err)
	}
	if err := WriteFile(root, filepath.Join(home, ".zshrc"), []byte("x")); !errors.Is(err, ErrHidden) {
		t.Errorf("write ~/.zshrc from ~ = %v, want ErrHidden", err)
	}
	if data, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(data) != "rc" {
		t.Errorf(".zshrc changed to %q", data)
	}
	down := Root(filepath.Join(home, "Downloads"))
	if _, err := ReadFile(down, filepath.Join(home, "tax.txt"), 0); !errors.Is(err, ErrOutside) {
		t.Errorf("~/tax.txt from ~/Downloads = %v, want ErrOutside", err)
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

func TestHidden(t *testing.T) {
	root := tempDir(t)
	os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755)
	os.WriteFile(filepath.Join(root, ".git", "config"), []byte("url = https://u:ghp_x@github.com/a/b"), 0o644)
	os.MkdirAll(filepath.Join(root, "vendor", "lib", ".hg"), 0o755)
	os.WriteFile(filepath.Join(root, "vendor", "lib", ".hg", "hgrc"), []byte("token"), 0o644)
	os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=x"), 0o644)
	os.WriteFile(filepath.Join(root, ".gitignore"), []byte("x"), 0o644)
	os.Symlink(filepath.Join(root, ".git", "config"), filepath.Join(root, "config.txt"))
	os.Symlink(filepath.Join(root, ".env"), filepath.Join(root, "env.txt"))
	os.WriteFile(filepath.Join(root, "a.b.txt"), []byte("ok"), 0o644)

	for _, p := range []string{
		filepath.Join(root, ".git", "config"),
		filepath.Join(root, ".env"),
		filepath.Join(root, ".gitignore"),
		filepath.Join(root, "config.txt"), // a symlink into .git
		filepath.Join(root, "env.txt"),    // a symlink to .env
		filepath.Join(root, "vendor", "lib", ".hg", "hgrc"),
		filepath.Join(root, ".git"),
	} {
		_, err := ReadFile(root, p, 0)
		if !errors.Is(err, ErrHidden) || !errors.Is(err, ErrOutside) {
			t.Errorf("ReadFile(%s) = %v, want ErrHidden and ErrOutside", p, err)
		}
	}
	for _, p := range []string{
		filepath.Join(root, ".git", "hooks", "pre-commit"),
		filepath.Join(root, ".jj", "new"),
		filepath.Join(root, "sub", ".config", "x"),
		filepath.Join(root, ".bashrc"),
	} {
		if err := WriteFile(root, p, []byte("x")); !errors.Is(err, ErrHidden) {
			t.Errorf("WriteFile(%s) = %v, want ErrHidden", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-commit")); err == nil {
		t.Error("a hook was written")
	}
	// A dot inside a name isn't hidden.
	if data, err := ReadFile(root, filepath.Join(root, "a.b.txt"), 0); err != nil || string(data) != "ok" {
		t.Errorf("a.b.txt = %q, %v", data, err)
	}
	// A document kept in a hidden folder reaches its own folder.
	inner := filepath.Join(root, ".notes")
	os.Mkdir(inner, 0o755)
	os.WriteFile(filepath.Join(inner, "in.txt"), []byte("in"), 0o644)
	if data, err := ReadFile(Root(inner), filepath.Join(inner, "in.txt"), 0); err != nil || string(data) != "in" {
		t.Errorf("in a hidden root = %q, %v", data, err)
	}
	// With no root, as with --allow-outside, nothing is refused.
	if _, err := ReadFile("", filepath.Join(root, ".git", "config"), 0); err != nil {
		t.Errorf("ReadFile with no root = %v", err)
	}
}
