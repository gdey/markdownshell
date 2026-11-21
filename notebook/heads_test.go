package notebook

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/internal/stress"
)

// headsDocs are documents for the head tests: the stress shapes, the
// examples, and a few made to move a head after it is sent.
func headsDocs(t *testing.T) map[string][]byte {
	docs := map[string][]byte{}
	for _, shape := range stress.Shapes {
		docs[shape] = stress.Write(shape, 3000)
	}
	examples, _ := filepath.Glob("../examples/**/*.md")
	more, _ := filepath.Glob("../examples/*.md")
	for _, f := range append(examples, more...) {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		docs[f] = src
	}
	var b strings.Builder
	b.WriteString("# Top\n\nSee [the docs][d] and - [ ] a task.\n\n")
	for i := range 40 {
		b.WriteString("## Part\n\n<!-- mdsh:collapse -->\n\n- [ ] task\n- [x] done\n\n")
		b.WriteString("```sh {#s" + string(rune('a'+i%26)) + strings.Repeat("x", i/26) + "}\necho hi\n```\n\n")
		if i%7 == 3 {
			b.WriteString("<!-- mdsh:if false -->\n\n```sh {#h" + strings.Repeat("y", i) + "}\necho hidden\n```\n\n<!-- mdsh:end -->\n\n")
		}
	}
	b.WriteString("# End\n\n[d]: https://example.com\n")
	docs["folds"] = []byte(b.String())
	return docs
}

// TestHeadsAreCells lays documents out a few cells at a time, as the page
// asks for them, and checks that the heads, with their fixes and bodies,
// are the cells Cells lays out.
func TestHeadsAreCells(t *testing.T) {
	for name, src := range headsDocs(t) {
		t.Run(filepath.Base(name), func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "doc.md")
			if err := os.WriteFile(file, src, 0o644); err != nil {
				t.Fatal(err)
			}
			n, err := Open(file, Options{})
			if err != nil {
				t.Skip(err)
			}
			defer n.Close()
			want := n.Cells()
			for _, seed := range []uint64{1, 2, 3} {
				r := rand.New(rand.NewPCG(seed, 0))
				h := n.Attach(1 + r.IntN(30))
				gen, since := h.Gen, h.Step
				got := decodeHeads(t, h.Heads)
				for !h.Done {
					h, err = n.MoreHeads(gen, len(got), since, 1+r.IntN(200))
					if err != nil {
						t.Fatal(err)
					}
					if h.At != len(got) {
						t.Fatalf("chunk at %d, want %d", h.At, len(got))
					}
					for k, f := range decodeHeads(t, h.Fixes) {
						got[h.FixAt[k]] = f
					}
					got = append(got, decodeHeads(t, h.Heads)...)
					since = h.Step
				}
				if h.Laid != h.Size {
					t.Errorf("done with %d of %d bytes laid out", h.Laid, h.Size)
				}
				if len(got) != len(want) {
					t.Fatalf("seed %d: %d cells, want %d", seed, len(got), len(want))
				}
				all := make([]int, len(got))
				for i := range all {
					all[i] = i
				}
				bodies, err := n.Bodies(gen, all)
				if err != nil {
					t.Fatal(err)
				}
				for i := range want {
					if w := headOf(&want[i]); !reflect.DeepEqual(got[i], w) {
						t.Fatalf("seed %d: head %d is\n%+v\nwant\n%+v", seed, i, got[i], w)
					}
					if !reflect.DeepEqual(bodies[i], want[i]) {
						t.Fatalf("seed %d: cell %d is\n%+v\nwant\n%+v", seed, i, bodies[i], want[i])
					}
				}
			}
		})
	}
}

// decodeHeads reads heads back from their JSON, as the page does.
func decodeHeads(t *testing.T, l HeadList) []Head {
	t.Helper()
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var rows [][]any
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	var out []Head
	line, from := 0, 0
	for _, r := range rows {
		num := func(k, def int) int {
			if k < len(r) {
				return int(r[k].(float64))
			}
			return def
		}
		str := func(k int) string {
			if k < len(r) {
				return r[k].(string)
			}
			return ""
		}
		h := Head{Line: line + num(0, 0), From: from + num(1, 0), Hash: str(3), Block: num(4, -1), Kind: num(5, 0), Flags: num(6, 0), Level: num(7, 0), CodeLines: num(8, 0), OutputLines: num(9, 0), SectionFrom: num(10, 0), SectionTo: num(11, 0), ID: str(12), Name: str(13), Title: str(14)}
		h.To = h.From + num(2, 0)
		if len(r) > 15 {
			for _, d := range r[15].([]any) {
				h.Depends = append(h.Depends, d.(string))
			}
		}
		line, from = h.Line, h.From
		out = append(out, h)
	}
	return out
}

// TestHeadsStale checks that a layout is turned down once the document
// changes, and once another is started.
func TestHeadsStale(t *testing.T) {
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, stress.Write("runbook", 500), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := Open(file, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	h := n.Attach(5)
	if _, err := n.Bodies(h.Gen, []int{0, 1}); err != nil {
		t.Fatal(err)
	}
	h2 := n.Attach(5)
	if _, err := n.MoreHeads(h.Gen, 5, 0, 5); !errors.Is(err, ErrLayoutStale) {
		t.Errorf("an older layout's heads: %v, want ErrLayoutStale", err)
	}
	text, v := n.TextAt()
	if _, err := n.Edit(v, []LineEdit{{First: 2, Last: 2, Text: "More prose.\n\n"}}); err != nil {
		t.Fatal(err, text[:10])
	}
	if _, err := n.Bodies(h2.Gen, []int{0}); !errors.Is(err, ErrLayoutStale) {
		t.Errorf("bodies after an edit: %v, want ErrLayoutStale", err)
	}
}

// TestHeadsHashes checks that cells with bodies alike have hashes alike,
// and cells with bodies that differ have hashes that differ.
func TestHeadsHashes(t *testing.T) {
	n, _ := openStress(t, "runbook", 2000)
	cells := n.Cells()
	type body struct {
		html, body, output string
		earlier            []EarlierRun
		http               *HTTPCell
	}
	byHash := map[string]body{}
	for _, c := range cells {
		b := body{c.HTML, c.Body, c.Output, c.Earlier, c.HTTP}
		if was, ok := byHash[c.Hash]; ok && !reflect.DeepEqual(was, b) {
			t.Fatalf("two bodies with hash %s:\n%+v\n%+v", c.Hash, was, b)
		}
		byHash[c.Hash] = b
	}
	if len(byHash) < len(cells)/4 {
		t.Errorf("%d hashes for %d cells", len(byHash), len(cells))
	}
}

// TestLinkRefs checks that link reference definitions are found as the
// regexp over the whole file found them.
func TestLinkRefs(t *testing.T) {
	src := []byte("[a]: http://a\n  [b c]: /b \"t\"\n    [code]: no\n[d]:\n[e]: x\r\ntext [f]: no\n[g]:\thttp://g")
	got := string(linkRefs(src))
	want := "[a]: http://a\n  [b c]: /b \"t\"\n[e]: x\r\n[g]:\thttp://g"
	if got != want {
		t.Errorf("linkRefs = %q, want %q", got, want)
	}
}
