package doc

import (
	"math/rand"
	"reflect"
	"slices"
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
				// And one reparsed, whose blocks keep their identities as
				// the splice's must.
				f, err := Parse([]byte(text))
				if err != nil {
					t.Fatal(err)
				}
				cWas, fWas := slices.Clone(c.Blocks), slices.Clone(f.Blocks)
				if c.splice([]byte(edited)) {
					spliced++
					if !reflect.DeepEqual(c, want) {
						t.Fatalf("edit %d (line %d to %q): the spliced document isn't a parse's", k, i, next)
					}
					if err := f.reparse([]byte(edited)); err != nil {
						t.Fatal(err)
					}
					for j := range c.Blocks {
						if slices.Index(cWas, c.Blocks[j]) != slices.Index(fWas, f.Blocks[j]) {
							t.Fatalf("edit %d (line %d to %q): block %d's identity isn't a parse's", k, i, next, j)
						}
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
		{"echo one", "echo two", true},
		{"echo one", "echo one\n```\n", false},
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

// TestSpliceBody types in each block's body in turn, and checks that a
// plain block's edit is spliced, to the document a parse has, with the
// edited block new and the others kept.
func TestSpliceBody(t *testing.T) {
	for _, shape := range stress.Shapes {
		t.Run(shape, func(t *testing.T) {
			text := string(stress.Write(shape, 2000))
			d, err := Parse([]byte(text))
			if err != nil {
				t.Fatal(err)
			}
			for i, b := range d.Blocks {
				if b.BodySpan.End <= b.BodySpan.Start {
					continue
				}
				for _, typed := range []string{"x", "\nmore", ""} {
					at := b.BodySpan.End - 1
					edited := text[:at] + typed + text[at:]
					if typed == "" {
						edited = text[:at-1] + text[at:] // a backspace
					}
					want, err := Parse([]byte(edited))
					if err != nil {
						continue
					}
					c, err := Parse([]byte(text))
					if err != nil {
						t.Fatal(err)
					}
					was := slices.Clone(c.Blocks)
					plain := !b.IsInput() && !b.IsInclude() && !b.IsTape() && !b.IsHTTP() && b.Prefix == "" && !c.twin(c.Blocks[i], want.Blocks[i]) &&
						(i+1 == len(c.Blocks) || !c.Blocks[i+1].IsInput())
					if got := c.splice([]byte(edited)); got != plain {
						t.Fatalf("block %d (%s), %q: spliced %t", i, b.ID, typed, got)
					} else if !got {
						continue
					}
					if !reflect.DeepEqual(c, want) {
						t.Fatalf("block %d (%s), %q: the spliced document isn't a parse's", i, b.ID, typed)
					}
					for j := range c.Blocks {
						if (c.Blocks[j] == was[j]) != (j != i) {
							t.Fatalf("block %d (%s), %q: block %d's identity is wrong", i, b.ID, typed, j)
						}
					}
				}
			}
		})
	}
}
