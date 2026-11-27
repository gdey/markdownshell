package termstatus

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestSequence(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Report
		want string
	}{
		{"state only", Report{State: Idle, Progress: NoProgress}, "\x1b]7501;state=idle\x1b\\"},
		{"working with progress", Report{State: Working, Progress: 40, App: "mdsh", Msg: "build"},
			"\x1b]7501;state=working:progress=40:app=mdsh:msg=" + b64("build") + "\x1b\\"},
		{"progress capped", Report{State: Working, Progress: 140}, "\x1b]7501;state=working:progress=100\x1b\\"},
		{"blocked", Report{State: Blocked, Kind: Permission, Progress: NoProgress, Msg: "Run deploy?"},
			"\x1b]7501;state=blocked:kind=permission:msg=" + b64("Run deploy?") + "\x1b\\"},
		{"kind and progress dropped from done", Report{State: Done, Kind: Permission, Progress: 50}, "\x1b]7501;state=done\x1b\\"},
		{"id and title", Report{ID: "build/test", State: Error, Progress: NoProgress, Title: "tests"},
			"\x1b]7501;state=error:id=build/test:title=" + b64("tests") + "\x1b\\"},
		{"control characters become spaces", Report{State: Working, Progress: NoProgress, Msg: "a\x1b[31mb\n\tc\x00"},
			"\x1b]7501;state=working:msg=" + b64("a [31mb c") + "\x1b\\"},
		{"clear", Report{State: Clear}, "\x1b]7501;state=clear\x1b\\"},
		{"no state", Report{Msg: "x"}, ""},
		{"bad id", Report{State: Idle, ID: "a b"}, ""},
		{"bad app", Report{State: Idle, App: "md sh"}, ""},
		{"deep id", Report{State: Idle, ID: strings.Repeat("a/", MaxDepth) + "a"}, ""},
	} {
		if got := tc.r.Sequence(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSequenceCutsToFit(t *testing.T) {
	long := strings.Repeat("é", MaxMsg) // 2 bytes each: twice the limit
	seq := Report{State: Working, Progress: NoProgress, Msg: long, Title: long}.Sequence()
	if len(seq) > MaxSequence {
		t.Fatalf("sequence is %d bytes, over %d", len(seq), MaxSequence)
	}
	var msg, title string
	for _, pair := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]7501;"), "\x1b\\"), ":") {
		k, v, _ := strings.Cut(pair, "=")
		dec, err := base64.StdEncoding.DecodeString(v)
		switch k {
		case "msg":
			if err != nil {
				t.Fatal(err)
			}
			msg = string(dec)
		case "title":
			if err != nil {
				t.Fatal(err)
			}
			title = string(dec)
		}
	}
	if len(msg) > MaxMsg || len(msg) < MaxMsg-3 || !strings.HasPrefix(long, msg) {
		t.Errorf("msg cut to %d bytes", len(msg))
	}
	if len(title) > MaxTitle || len(title) < MaxTitle-3 || !strings.HasPrefix(long, title) {
		t.Errorf("title cut to %d bytes", len(title))
	}
}

func TestName(t *testing.T) {
	for in, want := range map[string]string{
		"deploy":                "deploy",
		"history-3":             "history-3",
		"a b/c":                 "a_b_c",
		"":                      "_",
		strings.Repeat("x", 40): strings.Repeat("x", MaxApp),
		"café":                  "caf__",
	} {
		if got := Name(in); got != want {
			t.Errorf("Name(%q) = %q, want %q", in, got, want)
		}
	}
}

// states is the state= of each report written to out.
func states(out *bytes.Buffer) []string {
	var got []string
	for _, s := range strings.Split(out.String(), "\x1b\\") {
		if s == "" {
			continue
		}
		body := strings.TrimPrefix(s, "\x1b]7501;")
		for _, pair := range strings.Split(body, ":") {
			if v, ok := strings.CutPrefix(pair, "state="); ok {
				got = append(got, v)
			}
		}
	}
	return got
}

func TestTracker(t *testing.T) {
	var out bytes.Buffer
	tr := NewTracker(&out, "mdsh")
	tr.Working(0, "setup")
	tr.Working(0, "setup") // the same again: not sent
	tr.Blocked(Permission, "Run deploy?")
	tr.Unblocked()
	tr.Unblocked() // not blocked: nothing
	tr.Working(50, "deploy")
	tr.Done("2 blocks ran")
	if tr.Settled() != true {
		t.Error("not settled after Done")
	}
	tr.Idle()
	if tr.Settled() {
		t.Error("settled after Idle")
	}
	tr.Clear()
	want := []string{"working", "blocked", "working", "working", "done", "idle", "clear"}
	if got := states(&out); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("states %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), ":app=mdsh:") {
		t.Errorf("no app in %q", out.String())
	}
	// Blocked carries the run's progress along, and Unblocked goes back
	// to the Working report.
	out.Reset()
	tr.Working(30, "build")
	tr.Blocked(Question, "which?")
	tr.Unblocked()
	if s := out.String(); strings.Count(s, "progress=30") != 3 {
		t.Errorf("progress not kept across blocked: %q", s)
	}
	// Unblocked before any Working is Idle.
	out.Reset()
	tr.Idle()
	out.Reset()
	tr.Blocked(Auth, "token")
	tr.Unblocked()
	if got := states(&out); strings.Join(got, " ") != "blocked idle" {
		t.Errorf("states %v", got)
	}
}

func TestNilTracker(t *testing.T) {
	var tr *Tracker
	tr.Working(1, "x")
	tr.Blocked(Question, "x")
	tr.Unblocked()
	tr.Done("")
	tr.Error("")
	tr.Idle()
	tr.Clear()
	if tr.Settled() {
		t.Error("nil tracker settled")
	}
	if NewTracker(nil, "mdsh") != nil {
		t.Error("a tracker with no writer should be nil")
	}
}
