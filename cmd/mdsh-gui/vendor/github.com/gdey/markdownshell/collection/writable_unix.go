//go:build unix

package collection

import "golang.org/x/sys/unix"

// canWrite reports whether this process may write to path.
func canWrite(path string) bool { return unix.Access(path, unix.W_OK) == nil }
