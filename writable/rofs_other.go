//go:build !unix

package writable

func isROFS(error) bool { return false }
