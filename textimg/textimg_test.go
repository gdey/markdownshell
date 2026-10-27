package textimg

import (
	"bytes"
	"image/png"
	"reflect"
	"strings"
	"testing"
)

func TestLines(t *testing.T) {
	got := Lines("\x1b[31mred\x1b[0m\ta\r\nprogress 10%\rdone\n")
	want := []string{"red     a", "done"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lines = %q, want %q", got, want)
	}
}

func TestSVG(t *testing.T) {
	s := string(SVG("a < b & c\n\n  x"))
	for _, want := range []string{"a &lt; b &amp; c", `xml:space="preserve">  x<`, `<svg xmlns="http://www.w3.org/2000/svg"`} {
		if !strings.Contains(s, want) {
			t.Errorf("SVG missing %q:\n%s", want, s)
		}
	}
}

func TestPNG(t *testing.T) {
	b, err := PNG("hello\nworld")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if r := img.Bounds(); r.Dx() < 100 || r.Dy() < 2*2*18 {
		t.Errorf("image too small: %v", r)
	}
}
