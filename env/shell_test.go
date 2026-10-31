package env

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shells are the shells on this machine to run a test in.
func shells(t *testing.T) []string {
	t.Helper()
	var found []string
	for _, name := range []string{"sh", "bash", "zsh"} {
		if p, err := exec.LookPath(name); err == nil {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		t.Skip("no shell found")
	}
	return found
}

func TestErrorsNameTheBlock(t *testing.T) {
	for _, path := range shells(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			sh, err := Start(Config{Path: path, Env: []string{"PATH=" + os.Getenv("PATH")}})
			if err != nil {
				t.Fatal(err)
			}
			defer sh.Close()
			var out, errb bytes.Buffer
			rc, _, err := sh.Run(context.Background(), "\n./no-such-program\n", &out, &errb)
			if err != nil {
				t.Fatal(err)
			}
			if rc != 127 {
				t.Errorf("status %d, want 127", rc)
			}
			got := errb.String()
			if strings.Contains(got, sh.tmp) || !strings.Contains(got, "block") || !strings.Contains(got, "2") {
				t.Errorf("stderr %q should name the block and line 2, not the temp file", got)
			}
		})
	}
}

func TestCdDashAcrossRuns(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range shells(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			sh, err := Start(Config{Path: path, Dir: root, Env: []string{"PATH=" + os.Getenv("PATH")}})
			if err != nil {
				t.Fatal(err)
			}
			defer sh.Close()
			ctx := context.Background()
			for _, block := range []string{"cd a/b", "true", "cd - >/dev/null"} {
				if rc, _, err := sh.Run(ctx, block, &bytes.Buffer{}, &bytes.Buffer{}); err != nil || rc != 0 {
					t.Fatalf("%q: status %d, %v", block, rc, err)
				}
			}
			var out bytes.Buffer
			sh.Run(ctx, "pwd -P", &out, &bytes.Buffer{})
			want, _ := filepath.EvalSymlinks(root)
			if got := strings.TrimSpace(out.String()); got != want {
				t.Errorf("after cd -: in %s, want %s", got, want)
			}
		})
	}
}

func TestRenamerSplitWrites(t *testing.T) {
	var b bytes.Buffer
	r := newRenamer(&b, "/tmp/x/block.sh", "block")
	for _, p := range []string{"err /tm", "p/x/blo", "ck.sh:2: no\n", "/tmp/y /"} {
		r.Write([]byte(p))
	}
	r.Flush()
	if want := "err block:2: no\n/tmp/y /"; b.String() != want {
		t.Errorf("got %q, want %q", b.String(), want)
	}
}
