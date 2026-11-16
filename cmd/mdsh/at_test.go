package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
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
