package termimg

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func square(c color.Color, n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestLoadAndHalfBlocks(t *testing.T) {
	var buf bytes.Buffer
	png.Encode(&buf, square(color.RGBA{255, 0, 0, 255}, 8))
	file := filepath.Join(t.TempDir(), "red.png")
	os.WriteFile(file, buf.Bytes(), 0o644)
	for _, link := range []string{file, "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())} {
		img, err := Load(link)
		if err != nil {
			t.Fatalf("%.30s: %v", link, err)
		}
		got := HalfBlocks(img, 4, 10)
		lines := strings.Split(got, "\n")
		if len(lines) != 2 || strings.Count(lines[0], "▀") != 4 || !strings.Contains(got, "38;2;255;0;0;48;2;255;0;0m") {
			t.Errorf("%.30s: %q", link, got)
		}
	}
	if _, err := Decode([]byte("<svg xmlns='x'/>")); err != ErrSVG {
		t.Errorf("svg: %v", err)
	}
}

func TestFit(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for _, tc := range []struct{ cols, rows, w, h int }{
		{40, 40, 40, 10},
		{40, 5, 20, 5},
		{400, 400, 200, 50},
	} {
		if w, h := Fit(img, tc.cols, tc.rows); w != tc.w || h != tc.h {
			t.Errorf("Fit(%d, %d) = %d, %d; want %d, %d", tc.cols, tc.rows, w, h, tc.w, tc.h)
		}
	}
}

func TestGIFLastFrame(t *testing.T) {
	pal := color.Palette{color.Black, color.White}
	f1 := image.NewPaletted(image.Rect(0, 0, 2, 2), pal)
	f2 := image.NewPaletted(image.Rect(1, 1, 2, 2), pal)
	f2.SetColorIndex(1, 1, 1)
	var buf bytes.Buffer
	gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{f1, f2}, Delay: []int{0, 0}, Disposal: []byte{gif.DisposalNone, gif.DisposalNone}})
	img, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := img.At(1, 1).RGBA(); r != 0xffff {
		t.Error("last frame not drawn")
	}
	if r, _, _, _ := img.At(0, 0).RGBA(); r != 0 {
		t.Error("first frame lost")
	}
}

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want Protocol
	}{
		{map[string]string{"TERM": "xterm-kitty"}, Kitty},
		{map[string]string{"TERM_PROGRAM": "iTerm.app"}, ITerm2},
		{map[string]string{"TERM": "foot"}, Sixel},
		{map[string]string{"TERM": "xterm-256color"}, None},
	} {
		if got := Detect(func(k string) string { return tc.env[k] }); got != tc.want {
			t.Errorf("%v: %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestShow(t *testing.T) {
	img := square(color.RGBA{0, 0, 255, 255}, 7)
	for p, want := range map[Protocol]string{Kitty: "\x1b_Ga=T,f=100,m=0;", ITerm2: "\x1b]1337;File=inline=1;", Sixel: "\x1bPq\"1;1;7;7"} {
		var buf bytes.Buffer
		if err := Show(&buf, p, img, 80); err != nil || !strings.HasPrefix(buf.String(), want) {
			t.Errorf("%v: err %v, %q", p, err, buf.String()[:min(40, buf.Len())])
		}
	}
	if err := Show(&bytes.Buffer{}, None, img, 80); err == nil {
		t.Error("None showed an image")
	}
}
