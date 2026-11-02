// Package session makes and finds the dated markdown files that the
// start screen's New Session and Last Session open: one file a day,
// named for the date, in a sessions folder.
package session

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultDir is the sessions folder when none is set, under home.
func DefaultDir(home string) string { return filepath.Join(home, ".mdsh", "sessions") }

// Dir is the sessions folder: folder, with a leading ~ for home, or
// DefaultDir when folder is "".
func Dir(folder, home string) string {
	folder = strings.TrimSpace(folder)
	switch {
	case folder == "":
		return DefaultDir(home)
	case folder == "~":
		return home
	case strings.HasPrefix(folder, "~/"):
		return filepath.Join(home, folder[2:])
	}
	return folder
}

// Name is the file name of day's session, such as 2026-11-02.md.
func Name(day time.Time) string { return day.Format(time.DateOnly) + ".md" }

// Today is the path of the session for now's day in dir. If it isn't
// there it is made, with dir, as a copy of template, or a heading for
// the date when template is "". It reports whether it made the file;
// a file already there is left as it is.
func Today(dir, template string, now time.Time) (file string, made bool, err error) {
	file = filepath.Join(dir, Name(now))
	if _, err := os.Stat(file); err == nil {
		return file, false, nil
	}
	src := []byte("# " + now.Format(time.DateOnly) + "\n")
	if template != "" {
		if src, err = os.ReadFile(template); err != nil {
			return "", false, err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	// O_EXCL, so a file made since the Stat above is kept.
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return file, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if _, err := f.Write(src); err != nil {
		f.Close()
		return "", false, err
	}
	return file, true, f.Close()
}

// Last is the markdown file in dir changed last, or "" if dir has none
// or isn't there. Files in folders under dir aren't looked at.
func Last(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var last string
	var at time.Time
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !e.Type().IsRegular() || (ext != ".md" && ext != ".markdown") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		// Entries come sorted by name, so of files changed at the same
		// time the later name, the later date, wins.
		if last == "" || !fi.ModTime().Before(at) {
			last, at = filepath.Join(dir, e.Name()), fi.ModTime()
		}
	}
	return last, nil
}
