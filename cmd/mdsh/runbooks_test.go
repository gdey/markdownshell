package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
)

// TestRunbooks runs every runbook in examples/runbooks as a real file.
// A name ending in _linux.md or _darwin.md (like Go's file suffixes) runs
// only on that OS. Each runbook must finish with no failed blocks, and a
// second run over the written file must replace its output, not add to it.
// test-coverage.md runs go test with -skip '^TestRunbooks$', so it doesn't
// start this test again.
func TestRunbooks(t *testing.T) {
	if testing.Short() {
		t.Skip("runbooks are slow; skipped with -short")
	}
	dir := filepath.Join("..", "..", "examples", "runbooks")
	files, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no runbooks in %s: %v", dir, err)
	}
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(strings.TrimSuffix(name, ".md"), func(t *testing.T) {
			if goos, ok := runbookOS(name); ok && goos != runtime.GOOS {
				t.Skipf("runs on %s only", goos)
			}
			// Write next to the runbook, so blocks run from its directory.
			out := filepath.Join(dir, ".test-"+name)
			t.Cleanup(func() { os.Remove(out) })

			first := runRunbook(t, "-o", out, f)
			second := runRunbook(t, "-w", out)
			if got, want := outputs(t, second), outputs(t, first); got != want {
				t.Errorf("second run has %d outputs, first had %d", got, want)
			}
		})
	}
}

// runbookOS returns the OS a runbook is limited to by its name.
func runbookOS(name string) (string, bool) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		if strings.HasSuffix(name, "_"+goos+".md") {
			return goos, true
		}
	}
	return "", false
}

// runRunbook runs mdsh with args and returns the file it wrote, the last
// argument for -w or the one after -o.
func runRunbook(t *testing.T, args ...string) []byte {
	t.Helper()
	target := args[len(args)-1]
	if args[0] == "-o" {
		target = args[1]
	}
	var stdout, stderr bytes.Buffer
	rc := mdsh(append([]string{"--yes", "--no-color"}, args...), nil, &stdout, &stderr)
	b, err := os.ReadFile(target)
	if rc != 0 {
		// The written file holds each failed block's stderr.
		t.Fatalf("mdsh %s: rc = %d\n%s\n%s", strings.Join(args, " "), rc, stderr.String(), b)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// outputs counts the blocks with output attached.
func outputs(t *testing.T, src []byte) int {
	t.Helper()
	d, err := doc.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range d.Blocks {
		if b.Output.End > b.Output.Start {
			n++
		}
	}
	return n
}
