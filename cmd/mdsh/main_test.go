package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func TestConfirmPrompt(t *testing.T) {
	b := &doc.Block{ID: "deploy", Body: "echo hi\n", Attrs: doc.Attrs{"confirm": ""}}
	for in, want := range map[string]run.Answer{
		"r\n": run.Go, "yes\n": run.Go, "\n": run.Stop, "s\n": run.Stop, "huh\nrun\n": run.Go, "": run.Stop,
		"k\n": run.Skip, "skip\n": run.Skip, "o\n": run.Go,
	} {
		var out bytes.Buffer
		if got := newPrompter(strings.NewReader(in), &out).confirm(b); got != want {
			t.Errorf("input %q: got %v, want %v", in, got, want)
		}
		if !strings.Contains(out.String(), "echo hi") || !strings.Contains(out.String(), "Run this block?") {
			t.Errorf("prompt = %q", out.String())
		}
	}
}

func TestDependPrompt(t *testing.T) {
	dep := &doc.Block{ID: "build", Body: "make\n"}
	b := &doc.Block{ID: "deploy", Body: "./deploy\n", Attrs: doc.Attrs{"depends": "build"}}
	for in, want := range map[string]run.Answer{
		"r\n": run.Go, "y\n": run.Go, "k\n": run.Skip, "\n": run.Stop, "s\n": run.Stop, "o\nk\n": run.Skip, "": run.Stop,
	} {
		var out bytes.Buffer
		if got := newPrompter(strings.NewReader(in), &out).depend(dep, b); got != want {
			t.Errorf("input %q: got %v, want %v", in, got, want)
		}
		if !strings.Contains(out.String(), "deploy depends on build") || !strings.Contains(out.String(), "make") || !strings.Contains(out.String(), "Run build first?") {
			t.Errorf("prompt = %q", out.String())
		}
	}
}

// TestDependsYes checks that --yes runs a later dependency first,
// without asking, and only once.
func TestDependsYes(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#deploy depends=build}\necho deploy\n```\n\n```sh {#build}\necho built\n```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--yes", "--no-color", f}, nil, &out, &errb); rc != 0 || strings.Count(out.String(), "```output {for=build exit=0}\nbuilt\n") != 1 || !strings.Contains(out.String(), "```output {for=deploy exit=0}\ndeploy\n") {
		t.Errorf("--yes: rc = %d, stdout %s, stderr %s", rc, out.String(), errb.String())
	}
}

func TestConfirmSkipSaved(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\na\n```\n\n```sh {#b}\necho b\n```\n\n```sh {#c}\necho c\n```\n\n```output {for=c exit=0}\nc\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("o\nr\n"), &out)
	var got []run.Answer
	for _, b := range d.Blocks {
		got = append(got, p.confirm(b))
	}
	if want := []run.Answer{run.Skip, run.Go, run.Skip}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if !strings.Contains(out.String(), "block a (has saved output)") || !strings.Contains(out.String(), "block c: skipped") {
		t.Errorf("prompt = %q", out.String())
	}
}

func TestStepFlag(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "d.md")
	os.WriteFile(f, []byte("```sh {#a}\necho a\n```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--step", "--yes", "--no-color", f}, nil, &out, &errb); rc != 0 || !strings.Contains(out.String(), "```output") {
		t.Errorf("rc = %d, stdout %s, stderr %s", rc, out.String(), errb.String())
	}
}

// TestStream checks that a run that asks prints the document as it
// goes: each question comes after the prose and code before it, and a
// failed block's output and stderr show before the next question.
func TestStream(t *testing.T) {
	d, err := doc.Parse([]byte("# T\n\n```sh {#a}\necho out; echo oops >&2; false\n```\n\nnext\n\n```sh {#b}\necho b\n```\n\nend\n"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	live := newStreamer(d, &out, false, render.Options{})
	var seen []string
	results, err := run.Run(context.Background(), d, run.Options{
		Step:    true,
		OnEvent: live.event,
		Confirm: func(b *doc.Block) run.Answer {
			live.upTo(b)
			seen = append(seen, out.String())
			return run.Go
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.finish(results); err != nil {
		t.Fatal(err)
	}
	if want := "# T\n\n```sh {#a}\necho out; echo oops >&2; false\n```\n"; seen[0] != want {
		t.Errorf("before a: %q, want %q", seen[0], want)
	}
	if !strings.Contains(seen[1], "out\n```") || !strings.Contains(seen[1], "oops") || !strings.HasSuffix(seen[1], "next\n\n```sh {#b}\necho b\n```\n") {
		t.Errorf("before b: %q", seen[1])
	}
	if want := string(render.Markdown(d, results, render.Options{})); out.String() != want {
		t.Errorf("streamed %q, want %q", out.String(), want)
	}
}

// TestWriteShowsFailure checks that -w prints a failed block's output,
// since it goes to the file and not to the screen.
func TestWriteShowsFailure(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#a}\necho out; echo oops >&2; false\n```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"-w", f}, nil, &out, &errb); rc == 0 {
		t.Fatal("rc = 0, want failure")
	}
	if s := errb.String(); !strings.Contains(s, "a: failed") || !strings.Contains(s, "out\n") || !strings.Contains(s, "oops\n") {
		t.Errorf("stderr = %q", s)
	}
}

// TestWriteNamesBlocks checks that -w writes an id into each unnamed
// block and -o leaves the source as it is.
func TestWriteNamesBlocks(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "d.md")
	src := "```sh\necho hi\n```\n\n```sh {#a}\ntrue\n```\n"
	os.WriteFile(f, []byte(src), 0o644)
	var errb bytes.Buffer
	if rc := mdsh([]string{"-o", filepath.Join(dir, "out.md"), f}, nil, &bytes.Buffer{}, &errb); rc != 0 {
		t.Fatalf("-o: rc = %d: %s", rc, errb.String())
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "out.md")); !strings.Contains(string(got), "```output {for=block-1 exit=0}") {
		t.Errorf("-o wrote:\n%s", got)
	}
	if rc := mdsh([]string{"-w", f}, nil, &bytes.Buffer{}, &errb); rc != 0 {
		t.Fatalf("-w: rc = %d: %s", rc, errb.String())
	}
	if !strings.Contains(errb.String(), "block at line 1: ok\n") || !strings.Contains(errb.String(), "a: ok\n") {
		t.Errorf("stderr = %q", errb.String())
	}
	got, _ := os.ReadFile(f)
	m := regexp.MustCompile("^```sh \\{#(block-[0-9a-f-]{36})\\}\necho hi\n```\n\n```output \\{for=(block-[0-9a-f-]{36}) exit=0\\}\nhi\n```\n\n```sh \\{#a\\}\n").FindStringSubmatch(string(got))
	if m == nil || m[1] != m[2] {
		t.Fatalf("-w wrote:\n%s", got)
	}
	// The next save keeps the id.
	if rc := mdsh([]string{"-w", f}, nil, &bytes.Buffer{}, &errb); rc != 0 {
		t.Fatalf("-w again: rc = %d", rc)
	}
	if again, _ := os.ReadFile(f); string(again) != string(got) {
		t.Errorf("second -w changed the file:\n%s", again)
	}
}

// TestWriteStep checks that a step-wise run with -w prints the document
// as well as writing it.
func TestWriteStep(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("# T\n\n```sh {#a}\necho out; echo oops >&2; false\n```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"-w", "--step", "--yes", "--no-color", f}, nil, &out, &errb); rc == 0 {
		t.Fatal("rc = 0, want failure")
	}
	if s := out.String(); !strings.HasPrefix(s, "# T\n") || !strings.Contains(s, "out\n```") || !strings.Contains(s, "oops\n") {
		t.Errorf("stdout = %q", s)
	}
	if b, _ := os.ReadFile(f); !strings.Contains(string(b), "out\n```") {
		t.Errorf("file = %s", b)
	}
	if strings.Contains(errb.String(), "a: failed") {
		t.Errorf("stderr = %q, want no status lines", errb.String())
	}
}

// TestStopNote checks that a run that stops early says why, before
// and after the document.
func TestStopNote(t *testing.T) {
	for body, want := range map[string]string{
		"exit 3":        "block a ended its shell (exit status 3",
		"set -e; false": "block a ended its shell (exit status 1",
		"echo a; false": "block a failed (exit status 1)",
		"sleep 5":       "block a timed out after 100ms, which ended its shell",
	} {
		f := filepath.Join(t.TempDir(), "d.md")
		os.WriteFile(f, []byte("```sh {#a timeout=100ms}\n"+body+"\n```\n\n```sh {#b}\necho b\n```\n"), 0o644)
		var out, errb bytes.Buffer
		mdsh([]string{"--stop-on-error", "--no-color", f}, nil, &out, &errb)
		if n := strings.Count(errb.String(), want); n != 2 {
			t.Errorf("%s: stderr = %q, want %q before and after the document", body, errb.String(), want)
		}
	}
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#a}\nfalse\n```\n\n```sh {#b}\necho b\n```\n"), 0o644)
	var out, errb bytes.Buffer
	mdsh([]string{"--no-color", f}, nil, &out, &errb)
	if strings.Contains(errb.String(), "stopped there") {
		t.Errorf("run went on, but stderr = %q", errb.String())
	}
}

func TestYesFlag(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "d.md")
	os.WriteFile(f, []byte("```sh {#a confirm}\necho ran\n```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--yes", "--no-color", f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "ran") {
		t.Errorf("stdout = %s", out.String())
	}
}

func TestOutputFile(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.md")
	src := "```sh {#a}\necho hi\n```\n\n```sh {#s out=screenshot}\necho shot\n```\n"
	os.WriteFile(in, []byte(src), 0o755)
	out := filepath.Join(dir, "sub", "out.md")
	os.Mkdir(filepath.Dir(out), 0o755)

	var stdout, stderr bytes.Buffer
	if rc := mdsh([]string{"-o", out, in}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	if b, _ := os.ReadFile(in); string(b) != src {
		t.Errorf("input changed:\n%s", b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"```output {for=a exit=0}\nhi\n```", "![s](../in.md.assets/s.svg)"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("output missing %q:\n%s", want, b)
		}
	}
	if info, _ := os.Stat(out); info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want the input's 0755", info.Mode().Perm())
	}
}

func TestOutputFileFromStdin(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.md")
	var stdout, stderr bytes.Buffer
	if rc := mdsh([]string{"-o", out, "-"}, strings.NewReader("```sh\necho x\n```\n"), &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	if b, _ := os.ReadFile(out); !strings.Contains(string(b), "x\n```") {
		t.Errorf("out = %s", b)
	}
}

func TestVersion(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--version"}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d, stderr %q", rc, errb.String())
	}
	if !strings.HasPrefix(out.String(), "mdsh ") {
		t.Errorf("got %q", out.String())
	}
}

func TestInteractiveErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "doc.md")
	os.WriteFile(file, []byte("```sh\necho hi\n```\n"), 0o644)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-i", "-w", file}, "-w can't be used with -i"},
		{[]string{"-i", "--pdf", "x.pdf", file}, "-pdf can't be used with -i"},
		{[]string{"-i", "--html", "x.html", file}, "-html can't be used with -i"},
		{[]string{"-i", "-"}, "needs a file"},
		{[]string{"-i", file}, "needs a terminal"},
	} {
		var out, errb bytes.Buffer
		if rc := mdsh(tc.args, strings.NewReader(""), &out, &errb); rc != 2 || !strings.Contains(errb.String(), tc.want) {
			t.Errorf("%v: rc = %d, stderr %q, want %q", tc.args, rc, errb.String(), tc.want)
		}
	}
}

// TestIncludeExample checks that examples/include.md is up to date: running
// it again writes the same file.
func TestIncludeExample(t *testing.T) {
	dir := filepath.Join("..", "..", "examples")
	want, err := os.ReadFile(filepath.Join(dir, "include.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Write next to the example, so its relative paths still resolve.
	out := filepath.Join(dir, ".test-include.md")
	t.Cleanup(func() { os.Remove(out) })
	var stdout, stderr bytes.Buffer
	if rc := mdsh([]string{"-o", out, filepath.Join(dir, "include.md")}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	if got, _ := os.ReadFile(out); string(got) != string(want) {
		t.Errorf("examples/include.md is stale; run mdsh -w on it. Got:\n%s", got)
	}
}

func TestNoRun(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "d.md")
	marker := filepath.Join(dir, "ran")
	src := "# T\n\n```sh {#a}\ntouch " + marker + "\necho fresh-output\n```\n\n```output {for=a exit=0}\nsaved-output\n```\n"
	os.WriteFile(f, []byte(src+"\n```sh {<include file=inc.sh}\nstale-line\n```\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "inc.sh"), []byte("included-line\n"), 0o644)

	for _, tc := range []struct {
		flags      []string
		want, nope []string
	}{
		{[]string{"-n"}, []string{"# T", "touch", "saved-output", "included-line"}, []string{"```\nfresh-output", "stale-line"}},
		{[]string{"--no-run", "--hide-code"}, []string{"# T", "saved-output"}, []string{"touch"}},
		{[]string{"--no-run", "--hide-output"}, []string{"# T", "touch"}, []string{"saved-output"}},
	} {
		var out, errb bytes.Buffer
		if rc := mdsh(append(tc.flags, "--no-color", f), nil, &out, &errb); rc != 0 {
			t.Fatalf("%v: rc = %d, stderr %s", tc.flags, rc, errb.String())
		}
		for _, w := range tc.want {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%v: missing %q:\n%s", tc.flags, w, out.String())
			}
		}
		for _, w := range tc.nope {
			if strings.Contains(out.String(), w) {
				t.Errorf("%v: has %q:\n%s", tc.flags, w, out.String())
			}
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a block ran")
	}
	if rc := mdsh([]string{"-n", "-w", f}, nil, &bytes.Buffer{}, &bytes.Buffer{}); rc != 2 {
		t.Errorf("-n -w: rc = %d, want 2", rc)
	}
	var printed bytes.Buffer
	mdsh([]string{"-n", "--hide-code", "--no-color", f}, nil, &printed, &bytes.Buffer{})
	copyFile := filepath.Join(dir, "copy.md")
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"-n", "--hide-code", "-o", copyFile, f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("-n -o: rc = %d, stderr %s", rc, errb.String())
	}
	if got, _ := os.ReadFile(copyFile); out.Len() != 0 || string(got) != printed.String() {
		t.Errorf("-n -o wrote:\n%s\nwant what -n prints:\n%s", got, printed.String())
	}
	if b, _ := os.ReadFile(f); !strings.HasPrefix(string(b), src) || strings.Contains(string(b), "included-line") {
		t.Errorf("file changed:\n%s", b)
	}
}

func TestPDF(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.md")
	src := "# Doc\n\n```sh {#a}\necho hi\n```\n"
	os.WriteFile(in, []byte(src), 0o644)

	// --pdf replaces printing.
	out := filepath.Join(dir, "doc.pdf")
	var stdout, stderr bytes.Buffer
	if rc := mdsh([]string{"--pdf", out, "--hide-code", in}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want nothing", stdout.String())
	}
	if b, _ := os.ReadFile(out); !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Errorf("%s is not a PDF", out)
	}

	// --pdf - writes it to stdout; prose-only runs nothing.
	stdout.Reset()
	if rc := mdsh([]string{"--pdf", "-", "--hide-code", "--hide-output", in}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	if !bytes.HasPrefix(stdout.Bytes(), []byte("%PDF-")) {
		t.Errorf("stdout is not a PDF: %q", stdout.String()[:min(stdout.Len(), 20)])
	}

	// With -o both the markdown and the PDF are written.
	md := filepath.Join(dir, "out.md")
	os.Remove(out)
	stdout.Reset()
	if rc := mdsh([]string{"-o", md, "--pdf", out, in}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	if b, _ := os.ReadFile(md); !strings.Contains(string(b), "```output {for=a exit=0}\nhi\n```") {
		t.Errorf("out.md = %s", b)
	}
	if b, _ := os.ReadFile(out); !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Errorf("%s is not a PDF", out)
	}

	// -n lays out the saved output without running anything.
	os.Remove(out)
	stdout.Reset()
	if rc := mdsh([]string{"-n", "--pdf", out, md}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("-n: rc = %d, stderr %s", rc, stderr.String())
	}
	if b, _ := os.ReadFile(out); stdout.Len() != 0 || !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Errorf("-n --pdf: stdout %q, %s not a PDF", stdout.String(), out)
	}
}

func TestHTML(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.md")
	os.WriteFile(in, []byte("# Doc\n\n```sh {#a}\necho hi\n```\n"), 0o644)

	out := filepath.Join(dir, "doc.html")
	var stdout, stderr bytes.Buffer
	if rc := mdsh([]string{"--html", out, "--hide-code", in}, nil, &stdout, &stderr); rc != 0 {
		t.Fatalf("rc = %d, stderr %s", rc, stderr.String())
	}
	b, _ := os.ReadFile(out)
	if s := string(b); stdout.Len() != 0 || !strings.HasPrefix(s, "<!doctype html>") || !strings.Contains(s, "<title>in</title>") ||
		strings.Contains(s, "echo hi") || !strings.Contains(s, `<figure class="output"><pre><code>hi`) {
		t.Errorf("stdout %q, %s:\n%s", stdout.String(), out, s)
	}

	// -n --html - writes the saved output to stdout.
	stdout.Reset()
	if rc := mdsh([]string{"-n", "--html", "-", in}, nil, &stdout, &stderr); rc != 0 || !strings.Contains(stdout.String(), "echo hi") {
		t.Errorf("-n: rc = %d, stdout %s", rc, stdout.String())
	}

	if rc := mdsh([]string{"--html", out, "--pdf", "x.pdf", in}, nil, &stdout, &stderr); rc != 2 || !strings.Contains(stderr.String(), "can't be used together") {
		t.Errorf("--html --pdf: rc = %d, stderr %q", rc, stderr.String())
	}
}

func TestMode(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.md")
	os.WriteFile(in, []byte("```sh {#a}\necho \"mode=$MDSH_MODE\"\n```\n\n```sh {#b show={{$MDSH_MODE}}!=report}\necho not in reports\n```\n"), 0o644)
	for _, c := range []struct {
		args   []string
		want   string
		shownB bool
	}{
		{[]string{"--color=never", in}, "mode=running", true},
		{[]string{"--html", "-", in}, "mode=report", false},
		{[]string{"-n", "--html", "-", in}, "", false},
	} {
		var stdout, stderr bytes.Buffer
		if rc := mdsh(c.args, nil, &stdout, &stderr); rc != 0 || !strings.Contains(stdout.String(), c.want) {
			t.Errorf("%v: rc = %d, stdout %q, want %s", c.args, rc, stdout.String(), c.want)
		}
		if shown := strings.Contains(stdout.String(), "echo not in reports"); shown != c.shownB {
			t.Errorf("%v: block b shown = %v", c.args, shown)
		}
	}
}

// TestPDFExamples checks that the examples still lay out as PDFs.
func TestPDFExamples(t *testing.T) {
	for _, name := range []string{"hello.md", "include.md", "report.md", "terminal.md"} {
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.pdf")
			run := "--yes"
			if name == "terminal.md" {
				// Running it would rewrite its recordings, whose
				// timings differ every run.
				run = "-n"
			}
			var stdout, stderr bytes.Buffer
			mdsh([]string{run, "--pdf", out, filepath.Join("..", "..", "examples", name)}, nil, &stdout, &stderr)
			if b, err := os.ReadFile(out); err != nil || !bytes.HasPrefix(b, []byte("%PDF-")) {
				t.Errorf("no PDF (%v), stderr %s", err, stderr.String())
			}
		})
	}
}

// TestWriteCopies checks that -w refills output copies, and warns about a
// copy for a block that doesn't exist.
func TestWriteCopies(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	src := "```sh {#a}\necho \"$N\"\n```\n\n## Summary\n\n```output {for=a exit=0}\nold\n```\n\n```output {for=gone exit=0}\nx\n```\n"
	os.WriteFile(f, []byte(src), 0o644)
	t.Setenv("N", "1")
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"-w", f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d, stderr %q", rc, errb.String())
	}
	if !strings.Contains(errb.String(), `warning: line 11: output copy for unknown block "gone"`) {
		t.Errorf("stderr = %q", errb.String())
	}
	t.Setenv("N", "2")
	if rc := mdsh([]string{"-w", f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d, stderr %q", rc, errb.String())
	}
	got, _ := os.ReadFile(f)
	want := "```sh {#a}\necho \"$N\"\n```\n\n```output {for=a exit=0}\n2\n```\n\n## Summary\n\n```output {for=a exit=0}\n2\n```\n\n```output {for=gone exit=0}\nx\n```\n"
	if string(got) != want {
		t.Errorf("file:\ngot  %q\nwant %q", got, want)
	}
}

// TestCallerEnvStaysOut runs the v1.2.1 review's repro for finding 2:
// {{$VAR}} reads only variables the person running mdsh exposed, as
// env.share passes them, and -w never stores them.
func TestCallerEnvStaysOut(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "s3cr3t")
	dir := t.TempDir()
	shared := filepath.Join(dir, "s.md")
	os.WriteFile(shared, []byte("---\nmdsh:\n  env:\n    share: [PATH]\n---\nToken: {{$SECRET_TOKEN}}\n\n```sh\necho \"[$SECRET_TOKEN]\"\n```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--no-color", shared}, nil, &out, &errb); rc != 0 || strings.Contains(out.String(), "s3cr3t") {
		t.Errorf("print with share: rc = %d, stdout %q, stderr %s", rc, out.String(), errb.String())
	}

	f := filepath.Join(dir, "n.md")
	os.WriteFile(f, []byte("Token: {{$SECRET_TOKEN}}\n"), 0o644)
	for _, args := range [][]string{{"-n", "--html", "-"}, {"-n", "--html", filepath.Join(dir, "r.html")}} {
		out.Reset()
		if rc := mdsh(append(args, f), nil, &out, &errb); rc != 0 {
			t.Fatalf("%v: rc = %d, stderr %s", args, rc, errb.String())
		}
		got := out.String()
		if p := args[len(args)-1]; p != "-" {
			b, _ := os.ReadFile(p)
			got = string(b)
		}
		if strings.Contains(got, "s3cr3t") {
			t.Errorf("%v: the report holds the token", args)
		}
	}
	out.Reset()
	if rc := mdsh([]string{"-n", "--expose", "SECRET_TOKEN", "--html", "-", f}, nil, &out, &errb); rc != 0 || !strings.Contains(out.String(), "s3cr3t") {
		t.Errorf("--expose: rc = %d, stdout %q", rc, out.String())
	}
	o := filepath.Join(dir, "o.md")
	for _, args := range [][]string{{"-w", "--expose", "SECRET_TOKEN", f}, {"-o", o, f}} {
		if rc := mdsh(args, nil, &out, &errb); rc != 0 {
			t.Fatalf("%v: rc = %d, stderr %s", args, rc, errb.String())
		}
	}
	for _, p := range []string{f, o} {
		if b, _ := os.ReadFile(p); string(b) != "Token: {{$SECRET_TOKEN}}\n" {
			t.Errorf("%s: %q", p, b)
		}
	}
	// Printing reads only what the person exposed, whatever the
	// document's front matter asks for.
	all := filepath.Join(dir, "all.md")
	os.WriteFile(all, []byte("---\nmdsh:\n  env:\n    share: [\"...\"]\n---\nToken: {{$SECRET_TOKEN}}\n"), 0o644)
	out.Reset()
	if rc := mdsh([]string{"-n", "--no-color", all}, nil, &out, &errb); rc != 0 || !strings.Contains(out.String(), "Token: {{$SECRET_TOKEN}}") {
		t.Errorf("print: rc = %d, stdout %q", rc, out.String())
	}
	t.Setenv("MDSH_EXPOSE", "SECRET_TOKEN")
	out.Reset()
	if rc := mdsh([]string{"-n", "--no-color", all}, nil, &out, &errb); rc != 0 || !strings.Contains(out.String(), "Token: s3cr3t") {
		t.Errorf("MDSH_EXPOSE: rc = %d, stdout %q", rc, out.String())
	}
}

// TestConfirmShowsControlChars checks that escape sequences in a block's
// code are shown at the question instead of acted on, so a line that
// would erase itself on the terminal can't hide code that runs.
func TestConfirmShowsControlChars(t *testing.T) {
	b := &doc.Block{ID: "x\x1b[2K", Body: "echo visible\ntouch /tmp/PWNED #\r\x1b[2K\n", Attrs: doc.Attrs{"confirm": "Ok?\x1b[1A"}}
	for _, quiet := range []bool{false, true} {
		var out bytes.Buffer
		p := newPrompter(strings.NewReader("s\n"), &out)
		p.quiet = quiet
		p.confirm(b)
		got := out.String()
		if strings.ContainsAny(got, "\x1b\r") {
			t.Errorf("quiet %v: raw control characters reach the terminal: %q", quiet, got)
		}
		for _, want := range []string{"touch /tmp/PWNED #^M^[[2K", "control characters", "Ok?^[[1A", "block x^[[2K"} {
			if !strings.Contains(got, want) {
				t.Errorf("quiet %v: prompt %q lacks %q", quiet, got, want)
			}
		}
	}
	var out bytes.Buffer
	newPrompter(strings.NewReader("s\n"), &out).depend(b, &doc.Block{ID: "y"})
	if strings.ContainsAny(out.String(), "\x1b\r") || !strings.Contains(out.String(), "^M^[[2K") {
		t.Errorf("depend prompt = %q", out.String())
	}
}

// TestConfirmShowsHiddenCode checks that a quiet question, asked after
// the streamed document, still shows code the document left out.
func TestConfirmShowsHiddenCode(t *testing.T) {
	b := &doc.Block{ID: "evil", Body: "touch /tmp/PWNED\n"}
	var out bytes.Buffer
	p := newPrompter(strings.NewReader("s\n"), &out)
	p.quiet = true
	p.shown = func(*doc.Block) bool { return false }
	p.confirm(b)
	if !strings.Contains(out.String(), "touch /tmp/PWNED") {
		t.Errorf("hidden code not shown: %q", out.String())
	}
	out.Reset()
	p.shown = func(*doc.Block) bool { return true }
	p.confirm(b)
	if strings.Contains(out.String(), "touch /tmp/PWNED") {
		t.Errorf("printed code shown twice: %q", out.String())
	}
}

// TestStreamCodeShown checks which blocks the streamed document shows.
func TestStreamCodeShown(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#a}\necho a\n```\n\n```sh {#b show=false}\necho b\n```\n\n<!-- mdsh:if false -->\n```sh {#c}\necho c\n```\n<!-- mdsh:end -->\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := newStreamer(d, &bytes.Buffer{}, false, render.Options{Terminal: true})
	for i, want := range []bool{true, false, false} {
		if got := s.codeShown(d.Blocks[i]); got != want {
			t.Errorf("block %s: shown %v, want %v", d.Blocks[i].ID, got, want)
		}
	}
}

// TestPrintEscapesCode checks that printing the document to the
// terminal shows control characters in code, and -w keeps them.
func TestPrintEscapesCode(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	src := "```sh\necho a #\r\x1b[2K\n```\n"
	os.WriteFile(f, []byte(src), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"-n", "--no-color", f}, nil, &out, &errb); rc != 0 || strings.Contains(out.String(), "\x1b") || !strings.Contains(out.String(), "echo a #^M^[[2K") {
		t.Errorf("-n: rc = %d, stdout %q, stderr %s", rc, out.String(), errb.String())
	}
	if rc := mdsh([]string{"-w", f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("-w: rc = %d, stderr %s", rc, errb.String())
	}
	if got, _ := os.ReadFile(f); !strings.Contains(string(got), "echo a #\r\x1b[2K\n") {
		t.Errorf("-w changed the code: %q", got)
	}
}
