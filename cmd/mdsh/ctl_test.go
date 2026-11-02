//go:build !windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for mdsh, so blocks that call
// mdsh ctl reach the run under test.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "mdsh" {
		os.Exit(mdsh(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// mdshOnPath puts this test binary on $PATH as mdsh.
func mdshOnPath(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(exe, filepath.Join(dir, "mdsh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCtl(t *testing.T) {
	mdshOnPath(t)
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#a}\n"+
		"mdsh ctl set env prod\n"+
		"mdsh ctl status --step 1 --of 2 building\n"+
		"echo \"a: ${MDSH_VAR_env-unset} $(mdsh ctl get env)\"\n"+
		"mdsh ctl set name 2>/dev/null; echo \"args=$?\"\n"+
		"mdsh ctl set 'no good' x 2>/dev/null; echo \"name=$?\"\n"+
		"mdsh ctl set picked x 2>/dev/null; echo \"input=$?\"\n"+
		"printf 'two\\nlines\\n' | mdsh ctl set multi -\n"+
		"```\n\n"+
		"```textbox {var=picked}\nhi\n```\n\n"+
		"```sh {#b run={{$MDSH_VAR_env}}==prod}\n"+
		"echo \"b: $MDSH_VAR_env\"\n"+
		"echo \"$MDSH_VAR_multi\"\n"+
		"mdsh ctl unset env\n"+
		"```\n\n"+
		"```sh {#c run={{$MDSH_VAR_env}}==prod}\necho c ran\n```\n\n"+
		"```sh {#d}\n"+
		"echo \"d: ${MDSH_VAR_env-unset}\"\n"+
		"echo \"$MDSH_TOKEN\" > tok\n"+
		"```\n\n"+
		"```sh {#e}\n"+
		"MDSH_TOKEN=$(cat tok) mdsh ctl get multi 2>/dev/null; echo \"old token=$?\"\n"+
		"```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--no-color", f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d\n%s\n%s", rc, out.String(), errb.String())
	}
	for _, want := range []string{
		"a: unset prod\nargs=2\nname=2\ninput=2\n",
		"b: prod\ntwo\nlines\n",
		"d: unset\n",
		"old token=2\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "{for=c ") {
		t.Errorf("c ran after unset:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "a: [1/2] building\n") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestCtlOutsideMdsh(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := ctlCall([]string{"get", "x"}, "", "", nil, &out, &errb); rc != 3 {
		t.Errorf("get: rc = %d, %s", rc, errb.String())
	}
	if rc := ctlCall([]string{"status", "hi"}, "", "", nil, &out, &errb); rc != 0 {
		t.Errorf("status: rc = %d", rc)
	}
	if rc := ctlCall([]string{"frob"}, "", "", nil, &out, &errb); rc != 2 {
		t.Errorf("unknown: rc = %d", rc)
	}
	if rc := ctlCall([]string{"status", "--step", "1", "hi"}, "", "", nil, &out, &errb); rc != 2 {
		t.Errorf("--step without --of: rc = %d", rc)
	}
}
