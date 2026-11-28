//go:build unix

package env

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestInterruptsWhenIgnored checks that stopping a block works when
// mdsh was started with SIGINT ignored, as a command run with & from a
// script is: the shells mdsh starts mustn't inherit that. The test
// runs TestKeepInterrupts again in a copy of itself started so.
func TestInterruptsWhenIgnored(t *testing.T) {
	if os.Getenv("MDSH_TEST_SIGINT_IGNORED") != "" {
		t.Skip("already the copy")
	}
	if testing.Short() {
		t.Skip("runs TestKeepInterrupts again")
	}
	cmd := exec.Command("sh", "-c", `trap '' INT; exec "$0" -test.run '^TestKeepInterrupts$' -test.timeout 60s -test.v`, os.Args[0])
	cmd.Env = append(os.Environ(), "MDSH_TEST_SIGINT_IGNORED=1")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "\nPASS\n") {
		t.Fatalf("TestKeepInterrupts with SIGINT ignored: %v\n%s", err, out)
	}
}
