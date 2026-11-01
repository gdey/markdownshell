package doc

import (
	"errors"
	"strings"
	"testing"
)

func TestDeleteBlock(t *testing.T) {
	src := "# T\n\nIntro.\n\n" +
		"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
		"Between.\n" +
		"```sh {#b tty}\nsh\n```\n" +
		"```tape {for=b}\nType \"exit\"\n```\n" +
		"After.\n\n" +
		"```sh {#c}\necho c\n```\n"
	tests := []struct {
		name, block, want string
	}{{
		name:  "with its output",
		block: "a",
		want: "# T\n\nIntro.\n\n" +
			"Between.\n" +
			"```sh {#b tty}\nsh\n```\n" +
			"```tape {for=b}\nType \"exit\"\n```\n" +
			"After.\n\n" +
			"```sh {#c}\necho c\n```\n",
	}, {
		name:  "with its tape, keeping the paragraphs apart",
		block: "b",
		want: "# T\n\nIntro.\n\n" +
			"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
			"Between.\n\nAfter.\n\n" +
			"```sh {#c}\necho c\n```\n",
	}, {
		name:  "at the end of the file",
		block: "c",
		want: "# T\n\nIntro.\n\n" +
			"```sh {#a}\necho a\n```\n\n```output {for=a}\na\n```\n\n" +
			"Between.\n" +
			"```sh {#b tty}\nsh\n```\n" +
			"```tape {for=b}\nType \"exit\"\n```\n" +
			"After.\n",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := mustParse(t, src)
			var kept []*Block
			for _, b := range d.Blocks {
				if b.ID != tt.block && b.Attrs["for"] != tt.block {
					kept = append(kept, b)
				}
			}
			w, err := d.DeleteBlock(d.Block(tt.block))
			if err != nil {
				t.Fatal(err)
			}
			if string(d.Src) != tt.want {
				t.Errorf("src =\n%s\nwant\n%s", d.Src, tt.want)
			}
			if len(w) != 0 {
				t.Errorf("warnings %q", w)
			}
			if len(d.Blocks) != len(kept) {
				t.Fatalf("%d blocks, want %d", len(d.Blocks), len(kept))
			}
			for j, b := range kept {
				if d.Blocks[j] != b {
					t.Errorf("block %d isn't the old block %s", j, b.ID)
				}
			}
		})
	}
}

func TestDeleteBlockRefuses(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\n" +
		"```sh {#b depends=a}\necho b\n```\n\n" +
		"```sh {#c run={{#a/?}}==0}\necho c\n```\n\n" +
		"It said {{#a/env(X)}}.\n"
	d := mustParse(t, src)
	_, err := d.DeleteBlock(d.Block("a"))
	if err == nil {
		t.Fatal("deleted a block others use")
	}
	for _, want := range []string{"b's depends=", "c's run=", "{{#a/env(X)}}"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q doesn't name %s", err, want)
		}
	}
	if string(d.Src) != src {
		t.Error("the document changed")
	}
	// A block that only reads a by itself can go.
	if _, err := d.DeleteBlock(d.Block("c")); err != nil {
		t.Errorf("deleting c: %v", err)
	}
}

func TestDeleteBlockRenumbers(t *testing.T) {
	src := "```sh\necho one\n```\n\n```sh\necho two\n```\n\nTwo said {{#block-2/?}}.\n"
	d := mustParse(t, src)
	if _, err := d.DeleteBlock(d.Blocks[0]); !errors.Is(err, ErrRenumber) {
		t.Errorf("err %v, want ErrRenumber", err)
	}
	// With nothing reading the ids, the renumbering is harmless.
	d = mustParse(t, "```sh\necho one\n```\n\n```sh\necho two\n```\n")
	if _, err := d.DeleteBlock(d.Blocks[0]); err != nil || string(d.Src) != "```sh\necho two\n```\n" {
		t.Errorf("err %v, src %q", err, d.Src)
	}
}

func TestDeleteBlockCopies(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n\nText.\n\n```output {for=a}\na\n```\n"
	d := mustParse(t, src)
	w, err := d.DeleteBlock(d.Block("a"))
	if err != nil {
		t.Fatal(err)
	}
	if len(w) != 1 || !strings.Contains(w[0], "output copy") {
		t.Errorf("warnings %q", w)
	}
}
