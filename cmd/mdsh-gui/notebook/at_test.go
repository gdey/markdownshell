package notebook

import (
	"regexp"
	"strings"
	"testing"
)

// atStamp matches the at= a run stamps on its output.
var atStamp = regexp.MustCompile(` at="?\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ"?`)

// noAt drops the at= stamps from s, for tests that check output whose
// time they can't know.
func noAt[T string | []byte](s T) string { return atStamp.ReplaceAllString(string(s), "") }

// Each run of a block with a history pushes the one before it down,
// once, though the document is written as each run ends and again on
// save.
func TestRunHistory(t *testing.T) {
	file := write(t, "```sh {#n history=3}\ncd \"$(dirname \"$MDSH_FILE\")\"; n=$(($(cat count 2>/dev/null || echo 0) + 1)); echo $n > count; echo $n\n```\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	for i := 0; i < 3; i++ {
		if err := n.RunBlock(0); err != nil {
			t.Fatal(err)
		}
		if err := n.Save(); err != nil {
			t.Fatal(err)
		}
	}
	got := noAt(n.Text())
	want := "{for=n exit=0}\n3\n```\n\n```output {for=n exit=0 history=1}\n2\n```\n\n```output {for=n exit=0 history=2}\n1\n```\n"
	if !strings.HasSuffix(got, want) || strings.Count(got, "history=1") != 1 {
		t.Errorf("text:\n%s\nwant it to end\n%s", got, want)
	}
}
