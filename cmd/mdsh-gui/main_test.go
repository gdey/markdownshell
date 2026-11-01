package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestArgDir(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "doc.md")
	if err := os.WriteFile(file, []byte("# Doc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	for _, tc := range []struct {
		args []string
		want string
		err  bool
	}{
		{nil, "", false},
		{[]string{file}, "", false},
		{[]string{file, "a", "b"}, "", false},
		{[]string{"missing.md"}, "", false},
		{[]string{root}, root, false},
		{[]string{"."}, root, false},
		{[]string{root, "a"}, "", true},
	} {
		got, err := argDir(tc.args)
		if got != "" {
			got, _ = filepath.EvalSymlinks(got)
		}
		if (err != nil) != tc.err || got != tc.want {
			t.Errorf("argDir(%q) = %q, %v; want %q, error %v", tc.args, got, err, tc.want, tc.err)
		}
	}
}

func TestParseFlags(t *testing.T) {
	dir := t.TempDir()
	defs := filepath.Join(dir, "defs.sh")
	if err := os.WriteFile(defs, []byte("alias ll='ls -l'"), 0o644); err != nil {
		t.Fatal(err)
	}
	o, err := parseFlags([]string{"--source", defs, "--source", defs, "--step", "--only", "b", "doc.md", "--x", "y"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if want := "alias ll='ls -l'\nalias ll='ls -l'\n"; o.source != want {
		t.Errorf("source = %q, want %q", o.source, want)
	}
	if !o.run.Step || o.run.Only != "b" || o.run.StopOnError {
		t.Errorf("run = %+v", o.run)
	}
	if !slices.Equal(o.args, []string{"doc.md", "--x", "y"}) || !slices.Equal(o.run.Args, []string{"--x", "y"}) {
		t.Errorf("args = %q, run.Args = %q", o.args, o.run.Args)
	}

	if _, err := parseFlags([]string{"-h"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: err = %v, want flag.ErrHelp", err)
	}
	if _, err := parseFlags([]string{"--help"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("--help: err = %v, want flag.ErrHelp", err)
	}
	if o, err := parseFlags([]string{"--version"}, io.Discard); err != nil || !o.version {
		t.Errorf("--version: %+v, %v", o, err)
	}
	if o, err := parseFlags([]string{"--serve=stdio", "doc.md", "a"}, io.Discard); err != nil || !o.stdio || !slices.Equal(o.args, []string{"doc.md", "a"}) {
		t.Errorf("--serve=stdio: %+v, %v", o, err)
	}
	if _, err := parseFlags([]string{"--serve=localhost:9000"}, io.Discard); err == nil {
		t.Error("--serve with an address: no error")
	}
	if _, err := parseFlags([]string{"--serve=stdio", "--step", "doc.md"}, io.Discard); err == nil {
		t.Error("--serve=stdio --step: no error")
	}
	if _, err := parseFlags([]string{"--source", filepath.Join(dir, "missing")}, io.Discard); err == nil {
		t.Error("missing --source file: no error")
	}
	if _, err := parseFlags([]string{"--step"}, io.Discard); err == nil {
		t.Error("--step with no file: no error")
	}
	if o, err := parseFlags([]string{"--source", defs}, io.Discard); err != nil || len(o.args) != 0 {
		t.Errorf("--source alone: %+v, %v", o, err)
	}
}
