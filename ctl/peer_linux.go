package ctl

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// sameUser reports whether the process at the other end of conn runs as
// the current user. The socket's directory already keeps other users
// out; this checks again.
func sameUser(conn net.Conn) bool {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	var cred *unix.Ucred
	var cerr error
	if err := raw.Control(func(fd uintptr) {
		cred, cerr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil || cerr != nil {
		return false
	}
	return int(cred.Uid) == os.Getuid()
}
