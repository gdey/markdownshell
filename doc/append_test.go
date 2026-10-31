package doc

import "testing"

func TestAppendBlock(t *testing.T) {
	d, err := Parse([]byte("# T\n\n```sh\necho one\n```\n\nprose\n"))
	if err != nil {
		t.Fatal(err)
	}
	first := d.Blocks[0].Span
	b, err := d.AppendBlock("sh", "echo two")
	if err != nil {
		t.Fatal(err)
	}
	want := "# T\n\n```sh\necho one\n```\n\nprose\n\n```sh\necho two\n```\n"
	if string(d.Src) != want {
		t.Errorf("src = %q, want %q", d.Src, want)
	}
	if b.ID != "block-2" || b.Body != "echo two\n" || len(d.Blocks) != 2 || d.Blocks[0].Span != first {
		t.Errorf("block %q body %q, %d blocks", b.ID, b.Body, len(d.Blocks))
	}

	// A body with a fence in it gets a longer fence.
	if b, err = d.AppendBlock("sh", "cat <<'EOF'\n```\nEOF"); err != nil || b.Body != "cat <<'EOF'\n```\nEOF\n" {
		t.Errorf("body %q, err %v", b.Body, err)
	}

	// Attributes go in the info string.
	if b, err = d.AppendBlock("zsh {#typed}", "pwd"); err != nil || b.ID != "typed" || b.Lang != "zsh" {
		t.Errorf("block %+v, err %v", b, err)
	}

	open, _ := Parse([]byte("```sh\necho open\n"))
	if _, err := open.AppendBlock("sh", "echo x"); err == nil {
		t.Error("appended inside an unclosed fence")
	}
}

func TestSetBlock(t *testing.T) {
	src := "# T\n\n```sh {#a}\necho one\n```\n\n```output {for=a exit=0}\none\n```\n\n- ```sh {#b}\n  echo two\n\n  ```\n\n> ```sh\n> echo three\n> ```\n\nSee {{#b/env(X)}}.\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	a, b, c := d.Blocks[0], d.Blocks[1], d.Blocks[2]
	for blk, want := range map[*Block]string{
		a: "```sh {#a}\necho one\n```\n",
		b: "```sh {#b}\necho two\n\n```\n",
		c: "```sh\necho three\n```\n",
	} {
		if got := d.BlockText(blk); got != want {
			t.Errorf("BlockText(%s) = %q, want %q", blk.ID, got, want)
		}
	}

	// The attributes change, and the saved output follows the new id.
	if err := d.SetBlock(a, "```bash {#first timeout=5s}\necho one\necho more\n```"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetBlock(b, "```sh {#b env=x}\necho 2\n```\n"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetBlock(c, "```sh\necho 3\n```\n"); err != nil {
		t.Fatal(err)
	}
	want := "# T\n\n```bash {#first timeout=5s}\necho one\necho more\n```\n\n```output {for=first exit=0}\none\n```\n\n- ```sh {#b env=x}\n  echo 2\n  ```\n\n> ```sh\n> echo 3\n> ```\n\nSee {{#b/env(X)}}.\n"
	if string(d.Src) != want {
		t.Errorf("src = %q, want %q", d.Src, want)
	}
	if d.Blocks[0] != a || d.Blocks[1] != b || a.ID != "first" || a.Lang != "bash" || a.Attrs["timeout"] != "5s" || !a.HasOutput() {
		t.Errorf("a = %+v", a)
	}
	if b.Attrs["env"] != "x" || string(d.Src[b.BodySpan.Start:b.BodySpan.End]) != "  echo 2\n" {
		t.Errorf("b's span not moved: %q", d.Src[b.BodySpan.Start:b.BodySpan.End])
	}

	for text, why := range map[string]string{
		"```sh {#first}\n```\n\n```sh\nmore\n```\n": "two blocks",
		"```sh {#c}\necho\n```\n":                   "renames b, which text refers to",
		"```sh {#b}\necho\n```\n":                   "duplicate id",
		"```go {<include file=x}\n```\n":            "another kind",
		"echo no fence\n":                           "no fence",
		"```sh {#first}\necho\n":                    "unclosed",
	} {
		blk := a
		if why == "renames b, which text refers to" {
			blk = b
		}
		if err := d.SetBlock(blk, text); err == nil {
			t.Errorf("%s: taken", why)
		}
		if string(d.Src) != want {
			t.Fatalf("%s: a failed SetBlock changed the document", why)
		}
	}
}
