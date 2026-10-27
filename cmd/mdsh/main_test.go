package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
)

func TestConfirmPrompt(t *testing.T) {
	b := &doc.Block{ID: "deploy", Body: "echo hi\n", Attrs: doc.Attrs{"confirm": ""}}
	for in, want := range map[string]bool{"r\n": true, "yes\n": true, "\n": false, "s\n": false, "huh\nrun\n": true, "": false} {
		var out bytes.Buffer
		if got := confirm(strings.NewReader(in), &out, b); got != want {
			t.Errorf("input %q: got %v, want %v", in, got, want)
		}
		if !strings.Contains(out.String(), "echo hi") || !strings.Contains(out.String(), "Run this block?") {
			t.Errorf("prompt = %q", out.String())
		}
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
