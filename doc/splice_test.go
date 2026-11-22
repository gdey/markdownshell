package doc

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/internal/stress"
)

// TestSpliceParse makes random edits to each shape of document, the
// kinds typing makes and some that change its blocks and the lines mdsh
// reads in prose, and checks that SetSource leaves the document as a
// parse of the whole file has it, whether or not it could splice.
func TestSpliceParse(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	edits := []string{
		"x", " more words", "\n", "\n\n", "# A heading\n", "- [ ] a task\n", "  - nested\n", "> quoted\n",
		"```sh\necho hi\n```\n", "```", "~~~", "\n```output {for=step1}\nout\n```\n",
		"<!-- mdsh:collapse -->\n", "<!-- mdsh:if env.X -->\n", "<!-- mdsh:end -->\n",
		"{{#step1}}", "{{env.HOME}}", "<details>\n", "1. one\n", "    indented\n",
	}
	for _, shape := range stress.Shapes {
		t.Run(shape, func(t *testing.T) {
			text := string(stress.Write(shape, 3000))
			d, err := Parse([]byte(text))
			if err != nil {
				t.Fatal(err)
			}
			spliced := 0
			for k := 0; k < 300; k++ {
				lines := strings.SplitAfter(text, "\n")
				i := r.Intn(len(lines))
				var next string
				switch r.Intn(3) {
				case 0: // type at the end of a line
					l := strings.TrimSuffix(lines[i], "\n")
					next = l + edits[r.Intn(len(edits))]
					if strings.HasSuffix(lines[i], "\n") {
						next += "\n"
					}
				case 1: // delete a line
					next = ""
				default: // insert a line
					next = edits[r.Intn(len(edits))] + lines[i]
				}
				edited := strings.Join(lines[:i], "") + next + strings.Join(lines[i+1:], "")
				want, err := Parse([]byte(edited))
				if err != nil {
					continue // SetSource leaves d as it was
				}
				// A splice of a document of its own, as SetSource may
				// parse the whole file instead.
				c, err := Parse([]byte(text))
				if err != nil {
					t.Fatal(err)
				}
				blocks := map[*Block]bool{}
				for _, b := range c.Blocks {
					blocks[b] = true
				}
				if c.splice([]byte(edited)) {
					spliced++
					for _, b := range c.Blocks {
						if !blocks[b] {
							t.Fatalf("edit %d: a block is new", k)
						}
					}
					if !reflect.DeepEqual(c, want) {
						t.Fatalf("edit %d (line %d to %q): the spliced document isn't a parse's", k, i, next)
					}
				}
				if err := d.SetSource([]byte(edited)); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(d, want) {
					t.Fatalf("edit %d (line %d to %q): the document isn't a parse's", k, i, next)
				}
				text = edited
			}
			t.Logf("%d of 300 edits spliced", spliced)
		})
	}
}

// TestSpliceTop checks splices of the prose above the first block and
// below the last, and that the front matter is never spliced.
func TestSpliceTop(t *testing.T) {
	src := "#!/usr/bin/env mdsh\n---\ntitle: T\n---\n# Title\n\nSome prose.\n\n```sh {#one}\necho one\n```\n\nThe end.\n"
	for _, e := range []struct {
		old, text string
		splice    bool
	}{
		{"Some prose.", "Some typed prose.", true},
		{"Some prose.", "Some\nprose.", true},
		{"The end.", "The very end.", true},
		{"# Title\n", "", true},
		{"title: T", "title: U", false},
		{"echo one", "echo two", false},
		{"#!/usr/bin/env mdsh\n", "", false},
	} {
		edited := strings.Replace(src, e.old, e.text, 1)
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		want, err := Parse([]byte(edited))
		if err != nil {
			t.Fatal(err)
		}
		if got := d.splice([]byte(edited)); got != e.splice {
			t.Errorf("%q to %q: spliced %t, want %t", e.old, e.text, got, e.splice)
		}
		if err := d.SetSource([]byte(edited)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(d, want) {
			t.Errorf("%q to %q: the document isn't a parse's", e.old, e.text)
		}
	}
}

// TestSpliceFrontMatter checks that front matter typed at the top of a
// file without any is read, not spliced past.
func TestSpliceFrontMatter(t *testing.T) {
	src := "Some prose.\n\n```sh {#one}\necho one\n```\n"
	edited := "---\nmdsh:\n  interpolate: false\n---\n" + src
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if d.splice([]byte(edited)) {
		t.Fatal("front matter spliced")
	}
	want, err := Parse([]byte(edited))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetSource([]byte(edited)); err != nil || !reflect.DeepEqual(d, want) {
		t.Fatal("front matter not read", err)
	}
}
