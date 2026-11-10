package tui

import (
	"strings"
	"testing"
)

// A file whose front matter doesn't parse opens, says why, runs
// nothing, and F fixes it.
func TestBadFrontMatter(t *testing.T) {
	src := "---\ntitle: Setup: first steps\n---\n\n# Setup\n\n```sh {#one}\necho hi\n```\n"
	m, file := newModel(t, src, Options{})
	v := m.View()
	if !strings.Contains(v, "mapping values are not allowed") || !strings.Contains(v, "F add quotes") {
		t.Errorf("no front matter banner:\n%s", v)
	}
	for _, k := range []string{"enter", "a", "s"} {
		press(m, k)
		if m.running || !strings.Contains(m.status, "fix the front matter") {
			t.Errorf("%s: running %v, status %q", k, m.running, m.status)
		}
	}
	if r := m.results[0]; r.Ran {
		t.Error("the block ran")
	}
	press(m, "F")
	if m.d.FrontMatterErr != nil || !strings.Contains(string(m.d.Src), `title: "Setup: first steps"`) {
		t.Fatalf("not fixed: %v\n%s", m.d.FrontMatterErr, m.d.Src)
	}
	if v := m.View(); strings.Contains(v, "mapping values") {
		t.Errorf("banner still up:\n%s", v)
	}
	press(m, "enter")
	pump(t, m)
	if r := m.results[0]; !r.Ran || string(r.Stdout) != "hi\n" {
		t.Errorf("after the fix: %+v", r)
	}
	press(m, "w")
	if got := read(t, file); !strings.Contains(got, `title: "Setup: first steps"`) {
		t.Errorf("fix not saved:\n%s", got)
	}
}
