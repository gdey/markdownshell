//go:build linux

package main

import (
	"os"
	"path/filepath"
)

// newCLIHost finds the mdsh inside a running AppImage, to copy to
// ~/.local/bin. APPIMAGE names the image and APPDIR where it's mounted;
// outside an AppImage there's no bundled mdsh, since a .deb or .rpm
// installs it with the package.
func newCLIHost() *cliHost {
	h := &cliHost{copy: true, version: thisBuild().Version, pathDirs: envPathDirs()}
	if home, err := os.UserHomeDir(); err == nil {
		h.link = filepath.Join(home, ".local", "bin", "mdsh")
	}
	dir := os.Getenv("APPDIR")
	if dir == "" || os.Getenv("APPIMAGE") == "" || h.link == "" {
		return h
	}
	p := filepath.Join(dir, "usr", "bin", "mdsh")
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		h.bundled = p
	}
	return h
}
