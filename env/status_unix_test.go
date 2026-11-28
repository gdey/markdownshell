//go:build unix

package env

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gdey/markdownshell/termstatus"
)

// report is a script line that writes an OSC 7501 report.
func report(body string) string { return `printf '\033]7501;` + body + `\033\\'` + "\n" }

// recorded keeps the records a Terminal's Status was called with.
type recorded struct {
	mu    sync.Mutex
	calls [][]termstatus.Report
}

func (r *recorded) status(recs []termstatus.Report) {
	r.mu.Lock()
	r.calls = append(r.calls, recs)
	r.mu.Unlock()
}

func (r *recorded) states() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, call := range r.calls {
		if call == nil {
			out = append(out, "gone")
			continue
		}
		var s []string
		for _, rec := range call {
			s = append(s, rec.ID+"="+string(rec.State))
		}
		out = append(out, strings.Join(s, ","))
	}
	return strings.Join(out, " ")
}

// TestRunTerminalStatus: a program on a tty block's terminal reports
// what it is doing; the terminal's Status follows, and says when the
// terminal goes. The feature query is answered on the program's input.
func TestRunTerminalStatus(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	var rec recorded
	var shown syncBuf
	script := report("state=working:progress=10") +
		report("state=working:progress=10") + // the same again: no change
		report("state=blocked:kind=permission:id=sub") +
		report("state=done:id=sub") +
		report("?") + "stty raw -echo; dd bs=1 count=80 2>/dev/null | od -c | head -1; stty sane\n" +
		report("state=garbage") +
		"echo end\n"
	status, _, err := sh.RunTerminal(context.Background(), script, Terminal{Out: &shown, Status: rec.status}, io.Discard)
	if err != nil || status != 0 {
		t.Fatalf("status %d, err %v; shown %q", status, err, shown.String())
	}
	want := "=working =working,sub=blocked =working,sub=done gone"
	if got := rec.states(); got != want {
		t.Errorf("status calls %q, want %q", got, want)
	}
	// The reply to ? went to the script, as a terminal's would, and
	// the stream the terminal shows kept every byte.
	if s := shown.String(); !strings.Contains(s, "7   5   0   1   ;   ?") || !strings.Contains(s, "\x1b]7501;state=working:progress=10\x1b\\") {
		t.Errorf("shown %q", s)
	}
	// Without Status, nothing is scanned or answered, and a program
	// that asks gets no reply.
	var plain syncBuf
	status, _, err = sh.RunTerminal(context.Background(), report("state=working")+"echo fine\n", Terminal{Out: &plain}, io.Discard)
	if err != nil || status != 0 || !strings.Contains(plain.String(), "fine") {
		t.Errorf("without Status: %d %v %q", status, err, plain.String())
	}
}

// TestBackgroundTerminalStatus: the same for a background tty block,
// whose terminal goes once its job has ended.
func TestBackgroundTerminalStatus(t *testing.T) {
	dir := t.TempDir()
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir}, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	var rec recorded
	var out syncBuf
	script := report("state=working:app=make") + report("state=error:msg="+"ZmFpbGVk") + "echo end\n"
	j, err := sh.BackgroundTerminal(context.Background(), script, "", Terminal{Out: &out, Status: rec.status}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := waitJob(t, j); err != nil {
		t.Fatal(err)
	}
	<-j.Ended()
	if got, want := rec.states(), "=working =error gone"; got != want {
		t.Errorf("status calls %q, want %q", got, want)
	}
	rec.mu.Lock()
	last := rec.calls[1]
	rec.mu.Unlock()
	if len(last) != 1 || last[0].Msg != "failed" || last[0].App != "" {
		t.Errorf("error record %+v", last)
	}
}

// TestInteractiveStatus: a command typed in the History shell reports;
// the shell's "status" events carry the records, and the command's end
// drops its working and blocked ones.
func TestInteractiveStatus(t *testing.T) {
	for _, shell := range shells(t) {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			s := startI(t, shell, nil)
			var mu sync.Mutex
			var got []string
			s.Watch(func(e Event) {
				if e.Kind != "status" {
					return
				}
				var recs []string
				for _, r := range e.Records {
					recs = append(recs, r.ID+"="+string(r.State))
				}
				mu.Lock()
				got = append(got, strings.Join(recs, ","))
				mu.Unlock()
			})
			runI(t, s, report("state=working:progress=50")+report("state=done:id=part")+"echo ok")
			mu.Lock()
			g := strings.Join(got, " ")
			mu.Unlock()
			if want := "=working =working,part=done part=done"; g != want {
				t.Errorf("status events %q, want %q", g, want)
			}
		})
	}
}

// TestCopy checks that a program's OSC 52 copy reaches a Terminal's
// Clipboard, with no Status set, and the interactive shell's "copy"
// event.
func TestCopy(t *testing.T) {
	sh, err := Start(Config{Env: []string{"PATH=" + os.Getenv("PATH")}, TTY: true})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	var mu sync.Mutex
	var got []string
	copied := func(s string) { mu.Lock(); got = append(got, s); mu.Unlock() }
	var shown syncBuf
	script := "printf '\\033]52;c;aGkK\\007'; printf '\\033]52;c;?\\007'; echo end\n"
	if status, _, err := sh.RunTerminal(context.Background(), script, Terminal{Out: &shown, Clipboard: copied}, io.Discard); err != nil || status != 0 {
		t.Fatalf("status %d, err %v", status, err)
	}
	mu.Lock()
	if len(got) != 1 || got[0] != "hi\n" {
		t.Errorf("copied %q, want hi", got)
	}
	got = nil
	mu.Unlock()
	if !strings.Contains(shown.String(), "\x1b]52;c;aGkK\a") {
		t.Errorf("the stream lost the OSC 52: %q", shown.String())
	}
	for _, shell := range shells(t) {
		t.Run(filepath.Base(shell), func(t *testing.T) {
			s := startI(t, shell, nil)
			var cp []string
			s.Watch(func(e Event) {
				if e.Kind == "copy" {
					mu.Lock()
					cp = append(cp, e.Copied)
					mu.Unlock()
				}
			})
			runI(t, s, "printf '\\033]52;c;aGkK\\007'")
			mu.Lock()
			defer mu.Unlock()
			if len(cp) != 1 || cp[0] != "hi\n" {
				t.Errorf("copy events %q", cp)
			}
		})
	}
}
