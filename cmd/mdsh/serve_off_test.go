//go:build !serve

package main

import (
	"strings"
	"testing"
)

// An mdsh built without -tags serve has no --serve: not in its flags,
// its help or its completion.
func TestNoServe(t *testing.T) {
	var stderr strings.Builder
	if err := runCmd([]string{"--serve", "doc.md"}, nil, nil, &stderr); err == nil {
		t.Error("--serve was accepted")
	}
	if strings.Contains(stderr.String(), "--serve") {
		t.Errorf("help names --serve:\n%s", stderr.String())
	}
	for _, sh := range []string{"bash", "zsh"} {
		var out strings.Builder
		if err := completionCmd([]string{sh}, &out); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "--serve") {
			t.Errorf("%s completion offers --serve", sh)
		}
	}
}
