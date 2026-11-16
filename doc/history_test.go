package doc

import (
	"fmt"
	"strings"
	"testing"
)

// histSrc is a block with a current output and two earlier runs, the
// first with a stderr part, then a block after it.
const histSrc = "```sh {#check history=3}\ncurl -sf x\n```\n\n" +
	"```output {for=check exit=0 at=2026-11-13T19:08:00Z}\nok\n```\n\n" +
	"```output {for=check exit=7 history=1 at=2026-11-12T19:02:11Z}\nno\n```\n\n" +
	"```output {for=check history=1 stream=stderr}\ncurl: (7)\n```\n\n" +
	"<!-- mdsh:output for=\"check\" history=\"2\" at=\"2026-11-11T18:59:40Z\" -->\n*ok*\n<!-- /mdsh:output -->\n\n" +
	"```sh {#next}\necho next\n```\n"

func TestParseHistory(t *testing.T) {
	d := mustParse(t, histSrc)
	b := d.Blocks[0]
	if len(d.Blocks) != 2 || len(d.Copies) != 0 {
		t.Fatalf("%d blocks, %d copies", len(d.Blocks), len(d.Copies))
	}
	want := []string{
		"K=1 at=2026-11-12T19:02:11Z exit=7 region=false",
		"K=2 at=2026-11-11T18:59:40Z exit=0 region=true",
	}
	var got []string
	for _, e := range b.History {
		got = append(got, fmt.Sprintf("K=%d at=%s exit=%d region=%v", e.K, e.At, e.Exit, e.Region))
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("history = %q, want %q", got, want)
	}
	text := func(s Span) string { return string(d.Src[s.Start:s.End]) }
	if got := text(b.CurrentOutput()); got != "\n```output {for=check exit=0 at=2026-11-13T19:08:00Z}\nok\n```\n" {
		t.Errorf("current output = %q", got)
	}
	if got := text(b.History[0].Span); !strings.HasPrefix(got, "```output {for=check exit=7") || !strings.HasSuffix(got, "curl: (7)\n```\n") {
		t.Errorf("entry 1 = %q", got)
	}
	if got := text(b.Output); !strings.HasSuffix(got, "<!-- /mdsh:output -->\n") || !strings.HasPrefix(got, text(b.CurrentOutput())) {
		t.Errorf("output = %q", got)
	}
	if b := d.Blocks[1]; len(b.History) != 0 || b.CurrentOutput() != b.Output {
		t.Errorf("next: history %v, current %v, output %v", b.History, b.CurrentOutput(), b.Output)
	}
}

// An earlier run may follow a block with no current output, and it
// may be stderr only.
func TestParseHistoryNoCurrent(t *testing.T) {
	d := mustParse(t, "```sh {#a history}\nfalse\n```\n\n```output {for=a history=1 stream=stderr at=2026-11-12T19:02:11Z}\nbad\n```\n")
	b := d.Blocks[0]
	if len(b.History) != 1 || b.History[0].At != "2026-11-12T19:02:11Z" || b.CurrentOutput().End != b.Span.End || b.Output.End != len(d.Src) {
		t.Errorf("history %+v, current %v, output %v", b.History, b.CurrentOutput(), b.Output)
	}
}

// Earlier runs in a list item or block quote belong to their block, with
// the container's prefix.
func TestParseHistoryNested(t *testing.T) {
	for _, src := range []string{
		"- step:\n\n  ```sh {#a history=2}\n  echo a\n  ```\n\n  ```output {for=a exit=0}\n  a\n  ```\n\n  ```output {for=a exit=0 history=1}\n  old\n  ```\n\n- next\n",
		"> ```sh {#a history=2}\n> echo a\n> ```\n>\n> ```output {for=a exit=0}\n> a\n> ```\n>\n> ```output {for=a exit=0 history=1}\n> old\n> ```\n\nAfter.\n",
	} {
		d := mustParse(t, src)
		b := d.Blocks[0]
		if len(b.History) != 1 || !strings.Contains(string(d.Src[b.History[0].Span.Start:b.History[0].Span.End]), "old") {
			t.Errorf("%q: history %+v", src, b.History)
		}
	}
}

func TestParseHistoryErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"```sh {#a}\necho\n```\n\nProse.\n\n```output {for=a exit=0 history=1}\nx\n```\n", "line 7: an earlier run of a (history=1) must be right under its output"},
		{"```output {for=a exit=0 history=1}\nx\n```\n", "line 1: an earlier run of a (history=1) must be right under its output"},
		{"```sh {#a}\necho\n```\n\n<!-- mdsh:output for=\"b\" history=\"1\" -->\nx\n<!-- /mdsh:output -->\n", "line 5: an earlier run of b (history=1) must be right under its output"},
		{"```sh {#a}\necho\n```\n\n```output {for=a history=2}\nx\n```\n\n```output {for=a history=1}\ny\n```\n", "line 9: history=1 comes after history=2: earlier runs of a go newest first"},
		{"```sh {#a}\necho\n```\n\n```output {for=a history=1}\nx\n```\n\n```output {for=a history=1}\ny\n```\n", "line 9: a has two stdout output parts for history=1"},
		{"```sh {#a}\necho\n```\n\n```output {for=a history=0}\nx\n```\n", "line 5: history=0 on an output: want the run's place, a whole number from 1"},
		{"```sh {#a history=lots}\necho\n```\n", "line 1: history=lots: want how many earlier runs to keep, a whole number from 0"},
		{"```sh {#a history=-1}\necho\n```\n", "line 1: history=-1: want how many earlier runs to keep, a whole number from 0"},
		{"---\nmdsh:\n  history: -2\n---\n", "front matter: history must be how many earlier runs to keep, 0 or more, got -2"},
	} {
		d, err := Parse([]byte(tc.src))
		if err == nil && d.FrontMatterErr != nil {
			err = d.FrontMatterErr
		}
		if err == nil || err.Error() != tc.want {
			t.Errorf("%q: err = %v, want %s", tc.src, err, tc.want)
		}
	}
}

// After an earlier run, a plain output fence for the block is a copy,
// not more current output.
func TestHistoryThenCopy(t *testing.T) {
	d := mustParse(t, "```sh {#a}\necho\n```\n\n```output {for=a exit=0 history=1}\nx\n```\n\n```output {for=a exit=0}\ny\n```\n")
	if len(d.Copies) != 1 || len(d.Blocks[0].History) != 1 {
		t.Errorf("copies %d, history %d", len(d.Copies), len(d.Blocks[0].History))
	}
}

func TestHistoryCap(t *testing.T) {
	d := mustParse(t, "---\nmdsh:\n  history: 4\n---\n```sh {#a history}\n```\n\n```sh {#b history=2}\n```\n\n```sh {#c}\n```\n\n```sh {#d history=0}\n```\n")
	for i, want := range []int{4, 2, 0, 0} {
		if got := d.Blocks[i].HistoryCap(d.Options); got != want {
			t.Errorf("%s: cap %d, want %d", d.Blocks[i].ID, got, want)
		}
	}
	if got := d.Blocks[0].HistoryCap(Options{}); got != DefaultHistory {
		t.Errorf("bare without front matter: cap %d, want %d", got, DefaultHistory)
	}
}

// Moving, deleting and renaming a block carry its earlier runs along.
func TestHistoryMoveDeleteRename(t *testing.T) {
	d := mustParse(t, histSrc+"\nEnd.\n")
	if _, err := d.MoveBlock(d.Blocks[0], spot(t, d, "End.")); err != nil {
		t.Fatal(err)
	}
	if s := string(d.Src); !strings.HasPrefix(s, "```sh {#next}") || !strings.Contains(s, "curl: (7)\n```\n\n<!-- mdsh:output for=\"check\" history=\"2\"") || len(d.Blocks[1].History) != 2 {
		t.Errorf("moved:\n%s", s)
	}

	d = mustParse(t, histSrc)
	if _, err := d.DeleteBlock(d.Blocks[0]); err != nil {
		t.Fatal(err)
	}
	if s := string(d.Src); strings.Contains(s, "for=check") {
		t.Errorf("deleted, earlier runs left:\n%s", s)
	}

	src := strings.Replace(histSrc, "#check ", "", 1)
	src = strings.ReplaceAll(src, `for=check`, `for=block-1`)
	src = strings.ReplaceAll(src, `for="check"`, `for="block-1"`)
	d = mustParse(t, src)
	if _, err := d.NameBlocks(func(b *Block) bool { return b.ID == "block-1" }, func() string { return "block-new" }); err != nil {
		t.Fatal(err)
	}
	if s := string(d.Src); strings.Contains(s, "block-1") || strings.Count(s, "block-new") != 5 {
		t.Errorf("renamed:\n%s", s)
	}
	if len(d.Blocks[0].History) != 2 {
		t.Errorf("renamed: history %+v", d.Blocks[0].History)
	}
}
