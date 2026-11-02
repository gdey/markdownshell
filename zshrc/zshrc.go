// Package zshrc adds and removes the mdsh function in the user's .zshrc
// that passes the calling shell's aliases and functions to blocks.
package zshrc

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	begin = "# >>> mdsh: give blocks this shell's aliases and functions (added by mdsh-gui) >>>"
	end   = "# <<< mdsh <<<"
	// Wrapper is the line between the markers. The function keyword keeps
	// an alias named mdsh from breaking the definition.
	Wrapper = `function mdsh { command mdsh --source <(alias -L; typeset -f) "$@"; }`
)

// Path is the .zshrc zsh reads: in $ZDOTDIR if set, else the home folder.
func Path() (string, error) {
	if dir := os.Getenv("ZDOTDIR"); dir != "" {
		return filepath.Join(dir, ".zshrc"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zshrc"), nil
}

// Installed reports whether the file at path has the wrapper. A missing
// file doesn't.
func Installed(path string) (bool, error) {
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, _, ok := cut(string(src))
	return ok, nil
}

// Set adds the wrapper to the end of the file at path, creating the file
// if need be, or removes it, leaving the rest of the file as it was.
func Set(path string, on bool) error {
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if !on {
			return nil
		}
	} else if err != nil {
		return err
	}
	text := string(src)
	before, after, ok := cut(text)
	switch {
	case on == ok:
		return nil
	case on:
		if text != "" {
			text = strings.TrimSuffix(text, "\n") + "\n\n"
		}
		text += begin + "\n" + Wrapper + "\n" + end + "\n"
	default:
		// Drop the blank line Set put before the block, too.
		if strings.HasSuffix(before, "\n\n") {
			before = before[:len(before)-1]
		}
		text = before + after
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	return os.WriteFile(path, []byte(text), mode)
}

// cut splits text around the marked block, dropping the block and its
// line ending.
func cut(text string) (before, after string, ok bool) {
	i := strings.Index(text, begin)
	if i < 0 {
		return text, "", false
	}
	j := strings.Index(text[i:], end)
	if j < 0 {
		return text, "", false
	}
	j += i + len(end)
	if j < len(text) && text[j] == '\n' {
		j++
	}
	return text[:i], text[j:], true
}
