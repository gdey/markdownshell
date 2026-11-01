//go:build !windows

package run_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/ctl"
	"github.com/gdey/markdownshell/run"
)

// TestCtlBackground checks that a background block's token works while
// it runs and stops when it ends, that a set reaches blocks that start
// later and the conditions before them, and that Close removes the
// socket.
func TestCtlBackground(t *testing.T) {
	dir := t.TempDir()
	d := parse(t, "```sh {#bg background}\nprintf '%s\\n%s\\n' \"$MDSH_SOCK\" \"$MDSH_TOKEN\" > call.tmp && mv call.tmp call\nwhile [ ! -e go ]; do sleep 0.02; done\n```\n\n"+
		"```sh {#later run={{$MDSH_VAR_who}}==bg}\necho \"later: $MDSH_VAR_who\"\n```\n")
	var statuses []string
	s, err := run.NewSession(d, run.Options{Dir: dir, Environ: []string{}, OnEvent: func(e run.Event) {
		if e.Kind == run.Status {
			statuses = append(statuses, e.Block.ID+":"+e.Progress.Text)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.RunBlock(ctx, 0); err != nil {
		t.Fatal(err)
	}
	var sock, token string
	for deadline := time.Now().Add(5 * time.Second); ; {
		data, err := os.ReadFile(filepath.Join(dir, "call"))
		if err == nil {
			sock, token, _ = strings.Cut(strings.TrimSpace(string(data)), "\n")
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background block never wrote its token")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if resp, err := ctl.Call(sock, token, "set", ctl.SetArgs{Name: "who", Value: "bg"}); err != nil || !resp.OK {
		t.Fatalf("set: %+v, %v", resp, err)
	}
	if resp, err := ctl.Call(sock, token, "status", ctl.StatusArgs{Text: "half"}); err != nil || !resp.OK {
		t.Fatalf("status: %+v, %v", resp, err)
	}
	os.WriteFile(filepath.Join(dir, "go"), nil, 0o644)
	if err := s.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if resp, err := ctl.Call(sock, token, "get", ctl.NameArgs{Name: "who"}); err != nil || resp.OK || resp.Code != ctl.Refused {
		t.Errorf("token after its block ended: %+v, %v", resp, err)
	}
	if err := s.RunFrom(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if r := s.Results()[1]; !r.Ran || string(r.Stdout) != "later: bg\n" {
		t.Errorf("later: ran %v, stdout %q", r.Ran, r.Stdout)
	}
	if len(statuses) != 1 || statuses[0] != "bg:half" {
		t.Errorf("statuses = %q", statuses)
	}
	s.Close()
	if _, err := os.Stat(filepath.Dir(sock)); !os.IsNotExist(err) {
		t.Errorf("socket dir left after Close: %v", err)
	}
}
