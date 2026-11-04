package doc

import (
	"strings"
	"testing"
)

func TestSetSource(t *testing.T) {
	d, err := Parse([]byte("```sh {#a}\necho a\n```\n\n```sh\necho one\n```\n\n```sh\necho two\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	a, one, two := d.Blocks[0], d.Blocks[1], d.Blocks[2]

	// A new block goes first, two moves above one and a is edited.
	src := "```sh\necho new\n```\n\n```sh\necho two\n```\n\n```sh {#a}\necho A\n```\n\n```sh\necho one\n```\n"
	if err := d.SetSource([]byte(src)); err != nil {
		t.Fatal(err)
	}
	if string(d.Src) != src || len(d.Blocks) != 4 {
		t.Fatalf("got %d blocks from %q", len(d.Blocks), d.Src)
	}
	if d.Blocks[1] != two || d.Blocks[3] != one {
		t.Error("unchanged blocks lost their identity")
	}
	if d.Blocks[2] == a || d.Blocks[0] == a {
		t.Error("an edited block kept its identity")
	}
	if two.ID != "block-2" || one.ID != "block-4" || two.Span.Start != len("```sh\necho new\n```\n\n") {
		t.Errorf("moved blocks weren't updated: %s %s %v", two.ID, one.ID, two.Span)
	}

	// Duplicates are matched in order, and a bad source changes nothing.
	d, _ = Parse([]byte("```sh\nx\n```\n```sh\nx\n```\n"))
	first, second := d.Blocks[0], d.Blocks[1]
	if err := d.SetSource([]byte("```sh\nx\n```\n")); err != nil {
		t.Fatal(err)
	}
	if len(d.Blocks) != 1 || d.Blocks[0] != first || second.ID != "block-2" {
		t.Error("duplicates weren't matched in order")
	}
	src = string(d.Src)
	if err := d.SetSource([]byte("---\nmdsh: [\n---\n")); err == nil || string(d.Src) != src {
		t.Errorf("bad source: err %v, src %q", err, d.Src)
	}
}

func TestNoSaveInput(t *testing.T) {
	src := "```textbox {#host var=host nosave}\ndefault\n```\n\n" +
		"```choice {#env mode=multi var=env nosave}\n- [x] staging\n- [ ] prod\n```\n\n" +
		"```textbox {#host2 var=host nosave}\ndefault\n```\n\n" +
		"```textbox {#plain var=plain}\nkept\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Warnings) != 0 {
		t.Fatalf("warnings: %q", d.Warnings)
	}
	host, env, host2, plain := d.Blocks[0], d.Blocks[1], d.Blocks[2], d.Blocks[3]
	if !host.NoSave() || plain.NoSave() {
		t.Fatal("NoSave is wrong")
	}
	if d.SetValue(plain, "x") {
		t.Error("SetValue took a textbox without nosave")
	}
	if !d.SetValue(host, "db1") {
		t.Fatal("SetValue refused the nosave textbox")
	}
	// The value is the session's, on every copy; the source is as it was.
	for _, b := range []*Block{host, host2} {
		if _, v, ok := b.Var(); b.ID == "host" && (!ok || v != "db1") || b.Textbox().Text != "db1" {
			t.Errorf("%s: value %q, text %q", b.ID, v, b.Textbox().Text)
		}
	}
	if string(d.Src) != src {
		t.Errorf("source changed: %q", d.Src)
	}
	if !d.SetChecks(env, []bool{false, true}) {
		t.Fatal("SetChecks refused the nosave choice")
	}
	if _, v, _ := env.Var(); v != "prod" {
		t.Errorf("env = %q, want prod", v)
	}
	if got := env.Choice().Options; !got[1].Checked || got[0].Checked {
		t.Errorf("options = %+v", got)
	}

	// An edit elsewhere keeps the session values, by id.
	if err := d.SetSource([]byte("Intro.\n\n" + src)); err != nil {
		t.Fatal(err)
	}
	if v, ok := d.Blocks[0].SessionValue(); !ok || v != "db1" {
		t.Errorf("after SetSource, host = %q, %v", v, ok)
	}
	if _, v, _ := d.Blocks[1].Var(); v != "prod" {
		t.Errorf("after SetSource, env = %q", v)
	}
	// SetText still edits the source of a nosave textbox, for the
	// source pane; the session value stays until it is set again.
	if got := string(d.SetText(d.Blocks[0], "typed")); !strings.Contains(got, "nosave}\ntyped\n```") {
		t.Errorf("SetText: %q", got)
	}
}

func TestNoSaveMirrorMismatch(t *testing.T) {
	d, err := Parse([]byte("```textbox {#a var=x nosave}\n```\n\n```textbox {#b var=x}\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Warnings) != 1 || !strings.Contains(d.Warnings[0], "nosave differs") {
		t.Errorf("warnings = %q", d.Warnings)
	}
}
