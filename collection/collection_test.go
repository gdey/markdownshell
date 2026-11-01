package collection

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func real(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	if c, err := Load(dir); c != nil || err != nil {
		t.Fatalf("no file: Load = %v, %v; want nil, nil", c, err)
	}
	icon := write(t, dir, "img/rocket.svg", "<svg/>")
	bg := write(t, dir, "banner.png", "png")
	write(t, dir, FileName, `
title: "  Deploys  "
description: |
  Ship, roll back
  and check prod.
icon: img/rocket.svg
color: "#3b82f6"
background: banner.png
tags: [Ops, "#prod", ops, "", two words]
later: ignored
`)
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Deploys" || c.Description != "Ship, roll back and check prod." {
		t.Errorf("title, description = %q, %q", c.Title, c.Description)
	}
	if c.IconPath != real(t, icon) || c.Icon != "" || c.Background != real(t, bg) || c.Color != "#3b82f6" {
		t.Errorf("icon %q %q, background %q, color %q", c.Icon, c.IconPath, c.Background, c.Color)
	}
	if got := strings.Join(c.Tags, " "); got != "ops prod" {
		t.Errorf("tags = %s", got)
	}
	if len(c.Problems) != 0 {
		t.Errorf("problems = %q", c.Problems)
	}
}

func TestLoadErrors(t *testing.T) {
	for _, src := range []string{"just text", "- a\n- b", "title: [unclosed"} {
		dir := t.TempDir()
		write(t, dir, FileName, src)
		if c, err := Load(dir); err == nil {
			t.Errorf("%q: Load = %+v, want an error", src, c)
		}
	}
	dir := t.TempDir()
	write(t, dir, FileName, "")
	if c, err := Load(dir); err != nil || c == nil {
		t.Errorf("empty file: Load = %v, %v", c, err)
	}
}

func TestParseProblems(t *testing.T) {
	parent := t.TempDir()
	write(t, parent, "secret.png", "png")
	dir := filepath.Join(parent, "coll")
	write(t, dir, "notes.txt", "x")
	write(t, dir, "sub/ok.png", "png")
	if err := os.Symlink(filepath.Join(parent, "secret.png"), filepath.Join(dir, "link.png")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		src     string
		check   func(*Collection) bool
		problem string
	}{
		{"icon: 🚀", func(c *Collection) bool { return c.Icon == "🚀" && c.IconPath == "" }, ""},
		{"icon: sub/ok.png", func(c *Collection) bool { return strings.HasSuffix(c.IconPath, "ok.png") }, ""},
		{"icon: a very long text icon", func(c *Collection) bool { return c.Icon == "" }, "icon"},
		{"background: ../secret.png", func(c *Collection) bool { return c.Background == "" }, "outside"},
		{"background: link.png", func(c *Collection) bool { return c.Background == "" }, "outside"},
		{"background: /etc/x.png", func(c *Collection) bool { return c.Background == "" }, "relative"},
		{"background: missing.png", func(c *Collection) bool { return c.Background == "" }, "not found"},
		{"background: notes.txt", func(c *Collection) bool { return c.Background == "" }, "not an image"},
		{"background: sub", func(c *Collection) bool { return c.Background == "" }, "not an image"},
		{`color: "red; x: url(x)"`, func(c *Collection) bool { return c.Color == "" }, "color"},
		{"color: rgb(10 20 30 / 50%)", func(c *Collection) bool { return c.Color == "rgb(10 20 30 / 50%)" }, ""},
		{"title: [a, b]\ncolor: teal", func(c *Collection) bool { return c.Title == "" && c.Color == "teal" }, "cannot unmarshal"},
	} {
		got, err := Parse(dir, []byte(c.src))
		if err != nil {
			t.Errorf("%q: %v", c.src, err)
			continue
		}
		if !c.check(got) {
			t.Errorf("%q: got %+v", c.src, got)
		}
		problems := strings.Join(got.Problems, "; ")
		if c.problem == "" && problems != "" || !strings.Contains(problems, c.problem) {
			t.Errorf("%q: problems = %q, want one with %q", c.src, problems, c.problem)
		}
	}
}

func TestValidColor(t *testing.T) {
	for _, s := range []string{"#fff", "#ffff", "#3b82f6", "#3b82f680", "rebeccapurple", "rgb(1,2,3)", "hsla(210, 50%, 40%, 0.5)"} {
		if !ValidColor(s) {
			t.Errorf("ValidColor(%q) = false", s)
		}
	}
	for _, s := range []string{"", "#ff", "#ggg", "red;", "url(x.png)", "rgb(1,2,3);x", "var(--x)", "expression(alert(1))", `"red"`} {
		if ValidColor(s) {
			t.Errorf("ValidColor(%q) = true", s)
		}
	}
}

func TestTags(t *testing.T) {
	for _, c := range []struct {
		in   any
		want []string
	}{
		{nil, nil},
		{"ops, prod  db", []string{"ops", "prod", "db"}},
		{[]any{"Ops", 3, "#db", "OPS"}, []string{"ops", "db"}},
		{map[string]any{"a": "b"}, nil},
	} {
		if got := Tags(c.in); !slices.Equal(got, c.want) {
			t.Errorf("Tags(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := Merge([]string{"a", "b"}, []string{"b", "c"}); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("Merge = %q", got)
	}
}
