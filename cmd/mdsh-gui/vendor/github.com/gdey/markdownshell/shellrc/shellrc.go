// Package shellrc adds and removes the block in the user's .zshrc and
// .bashrc that defines the mdsh function, which passes the calling
// shell's aliases and functions to blocks, and loads mdsh's completion.
package shellrc

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
)

// RC is one shell's startup file and the lines that go in it.
type RC struct {
	// Wrapper defines the mdsh function. The function keyword keeps an
	// alias named mdsh from breaking the definition.
	Wrapper string
	// Completion loads mdsh's completion for the shell.
	Completion string
	name       string // the file in the home folder
	dirEnv     string // a variable naming another folder for it, or ""
}

var (
	// Zsh is ~/.zshrc, or $ZDOTDIR/.zshrc. Its completion line waits for
	// compinit, as has run by the end of most .zshrc files; without it,
	// compdef is missing and the line does nothing.
	Zsh = RC{
		Wrapper:    `function mdsh { command mdsh --source <(alias -L; typeset -f) "$@"; }`,
		Completion: `(( $+functions[compdef] )) && source <(command mdsh completion zsh)`,
		name:       ".zshrc",
		dirEnv:     "ZDOTDIR",
	}
	// Bash is ~/.bashrc. Its wrapper leaves out its own function and
	// the completion's: their bash-only syntax would stop a /bin/sh that
	// is dash from sourcing the rest. Its completion line only runs in an
	// interactive shell, where complete does something.
	Bash = RC{
		Wrapper:    `function mdsh { command mdsh --source <(unset -f mdsh _mdsh; alias -p; declare -f) "$@"; }`,
		Completion: `[[ $- == *i* ]] && source <(command mdsh completion bash)`,
		name:       ".bashrc",
	}
)

// block is the marked block, with its line ending.
func (r RC) block() string {
	return begin + "\n" + r.Wrapper + "\n" + r.Completion + "\n" + end + "\n"
}

// Path is the file the shell reads.
func (r RC) Path() (string, error) {
	if r.dirEnv != "" {
		if dir := os.Getenv(r.dirEnv); dir != "" {
			return filepath.Join(dir, r.name), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, r.name), nil
}

// Installed reports whether the file at path has the block. A missing
// file doesn't.
func (r RC) Installed(path string) (bool, error) {
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

// Set adds the block to the end of the file at path, creating the file
// if need be, or removes it, leaving the rest of the file as it was. A
// block from an older mdsh-gui is brought up to date in place.
func (r RC) Set(path string, on bool) error {
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
	block := r.block()
	switch {
	case !on && !ok:
		return nil
	case on && ok:
		if strings.Contains(text, block) {
			return nil
		}
		text = before + block + after
	case on:
		if text != "" {
			text = strings.TrimSuffix(text, "\n") + "\n\n"
		}
		text += block
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

// Update brings the block in the file at path up to date, if it has one,
// so a file set up by an older mdsh-gui gets the completion line.
func (r RC) Update(path string) error {
	on, err := r.Installed(path)
	if err != nil || !on {
		return err
	}
	return r.Set(path, true)
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
