//go:build !windows

package run_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/ctl"
)

// clientName is the name the test binary answers to as a stand-in for
// mdsh ctl ask and confirm, so blocks under test can call the socket.
const clientName = "ctl-client"

// TestMain runs the test binary as a small ctl client when it is
// called as clientName: ask|confirm QUESTION [choices]. It prints the
// answer, and exits with the call's code.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == clientName {
		os.Exit(client(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func client(args []string) int {
	a := ctl.AskArgs{}
	cmd, rest := args[0], args[1:]
	for len(rest) > 1 && strings.HasPrefix(rest[0], "--") {
		switch rest[0] {
		case "--choice":
			a.Choices = strings.Split(rest[1], ",")
		case "--default":
			a.Default = &rest[1]
		}
		rest = rest[2:]
	}
	a.Question = strings.Join(rest, " ")
	resp, err := ctl.Call(os.Getenv(ctl.SockVar), os.Getenv(ctl.TokenVar), cmd, a)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ctl.NotRunning
	}
	if !resp.OK {
		if resp.Error != "" {
			fmt.Fprintln(os.Stderr, resp.Error)
		}
		return resp.Code
	}
	if cmd == "ask" {
		fmt.Println(resp.Value)
	}
	return ctl.OK
}

// ctlClient puts the test binary on disk as clientName, and returns
// its path, quoted for the shell.
func ctlClient(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), clientName)
	if err := os.Symlink(exe, path); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(path)
	return string(data)
}
