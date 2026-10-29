package doc

import "testing"

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
