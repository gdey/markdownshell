//go:build !unix

package env

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

var errNoTTY = errors.New("blocks marked tty need a unix terminal")

// RunTTY is not supported here.
func (s *Shell) RunTTY(context.Context, string, *os.File, io.Writer) (int, bool, error) {
	return 0, false, errNoTTY
}

// RunTerminal is not supported here.
func (s *Shell) RunTerminal(context.Context, string, Terminal, io.Writer) (int, bool, error) {
	return 0, false, errNoTTY
}

// RunScripted is not supported here.
func (s *Shell) RunScripted(context.Context, string, int, int, io.Writer, func(context.Context, io.Writer) error) (int, bool, error) {
	return 0, false, errNoTTY
}

// SetSize does nothing here: there is no pseudo-terminal to size.
func (s *Shell) SetSize(int, int) {}

type ptyConn struct{}

func openPTY(*exec.Cmd) (*ptyConn, error) { return nil, errNoTTY }
func (*ptyConn) started(string)           {}
func (*ptyConn) wait(int)                 {}
func (*ptyConn) drain(string, int)        {}
func (*ptyConn) close()                   {}
