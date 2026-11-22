package notebook

import (
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/internal/stress"
)

// TestRelayoutSplice makes random edits to each shape of document, the
// kinds typing makes and some that change its blocks, and checks that
// the heads the page had, with Relayout's splice applied, are the heads
// of the document laid out afresh.
func TestRelayoutSplice(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	edits := []string{"x", " more words", "\n", "\n\n", "# A heading\n", "## Sub\n", "```sh\necho hi\n```\n", "```", "- [ ] a task\n", "<details>\n", "`", "*", "\n```sh name=fresh depends=step1\necho hey\n```\n"}
	for _, shape := range stress.Shapes {
		t.Run(shape, func(t *testing.T) {
			n, text := openStress(t, shape, 3000)
			h := n.Attach(1 << 30)
			have := slicesClone(h.Heads)
			gen := h.Gen
			moving, reused := 0, 0
			defer func() {
				t.Logf("%d of 60 splices came before the index, %d reused the cells", moving, reused)
			}()
			for k := 0; k < 60; k++ {
				lines := strings.SplitAfter(text, "\n")
				i := r.Intn(len(lines))
				var next string
				switch r.Intn(3) {
				case 0: // insert at the end of a line
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
				re, err := n.Edit(n.Version(), []LineEdit{{First: i, Last: i + 1, Text: next}})
				if err != nil || !re.Applied {
					t.Fatal(err, re)
				}
				text = strings.Join(lines[:i], "") + next + strings.Join(lines[i+1:], "")
				s := n.Relayout(gen)
				if s.Reset {
					t.Fatalf("edit %d: reset", k)
				}
				if n.lay.reused {
					reused++
				}
				have = applySplice(have, s)
				if s.Moving {
					moving++
					// The move limits, once the index is made.
					h, err := n.MoreHeads(s.Gen, len(have), s.Step, 0)
					if err != nil {
						t.Fatal(err)
					}
					for k, i := range h.FixAt {
						have[i] = h.Fixes[k]
					}
				}
				want := n.lay.cells
				if n.lay.x == nil {
					t.Fatalf("edit %d: no index", k)
				}
				if len(have) != len(want) {
					t.Fatalf("edit %d: %d heads, want %d", k, len(have), len(want))
				}
				for j := range want {
					if w := headOf(&want[j]); !sameHeadAs(have[j], w) {
						t.Fatalf("edit %d (line %d %q): head %d is\n%+v\nwant\n%+v", k, i, next, j, have[j], w)
					}
				}
				// And the cells, move limits and all, and their bodies,
				// are a fresh layout's.
				fresh := n.newCellLayout()
				for !fresh.done {
					fresh.chunk(1 << 30)
				}
				for j := range fresh.cells {
					if c, w := fresh.cells[j], want[j]; !reflect.DeepEqual(c, w) {
						t.Fatalf("edit %d (line %d %q, reused %t): cell %d is\n%+v\nwant\n%+v", k, i, next, n.lay.reused, j, w, c)
					}
					if c, w := fresh.body(j), n.lay.body(j); !reflect.DeepEqual(c, w) {
						t.Fatalf("edit %d (reused %t): body %d is\n%+v\nwant\n%+v", k, n.lay.reused, j, w, c)
					}
				}
				if !slices.Equal(fresh.marks, n.lay.marks) {
					t.Fatalf("edit %d: marks differ", k)
				}
				gen = s.Gen
			}
		})
	}
}

// applySplice is what the page does with s: heads, the page's, as the
// splice says they changed.
func applySplice(heads HeadList, s Splice) HeadList {
	out := make(HeadList, 0, len(heads)-s.Remove+len(s.Heads))
	out = append(out, heads[:s.At]...)
	out = append(out, s.Heads...)
	for _, h := range heads[s.At+s.Remove:] {
		out = append(out, s.moved(h))
	}
	for k, i := range s.FixAt {
		out[i] = s.Fixes[k]
	}
	return out
}

func slicesClone(l HeadList) HeadList { return append(HeadList(nil), l...) }

// TestRelayoutReuse checks which edits Relayout lays out by reusing the
// cells around them: typing in prose does, while an edit that turns a
// heading into a paragraph or changes a block lays out afresh.
func TestRelayoutReuse(t *testing.T) {
	src := "# Top\n\nSome prose.\n\n```sh {#one}\necho one\n```\n\nMore prose, and a [link][ref].\n\n## Next\n\n- [ ] a task\n\n```sh {#two}\necho two\n```\n\nThe end.\n\n[ref]: https://example.com\n"
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	gen := n.Attach(1 << 30).Gen
	for _, e := range []struct {
		old, text string
		reuse     bool
	}{
		{"More prose, and a [link][ref].\n", "More prose, typed, and a [link][ref].\n", true},
		{"More prose, typed, and a [link][ref].\n", "More prose,\nover two lines.\n", true},
		{"Some prose.\n", "Some prose.\nAnd more.\n", true},
		{"- [ ] a task\n", "- [x] a task\n", true},
		{"# Top\n", "Top\n", false},                                             // a heading no longer
		{"echo one\n", "echo uno\n", false},                                     // a block's code
		{"- [x] a task\n", "- [x] a task\n- [ ] another\n", false},              // a task more
		{"[ref]: https://example.com\n", "[ref]: https://example.org\n", false}, // a link reference
	} {
		lines := strings.SplitAfter(string(n.d.Src), "\n")
		i := slices.Index(lines, e.old)
		if i < 0 {
			t.Fatalf("no line %q", e.old)
		}
		re, err := n.Edit(n.Version(), []LineEdit{{First: i, Last: i + 1, Text: e.text}})
		if err != nil || !re.Applied {
			t.Fatal(err, re)
		}
		s := n.Relayout(gen)
		if n.lay.reused != e.reuse {
			t.Errorf("%q to %q: reused %t, want %t", e.old, e.text, n.lay.reused, e.reuse)
		}
		if e.reuse && (s.Remove != 1 || len(s.Heads) > 2) {
			// The cell typed in, and only that, goes to the page: two
			// when a line of it became a cell of its own.
			t.Errorf("%q to %q: splice replaces %d cells with %d", e.old, e.text, s.Remove, len(s.Heads))
		}
		gen = s.Gen
	}
}
