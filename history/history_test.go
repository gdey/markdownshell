package history_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/history"
	"github.com/gdey/markdownshell/run"
)

// session runs src's first block in dir and returns the session.
func session(t *testing.T, dir, src string) (*run.Session, *doc.Document) {
	t.Helper()
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "doc.md")
	s, err := run.NewSession(d, run.Options{File: file, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if len(d.Blocks) > 0 {
		if _, err := s.RunBlock(context.Background(), 0); err != nil {
			t.Fatal(err)
		}
	}
	return s, d
}

func TestComplete(t *testing.T) {
	for _, lang := range []string{"sh", "zsh"} {
		t.Run(lang, func(t *testing.T) {
			if _, err := exec.LookPath(lang); err != nil {
				t.Skip(err)
			}
			dir := t.TempDir()
			for _, p := range []string{"sub/alpha one.txt", "sub/alps/x", "sub/beta"} {
				if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, p), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			s, _ := session(t, dir, "```"+lang+"\ncd sub; MDSH_TEST_VAR=1; alias mdshtestalias=true\n```\n")
			for _, tc := range []struct {
				before, word string
				want         []string
				some         bool
			}{
				{"ls al", "al", []string{`alpha\ one.txt`, "alps/"}, false},
				{"cat alpha\\ o", "alpha\\ o", []string{`alpha\ one.txt`}, false},
				{"cat alps/", "alps/", []string{"alps/x"}, false},
				{"echo $MDSH_TEST_V", "$MDSH_TEST_V", []string{"$MDSH_TEST_VAR"}, false},
				{"echo ${MDSH_TEST_V", "${MDSH_TEST_V", []string{"${MDSH_TEST_VAR}"}, false},
				{"mdshtesta", "mdshtesta", []string{"mdshtestalias"}, false},
				{"true && ech", "ech", []string{"echo"}, true},
				{"ls nothing", "nothing", nil, false},
			} {
				c := history.Complete(context.Background(), s, tc.before, lang == "zsh")
				if c.Word != tc.word {
					t.Errorf("%q: word %q, want %q", tc.before, c.Word, tc.word)
				}
				if tc.some {
					for _, w := range tc.want {
						if !slices.Contains(c.Items, w) {
							t.Errorf("%q: %q not in %q", tc.before, w, c.Items)
						}
					}
				} else if !slices.Equal(c.Items, tc.want) {
					t.Errorf("%q: items %q, want %q", tc.before, c.Items, tc.want)
				}
			}
		})
	}
}

func TestAskPrompt(t *testing.T) {
	hash, zhash := "$", "%"
	if os.Geteuid() == 0 {
		hash, zhash = "#", "#"
	}
	for _, tc := range []struct {
		name, src, ps1, ps2 string
	}{
		{"sh", "```sh\ncd /; PS1='$PWD> '; PS2='more> '\n```\n", "/> ", "more> "},
		{"bash", "---\nmdsh:\n  shell: bash\n---\n```sh\ncd /; PS1='\\[\\e[31m\\]\\w\\$\\[\\e[0m\\] '\n```\n", "\x1b[31m/" + hash + "\x1b[0m ", ""},
		{"zsh", "```zsh\ncd /; PS1='%F{red}%~%f %# '; PS2='%_> '\nprecmd() { PS1=\"[hook] $PS1\" }\n```\n", "[hook] \x1b[31m/\x1b[39m " + zhash + " ", "> "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := exec.LookPath(tc.name); err != nil {
				t.Skip(err)
			}
			if tc.name == "bash" {
				v, _ := exec.Command("bash", "-c", `echo $((BASH_VERSINFO[0]*100+BASH_VERSINFO[1]))`).Output()
				if n, _ := strconv.Atoi(strings.TrimSpace(string(v))); n < 404 {
					t.Skipf("bash %s is older than 4.4", strings.TrimSpace(string(v)))
				}
			}
			s, d := session(t, t.TempDir(), tc.src)
			zsh := history.Lang(history.Shell(d, "")) == "zsh"
			p, err := history.AskPrompt(context.Background(), s, zsh)
			if err != nil {
				t.Fatal(err)
			}
			if p.PS1 != tc.ps1 || p.PS2 != tc.ps2 {
				t.Errorf("prompt %q, %q; want %q, %q", p.PS1, p.PS2, tc.ps1, tc.ps2)
			}
			if again, _ := history.AskPrompt(context.Background(), s, zsh); again != p {
				t.Errorf("again: %q", again)
			}
		})
	}
}

func TestWhole(t *testing.T) {
	for in, want := range map[string]bool{
		"echo hi":                 true,
		"echo )":                  true, // an error, but nothing to wait for
		"# it's a note":           true,
		"":                        true,
		"echo 'abc":               false,
		"if true; then":           false,
		"if true; then\necho\nfi": true,
		"for x in 1 2; do":        false,
		"echo one \\":             false,
		"echo one \\\\":           true,
		"ls |":                    false,
		"true &&":                 false,
		"cat <<EOF\nhi":           false,
		"cat <<'EOF'\nhi\nEOF":    true,
		"f() {":                   false,
	} {
		if got := history.Whole("/bin/sh", in); got != want {
			t.Errorf("Whole(%q) = %v", in, got)
		}
	}
}

func TestBackground(t *testing.T) {
	for in, want := range map[string]bool{
		"sleep 1 &": true, "a && b": false, "echo \\&": false, "cmd >&": false, "a |&": false, "x&": true, "echo hi": false,
	} {
		if _, got := history.Background(in); got != want {
			t.Errorf("Background(%q) = %v", in, got)
		}
	}
}

func TestNote(t *testing.T) {
	for in, want := range map[string]string{
		"# a note":       "a note",
		"#one\n  # two":  "one\ntwo",
		"echo # not one": "",
		"# one\nls":      "",
	} {
		got, ok := history.Note(in)
		if ok != (want != "") || got != want {
			t.Errorf("Note(%q) = %q, %v", in, got, ok)
		}
	}
}

func TestAddNote(t *testing.T) {
	d, _ := doc.Parse([]byte("# Log\n"))
	if err := history.AddNote(d, "first", false); err != nil {
		t.Fatal(err)
	}
	if err := history.AddNote(d, "carried on", true); err != nil {
		t.Fatal(err)
	}
	if want := "# Log\n\nfirst\ncarried on\n"; string(d.Src) != want {
		t.Errorf("src = %q, want %q", d.Src, want)
	}
	open, _ := doc.Parse([]byte("```sh\necho open\n"))
	if err := history.AddNote(open, "note", false); err == nil {
		t.Error("added a note inside an open fence")
	}
	if string(open.Src) != "```sh\necho open\n" {
		t.Errorf("src = %q", open.Src)
	}
}

func TestNextID(t *testing.T) {
	d, _ := doc.Parse([]byte("```sh {#history-7}\ntrue\n```\n```sh {#history-x}\n```\n"))
	if got := history.NextID(d); got != "history-8" {
		t.Errorf("NextID = %q", got)
	}
	// A moved history block keeps its old file until it runs again.
	moved, _ := doc.Parse([]byte("```sh {#history-2}\ntrue\n```\n```output {for=block-x}\n![](d.md.assets/history-9.png)\n```\n"))
	if got := history.NextID(moved); got != "history-10" {
		t.Errorf("NextID past a moved block's file = %q", got)
	}
	if got := history.Info("zsh", "history-8", true); got != "zsh {#history-8 skip background links}" {
		t.Errorf("Info = %q", got)
	}
}

func TestRCScript(t *testing.T) {
	if got := history.RCScript("~/.zshrc"); !strings.Contains(got, `"$HOME"/'.zshrc'`) {
		t.Errorf("RCScript = %q", got)
	}
}
