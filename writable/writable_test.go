package writable

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheck(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anything")
	}
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok.md")
	ro := filepath.Join(dir, "ro.md")
	os.WriteFile(ok, nil, 0o644)
	os.WriteFile(ro, nil, 0o444)
	locked := filepath.Join(dir, "locked")
	os.Mkdir(locked, 0o755)
	inLocked := filepath.Join(locked, "a.md")
	os.WriteFile(inLocked, nil, 0o644)
	os.Chmod(locked, 0o555)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	for _, c := range []struct {
		file string
		ro   bool
	}{
		{ok, false},
		{filepath.Join(dir, "new.md"), false},
		{ro, true},
		{inLocked, true},
		{filepath.Join(locked, "new.md"), true},
	} {
		err := Check(c.file)
		if got := errors.Is(err, ErrReadOnly); got != c.ro || (!c.ro && err != nil) {
			t.Errorf("Check(%s) = %v, want read-only %v", c.file, err, c.ro)
		}
		if c.ro && !errors.Is(err, os.ErrPermission) {
			t.Errorf("Check(%s) = %v doesn't wrap the permission error", c.file, err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("Check left files behind: %v", entries)
	}
}
