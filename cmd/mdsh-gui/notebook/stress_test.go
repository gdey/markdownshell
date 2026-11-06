package notebook

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/internal/stress"
)

// stressSizes are the lengths, in lines, the stress benchmarks try each
// shape at. A 1M-line document takes a while: pick sizes with -bench,
// such as -bench 'Stress/.*/100K'.
var stressSizes = []struct {
	name  string
	lines int
}{{"10K", 10_000}, {"100K", 100_000}, {"1M", 1_000_000}}

// openStress opens a document of shape and lines, in a file of its own.
func openStress(tb testing.TB, shape string, lines int) (*Notebook, string) {
	tb.Helper()
	src := stress.Write(shape, lines)
	file := filepath.Join(tb.TempDir(), shape+".md")
	if err := os.WriteFile(file, src, 0o644); err != nil {
		tb.Fatal(err)
	}
	n, err := Open(file, Options{})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(n.Close)
	return n, string(src)
}

// TestStressShapes checks that each shape makes a document mdsh reads as
// meant: its blocks run, its saved output is theirs, and nothing in it
// is a mistake the console would warn of.
func TestStressShapes(t *testing.T) {
	for _, shape := range stress.Shapes {
		t.Run(shape, func(t *testing.T) {
			src := stress.Write(shape, 2000)
			if n := bytes.Count(src, []byte("\n")); n < 2000 || n > 2600 {
				t.Errorf("%d lines, want about 2000", n)
			}
			n, _ := openStress(t, shape, 2000)
			cells := n.Cells()
			var blocks, outputs, inputs int
			for _, c := range cells {
				switch {
				case c.Textbox != nil || c.Choice != nil:
					inputs++
					if p := problemOf(c); p != "" {
						t.Errorf("input at line %d: %s", c.Line, p)
					}
				case c.Block >= 0:
					blocks++
					if c.Output != "" {
						outputs++
					}
					if c.Undeletable != "" && !strings.Contains(c.Undeletable, "depends=") {
						t.Errorf("block at line %d: %s", c.Line, c.Undeletable)
					}
				}
			}
			if want := blocks; shape == "flat" {
				if want = 0; outputs != want || blocks == 0 {
					t.Errorf("%d blocks, %d with output; want some, none with output", blocks, outputs)
				}
			} else if blocks == 0 || outputs != want {
				t.Errorf("%d blocks, %d with output; want some, all with output", blocks, outputs)
			}
			if (shape == "inputs") != (inputs > 0) {
				t.Errorf("%d inputs", inputs)
			}
		})
	}
}

func problemOf(c Cell) string {
	if c.Textbox != nil {
		return c.Textbox.Problem
	}
	return c.Choice.Problem
}

// TestStressScales guards against laying out a document taking time in
// the square of its length, as it did when every cell asked the
// document a question that scanned all of it. It times a pause in
// typing (see BenchmarkStressType) on each shape at two lengths, ten
// times apart, and fails if the longer takes more than 30 times as long:
// linear work takes about 10 times, quadratic about 100. Each length's
// time is its best of a few tries, so a busy machine slowing one try
// doesn't fail it.
func TestStressScales(t *testing.T) {
	if testing.Short() {
		t.Skip("times long documents")
	}
	const small, factor, limit = 1500, 10, 30.0
	for _, shape := range stress.Shapes {
		t.Run(shape, func(t *testing.T) {
			short := typingTime(t, shape, small)
			long := typingTime(t, shape, small*factor)
			ratio := float64(long) / float64(short)
			t.Logf("%d lines: %v; %d lines: %v; %.1f times as long", small, short, small*factor, long, ratio)
			if ratio > limit {
				t.Errorf("%d times the lines took %.0f times as long (%v, then %v); laying out the cells is no longer linear", factor, ratio, short, long)
			}
		})
	}
}

// typingTime is the best of a few times for typing a character in the
// middle of a document of shape and lines and laying out its cells.
func typingTime(t *testing.T, shape string, lines int) time.Duration {
	n, text := openStress(t, shape, lines)
	at := typingAt(t, text)
	best := time.Duration(1<<63 - 1)
	for i := range 3 {
		next := text[:at] + fmt.Sprint(i) + text[at:]
		start := time.Now()
		if ok, err := n.SetText(text, next); !ok || err != nil {
			t.Fatal(ok, err)
		}
		n.Cells()
		best = min(best, time.Since(start))
		text = next
	}
	return best
}

// typingAt is where typing goes in text: the end of a prose line about
// halfway down.
func typingAt(tb testing.TB, text string) int {
	mid := len(text) / 2
	i := strings.Index(text[mid:], " in it.\n")
	if i < 0 {
		tb.Fatal("no prose line past the middle")
	}
	return mid + i + len(" in it.")
}

// BenchmarkStressOpen reads, parses and lays out each shape, as opening
// it in mdsh-gui does.
func BenchmarkStressOpen(b *testing.B) {
	for _, shape := range stress.Shapes {
		for _, size := range stressSizes {
			b.Run(shape+"/"+size.name, func(b *testing.B) {
				src := stress.Write(shape, size.lines)
				file := filepath.Join(b.TempDir(), shape+".md")
				if err := os.WriteFile(file, src, 0o644); err != nil {
					b.Fatal(err)
				}
				b.SetBytes(int64(len(src)))
				for b.Loop() {
					n, err := Open(file, Options{})
					if err != nil {
						b.Fatal(err)
					}
					n.Cells()
					n.Close()
				}
			})
		}
	}
}

// BenchmarkStressCells lays out each shape's cells, as the rendered view
// asks for them after every change.
func BenchmarkStressCells(b *testing.B) {
	for _, shape := range stress.Shapes {
		for _, size := range stressSizes {
			b.Run(shape+"/"+size.name, func(b *testing.B) {
				n, _ := openStress(b, shape, size.lines)
				for b.Loop() {
					n.Cells()
				}
			})
		}
	}
}

// BenchmarkStressType is a pause in typing in the source view: a
// character typed in the middle of the document goes to Go with
// SetText, and the view reads the cells again.
func BenchmarkStressType(b *testing.B) {
	for _, shape := range stress.Shapes {
		for _, size := range stressSizes {
			b.Run(shape+"/"+size.name, func(b *testing.B) {
				n, text := openStress(b, shape, size.lines)
				at := typingAt(b, text)
				i := 0
				for b.Loop() {
					i++
					next := text[:at] + fmt.Sprint(i%10) + text[at:]
					if ok, err := n.SetText(text, next); !ok || err != nil {
						b.Fatal(ok, err)
					}
					n.Cells()
					text = next
				}
			})
		}
	}
}
