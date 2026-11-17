package tui

import (
	"strings"
	"testing"
	"time"
)

const counter = "```sh {#n history=3}\ncd \"$(dirname \"$MDSH_FILE\")\"; n=$(($(cat count 2>/dev/null || echo 0) + 1)); echo $n > count; echo run $n\n```\n"

// Several runs of a block with a history before a save each keep the
// run before them, and the earlier runs show with b.
func TestEarlierRunsBeforeSave(t *testing.T) {
	m, file := newModel(t, counter, Options{})
	for i := 0; i < 3; i++ {
		press(m, "enter")
		pump(t, m)
	}
	v := m.View()
	if !strings.Contains(v, "run 3") || !strings.Contains(v, "2 earlier runs (b shows them)") || strings.Contains(v, "run 2\n") {
		t.Errorf("view before b:\n%s", v)
	}
	press(m, "b")
	v = m.View()
	for _, want := range []string{"2 earlier runs (b hides)", "run 1 back", "run 2", "run 2 back", "run 1", "today"} {
		if !strings.Contains(v, want) {
			t.Errorf("view after b has no %q:\n%s", want, v)
		}
	}
	press(m, "w")
	got := read(t, file)
	want := "```output {for=n exit=0}\nrun 3\n```\n\n```output {for=n exit=0 history=1}\nrun 2\n```\n\n```output {for=n exit=0 history=2}\nrun 1\n```\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("saved:\n%s\nwant it to end\n%s", got, want)
	}
	// Saving again, with no run in between, pushes nothing.
	press(m, "w")
	if again := read(t, file); again != got {
		t.Errorf("saving again changed the file:\n%s", again)
	}
	press(m, "b")
	if v := m.View(); strings.Contains(v, "run 1 back") {
		t.Errorf("b again didn't hide them:\n%s", v)
	}
}

// An earlier run that failed says so, with its stderr.
func TestEarlierRunFailed(t *testing.T) {
	src := "```sh {#c history=2}\necho ok\n```\n\n```output {for=c exit=0 at=2026-11-16T10:00:00Z}\nok\n```\n\n" +
		"```output {for=c exit=7 at=2026-11-15T10:00:00Z history=1}\n```\n\n```output {for=c stream=stderr history=1}\nrefused\n```\n"
	m, _ := newModel(t, src, Options{})
	press(m, "b")
	v := m.View()
	for _, want := range []string{"1 earlier run (b hides)", "run 1 back", "✗ exit 7", "refused"} {
		if !strings.Contains(v, want) {
			t.Errorf("view has no %q:\n%s", want, v)
		}
	}
}

// Lowering history= says, when saving, that earlier runs were dropped.
func TestEarlierTrimStatus(t *testing.T) {
	src := "```sh {#c history=1}\necho ok\n```\n\n```output {for=c exit=0}\nok\n```\n\n" +
		"```output {for=c exit=0 history=1}\nold\n```\n\n```output {for=c exit=0 history=2}\nolder\n```\n"
	m, file := newModel(t, src, Options{})
	press(m, "w")
	if !strings.Contains(m.status, "dropped 1 earlier run of c") {
		t.Errorf("status: %q", m.status)
	}
	if strings.Contains(read(t, file), "older") {
		t.Errorf("saved the run past the cap:\n%s", read(t, file))
	}
}

func TestRunAt(t *testing.T) {
	loc := time.FixedZone("here", -8*3600)
	now := time.Date(2026, 11, 17, 9, 30, 0, 0, loc)
	for _, c := range []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 11, 17, 8, 5, 0, 0, loc), "today 08:05"},
		{time.Date(2026, 11, 17, 3, 2, 0, 0, time.UTC), "yesterday 19:02"},
		{time.Date(2026, 11, 13, 23, 59, 0, 0, loc), "Fri 23:59"},
		{time.Date(2026, 11, 10, 7, 0, 0, 0, loc), "Nov 10 07:00"},
		{time.Date(2025, 12, 31, 12, 0, 0, 0, loc), "Dec 31, 2025 12:00"},
	} {
		if got := runAt(c.t, now); got != c.want {
			t.Errorf("runAt(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

// The report form's Earlier runs switch puts --history in the command
// and the earlier runs in the report.
func TestReportEarlierRuns(t *testing.T) {
	src := "```sh {#c history=2}\necho ok\n```\n\n```output {for=c exit=0 at=2026-11-16T10:00:00Z}\nok\n```\n\n" +
		"```output {for=c exit=0 at=2026-11-15T10:00:00Z history=1}\nbefore\n```\n"
	m, file := newModel(t, src, Options{})
	press(m, "p", "right", "right", "down", "down", "down", "space")
	if v := m.View(); !strings.Contains(v, "mdsh -n --history -o doc-report.md doc.md") {
		t.Fatalf("the command doesn't have --history:\n%s", v)
	}
	press(m, "enter")
	got := read(t, strings.TrimSuffix(file, ".md")+"-report.md")
	if !strings.Contains(got, "*Run 1 back, 2026-11-15 10:00 UTC, exit 0*") || !strings.Contains(got, "before") {
		t.Errorf("report:\n%s", got)
	}
}
