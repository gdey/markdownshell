//go:build !windows

package ctl

import (
	"net"
	"os"
	"path/filepath"
)

// listen makes a directory only the current user can open, mode 0700,
// and listens on a Unix socket in it.
func listen() (ln net.Listener, dir, path string, err error) {
	// The temp dir's own name keeps the socket's path short: macOS
	// allows 104 bytes.
	dir, err = os.MkdirTemp("", "mdsh-")
	if err != nil {
		return nil, "", "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		os.RemoveAll(dir)
		return nil, "", "", err
	}
	path = filepath.Join(dir, "ctl.sock")
	ln, err = net.Listen("unix", path)
	if err != nil {
		os.RemoveAll(dir)
		return nil, "", "", err
	}
	// The directory already keeps others out; the socket's mode says
	// so too.
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		os.RemoveAll(dir)
		return nil, "", "", err
	}
	return ln, dir, path, nil
}

func dial(path string) (net.Conn, error) { return net.Dial("unix", path) }
