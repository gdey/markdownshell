package run_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func TestLines(t *testing.T) {
	data := []byte("one\ntwo\nthree\n")
	tests := []struct {
		from, to int
		want     string
	}{
		{0, 0, "one\ntwo\nthree\n"},
		{1, 1, "one\n"},
		{2, 0, "two\nthree\n"},
		{2, 3, "two\nthree\n"},
		{3, 9, "three\n"},
	}
	for _, tt := range tests {
		got, err := run.Lines(data, tt.from, tt.to)
		if err != nil || string(got) != tt.want {
			t.Errorf("Lines(%d, %d) = %q, %v; want %q", tt.from, tt.to, got, err, tt.want)
		}
	}
	if got, err := run.Lines([]byte("a\nb"), 2, 0); err != nil || string(got) != "b" {
		t.Errorf("no final newline: %q, %v", got, err)
	}
	if _, err := run.Lines(data, 4, 0); err == nil || !strings.Contains(err.Error(), "past the end") {
		t.Errorf("from past the end: err = %v", err)
	}
}

func TestInclude(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.sh"), []byte("echo ran\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "```sh {#a}\nexit 3\n```\n\n" +
		"```go {<include file=a.go from=3 number_on}\nstale\n```\n\n" +
		"```sh {<include file=a.sh}\n```\n\n" +
		"```go {<include file=nope.go}\nkept\n```\n\n" +
		"```go {<include file=nope.go skip}\n```\n"
	d, results := runDoc(t, src, run.Options{Dir: dir})
	inc := results[1:]
	// Includes are read even though the run stopped at a, and an sh
	// include never runs.
	if !inc[0].Ran || string(inc[0].Stdout) != "func A() {}\n" || string(inc[1].Stdout) != "echo ran\n" {
		t.Errorf("includes = %q, %q", inc[0].Stdout, inc[1].Stdout)
	}
	if !inc[2].Failed() || !strings.Contains(inc[2].Err.Error(), "include nope.go") {
		t.Errorf("missing err = %v", inc[2].Err)
	}
	if inc[3].Ran || !inc[3].Skipped {
		t.Errorf("skip: %+v", inc[3])
	}

	printed := string(render.Markdown(d, results, render.Options{HideCode: true, HideOutput: true}))
	if !strings.Contains(printed, "```go {<include file=a.go from=3 number_on}\n3  func A() {}\n```\n") {
		t.Errorf("printed:\n%s", printed)
	}

	once := string(render.Markdown(d, results, render.Options{WriteBack: true}))
	for _, want := range []string{
		"```go {<include file=a.go from=3 number_on}\nfunc A() {}\n```\n",
		"```sh {<include file=a.sh}\necho ran\n```\n",
		"```go {<include file=nope.go}\nkept\n```\n\n```output {for=<include-3 stream=stderr}\nmdsh: include nope.go:",
	} {
		if !strings.Contains(once, want) {
			t.Errorf("write-back missing %q:\n%s", want, once)
		}
	}
	d2, results2 := runDoc(t, once, run.Options{Dir: dir})
	if twice := string(render.Markdown(d2, results2, render.Options{WriteBack: true})); once != twice {
		t.Errorf("not stable:\n--- once\n%s\n--- twice\n%s", once, twice)
	}
}

func TestIncludeFence(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("```sh\necho\n```\n"), 0o644)
	// Lines that would close the fence get a longer one; nested blocks
	// keep their prefix; an unclosed fence is closed.
	src := "```md {<include file=a.md}\n```\n\n> ```md {<include file=a.md}\n> ```\n\n```md {<include file=a.md}\n"
	d, results := runDoc(t, src, run.Options{Dir: dir})
	got := string(render.Markdown(d, results, render.Options{WriteBack: true}))
	want := "````md {<include file=a.md}\n```sh\necho\n```\n````\n\n" +
		"> ````md {<include file=a.md}\n> ```sh\n> echo\n> ```\n> ````\n\n" +
		"````md {<include file=a.md}\n```sh\necho\n```\n````\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}
