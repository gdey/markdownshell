package zshrc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSet(t *testing.T) {
	for name, orig := range map[string]string{
		"missing":     "",
		"no newline":  "alias ll='ls -l'",
		"has newline": "alias ll='ls -l'\nexport X=1\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".zshrc")
			if orig != "" {
				if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 { // a second Set changes nothing
				if err := Set(path, true); err != nil {
					t.Fatal(err)
				}
			}
			src, _ := os.ReadFile(path)
			if on, err := Installed(path); err != nil || !on || strings.Count(string(src), Wrapper) != 1 {
				t.Fatalf("after Set(true): installed %v, err %v, file %q", on, err, src)
			}
			if !strings.HasPrefix(string(src), orig) {
				t.Errorf("file %q lost its start %q", src, orig)
			}
			if err := Set(path, false); err != nil {
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

func TestSetKeepsLaterLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	if err := Set(path, true); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("export Y=2\n")
	f.Close()
	if err := Set(path, false); err != nil {
		t.Fatal(err)
	}
	if src, _ := os.ReadFile(path); string(src) != "export Y=2\n" {
		t.Errorf("file = %q", src)
	}
}

func TestRemoveMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	if err := Set(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("Set(false) created the file: %v", err)
	}
}

func TestWrapperDefinesFunction(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("no zsh")
	}
	// An alias named mdsh mustn't break the definition.
	out, err := exec.Command(zsh, "-c", "alias mdsh=true\n"+Wrapper+"\nunalias mdsh\nwhence -w mdsh").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "mdsh: function" {
		t.Errorf("zsh: %v, %q", err, out)
	}
}
