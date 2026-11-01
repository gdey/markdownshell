package ctl

import (
	"errors"
	"net"
)

// ErrUnsupported is Listen's error where mdsh has no control socket yet.
var ErrUnsupported = errors.New("the control socket is not supported on Windows yet")

// listen will make a named pipe only the current user can open. Until
// then, blocks on Windows run without $MDSH_SOCK, and mdsh ctl exits 3.
func listen() (net.Listener, string, string, error) { return nil, "", "", ErrUnsupported }

func dial(string) (net.Conn, error) { return nil, ErrUnsupported }
