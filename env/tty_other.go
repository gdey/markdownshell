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

type ptyConn struct{}

func openPTY(*exec.Cmd) (*ptyConn, error) { return nil, errNoTTY }
func (*ptyConn) started()                 {}
func (*ptyConn) close()                   {}
