package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/shellrc"
)

// The setting writes the .zshrc, and the .bashrc only when there is one.
func TestSetZshWrapper(t *testing.T) {
	for _, hasBashrc := range []bool{false, true} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("ZDOTDIR", "")
		zshrc, bashrc := filepath.Join(home, ".zshrc"), filepath.Join(home, ".bashrc")
		if hasBashrc {
			os.WriteFile(bashrc, []byte("export X=1\n"), 0o644)
		}
		a := &App{}
		w, err := a.SetZshWrapper(true)
		if err != nil || !w.On || w.Path != zshrc {
			t.Fatalf("bashrc %v: SetZshWrapper(true) = %+v, %v", hasBashrc, w, err)
		}
		if src, _ := os.ReadFile(zshrc); !strings.Contains(string(src), shellrc.Zsh.Completion) {
			t.Errorf(".zshrc = %q", src)
		}
		src, err := os.ReadFile(bashrc)
		switch {
		case !hasBashrc && (err == nil || w.BashPath != ""):
			t.Errorf("made a .bashrc: %q, BashPath %q", src, w.BashPath)
		case hasBashrc && (w.BashPath != bashrc || !strings.Contains(string(src), shellrc.Bash.Wrapper) || !strings.Contains(string(src), shellrc.Bash.Completion)):
			t.Errorf(".bashrc = %q, BashPath %q", src, w.BashPath)
		}
		if w, err = a.SetZshWrapper(false); err != nil || w.On {
			t.Fatalf("SetZshWrapper(false) = %+v, %v", w, err)
		}
		if hasBashrc {
			if src, _ := os.ReadFile(bashrc); string(src) != "export X=1\n" {
				t.Errorf(".bashrc after off = %q", src)
			}
		}
	}
}
