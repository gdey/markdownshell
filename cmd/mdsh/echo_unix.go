//go:build linux || darwin

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// noEcho stops f, a terminal, showing what is typed, for a secret
// answer, and returns a function that turns it back on. On anything
// but a terminal it does nothing.
func noEcho(f *os.File) (restore func()) {
	if f == nil {
		return func() {}
	}
	fd := int(f.Fd())
	old, err := unix.IoctlGetTermios(fd, getTermios)
	if err != nil {
		return func() {}
	}
	t := *old
	t.Lflag &^= unix.ECHO
	t.Lflag |= unix.ICANON | unix.ISIG
	if unix.IoctlSetTermios(fd, setTermios, &t) != nil {
		return func() {}
	}
	return func() { unix.IoctlSetTermios(fd, setTermios, old) }
}
