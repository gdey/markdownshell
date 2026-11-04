package shellrc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var rcs = map[string]RC{"zsh": Zsh, "bash": Bash}

func TestSet(t *testing.T) {
	for sh, rc := range rcs {
		for name, orig := range map[string]string{
			"missing":     "",
			"no newline":  "alias ll='ls -l'",
			"has newline": "alias ll='ls -l'\nexport X=1\n",
		} {
			t.Run(sh+"/"+name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), rc.name)
				if orig != "" {
					if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				for range 2 { // a second Set changes nothing
					if err := rc.Set(path, true); err != nil {
						t.Fatal(err)
					}
				}
				src, _ := os.ReadFile(path)
				if on, err := rc.Installed(path); err != nil || !on || strings.Count(string(src), rc.Wrapper) != 1 || strings.Count(string(src), rc.Completion) != 1 {
					t.Fatalf("after Set(true): installed %v, err %v, file %q", on, err, src)
				}
				if !strings.HasPrefix(string(src), orig) {
					t.Errorf("file %q lost its start %q", src, orig)
				}
				if err := rc.Set(path, false); err != nil {
					t.Fatal(err)
				}
				src, _ = os.ReadFile(path)
				want := orig
				if name == "no newline" {
					want += "\n"
				}
				if string(src) != want {
					t.Errorf("after Set(false) = %q, want %q", src, want)
				}
				if fi, err := os.Stat(path); err == nil && orig != "" && fi.Mode().Perm() != 0o600 {
					t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
				}
			})
		}
	}
}

func TestSetKeepsLaterLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	if err := Zsh.Set(path, true); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("export Y=2\n")
	f.Close()
	if err := Zsh.Set(path, false); err != nil {
		t.Fatal(err)
	}
	if src, _ := os.ReadFile(path); string(src) != "export Y=2\n" {
		t.Errorf("file = %q", src)
	}
}

func TestRemoveMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	if err := Zsh.Set(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Set(false) created the file: %v", err)
	}
}

// A block written by an older mdsh-gui, with only the wrapper, gets the
// completion line, and the lines around it stay.
func TestUpdateOldBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	old := "export X=1\n\n" + begin + "\n" + Zsh.Wrapper + "\n" + end + "\nexport Y=2\n"
	os.WriteFile(path, []byte(old), 0o600)
	for range 2 {
		if err := Zsh.Update(path); err != nil {
			t.Fatal(err)
		}
	}
	src, _ := os.ReadFile(path)
	want := "export X=1\n\n" + Zsh.block() + "export Y=2\n"
	if string(src) != want {
		t.Errorf("file = %q, want %q", src, want)
	}

	// Update leaves a file without the block alone.
	none := filepath.Join(t.TempDir(), ".zshrc")
	if err := Zsh.Update(none); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(none); !os.IsNotExist(err) {
		t.Errorf("Update created the file: %v", err)
	}
}

func TestZshPath(t *testing.T) {
	t.Setenv("ZDOTDIR", "/z")
	if p, _ := Zsh.Path(); p != "/z/.zshrc" {
		t.Errorf("zsh path = %q", p)
	}
	t.Setenv("HOME", "/h")
	if p, _ := Bash.Path(); p != "/h/.bashrc" {
		t.Errorf("bash path = %q", p)
	}
}

// The wrapper defines a function even when an alias named mdsh exists.
func TestWrapperDefinesFunction(t *testing.T) {
	for sh, script := range map[string]string{
		"zsh":  "alias mdsh=true\n" + Zsh.Wrapper + "\nunalias mdsh\nwhence -w mdsh",
		"bash": "shopt -s expand_aliases\nalias mdsh=true\n" + Bash.Wrapper + "\nunalias mdsh\ntype -t mdsh",
	} {
		path, err := exec.LookPath(sh)
		if err != nil {
			t.Logf("no %s", sh)
			continue
		}
		out, err := exec.Command(path, "-c", script).CombinedOutput()
		if want := map[string]string{"zsh": "mdsh: function", "bash": "function"}[sh]; err != nil || strings.TrimSpace(string(out)) != want {
			t.Errorf("%s: %v, %q", sh, err, out)
		}
	}
}

// Where completion can't load (zsh before compinit, bash not
// interactive) the line must not run mdsh or print an error.
func TestCompletionLineQuiet(t *testing.T) {
	for sh, args := range map[string][]string{"zsh": {"-f", "-c"}, "bash": {"--norc", "-c"}} {
		path, err := exec.LookPath(sh)
		if err != nil {
			t.Logf("no %s", sh)
			continue
		}
		out, err := exec.Command(path, append(args, "function mdsh { echo ran; }\n"+rcs[sh].Completion+"\nexit 0")...).CombinedOutput()
		if err != nil || len(out) != 0 {
			t.Errorf("%s: %v, %q", sh, err, out)
		}
	}
}

// What the bash wrapper passes on must parse in /bin/sh, even with the
// completion loaded, whose function uses bash arrays.
func TestBashWrapperSourceParsesInSh(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	dir := t.TempDir()
	// A stand-in mdsh that saves the file --source names.
	os.WriteFile(filepath.Join(dir, "mdsh"), []byte("#!/bin/sh\ncat \"$2\" > \"$(dirname \"$0\")/src.sh\"\n"), 0o755)
	script := "hi() { echo hi; }\n_mdsh() { COMPREPLY=(x); }\n" + Bash.Wrapper + "\nmdsh file.md"
	cmd := exec.Command(bash, "--norc", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash: %v, %s", err, out)
	}
	src, _ := os.ReadFile(filepath.Join(dir, "src.sh"))
	if !strings.Contains(string(src), "hi ()") || strings.Contains(string(src), "_mdsh") || strings.Contains(string(src), "mdsh ()") {
		t.Errorf("source = %q", src)
	}
	if out, err := exec.Command("sh", "-n", filepath.Join(dir, "src.sh")).CombinedOutput(); err != nil {
		t.Errorf("sh -n: %v, %s", err, out)
	}
}
