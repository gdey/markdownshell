//go:build darwin

package main

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// newCLIHost finds the mdsh beside mdsh-gui in mdsh.app/Contents/MacOS,
// to link from /usr/local/bin, which /etc/paths puts on every login
// shell's PATH.
func newCLIHost() *cliHost {
	h := &cliHost{
		link:     "/usr/local/bin/mdsh",
		version:  thisBuild().Version,
		pathDirs: darwinPathDirs(),
	}
	exe, err := os.Executable()
	if err != nil {
		return h
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	p := filepath.Join(filepath.Dir(exe), "mdsh")
	if fi, err := os.Stat(p); err != nil || fi.IsDir() || !isAppHelper(p) {
		return h
	}
	h.bundled = p
	app := strings.TrimSuffix(p, "/Contents/MacOS/mdsh")
	h.admin = osascriptAdmin
	h.unquarantine = func() error {
		// A .dmg's download flag is on everything copied out of it, and
		// a quarantined helper is blocked when run from a terminal.
		if exec.Command("xattr", "-p", "com.apple.quarantine", app).Run() != nil {
			return nil
		}
		return exec.Command("xattr", "-dr", "com.apple.quarantine", app).Run()
	}
	return h
}

// osascriptAdmin runs cmd as root through macOS's administrator password
// prompt, as VS Code and Zed do for their command line tools.
func osascriptAdmin(cmd string) error {
	out, err := exec.Command("osascript", "-e", adminScript(cmd)).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(msg, "(-128)") {
		return errCanceled
	}
	if msg == "" {
		return err
	}
	return errors.New(msg)
}

// darwinPathDirs are the folders a login shell in Terminal looks in. An
// app opened from the Finder gets launchd's short PATH, without
// /usr/local/bin or Homebrew's, so these are added in the order a shell
// with Homebrew's shellenv would have them.
func darwinPathDirs() []string {
	dirs := []string{"/opt/homebrew/bin"}
	add := func(file string) {
		f, err := os.Open(file)
		if err != nil {
			return
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			if d := strings.TrimSpace(s.Text()); d != "" && !slices.Contains(dirs, d) {
				dirs = append(dirs, d)
			}
		}
	}
	add("/etc/paths")
	if more, err := filepath.Glob("/etc/paths.d/*"); err == nil {
		for _, f := range more {
			add(f)
		}
	}
	for _, d := range envPathDirs() {
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}
