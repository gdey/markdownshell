package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// atStamp matches the at= a run stamps on its output.
var atStamp = regexp.MustCompile(` at="?\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ"?`)

// noAt drops the at= stamps from s, for tests that check output whose
// time they can't know.
func noAt[T string | []byte](s T) string { return atStamp.ReplaceAllString(string(s), "") }

// TestWriteHistory checks that each -w run of a block with a history
// keeps the run before under the new output.
func TestWriteHistory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#n history=2}\ncd \"$(dirname \"$MDSH_FILE\")\"; n=$(($(cat count 2>/dev/null || echo 0) + 1)); echo $n > count; echo $n\n```\n"), 0o644)
	for i := 0; i < 4; i++ {
		var errb bytes.Buffer
		if rc := mdsh([]string{"-w", f}, nil, &bytes.Buffer{}, &errb); rc != 0 {
			t.Fatalf("run %d: rc = %d: %s", i, rc, errb.String())
		}
	}
	got := noAt(mustRead(t, f))
	want := "```sh {#n history=2}\ncd \"$(dirname \"$MDSH_FILE\")\"; n=$(($(cat count 2>/dev/null || echo 0) + 1)); echo $n > count; echo $n\n```\n\n" +
		"```output {for=n exit=0}\n4\n```\n\n```output {for=n exit=0 history=1}\n3\n```\n\n```output {for=n exit=0 history=2}\n2\n```\n"
	if got != want {
		t.Errorf("file:\ngot  %q\nwant %q", got, want)
	}
}

func mustRead(t *testing.T, f string) string {
	t.Helper()
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// --history prints the earlier runs under the output; without it,
// printing shows the current output only.
func TestPrintHistoryFlag(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#n history=2}\necho 3\n```\n\n```output {for=n exit=0 at=2026-11-16T10:00:00Z}\n3\n```\n\n```output {for=n exit=1 at=2026-11-15T10:00:00Z history=1}\ntwo\n```\n"), 0o644)
	// A run makes what the file holds the run before it.
	for _, c := range []struct {
		args []string
		want bool
		line string
	}{{[]string{"-n", f}, false, "Run 1 back, 2026-11-15 10:00 UTC, exit 1"}, {[]string{"-n", "--history", f}, true, "Run 1 back, 2026-11-15 10:00 UTC, exit 1"}, {[]string{"--history", f}, true, "Run 2 back, 2026-11-15 10:00 UTC, exit 1"}} {
		var out, errb bytes.Buffer
		mdsh(c.args, nil, &out, &errb)
		got := out.String()
		if has := strings.Contains(got, c.line) && strings.Contains(got, "two"); has != c.want {
			t.Errorf("mdsh %v: earlier run shown %v, want %v:\n%s%s", c.args, has, c.want, got, errb.String())
		}
	}
	var errb bytes.Buffer
	if rc := mdsh([]string{"-i", "--history", f}, nil, &bytes.Buffer{}, &errb); rc == 0 || !strings.Contains(errb.String(), "-history can't be used with -i") {
		t.Errorf("-i --history: rc %d: %s", rc, errb.String())
	}
}
