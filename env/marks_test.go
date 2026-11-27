//go:build unix

package env

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMarkScanner(t *testing.T) {
	in := "a\x1b]133;A;mdsh\x07$ \x1b]133;B;mdsh\x1b\\ls\r\n\x1b[1mx\x1b]133;A\x07\x1b]133;D;148;mdsh\x07z\x1b]133;Q;mdsh\x07"
	want := "T(a)M(A)T($ )M(B)T(ls\r\n)T(\x1b)T([1mx)T(\x1b]133;A\x07)M(D148)T(z)T(\x1b]133;Q;mdsh\x07)"
	// Fed whole, and a byte at a time, the result is the same once
	// adjacent text is joined.
	for _, step := range []int{len(in), 1, 2, 5} {
		var got strings.Builder
		m := markScanner{
			text: func(p []byte) { fmt.Fprintf(&got, "T(%s)", p) },
			mark: func(mk mark) {
				if mk.kind == markDone {
					fmt.Fprintf(&got, "M(D%d)", mk.status)
				} else {
					fmt.Fprintf(&got, "M(%c)", mk.kind)
				}
			},
		}
		for i := 0; i < len(in); i += step {
			m.Write([]byte(in[i:min(i+step, len(in))]))
		}
		if join(got.String()) != join(want) {
			t.Errorf("step %d:\n got %q\nwant %q", step, join(got.String()), join(want))
		}
	}
}

// join merges adjacent T(...) runs.
func join(s string) string { return strings.ReplaceAll(s, ")T(", "") }

func TestParseMark(t *testing.T) {
	for body, want := range map[string]mark{
		"D;0;mdsh":   {markDone, 0},
		"D;mdsh":     {markDone, -1},
		"D;x;mdsh":   {markDone, -1},
		"C;mdsh":     {markCommand, -1},
		"A;mdsh":     {markPrompt, -1},
		"B;mdsh":     {markInput, -1},
		"D;130;mdsh": {markDone, 130},
	} {
		if got, ok := parseMark(body); !ok || got != want {
			t.Errorf("%q: %v %v", body, got, ok)
		}
	}
	for _, body := range []string{"D;0", "A", "Z;mdsh", "AA;mdsh", ""} {
		if _, ok := parseMark(body); ok {
			t.Errorf("%q taken", body)
		}
	}
}

func TestParseJob(t *testing.T) {
	for line, want := range map[string]ShellJob{
		"[1]+   807 Stopped                 sleep 100":    {1, 807, "stopped", "sleep 100"},
		"[2]-   817 Running                 sleep 1 &":    {2, 817, "running", "sleep 1 &"},
		"[1] + 756 Stopped (SIGTSTP)    sleep 100":        {1, 756, "stopped", "sleep 100"},
		"[1]  + 4242 suspended  vim notes.md":             {1, 4242, "stopped", "vim notes.md"},
		"[3]  - 4243 suspended (tty output)  cat >/dev/x": {3, 4243, "stopped", "cat >/dev/x"},
		"[2]+   817 Done                    sleep 1":      {2, 817, "done", "sleep 1"},
		"[4]  + 99 exit 1     false":                      {4, 99, "done", "false"},
		"[1]  + 12 running    make test":                  {1, 12, "running", "make test"},
		"[5]   13 Weird thing":                            {5, 13, "weird", "thing"},
	} {
		got, ok := parseJob(line)
		if !ok || got != want {
			t.Errorf("%q: %+v %v", line, got, ok)
		}
	}
	if _, ok := parseJob("       818                       | cat"); ok {
		t.Error("pipeline continuation taken")
	}
}

func TestReadRecords(t *testing.T) {
	in := `state
status 148
pwd /tmp/a\nb
oldpwd /
pid 42
jobs
job [1]+   807 Stopped                 sleep 100
env HOME=/home/j
env MULTI=x\ny\\z
var x=5
end
cmd
text echo a\nb
end
junk
state
status 0
end
`
	var got []record
	readRecords(strings.NewReader(in), func(r record) { got = append(got, r) })
	if len(got) != 3 {
		t.Fatalf("records: %d", len(got))
	}
	want := State{
		Status: 148, PWD: "/tmp/a\nb", OldPWD: "/", PID: 42,
		Jobs: []ShellJob{{1, 807, "stopped", "sleep 100"}}, JobsKnown: true,
		Exported: map[string]string{"HOME": "/home/j", "MULTI": "x\ny\\z"},
		Vars:     map[string]string{"x": "5"},
	}
	if !reflect.DeepEqual(*got[0].state, want) {
		t.Errorf("state:\n got %+v\nwant %+v", *got[0].state, want)
	}
	if got[1].cmd == nil || *got[1].cmd != "echo a\nb" {
		t.Errorf("cmd: %+v", got[1])
	}
	if got[2].state == nil || got[2].state.Status != 0 || got[2].state.JobsKnown {
		t.Errorf("second state: %+v", got[2])
	}
}

// TestLostMark: a prompt that comes with no D, as when ctrl+c threw
// the rest of a line away, ends the command with status unknown.
func TestLostMark(t *testing.T) {
	ia := &interactive{kind: kindBash, exitStatus: -1}
	ia.cond = newCond(ia)
	var events []string
	ia.watch = func(e Event) { events = append(events, fmt.Sprintf("%s %q %d", e.Kind, e.Command, e.Status)) }
	ia.scan = markScanner{text: ia.text, mark: ia.onMark}
	feed := func(s string) {
		ia.mu.Lock()
		ia.scan.Write([]byte(s))
		ia.mu.Unlock()
	}
	feed("\x1b]133;A;mdsh\x07$ \x1b]133;B;mdsh\x07make\r\n\x1b]133;C;mdsh\x07out\r\n")
	feed("\x1b]133;A;mdsh\x07$ \x1b]133;B;mdsh\x07")
	want := []string{`start "make" 0`, `end "" -1`}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("events: %q", events)
	}
	if ia.phase != phaseInput || !ia.ended || ia.exitStatus != -1 {
		t.Errorf("phase %v ended %v status %d", ia.phase, ia.ended, ia.exitStatus)
	}
}

// TestShNoC: sh has no C mark; the command starts after its echoed line.
func TestShNoC(t *testing.T) {
	ia := &interactive{kind: kindSh, exitStatus: -1}
	ia.cond = newCond(ia)
	var out strings.Builder
	ia.out, ia.typed = &out, true
	ia.scan = markScanner{text: ia.text, mark: ia.onMark}
	ia.mu.Lock()
	ia.scan.Write([]byte("\x1b]133;A;mdsh\x07$ \x1b]133;B;mdsh\x07ech"))
	ia.scan.Write([]byte("o hi\r\nhi\r\n\x1b]133;D;0;mdsh\x07"))
	ia.mu.Unlock()
	if out.String() != "hi\r\n" || !ia.ended || ia.exitStatus != 0 {
		t.Errorf("out %q ended %v status %d", out.String(), ia.ended, ia.exitStatus)
	}
}

func newCond(ia *interactive) *sync.Cond { return sync.NewCond(&ia.mu) }
