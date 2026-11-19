//go:build unix && !linux && !darwin

package env

// isStopped can't tell here without running ps, whose own exit would
// wake watchStops again.
func isStopped(int) bool { return false }
