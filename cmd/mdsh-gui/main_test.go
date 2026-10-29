package main

import (
	"os"
	"path/filepath"
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
