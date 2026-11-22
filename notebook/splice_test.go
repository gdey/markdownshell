package notebook

import (
	"math/rand"
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
			moving := 0
			defer func() { t.Logf("%d of 60 splices came before the index", moving) }()
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
				// And the move limits, which bodies carry, are a fresh
				// layout's.
				for j, c := range n.cells() {
					if moveOf(&c) != moveOf(&want[j]) {
						t.Fatalf("edit %d: cell %d moves %+v, want %+v", k, j, moveOf(&want[j]), moveOf(&c))
					}
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
