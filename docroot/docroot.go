// Package docroot keeps a document's reads and writes inside the folder
// it belongs to: the root of the git (or Mercurial, Jujutsu or
// Subversion) repository holding it, or the document's own folder when
// it isn't in one. Includes, report images, casts and out= assets go
// through it, so a document can't read ~/.ssh/id_ed25519 into its output
// or write over ~/.zshrc. Callers that trust the document skip it, as
// mdsh --allow-outside does.
package docroot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// MaxRead is the most ReadFile reads by default: 16 MB.
const MaxRead = 16 << 20

// ErrOutside is returned for a path outside the document's root.
var ErrOutside = errors.New("outside the document's folder or repository (mdsh --allow-outside allows it)")

// ErrNotRegular is returned for a folder, device, FIFO or other file
// that isn't a regular file.
var ErrNotRegular = errors.New("not a regular file")

// ErrTooBig is returned for a file larger than ReadFile's limit.
var ErrTooBig = errors.New("file too large")

// markers name the folders that mark a repository's root.
var markers = []string{".git", ".hg", ".jj", ".svn"}

// Root returns the folder files for a document in dir must stay in: the
// root of the repository holding dir, else dir itself. A repository at
// the home folder or the file system's root, such as a dotfiles
// repository, is not used, as it would let a document reach everything
// under it; dir is used instead. The result is absolute with symlinks
// resolved.
func Root(dir string) string {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if real, err := filepath.EvalSymlinks(home); err == nil {
			home = real
		}
	}
	for d := abs; ; {
		if d == home || d == filepath.Dir(d) {
			break
		}
		for _, m := range markers {
			if _, err := os.Lstat(filepath.Join(d, m)); err == nil {
				return d
			}
		}
		d = filepath.Dir(d)
	}
	return abs
}

// Inside reports whether path, absolute and with symlinks resolved, is
// root or under it.
func Inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Resolve returns path with symlinks resolved, or ErrOutside if that is
// outside root. A path that doesn't exist yet, such as a file about to
// be written, is resolved through its nearest existing folder.
func Resolve(root, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := evalExisting(abs)
	if err != nil {
		return "", err
	}
	if !Inside(root, real) {
		return "", &fs.PathError{Op: "open", Path: path, Err: ErrOutside}
	}
	return real, nil
}

// evalExisting resolves symlinks in the longest part of abs that
// exists, and adds the rest back on.
func evalExisting(abs string) (string, error) {
	var rest []string
	for d := abs; ; {
		real, err := filepath.EvalSymlinks(d)
		if err == nil {
			return filepath.Join(append([]string{real}, rest...)...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(d)
		if parent == d {
			return abs, nil
		}
		rest = append([]string{filepath.Base(d)}, rest...)
		d = parent
	}
}

// ReadFile reads path, which must be a regular file of at most max bytes
// (MaxRead if max <= 0). With root set, it must also be inside root;
// with root empty, anywhere is allowed.
func ReadFile(root, path string, max int64) ([]byte, error) {
	if max <= 0 {
		max = MaxRead
	}
	real := path
	if root != "" {
		var err error
		if real, err = Resolve(root, path); err != nil {
			return nil, err
		}
	}
	f, err := os.Open(real)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "read", Path: path, Err: ErrNotRegular}
	}
	if st.Size() > max {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fmt.Errorf("%w: %d bytes, at most %d", ErrTooBig, st.Size(), max)}
	}
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fmt.Errorf("%w: at most %d bytes", ErrTooBig, max)}
	}
	return data, nil
}

// WriteFile writes data to path, making its folder. With root set, the
// path must be inside root once symlinks in it are resolved, and a
// symlink at path itself is not followed: the file is written to a
// temporary file beside it and renamed over it.
func WriteFile(root, path string, data []byte) error {
	real := path
	if root != "" {
		var err error
		if real, err = Resolve(root, path); err != nil {
			return err
		}
	}
	dir := filepath.Dir(real)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if root != "" {
		// MkdirAll may have followed a symlink made since Resolve.
		if d, err := filepath.EvalSymlinks(dir); err != nil || !Inside(root, d) {
			return &fs.PathError{Op: "write", Path: path, Err: ErrOutside}
		}
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(real)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, real); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
