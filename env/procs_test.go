//go:build unix

package env

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseEtime(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"00:05":       5 * time.Second,
		"01:02:03":    time.Hour + 2*time.Minute + 3*time.Second,
		"2-00:00:01":  48*time.Hour + time.Second,
		"10-10:10:10": 250*time.Hour + 10*time.Minute + 10*time.Second,
	} {
		if got, ok := parseEtime(in); !ok || got != want {
			t.Errorf("parseEtime(%q) = %s, %v, want %s", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "5", "a:b", "x-00:01"} {
		if _, ok := parseEtime(bad); ok {
			t.Errorf("parseEtime(%q) ok", bad)
		}
	}
}

// Processes shows what a block left running under its shell, and
// KillProcess ends it, but not a process outside the shells.
func TestProcessesAndKill(t *testing.T) {
	s := NewStack(Config{Dir: t.TempDir(), Env: []string{"PATH=" + os.Getenv("PATH")}})
	defer s.Close()
	e, err := s.Get(context.Background(), TopLevel, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Shell.Run(context.Background(), "sleep 60 &\n", nil, nil); err != nil {
		t.Fatal(err)
	}
	find := func() *Proc {
		for _, sh := range s.Processes() {
			for _, p := range sh.Children {
				if strings.HasPrefix(p.Command, "sleep 60") {
					return p
				}
			}
		}
		return nil
	}
	p := find()
	if p == nil {
		t.Fatalf("sleep isn't under the shell: %+v", s.Processes())
	}
	if s.KillProcess(e.Shell.PID()) {
		t.Error("KillProcess killed the shell itself")
	}
	if s.KillProcess(1) {
		t.Error("KillProcess killed a process outside the shells")
	}
	if !s.KillProcess(p.PID) {
		t.Fatal("KillProcess = false")
	}
	deadline := time.Now().Add(5 * time.Second)
	for find() != nil {
		if time.Now().After(deadline) {
			t.Fatal("sleep still runs")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
