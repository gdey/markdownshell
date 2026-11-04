//go:build unix

package writable

import (
	"errors"
	"syscall"
)

func isROFS(err error) bool { return errors.Is(err, syscall.EROFS) }
