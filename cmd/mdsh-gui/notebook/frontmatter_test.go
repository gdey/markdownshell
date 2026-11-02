package notebook

import (
	"strings"
	"testing"
)

func TestFrontMatterRead(t *testing.T) {
	file := write(t, "---\ntitle: Deploy\nsummary: Ships it\ntags: ops, prod\nauthor: j # who\nmdsh:\n  shell: zsh\n  show_stderr: false\n  env:\n    share: [HOME]\n---\n# Deploy\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	f, err := n.FrontMatter()
	if err != nil {
		t.Fatal(err)
	}
	if !f.Exists || f.Title != "Deploy" || f.Description != "Ships it" || f.DescriptionKey != "summary" {
		t.Errorf("fields = %+v", f)
	}
	if strings.Join(f.Tags, ",") != "ops,prod" || f.Shell != "zsh" || f.ShowStderr != "off" || f.Interpolate != "" {
		t.Errorf("fields = %+v", f)
	}
	if f.ShareAll || strings.Join(f.Share, ",") != "HOME" {
		t.Errorf("share = %v %q", f.ShareAll, f.Share)
	}
	if f.Other != "author: j # who\n" {
		t.Errorf("other = %q", f.Other)
	}
}

func TestSetFrontMatter(t *testing.T) {
	const top = "---\ntitle: Deploy # the name\nauthor: j\nmdsh:\n  shell: zsh\n---\n# Deploy\n"
	file := write(t, top)
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	f, _ := n.FrontMatter()
	// One field changed: only its key changes, in its place.
	f.Title = "Ship"
	if err := n.SetFrontMatter(f); err != nil {
		t.Fatal(err)
	}
	if got, want := string(n.d.Src), "---\ntitle: Ship # the name\nauthor: j\nmdsh:\n  shell: zsh\n---\n# Deploy\n"; got != want {
		t.Errorf("after title:\n%s\nwant\n%s", got, want)
	}
	if what, err := n.Undo(); err != nil || what != "change title" {
		t.Errorf("undo = %q, %v", what, err)
	}
	if string(n.d.Src) != top {
		t.Errorf("after undo:\n%s", n.d.Src)
	}

	// New keys go at the end, nested ones under mdsh; empty ones go.
	f, _ = n.FrontMatter()
	f.Shell, f.OnError, f.Cols, f.ShareAll, f.Share, f.Tags = "", "stop", 100, false, nil, []string{"#ops, prod"}
	f.Other = "author: j\nversion: 2\n"
	if err := n.SetFrontMatter(f); err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: Deploy # the name\nauthor: j\nmdsh:\n  on_error: stop\n  terminal:\n    cols: 100\n  env:\n    share: []\ntags: [ops, prod]\nversion: 2\n---\n# Deploy\n"
	if got := string(n.d.Src); got != want {
		t.Errorf("after many:\n%s\nwant\n%s", got, want)
	}
	if what, _ := n.Undo(); what != "edit front matter" {
		t.Errorf("undo = %q", what)
	}

	// What mdsh refuses changes nothing.
	f, _ = n.FrontMatter()
	f.OnError = "maybe"
	if err := n.SetFrontMatter(f); err == nil || !strings.Contains(err.Error(), "on_error") {
		t.Errorf("on_error maybe: err = %v", err)
	}
	f.OnError = ""
	f.Other = "title: again\n"
	if err := n.SetFrontMatter(f); err == nil || !strings.Contains(err.Error(), "field of its own") {
		t.Errorf("title in other: err = %v", err)
	}
	f.Other = "- a list\n"
	if err := n.SetFrontMatter(f); err == nil {
		t.Error("a list as other keys was taken")
	}
	if string(n.d.Src) != top {
		t.Errorf("refused edits changed the file:\n%s", n.d.Src)
	}

	// Emptying every field takes the front matter out.
	if err := n.SetFrontMatter(FrontMatter{ShareAll: true}); err != nil {
		t.Fatal(err)
	}
	if got := string(n.d.Src); got != "# Deploy\n" {
		t.Errorf("after emptying:\n%q", got)
	}
}

func TestSetFrontMatterNew(t *testing.T) {
	file := write(t, "#!/usr/bin/env mdsh\n# Notes\n")
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	f, _ := n.FrontMatter()
	if f.Exists || !f.ShareAll || f.DescriptionKey != "description" {
		t.Errorf("fields = %+v", f)
	}
	f.Description = "Things: to do"
	f.Interpolate = "off"
	if err := n.SetFrontMatter(f); err != nil {
		t.Fatal(err)
	}
	want := "#!/usr/bin/env mdsh\n---\ndescription: 'Things: to do'\nmdsh:\n  interpolate: false\n---\n# Notes\n"
	if got := string(n.d.Src); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if n.d.Options.Interpolating() {
		t.Error("interpolate: false wasn't read")
	}
}
