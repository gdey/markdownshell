// Package textimg renders terminal text as an image, so a block's
// output can be shown as a picture of what it printed.
package textimg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	pad        = 12
	fontSize   = 14
	lineHeight = 18
	// charWidth approximates a monospace glyph at fontSize in SVG, where
	// the viewer picks the font.
	charWidth = 8.4
	bg        = "#1e1e1e"
	fg        = "#d4d4d4"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07]*(\x07|\x1b\\)`)

// Lines cleans text for rendering: ANSI escapes and carriage returns are
// dropped, tabs expand to 8 columns, and a final newline is ignored.
func Lines(text string) []string {
	text = ansiRE.ReplaceAllString(text, "")
	text = strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if j := strings.LastIndexByte(l, '\r'); j >= 0 {
			l = l[j+1:]
		}
		lines[i] = expandTabs(l)
	}
	return lines
}

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

func columns(lines []string) int {
	w := 1
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > w {
			w = n
		}
	}
	return w
}

// SVG renders text as a terminal-style SVG.
func SVG(text string) []byte {
	lines := Lines(text)
	w := int(float64(columns(lines))*charWidth) + 2*pad
	h := len(lines)*lineHeight + 2*pad
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, w, h, w, h)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" rx="6" fill="%s"/>`, bg)
	fmt.Fprintf(&b, `<g font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace" font-size="%d" fill="%s">`, fontSize, fg)
	for i, l := range lines {
		if l == "" {
			continue
		}
		fmt.Fprintf(&b, `<text x="%d" y="%d" xml:space="preserve">%s</text>`, pad, pad+(i+1)*lineHeight-5, html.EscapeString(l))
	}
	b.WriteString("</g></svg>\n")
	return b.Bytes()
}

// FromSVG recovers the text of an SVG that SVG made. ok is false for any
// other SVG.
func FromSVG(svg []byte) (text string, ok bool) {
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var lines []string
	var y int
	mono := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		se, isStart := tok.(xml.StartElement)
		if !isStart {
			continue
		}
		switch se.Name.Local {
		case "g":
			mono = strings.HasPrefix(attr(se, "font-family"), "ui-monospace")
		case "text":
			if !mono {
				return "", false
			}
			if _, err := fmt.Sscan(attr(se, "y"), &y); err != nil {
				return "", false
			}
			var body string
			if err := dec.DecodeElement(&body, &se); err != nil {
				return "", false
			}
			i := (y-pad+5)/lineHeight - 1
			if i < 0 || i > 1<<16 {
				return "", false
			}
			for len(lines) <= i {
				lines = append(lines, "")
			}
			lines[i] = body
		}
	}
	if !mono {
		return "", false
	}
	return strings.Join(lines, "\n"), true
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// PNG renders text as a terminal-style PNG at 2x scale, using Go Mono.
func PNG(text string) ([]byte, error) {
	const scale = 2
	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: fontSize * scale, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer face.Close()

	lines := Lines(text)
	adv, _ := face.GlyphAdvance('M')
	w := columns(lines)*adv.Ceil() + 2*pad*scale
	h := (len(lines)*lineHeight + 2*pad) * scale
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{hex(bg)}, image.Point{}, draw.Src)

	d := &font.Drawer{Dst: img, Src: &image.Uniform{hex(fg)}, Face: face}
	for i, l := range lines {
		d.Dot = fixed.P(pad*scale, (pad+(i+1)*lineHeight-5)*scale)
		d.DrawString(l)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func hex(s string) color.RGBA {
	var c color.RGBA
	fmt.Sscanf(s, "#%02x%02x%02x", &c.R, &c.G, &c.B)
	c.A = 0xff
	return c
}
