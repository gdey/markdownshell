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
		{"```sh {#a depends=nope}\n```\n", `unknown block "nope"`},
		{"```sh {#a depends=a}\n```\n", "cycle: a → a"},
		{"```sh {#a depends=b}\n```\n\n```sh {#b depends=c}\n```\n\n```sh {#c depends=b}\n```\n", "cycle: b → c → b"},
		{"```textbox {#name}\n```\n\n```sh {#a depends=name}\n```\n", "never runs"},
		{"```sh {<include file=x.sh depends=a}\n```\n", "can't have depends="},
	} {
		_, err := Parse([]byte(tc.src))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want an error with %q", tc.src, err, tc.want)
		}
	}
}
