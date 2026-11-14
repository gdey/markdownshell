package run_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gdey/markdownshell/docroot"
	"github.com/gdey/markdownshell/run"
)

// Outside lists the includes and out= files that leave the document's
// root, as the document writes them, and nothing that stays inside.
func TestOutsideList(t *testing.T) {
	top := t.TempDir()
	repo := filepath.Join(top, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A link in the document's folder that points out of it counts as out.
	if err := os.Symlink(top, filepath.Join(dir, "up")); err != nil {
		t.Fatal(err)
	}
	src := "```sh {<include file=../README.md}\n```\n\n" +
		"```sh {<include file=../../shared/x.sh}\n```\n\n" +
		"```sh {<include file=../../shared/x.sh skip}\n```\n\n" +
		"```sh {<include file=up/y.sh}\n```\n\n" +
		"```sh {<include file=res://a.sh}\n```\n\n" +
		"```sh {out=screenshot:shot.png}\necho\n```\n\n" +
		"```sh {out=screenshot:../../.zshrc}\necho\n```\n\n" +
		"```sh {out=image:/tmp/x.png}\necho\n```\n\n" +
		"```sh\necho plain\n```\n"
	d := parse(t, src)
	got := run.Outside(d, run.Options{File: filepath.Join(dir, "doc.md")})
	want := []string{"../README.md", "../../shared/x.sh", "up/y.sh", "../../.zshrc", "/tmp/x.png"}
	if !slices.Equal(got, want) {
		t.Errorf("Outside = %q, want %q", got, want)
	}
	if got := run.Outside(parse(t, "```sh {<include file=sub/README.md}\n```\n"), run.Options{File: filepath.Join(dir, "doc.md")}); got != nil {
		t.Errorf("inside only: Outside = %q, want none", got)
	}
}

// AllowedOutside lets the files it names reach outside the root, and
// only those: a file the document names after it was allowed stays in.
func TestAllowedOutside(t *testing.T) {
	top := t.TempDir()
	dir := filepath.Join(top, "sub")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"shared", "secret"} {
		if err := os.WriteFile(filepath.Join(top, f), []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := "```txt {<include file=../shared}\n```\n\n" +
		"```txt {<include file=../secret}\n```\n\n" +
		"```sh {#img out=image:../out.svg on_error=continue}\nprintf '<svg/>'\n```\n"
	opts := run.Options{Dir: dir, File: filepath.Join(dir, "d.md"), AllowedOutside: []string{"../shared", "../out.svg"}}
	_, results := runDoc(t, src, opts)
	if r := results[0]; r.Err != nil || string(r.Stdout) != "shared\n" {
		t.Errorf("allowed include = %q, %v", r.Stdout, r.Err)
	}
	if r := results[1]; !errors.Is(r.Err, docroot.ErrOutside) {
		t.Errorf("include not allowed: err = %v, want ErrOutside", r.Err)
	}
	if r := results[2]; r.Err != nil {
		t.Errorf("allowed out=: err = %v", r.Err)
	}
	if _, err := os.Stat(filepath.Join(top, "out.svg")); err != nil {
		t.Errorf("allowed out= file: %v", err)
	}
}

// An include of the repository's .git/config, which can carry a token in
// a remote's URL, fails without --allow-outside and is listed by
// Outside, so mdsh-gui asks about it.
func TestIncludeGitMetadata(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := "[remote \"origin\"]\n\turl = https://me:ghp_secret@github.com/a/b\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	src := "```ini {<include file=.git/config}\n```\n"
	opts := run.Options{Dir: repo, File: filepath.Join(repo, "d.md")}
	_, results := runDoc(t, src, opts)
	if r := results[0]; !errors.Is(r.Err, docroot.ErrHidden) || len(r.Stdout) != 0 {
		t.Errorf("include .git/config = %q, %v, want ErrHidden", r.Stdout, r.Err)
	}
	if got := run.Outside(parse(t, src), opts); !slices.Equal(got, []string{".git/config"}) {
		t.Errorf("Outside = %q, want [.git/config]", got)
	}
	opts.AllowOutside = true
	if _, results := runDoc(t, src, opts); results[0].Err != nil || string(results[0].Stdout) != conf {
		t.Errorf("with AllowOutside = %q, %v", results[0].Stdout, results[0].Err)
	}
}

// A document reaches only its own folder, repository or not, and no
// hidden file in it: an include of ../scripts/setup.sh from docs/ in a
// repository, or of .env beside the document, fails and is listed by
// Outside (finding 3 of the v1.5.0 security review).
func TestIncludeOwnFolderOnly(t *testing.T) {
	repo := t.TempDir()
	docs := filepath.Join(repo, "docs")
	for _, d := range []string{filepath.Join(repo, ".git"), filepath.Join(repo, "scripts"), filepath.Join(docs, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(repo, "scripts", "setup.sh"): "echo setup\n",
		filepath.Join(docs, ".env"):                "TOKEN=x\n",
		filepath.Join(docs, "sub", "notes.txt"):    "notes\n",
	}
	for p, s := range files {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := "```sh {<include file=../scripts/setup.sh}\n```\n\n```sh {<include file=.env}\n```\n\n```text {<include file=sub/notes.txt}\n```\n"
	opts := run.Options{Dir: docs, File: filepath.Join(docs, "d.md")}
	_, results := runDoc(t, src, opts)
	if r := results[0]; !errors.Is(r.Err, docroot.ErrOutside) || len(r.Stdout) != 0 {
		t.Errorf("include ../scripts/setup.sh = %q, %v, want ErrOutside", r.Stdout, r.Err)
	}
	if r := results[1]; !errors.Is(r.Err, docroot.ErrHidden) || len(r.Stdout) != 0 {
		t.Errorf("include .env = %q, %v, want ErrHidden", r.Stdout, r.Err)
	}
	if r := results[2]; r.Err != nil || string(r.Stdout) != "notes\n" {
		t.Errorf("include sub/notes.txt = %q, %v", r.Stdout, r.Err)
	}
	if got, want := run.Outside(parse(t, src), opts), []string{"../scripts/setup.sh", ".env"}; !slices.Equal(got, want) {
		t.Errorf("Outside = %q, want %q", got, want)
	}
}
