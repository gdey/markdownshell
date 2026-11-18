package tui

import (
	"strings"
	"testing"
	"time"
)

// runAgo counts back from now in the largest whole unit that fits, and
// says when its words next change.
func TestRunAgo(t *testing.T) {
	now := time.Date(2026, 11, 17, 9, 30, 0, 0, time.UTC)
	const day = 24 * time.Hour
	for _, c := range []struct {
		back time.Duration
		want string
	}{
		{-5 * time.Second, "just now"},
		{0, "just now"},
		{time.Second, "1 sec ago"},
		{42 * time.Second, "42 secs ago"},
		{59 * time.Second, "59 secs ago"},
		{time.Minute, "1 min ago"},
		{3*time.Minute + 10*time.Second, "3 mins ago"},
		{59*time.Minute + 59*time.Second, "59 mins ago"},
		{time.Hour, "1 hour ago"},
		{23*time.Hour + 59*time.Minute, "23 hours ago"},
		{day, "1 day ago"},
		{6 * day, "6 days ago"},
		{7 * day, "1 week ago"},
		{29 * day, "4 weeks ago"},
		{30 * day, "1 month ago"},
		{364 * day, "11 months ago"},
		{365 * day, "1 year ago"},
		{800 * day, "2 years ago"},
	} {
		at := now.Add(-c.back)
		got, next := runAgo(at, now)
		if got != c.want {
			t.Errorf("runAgo(now-%v) = %q, want %q", c.back, got, c.want)
		}
		// The words hold until next, and change there.
		if !next.After(now) {
			t.Errorf("runAgo(now-%v): next %v isn't after now", c.back, next)
			continue
		}
		if c.back >= 0 {
			if w, _ := runAgo(at, next.Add(-time.Second)); w != got {
				t.Errorf("runAgo(now-%v) a second before next = %q, want %q still", c.back, w, got)
			}
			if w, _ := runAgo(at, next); w == got {
				t.Errorf("runAgo(now-%v) at next is still %q", c.back, w)
			}
		}
	}
}

// Saved output says how long ago its run was, from its at=, and a run
// this session says so in the block's header; both are drawn again as
// the words change.
func TestOutputAgoShown(t *testing.T) {
	at := time.Now().Add(-3*time.Minute - 10*time.Second).UTC().Format(time.RFC3339)
	src := "```sh {#old}\necho x\n```\n\n```output {for=old exit=0 at=" + at + "}\nfrom last time\n```\n"
	m, _ := newModel(t, src, Options{})
	if v := m.View(); !strings.Contains(v, "saved output, 3 mins ago:") {
		t.Errorf("saved output's time not shown:\n%s", v)
	}
	if want := time.Now().Add(-10 * time.Second).Add(time.Minute); m.agoNext.Sub(want).Abs() > 2*time.Second {
		t.Errorf("agoNext = %v, want about %v", m.agoNext, want)
	}
	press(m, "r")
	pump(t, m)
	if v := m.View(); !strings.Contains(v, "· just now") && !strings.Contains(v, "· 1 sec ago") {
		t.Errorf("run's time not shown in its header:\n%s", v)
	}
}
