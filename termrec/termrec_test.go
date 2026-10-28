package termrec

import (
	"bytes"
	"image/gif"
	"testing"
	"time"
)

func rec(events ...string) Recording {
	r := Recording{Cols: 20, Rows: 4}
	for i, e := range events {
		r.Events = append(r.Events, Event{At: time.Duration(i) * time.Second, Data: []byte(e)})
	}
	return r
}

func TestGIF(t *testing.T) {
	data, err := GIF(rec("hello\r\n", "\x1b[31mred\x1b[0m\r\n", "\x1b[2J\x1b[Hcleared"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 3 {
		t.Fatalf("got %d frames, want 3", len(g.Image))
	}
	if g.Delay[0] != 100 || g.Delay[2] != 200 {
		t.Errorf("delays = %v", g.Delay)
	}
	// The second frame stores only the row that changed.
	if h, full := g.Image[1].Bounds().Dy(), g.Config.Height; h >= full {
		t.Errorf("second frame is %d high, the full %d", h, full)
	}
}

func TestGIFMergesQuickWrites(t *testing.T) {
	r := Recording{Cols: 20, Rows: 4}
	for i, s := range []string{"a", "b", "c"} {
		r.Events = append(r.Events, Event{At: time.Duration(i) * time.Millisecond, Data: []byte(s)})
	}
	data, err := GIF(r)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := gif.DecodeAll(bytes.NewReader(data))
	if len(g.Image) != 2 {
		t.Errorf("got %d frames, want the first write and then the rest", len(g.Image))
	}
}

func TestGIFEmpty(t *testing.T) {
	if _, err := GIF(Recording{}); err != ErrEmpty {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

func TestFrame(t *testing.T) {
	data, err := GIF(rec("one\r\n", "two\r\n", "\x1b[2J"))
	if err != nil {
		t.Fatal(err)
	}
	// Text is drawn in the default foreground, so a frame with text has
	// more foreground pixels than the cleared last one.
	count := func(n int) int {
		img, err := Frame(data, n)
		if err != nil {
			t.Fatal(err)
		}
		fg := palette[defaultFG]
		c := 0
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r1, g1, b1, _ := img.At(x, y).RGBA()
				r2, g2, b2, _ := fg.RGBA()
				if r1 == r2 && g1 == g2 && b1 == b2 {
					c++
				}
			}
		}
		return c
	}
	one, two, last := count(1), count(2), count(0)
	if !(one > 0 && two > one) {
		t.Errorf("foreground pixels: frame 1 %d, frame 2 %d", one, two)
	}
	if last >= one {
		t.Errorf("last frame has %d foreground pixels, want fewer than frame 1's %d", last, one)
	}
	if count(99) != last {
		t.Error("a frame past the end isn't the last")
	}
}

func TestScreen(t *testing.T) {
	s := NewScreen(10, 3)
	s.Write([]byte("hello\r\nx\x1b[31my\x1b[0m\r\n"))
	if got := s.Text(); got != "hello\nxy\n\n" {
		t.Errorf("Text() = %q", got)
	}
}
