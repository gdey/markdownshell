//go:build !unix

package collection

import "os"

// canWrite reports whether path is writable by its permission bits; on
// Windows that is the read-only attribute.
func canWrite(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().Perm()&0o200 != 0
}
