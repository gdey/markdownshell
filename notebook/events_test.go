package notebook

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// keepSrc sets a variable, a function and an alias, and the block after
// it shows whether they are still there.
const keepSrc = "```sh {#setup}\nV=kept\nf() { echo fn; }\nalias a='echo alias'\n```\n\n```sh {#show}\necho \"$V ${1-}\"; f\n```\n\n```sh {#bye}\nexit 4\n```\n\n```sh {#slow timeout=50ms}\nsleep 5\n```\n"

// openLogged opens src with an event log, and returns the log too.
func openLogged(t *testing.T, src string) (*Notebook, *EventLog) {
	t.Helper()
	l := NewEventLog(nil)
	n, err := Open(write(t, src), Options{Events: l})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	return n, l
}

// shown runs block 1, show, and returns what it printed.
func shown(t *testing.T, n *Notebook) string {
	t.Helper()
	if err := n.RunBlock(1); err != nil {
		t.Fatal(err)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return string(n.results[1].Stdout)
}

// events lists l's events as "name" or "name lost", in order.
func events(l *EventLog) []string {
	var got []string
	for _, e := range l.List() {
		s := e.Event
		if e.Lost {
			s += " lost"
		}
		got = append(got, s)
	}
	return got
}

func TestSetRunOptionsKeepsShells(t *testing.T) {
	n, l := openLogged(t, keepSrc)
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	ro := n.RunOptions()
	ro.Step, ro.StopOnError, ro.Only = true, true, "show"
	ro.Args = []string{"one"}
	waiting, err := n.SetRunOptions(ro)
	if err != nil || waiting {
		t.Fatalf("SetRunOptions = %v, %v; want nothing waiting on a restart", waiting, err)
	}
	if got := shown(t, n); got != "kept one\nfn\n" {
		t.Errorf("after new arguments, show printed %q, want what setup set and $1 one", got)
	}
	ro.Shell = "bash"
	if waiting, err = n.SetRunOptions(ro); err != nil || !waiting {
		t.Fatalf("SetRunOptions with a new shell = %v, %v; want it waiting on Restart Shells", waiting, err)
	}
	if got := shown(t, n); got != "kept one\nfn\n" {
		t.Errorf("after a new shell, show printed %q; the shells should stay until Restart Shells", got)
	}
	got := strings.Join(events(l), " ")
	if strings.Contains(got, "session.end") || !strings.Contains(got, "args") || !strings.Contains(got, "restart.needed") {
		t.Errorf("events = %s, want args and restart.needed, and no session.end", got)
	}
}

func TestReloadKeepsShells(t *testing.T) {
	n, l := openLogged(t, keepSrc)
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	// Changed on disk: a note added at the top.
	if err := os.WriteFile(n.File(), []byte("Note.\n\n"+keepSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	waiting, err := n.Reload()
	if err != nil || waiting {
		t.Fatalf("Reload = %v, %v; want nothing waiting", waiting, err)
	}
	if n.Changed() || n.Dirty() {
		t.Errorf("after Reload, Changed = %v and Dirty = %v, want both false", n.Changed(), n.Dirty())
	}
	if c := n.Cells(); c[1].Ran {
		t.Errorf("Reload should drop the output not saved: %+v", c[1])
	}
	if got := shown(t, n); got != "kept \nfn\n" {
		t.Errorf("after Reload, show printed %q, want what setup set", got)
	}

	// A new shell in the front matter waits for Restart Shells.
	if err := os.WriteFile(n.File(), []byte("---\nmdsh:\n  shell: bash\n---\n"+keepSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if waiting, err := n.Reload(); err != nil || !waiting {
		t.Fatalf("Reload with a new shell = %v, %v; want it waiting on Restart Shells", waiting, err)
	}
	if got := shown(t, n); got != "kept \nfn\n" {
		t.Errorf("after Reload with a new shell, show printed %q, want what setup set", got)
	}
	got := strings.Join(events(l), " ")
	if !strings.Contains(got, "file.reload") || strings.Contains(got, "session.end") || !strings.Contains(got, "restart.needed") {
		t.Errorf("events = %s, want file.reload, restart.needed and no session.end", got)
	}
}

func TestClearOutputsKeepsShells(t *testing.T) {
	n, _ := openLogged(t, keepSrc)
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if err := n.ClearOutputs(); err != nil {
		t.Fatal(err)
	}
	if c := n.Cells(); c[0].Ran {
		t.Errorf("ClearOutputs should forget what ran: %+v", c[0])
	}
	if got := shown(t, n); got != "kept \nfn\n" {
		t.Errorf("after ClearOutputs, show printed %q, want what setup set", got)
	}
}

func TestStopKeepsShells(t *testing.T) {
	n, l := openLogged(t, keepSrc)
	for _, i := range []int{0, 3} {
		if err := n.RunBlock(i); err != nil {
			t.Fatal(err)
		}
	}
	// slow timed out, and was interrupted, not its shell.
	if got := shown(t, n); got != "kept \nfn\n" {
		t.Errorf("after a timeout, show printed %q, want what setup set", got)
	}
	// Stop interrupts too.
	go func() {
		time.Sleep(300 * time.Millisecond)
		n.Stop()
	}()
	n.mu.Lock()
	n.d.Blocks[3].Attrs["timeout"] = "1m"
	n.mu.Unlock()
	if err := n.RunBlock(3); err != nil {
		t.Fatal(err)
	}
	if got := shown(t, n); got != "kept \nfn\n" {
		t.Errorf("after Stop, show printed %q, want what setup set", got)
	}
	for _, e := range l.List() {
		if e.Event == "env.end" || e.Lost {
			t.Errorf("a timeout and a stop should lose nothing: %+v", e)
		}
	}
}

func TestKillEndsTheShell(t *testing.T) {
	n, l := openLogged(t, "```sh {#setup}\nV=kept\n```\n\n```sh {#show}\necho \"$V\"\n```\n\n```sh {#stuck}\ntrap '' INT\nwhile :; do :; done\n```\n")
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(300 * time.Millisecond)
		n.Stop()
		time.Sleep(time.Second)
		if !n.Kill() {
			t.Error("Kill found no block running")
		}
	}()
	if err := n.RunBlock(2); err != nil {
		t.Fatal(err)
	}
	if got := shown(t, n); got != "\n" {
		t.Errorf("after Kill, show printed %q, want a new shell", got)
	}
	var lost []string
	for _, e := range l.List() {
		if e.Lost {
			lost = append(lost, e.Text)
		}
	}
	if len(lost) != 1 || !strings.Contains(lost[0], "You killed the shell") {
		t.Errorf("lost events = %q, want the kill", lost)
	}
}

func TestEventsShellEnds(t *testing.T) {
	n, l := openLogged(t, keepSrc)
	for _, i := range []int{0, 2, 0} {
		if err := n.RunBlock(i); err != nil {
			t.Fatal(err)
		}
	}
	var ends []string
	again := 0
	for _, e := range l.List() {
		switch e.Event {
		case "env.end":
			if !e.Lost {
				t.Errorf("env.end not marked lost: %+v", e)
			}
			ends = append(ends, e.Text)
		case "env.start":
			if strings.Contains(e.JSON, `"again":true`) {
				again++
			}
		}
	}
	if len(ends) != 1 || !strings.Contains(ends[0], "bye ended the shell") || !strings.Contains(ends[0], "status 4") {
		t.Errorf("env.end texts = %q, want bye's exit", ends)
	}
	if again != 1 {
		t.Errorf("env.start again = %d, want 1: the shell after bye's exit", again)
	}
	if err := n.Restart(); err != nil {
		t.Fatal(err)
	}
	got := events(l)
	if tail := strings.Join(got[len(got)-2:], " "); tail != "session.end session.start lost" {
		t.Errorf("Restart logged %s, want session.end then session.start, lost", tail)
	}
}

func TestEventLogNotifiesAndKeeps(t *testing.T) {
	var mu sync.Mutex
	var seqs []int
	l := NewEventLog(func(e AuditEvent) {
		mu.Lock()
		seqs = append(seqs, e.Seq)
		mu.Unlock()
	})
	for range eventsMax + 5 {
		l.Event("file.changed", nil)
	}
	list := l.List()
	if len(list) != eventsMax || list[0].Seq != 6 {
		t.Errorf("kept %d events from seq %d, want %d from 6", len(list), list[0].Seq, eventsMax)
	}
	if len(seqs) != eventsMax+5 {
		t.Errorf("notified %d times, want %d", len(seqs), eventsMax+5)
	}
	if e := list[0]; e.Text != "The file changed on disk." || !strings.HasPrefix(e.JSON, `{"v":1,"seq":6,`) {
		t.Errorf("event = %+v", e)
	}
	var nilLog *EventLog
	nilLog.Event("x", nil)
	if nilLog.List() != nil || nilLog.Audit() != nil {
		t.Error("a nil log should log nothing")
	}
}

func TestDescribeUnknown(t *testing.T) {
	text, lost := Describe("new.thing", map[string]any{"b": 2.0, "a": "x"})
	if text != "new.thing a=x b=2" || lost {
		t.Errorf("Describe = %q, %v", text, lost)
	}
}
