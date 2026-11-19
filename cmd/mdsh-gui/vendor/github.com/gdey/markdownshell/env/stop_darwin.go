package env

import "golang.org/x/sys/unix"

// sstop is the P_stat of a stopped process, SSTOP in <sys/proc.h>.
const sstop = 4

// isStopped reports whether process pid is stopped, as by SIGSTOP or
// SIGTSTP.
func isStopped(pid int) bool {
	p, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && p.Proc.P_stat == sstop
}
