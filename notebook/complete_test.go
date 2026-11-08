package notebook

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestCompletions(t *testing.T) {
	for _, lang := range []string{"sh", "zsh"} {
		t.Run(lang, func(t *testing.T) {
			if _, err := exec.LookPath(lang); err != nil {
				t.Skip(err)
			}
			testCompletions(t, lang)
		})
	}
}

func testCompletions(t *testing.T, lang string) {
	file := write(t, "```"+lang+"\ncd sub; MDSH_TEST_VAR=1; alias mdshtestalias=true\n```\n")
	dir := filepath.Dir(file)
	for _, p := range []string{"sub/alpha one.txt", "sub/alps/x", "sub/beta"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		before, word string
		want         []string // all of them, or some of them if some
		some         bool
	}{
		{"ls al", "al", []string{`alpha\ one.txt`, "alps/"}, false},
		{"cat alpha\\ o", "alpha\\ o", []string{`alpha\ one.txt`}, false},
		{"cat alps/", "alps/", []string{"alps/x"}, false},
		{"echo $MDSH_TEST_V", "$MDSH_TEST_V", []string{"$MDSH_TEST_VAR"}, false},
		{"echo ${MDSH_TEST_V", "${MDSH_TEST_V", []string{"${MDSH_TEST_VAR}"}, false},
		{"mdshtesta", "mdshtesta", []string{"mdshtestalias"}, false},
		{"true && ech", "ech", []string{"echo"}, true},
		{"ls nothing", "nothing", nil, false},
	} {
		c, err := n.Completions(tc.before)
		if err != nil {
			t.Fatalf("%q: %v", tc.before, err)
		}
		if c.Word != tc.word {
			t.Errorf("%q: word %q, want %q", tc.before, c.Word, tc.word)
		}
		if tc.some {
			for _, w := range tc.want {
				if !slices.Contains(c.Items, w) {
					t.Errorf("%q: items %q lack %q", tc.before, c.Items, w)
				}
			}
		} else if !slices.Equal(c.Items, tc.want) {
			t.Errorf("%q: items %q, want %q", tc.before, c.Items, tc.want)
		}
	}
	// Completing left the shell where it was.
	if c, _ := n.Completions("ls be"); !slices.Equal(c.Items, []string{"beta"}) {
		t.Errorf("after completing: %q", c.Items)
	}
}
