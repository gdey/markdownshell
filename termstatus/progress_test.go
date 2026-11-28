package termstatus

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProgressSequence(t *testing.T) {
	for _, tc := range []struct {
		r    Report
		want string
	}{
		{Report{State: Working, Progress: 40}, "\x1b]9;4;1;40\x1b\\"},
		{Report{State: Working, Progress: 140}, "\x1b]9;4;1;100\x1b\\"},
		{Report{State: Working, Progress: NoProgress}, "\x1b]9;4;3;0\x1b\\"},
		{Report{State: Blocked, Progress: 60}, "\x1b]9;4;4;60\x1b\\"},
		{Report{State: Blocked, Progress: NoProgress}, "\x1b]9;4;4;0\x1b\\"},
		{Report{State: Error}, "\x1b]9;4;2;100\x1b\\"},
		{Report{State: Done}, "\x1b]9;4;0;0\x1b\\"},
		{Report{State: Idle}, "\x1b]9;4;0;0\x1b\\"},
		{Report{State: Clear}, "\x1b]9;4;0;0\x1b\\"},
	} {
		if got := ProgressSequence(tc.r); got != tc.want {
			t.Errorf("%+v: got %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestProgressTerminal(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{"TERM_PROGRAM": "ghostty"}, true},
		{map[string]string{"TERM": "xterm-ghostty"}, true},
		{map[string]string{"TERM_PROGRAM": "vscode"}, true},
		{map[string]string{"WT_SESSION": "x"}, true},
		{map[string]string{"ConEmuPID": "12"}, true},
		{map[string]string{"TERM_PROGRAM": "tmux", "WT_SESSION": "x"}, false},
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, false},
		{map[string]string{"TERM_PROGRAM": "WezTerm"}, false},
		{map[string]string{"TERM": "xterm-kitty"}, false},
		{nil, false},
	} {
		if got := ProgressTerminal(func(k string) string { return tc.env[k] }); got != tc.want {
			t.Errorf("%v: got %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestTitleText(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Report
		want string
	}{
		{"deploy.md", Report{}, "deploy.md - mdsh"},
		{"", Report{State: Idle}, "mdsh"},
		{"deploy.md", Report{State: Working, Progress: 42}, "[42%] deploy.md - mdsh"},
		{"deploy.md", Report{State: Working, Progress: NoProgress}, "[running] deploy.md - mdsh"},
		{"deploy.md", Report{State: Blocked, Progress: 42}, "[waiting] deploy.md - mdsh"},
		{"deploy.md", Report{State: Error}, "[failed] deploy.md - mdsh"},
		{"deploy.md", Report{State: Done}, "[done] deploy.md - mdsh"},
	} {
		if got := TitleText("mdsh", tc.name, tc.r); got != tc.want {
			t.Errorf("%q %+v: got %q, want %q", tc.name, tc.r, got, tc.want)
		}
	}
}

func TestTitleSequenceIsOneLine(t *testing.T) {
	got := TitleSequence("a\x1b]0;evil\x07b\nc")
	if want := "\x1b]0;a ]0;evil b c\x1b\\"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestTrackerProgressAndTitle follows mdsh -i through a run: the bar and
// title move with the OSC 7501 state, nothing is sent twice, and Clear
// hides the bar and gives the terminal its title back.
func TestTrackerProgressAndTitle(t *testing.T) {
	var b bytes.Buffer
	tr := NewTracker(&b, "mdsh").WithProgress(true)
	tr.Title("deploy.md")
	tr.Idle()
	tr.Working(NoProgress, "build")
	tr.Working(50, "build: half")
	tr.Title("deploy.md") // every key does this; nothing changed
	tr.Blocked(Permission, "Run deploy?")
	tr.Unblocked()
	tr.Error("deploy failed")
	tr.Idle()
	tr.Clear()

	want := []string{
		pushTitle,
		TitleSequence("deploy.md - mdsh"),
		Report{State: Idle, App: "mdsh"}.Sequence(),
		// The bar isn't hidden before it was ever shown.
		Report{State: Working, App: "mdsh", Progress: NoProgress, Msg: "build"}.Sequence(),
		"\x1b]9;4;3;0\x1b\\",
		TitleSequence("[running] deploy.md - mdsh"),
		Report{State: Working, App: "mdsh", Progress: 50, Msg: "build: half"}.Sequence(),
		"\x1b]9;4;1;50\x1b\\",
		TitleSequence("[50%] deploy.md - mdsh"),
		Report{State: Blocked, App: "mdsh", Kind: Permission, Progress: 50, Msg: "Run deploy?"}.Sequence(),
		"\x1b]9;4;4;50\x1b\\",
		TitleSequence("[waiting] deploy.md - mdsh"),
		Report{State: Working, App: "mdsh", Progress: 50, Msg: "build: half"}.Sequence(),
		"\x1b]9;4;1;50\x1b\\",
		TitleSequence("[50%] deploy.md - mdsh"),
		Report{State: Error, App: "mdsh", Msg: "deploy failed"}.Sequence(),
		"\x1b]9;4;2;100\x1b\\",
		TitleSequence("[failed] deploy.md - mdsh"),
		Report{State: Idle, App: "mdsh"}.Sequence(),
		"\x1b]9;4;0;0\x1b\\",
		TitleSequence("deploy.md - mdsh"),
		Report{State: Clear, App: "mdsh"}.Sequence(),
		popTitle,
	}
	if got := b.String(); got != strings.Join(want, "") {
		t.Errorf("got\n%q\nwant\n%q", got, strings.Join(want, ""))
	}
}

// TestTrackerWithoutProgress is a terminal that would read OSC 9;4 as a
// notification: only OSC 7501 goes out, and Exit writes nothing.
func TestTrackerWithoutProgress(t *testing.T) {
	var b bytes.Buffer
	tr := NewTracker(&b, "mdsh").WithProgress(false)
	tr.Working(10, "x")
	tr.Error("x failed")
	tr.Exit()
	if got := b.String(); strings.Contains(got, "\x1b]9;") || strings.Contains(got, "\x1b]0;") || strings.Contains(got, "\x1b[2") {
		t.Errorf("got %q", got)
	}
}

// TestTrackerExitHidesBar is the CLI ending: Exit hides a bar still up
// but keeps the OSC 7501 record.
func TestTrackerExitHidesBar(t *testing.T) {
	var b bytes.Buffer
	tr := NewTracker(&b, "mdsh").WithProgress(true)
	tr.Working(10, "x")
	tr.Error("x failed")
	b.Reset()
	tr.Exit()
	if got, want := b.String(), "\x1b]9;4;0;0\x1b\\"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// TestTrackerRefreshesBar: while working the bar is sent again, for
// Ghostty, which hides one it hasn't heard of in 15 seconds; once done
// it isn't.
func TestTrackerRefreshesBar(t *testing.T) {
	var b syncBuffer
	tr := NewTracker(&b, "mdsh").WithProgress(true)
	tr.refreshEvery = time.Millisecond
	tr.Working(30, "x")
	bar := "\x1b]9;4;1;30\x1b\\"
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(b.String(), bar) < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("bar not sent again: %q", b.String())
		}
		time.Sleep(time.Millisecond)
	}
	tr.Done("")
	n := len(b.String())
	time.Sleep(20 * time.Millisecond)
	if got := b.String(); len(got) != n {
		t.Errorf("written after done: %q", got[n:])
	}
}

func TestNilTrackerProgress(t *testing.T) {
	var tr *Tracker
	tr.WithProgress(true).Title("x")
	tr.Exit()
}
