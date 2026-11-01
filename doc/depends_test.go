package doc

import (
	"slices"
	"strings"
	"testing"
)

func TestDepends(t *testing.T) {
	d, err := Parse([]byte("```sh {#a}\n```\n\n```sh {#b depends=\"a, c\"}\n```\n\n```sh {#c}\n```\n\n```sh {#d depends=}\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Block("b").Depends(); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("b: Depends = %q", got)
	}
	if got := d.Block("d").Depends(); len(got) != 0 {
		t.Errorf("d: Depends = %q, want none", got)
	}
}

func TestDependsErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"```sh {#a depends=a}\n```\n", "cycle: a → a"},
		{"```sh {#a depends=b}\n```\n\n```sh {#b depends=c}\n```\n\n```sh {#c depends=b}\n```\n", "cycle: b → c → b"},
		{"```sh {<include file=x.sh depends=a}\n```\n", "can't have depends="},
	} {
		_, err := Parse([]byte(tc.src))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want an error with %q", tc.src, err, tc.want)
		}
	}
}

// A depends= naming a block that isn't there, or one that never runs,
// is a warning, not an error, so the document still opens.
func TestDependsWarnings(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"```sh {#a depends=nope}\n```\n", `unknown block "nope"; it is ignored`},
		{"```textbox {#name}\n```\n\n```sh {#a depends=name}\n```\n", "never runs; it is ignored"},
		// A cycle through a bad name is no cycle.
		{"```sh {#a depends=\"nope, a2\"}\n```\n\n```sh {#a2}\n```\n", `unknown block "nope"`},
	} {
		d, err := Parse([]byte(tc.src))
		if err != nil {
			t.Errorf("%q: %v", tc.src, err)
			continue
		}
		if len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], tc.want) {
			t.Errorf("%q: warnings = %q, want one with %q", tc.src, d.Warnings, tc.want)
		}
		if got := d.Block("a").Depends(); len(got) == 0 {
			t.Errorf("%q: Depends dropped the names: %q", tc.src, got)
		}
		if d.Dep("nope") != nil || d.Dep("name") != nil {
			t.Errorf("%q: Dep of a bad name isn't nil", tc.src)
		}
	}
}
