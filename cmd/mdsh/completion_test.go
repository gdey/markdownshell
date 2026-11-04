package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCompletionScriptsParse(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		var out, errb bytes.Buffer
		if rc := mdsh([]string{"completion", sh}, nil, &out, &errb); rc != 0 || out.Len() == 0 {
			t.Fatalf("%s: rc = %d, stderr %s", sh, rc, errb.String())
		}
		path, err := exec.LookPath(sh)
		if err != nil {
			t.Logf("%s not installed; skipping syntax check", sh)
			continue
		}
		f := filepath.Join(t.TempDir(), "comp")
		os.WriteFile(f, out.Bytes(), 0o644)
		if b, err := exec.Command(path, "-n", f).CombinedOutput(); err != nil {
			t.Errorf("%s -n: %v\n%s", sh, err, b)
		}
	}
	var errb bytes.Buffer
	if rc := mdsh([]string{"completion", "fish"}, nil, &bytes.Buffer{}, &errb); rc == 0 {
		t.Error("fish should be an error")
	}
}

func TestBlocksCmd(t *testing.T) {
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#a}\n```\n```go\n```\n```sh {<include file=x}\n```\n```sh\n```\n```zsh {#z}\n```\n"), 0o644)
	var out bytes.Buffer
	if rc := mdsh([]string{"__blocks", f}, nil, &out, &bytes.Buffer{}); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if got := out.String(); got != "a\nz\nblock-3\n" {
		t.Errorf("blocks = %q", got)
	}
}

// The zsh wrapper puts --source ahead of everything, so a subcommand is
// not always the first argument.
func TestSubcommandAfterSource(t *testing.T) {
	defs := filepath.Join(t.TempDir(), "defs.sh")
	os.WriteFile(defs, []byte("hi() { echo hi-from-source; }\n"), 0o644)
	for _, args := range [][]string{
		{"--source", defs, "completion", "zsh"},
		{"--source=" + defs, "completion", "bash"},
		{"--source", defs, "--source", defs, "completion", "zsh"},
	} {
		var out, errb bytes.Buffer
		if rc := mdsh(args, nil, &out, &errb); rc != 0 || !bytes.Contains(out.Bytes(), []byte("mdsh")) {
			t.Errorf("%q: rc = %d, stderr %s", args, rc, errb.String())
		}
	}
	var errb bytes.Buffer
	if rc := mdsh([]string{"--source", defs, "completion"}, nil, &bytes.Buffer{}, &errb); rc == 0 || !bytes.Contains(errb.Bytes(), []byte("usage: mdsh completion")) {
		t.Errorf("bare completion: rc = %d, stderr %s", rc, errb.String())
	}

	// run keeps the --source flags.
	doc := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(doc, []byte("```sh\nhi\n```\n"), 0o644)
	var out bytes.Buffer
	errb.Reset()
	if rc := mdsh([]string{"--source", defs, "run", doc}, nil, &out, &errb); rc != 0 || !bytes.Contains(out.Bytes(), []byte("hi-from-source")) {
		t.Errorf("run: rc = %d, out %s, stderr %s", rc, out.String(), errb.String())
	}
}
