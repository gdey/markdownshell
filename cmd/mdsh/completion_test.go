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
	os.WriteFile(f, []byte("```sh {#a}\n```\n```go\n```\n```sh\n```\n"), 0o644)
	var out bytes.Buffer
	if rc := mdsh([]string{"__blocks", f}, nil, &out, &bytes.Buffer{}); rc != 0 {
		t.Fatalf("rc = %d", rc)
	}
	if got := out.String(); got != "a\nblock-3\n" {
		t.Errorf("blocks = %q", got)
	}
}
