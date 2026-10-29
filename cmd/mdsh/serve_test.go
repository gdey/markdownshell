package main

import (
	"flag"
	"io"
	"strings"
	"testing"
)

func TestServeFlag(t *testing.T) {
	for _, tc := range []struct {
		args []string
		on   bool
		addr string
		rest int
	}{
		{nil, false, "", 0},
		{[]string{"--serve"}, true, "", 0},
		{[]string{"--serve", "doc.md"}, true, "", 1},
		{[]string{"--serve=:9000", "doc.md", "a"}, true, ":9000", 2},
		{[]string{"--serve=localhost:9000"}, true, "localhost:9000", 0},
	} {
		fs := flag.NewFlagSet("", flag.ContinueOnError)
		var f serveFlag
		fs.Var(&f, "serve", "")
		if err := fs.Parse(tc.args); err != nil {
			t.Fatal(err)
		}
		if f.on != tc.on || f.addr != tc.addr || fs.NArg() != tc.rest {
			t.Errorf("%q: on %v addr %q rest %d; want %v %q %d", tc.args, f.on, f.addr, fs.NArg(), tc.on, tc.addr, tc.rest)
		}
	}
}

func TestServeConflicts(t *testing.T) {
	var stderr strings.Builder
	code := mdsh([]string{"--serve", "-w", "doc.md"}, strings.NewReader(""), io.Discard, &stderr)
	if code != 2 || !strings.Contains(stderr.String(), "-w can't be used with --serve") {
		t.Errorf("--serve -w: %d %q", code, stderr.String())
	}
}
