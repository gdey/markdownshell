//go:build !linux && !darwin

package ctl

import "net"

// sameUser can't ask the system who is calling here, so it relies on
// the socket's directory, which only the current user can open.
func sameUser(net.Conn) bool { return true }
