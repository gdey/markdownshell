// Package docroot keeps a document's reads and writes inside the folder
// it is in, and out of hidden files: the document's own folder and the
// folders under it, less any file or folder whose name starts with a
// dot (.env, .ssh, .git, .zshrc), at any depth. Includes, report images,
// casts and out= assets go through it, so a document in ~ can't read
// ~/.ssh/id_ed25519 into its output or write over ~/.zshrc, and one in
// docs/ can't reach the rest of its repository. Being in a repository
// changes nothing: a document reaches its own folder only. Callers
// that trust the document skip it, as mdsh --allow-outside does.
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

// ErrOutside is returned for a path outside the document's folder.
var ErrOutside = errors.New("outside the document's folder (mdsh --allow-outside allows it)")

// ErrHidden is returned for a path inside the root that is, or is in, a
// hidden file or folder: one whose name starts with a dot, at any depth
// below the root. Those hold settings and secrets (.env, .ssh, .git's
// config with a token in a remote's URL, .git/hooks that run later), so
// they count as outside. It matches ErrOutside with errors.Is.
var ErrHidden error = hiddenError{}

type hiddenError struct{}

func (hiddenError) Error() string {
	return "a hidden file or folder (its name starts with a dot), which counts as outside (mdsh --allow-outside allows it)"
}

func (hiddenError) Is(target error) bool { return target == ErrOutside }

// ErrNotRegular is returned for a folder, device, FIFO or other file
// that isn't a regular file.
var ErrNotRegular = errors.New("not a regular file")

// ErrTooBig is returned for a file larger than ReadFile's limit.
var ErrTooBig = errors.New("file too large")

// Root returns the folder files for a document in dir must stay in: dir
// itself, absolute with symlinks resolved. A repository holding dir
// makes no difference.
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
	return abs
}

// Inside reports whether path, absolute and with symlinks resolved, is
// root or under it, and not hidden: see ErrHidden.
func Inside(root, path string) bool {
	rel, ok := relInside(root, path)
	return ok && !Hidden(rel)
}

// relInside returns path relative to root, and whether it is root or
// under it.
func relInside(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	ok := err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
	return rel, ok
}

// Hidden reports whether rel, a clean path relative to a root, names a
// hidden file or folder or something in one: any part of it starting
// with a dot. The root itself (".") isn't hidden, so a document kept in
// a hidden folder still reaches its own folder.
func Hidden(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part != "." && part != ".." && strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// Resolve returns path with symlinks resolved, or ErrOutside if that is
// outside root, or ErrHidden if it is hidden. A path that doesn't exist
// yet, such as a file about to be written, is resolved through its
// nearest existing folder.
func Resolve(root, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := evalExisting(abs)
	if err != nil {
		return "", err
	}
	rel, ok := relInside(root, real)
	if !ok {
		return "", &fs.PathError{Op: "open", Path: path, Err: ErrOutside}
	}
	if Hidden(rel) {
		return "", &fs.PathError{Op: "open", Path: path, Err: ErrHidden}
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
