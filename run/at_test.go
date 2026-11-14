package run_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/gdey/markdownshell/run"
)

// fixedNow is a clock for run.Options.Now, so tests know the at= stamp.
func fixedNow() time.Time {
	return time.Date(2026, 11, 13, 11, 8, 0, 500, time.FixedZone("PST", -8*3600))
}

// atStamp matches the at= a run stamps on its output.
var atStamp = regexp.MustCompile(` at="?\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ"?`)

// noAt drops the at= stamps from s, for tests that check output whose
// time they don't care about.
func noAt(s string) string { return atStamp.ReplaceAllString(s, "") }

// TestStartStamp checks that a run stamps each block that ran with its
// start time, in UTC to the second, and leaves the rest zero.
func TestStartStamp(t *testing.T) {
	_, results := runDoc(t, "```sh {#a}\necho a\n```\n\n```sh {#b skip}\necho b\n```\n", run.Options{Now: fixedNow})
	want := time.Date(2026, 11, 13, 19, 8, 0, 0, time.UTC)
	if got := results[0].Start; !got.Equal(want) || got.Location() != time.UTC {
		t.Errorf("a: Start = %v, want %v", got, want)
	}
	if !results[1].Start.IsZero() {
		t.Errorf("skipped b: Start = %v, want zero", results[1].Start)
	}

	before := time.Now().Truncate(time.Second)
	_, results = runDoc(t, "```sh {#a}\necho a\n```\n", run.Options{})
	if got := results[0].Start; got.Before(before) || got.After(time.Now()) {
		t.Errorf("default clock: Start = %v, want about %v", got, before)
	}
}
