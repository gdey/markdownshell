package env

import (
	"os"
	"strconv"
	"strings"
)

// isStopped reports whether process pid is stopped, as by SIGSTOP or
// SIGTSTP.
func isStopped(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// The state follows the command, which is in parentheses and may
	// hold spaces or parentheses itself.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	return i >= 0 && strings.HasPrefix(s[i+1:], " T")
}
