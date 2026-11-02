// Package writable checks, before any work is done, that a file can be
// saved.
package writable

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrReadOnly is wrapped by the error Check returns when the file, or
// the folder it is saved through, can't be written.
var ErrReadOnly = errors.New("read-only")

// Check reports whether file can be saved: opened for writing, and
// replaced through a temporary file in its folder, as mdsh saves. It
// tries both rather than reading mode bits, so ownership, ACLs, a
// read-only folder and a read-only file system all count. If file
// doesn't exist, only its folder counts. The error wraps ErrReadOnly
// when permission is what stops it.
func Check(file string) error {
	f, err := os.OpenFile(file, os.O_WRONLY, 0)
	switch {
	case err == nil:
		f.Close()
	case !errors.Is(err, os.ErrNotExist):
		return wrap(file, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".mdsh-check-*")
	if err != nil {
		return wrap(file, err)
	}
	tmp.Close()
	os.Remove(tmp.Name())
	return nil
}

func wrap(file string, err error) error {
	var pe *os.PathError
	if errors.Is(err, os.ErrPermission) || (errors.As(err, &pe) && isROFS(pe.Err)) {
		return &Error{File: file, Err: err}
	}
	return err
}

// Error says file can't be saved, and why.
type Error struct {
	File string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("%s is read-only", e.File) }

func (e *Error) Unwrap() []error { return []error{ErrReadOnly, e.Err} }
