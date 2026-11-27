package termstatus

import (
	"bytes"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want Report
		ok   bool
	}{
		{"state only", "state=idle", Report{State: Idle, Progress: NoProgress}, true},
		{"the post's example", "state=blocked:kind=permission:app=terraform:msg=" + b64("Apply 3 to add"),
			Report{State: Blocked, Kind: Permission, App: "terraform", Msg: "Apply 3 to add", Progress: NoProgress}, true},
		{"unpadded base64", "state=done:msg=" + strings.TrimRight(b64("hi"), "="), Report{State: Done, Msg: "hi", Progress: NoProgress}, true},
		{"progress", "state=working:progress=40", Report{State: Working, Progress: 40}, true},
		{"progress out of range", "state=working:progress=140", Report{State: Working, Progress: NoProgress}, true},
		{"progress on done dropped", "state=done:progress=40", Report{State: Done, Progress: NoProgress}, true},
		{"kind on working dropped", "state=working:kind=auth", Report{State: Working, Progress: NoProgress}, true},
		{"unknown kind", "state=blocked:kind=coffee", Report{State: Blocked, Progress: NoProgress}, true},
		{"spaces trimmed", " state = working : id = a/b ", Report{State: Working, ID: "a/b", Progress: NoProgress}, true},
		{"unknown key ignored", "state=idle:flavor=x", Report{State: Idle, Progress: NoProgress}, true},
		{"malformed pair skipped", "state=idle:nonsense:x=y", Report{State: Idle, Progress: NoProgress}, true},
		{"bad value skipped", "state=idle:app=md sh", Report{State: Idle, Progress: NoProgress}, true},
		{"last key wins", "state=idle:state=done", Report{State: Done, Progress: NoProgress}, true},
		{"clear", "state=clear:id=build", Report{State: Clear, ID: "build", Progress: NoProgress}, true},
		{"no state", "msg=" + b64("x"), Report{}, false},
		{"unknown state", "state=sleepy", Report{}, false},
		{"bad id", "state=idle:id=a//b", Report{}, false},
		{"deep id", "state=idle:id=" + strings.Repeat("a/", MaxDepth) + "a", Report{}, false},
		{"msg not base64", "state=idle:msg=a", Report{}, false},
		{"msg with control character", "state=idle:msg=" + b64("a\nb"), Report{}, false},
		{"msg with escape", "state=idle:msg=" + b64("a\x1b[2Jb"), Report{}, false},
		{"msg too long", "state=idle:msg=" + b64(strings.Repeat("x", MaxMsg+1)), Report{}, false},
		{"title too long", "state=idle:title=" + b64(strings.Repeat("x", MaxTitle+1)), Report{}, false},
		{"msg bad utf8", "state=idle:msg=" + b64("\xff\xfe"), Report{}, false},
	} {
		got, ok := Parse(tc.body)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: got %+v %v, want %+v %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
	// What Sequence writes, Parse reads back.
	r := Report{ID: "deploy/plan", State: Blocked, Kind: Question, Progress: 12, App: "mdsh", Title: "plan", Msg: "which region?"}
	seq := r.Sequence()
	got, ok := Parse(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b]7501;"), "\x1b\\"))
	if !ok || got != r {
		t.Errorf("round trip: got %+v %v, want %+v", got, ok, r)
	}
	if !IsQuery(" ? ") || IsQuery("state=idle") {
		t.Error("IsQuery")
	}
}

func TestStore(t *testing.T) {
	var s Store
	if !s.Apply(Report{State: Working, App: "cargo", Progress: 10}) {
		t.Error("first apply not a change")
	}
	if s.Apply(Report{State: Working, App: "cargo", Progress: 10}) {
		t.Error("same report a change")
	}
	s.Apply(Report{ID: "test", State: Blocked, Kind: Question, Progress: NoProgress})
	s.Apply(Report{ID: "test/unit", State: Done, Progress: NoProgress})
	s.Apply(Report{ID: "other", State: Error, App: "make", Progress: NoProgress})
	recs := s.Records()
	if len(recs) != 4 || recs[0].ID != "" || recs[1].ID != "other" || recs[2].ID != "test" || recs[3].ID != "test/unit" {
		t.Fatalf("records %+v", recs)
	}
	// App comes down from the nearest ancestor that has one.
	if recs[2].App != "cargo" || recs[3].App != "cargo" || recs[1].App != "make" {
		t.Errorf("apps %+v", recs)
	}
	// Clearing a record takes those under it.
	if !s.Apply(Report{ID: "test", State: Clear}) || s.Len() != 2 {
		t.Errorf("clear test: %+v", s.Records())
	}
	// The program ending drops working and blocked, keeps done and
	// error.
	s.Apply(Report{ID: "test", State: Blocked, Progress: NoProgress})
	s.Apply(Report{ID: "fin", State: Done, Progress: NoProgress})
	if !s.ProcessEnded() {
		t.Error("ended: no change")
	}
	recs = s.Records()
	if len(recs) != 2 || recs[0].ID != "fin" || recs[1].ID != "other" {
		t.Errorf("after end %+v", recs)
	}
	if s.ProcessEnded() {
		t.Error("ended again: a change")
	}
	// Clear with no id takes everything.
	if !s.Apply(Report{State: Clear}) || s.Len() != 0 {
		t.Error("clear all")
	}
	// Past the cap, the record updated longest ago goes.
	for i := 0; i <= StoreCap; i++ {
		s.Apply(Report{ID: "r" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + strings.Repeat("y", i/26), State: Idle, Progress: NoProgress})
	}
	if s.Len() != StoreCap {
		t.Errorf("len %d, want %d", s.Len(), StoreCap)
	}
	s.Reset()
	if s.Len() != 0 {
		t.Error("reset")
	}
}

func TestScanner(t *testing.T) {
	var out bytes.Buffer
	var got []Report
	queries := 0
	sc := NewScanner(&out, func(r Report) { got = append(got, r) }, func() { queries++ })
	in := "hello\x1b[31m red\x1b]7501;state=working:progress=5\x1b\\ more" + // ST
		"\x1b]7501;state=blocked:kind=auth\x07" + // BEL
		"\x1b]133;A\x07\x1b]0;title\x07" + // other OSCs pass
		"\x1b]7501;?\x1b\\" + // the query
		"\x1b]7501;state=nonsense\x1b\\" + // ignored
		"\x1b]7501;state=done\x1bXnot an ST" // an escape inside ends it as noise
	// Feed it a byte at a time, so every split is tried.
	for i := 0; i < len(in); i++ {
		sc.Write([]byte{in[i]})
	}
	if out.String() != in {
		t.Errorf("stream changed: %q", out.String())
	}
	if len(got) != 2 || got[0].State != Working || got[0].Progress != 5 || got[1].State != Blocked || got[1].Kind != Auth {
		t.Errorf("reports %+v", got)
	}
	if queries != 1 {
		t.Errorf("queries %d", queries)
	}
	// A sequence that never ends is given up on past MaxSequence, and
	// what follows is scanned.
	got = nil
	sc.Write([]byte("\x1b]7501;" + strings.Repeat("x", MaxSequence+10)))
	sc.Write([]byte("\x1b]7501;state=idle\x1b\\"))
	if len(got) != 1 || got[0].State != Idle {
		t.Errorf("after a runaway sequence: %+v", got)
	}
	// All at once works the same.
	got = nil
	sc.Write([]byte(in))
	if len(got) != 2 {
		t.Errorf("whole: %+v", got)
	}
}
