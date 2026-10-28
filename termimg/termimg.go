// Package termimg shows images in a terminal: as colored half blocks,
// which any terminal with 24-bit color can show and which mix with other
// text, or full size through the kitty, iTerm2 or sixel protocols.
package termimg

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg" // decode JPEGs
	"image/png"
	"io"
	"os"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// ErrSVG is returned by Decode for SVG images, which it can't draw.
var ErrSVG = errors.New("can't show SVG images in the terminal")

// Load reads the image at link: a file path or a data: URI.
func Load(link string) (image.Image, error) {
	var data []byte
	var err error
	if rest, ok := strings.CutPrefix(link, "data:"); ok {
		meta, enc, ok := strings.Cut(rest, ",")
		if !ok || !strings.HasSuffix(meta, ";base64") {
			return nil, errors.New("not a base64 data: URI")
		}
		data, err = base64.StdEncoding.DecodeString(enc)
	} else {
		data, err = os.ReadFile(link)
	}
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// Decode decodes a PNG, JPEG or GIF. An animated GIF gives its last
// frame, which for a recording is how the session ended.
func Decode(data []byte) (image.Image, error) {
	head := bytes.TrimSpace(data[:min(len(data), 512)])
	if bytes.HasPrefix(head, []byte("<svg")) || bytes.HasPrefix(head, []byte("<?xml")) {
		return nil, ErrSVG
	}
	if bytes.HasPrefix(data, []byte("GIF8")) {
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return lastFrame(g), nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// lastFrame draws g's frames in turn and returns the result.
func lastFrame(g *gif.GIF) image.Image {
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	canvas := image.NewRGBA(bounds)
	for i, f := range g.Image {
		var keep *image.RGBA
		if i < len(g.Disposal) && g.Disposal[i] == gif.DisposalPrevious {
			keep = image.NewRGBA(bounds)
			draw.Draw(keep, bounds, canvas, image.Point{}, draw.Src)
		}
		draw.Draw(canvas, f.Bounds(), f, f.Bounds().Min, draw.Over)
		if i == len(g.Image)-1 {
			break
		}
		switch {
		case keep != nil:
			canvas = keep
		case i < len(g.Disposal) && g.Disposal[i] == gif.DisposalBackground:
			draw.Draw(canvas, f.Bounds(), image.Transparent, image.Point{}, draw.Src)
		}
	}
	return canvas
}

// Fit returns the size, in terminal cells, to draw img as half blocks
// within cols by rows, keeping its shape. Each cell is two pixels tall.
func Fit(img image.Image, cols, rows int) (w, h int) {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 || cols <= 0 || rows <= 0 {
		return 0, 0
	}
	w = min(cols, b.Dx())
	h = (w*b.Dy()/b.Dx() + 1) / 2
	if h > rows {
		h = rows
		w = max(1, 2*h*b.Dx()/b.Dy())
	}
	return w, max(h, 1)
}

// HalfBlocks draws img within cols by rows cells as lines of ▀, each
// cell showing two pixels with 24-bit colors. Transparent parts show
// the terminal's background.
func HalfBlocks(img image.Image, cols, rows int) string {
	w, h := Fit(img, cols, rows)
	if w == 0 {
		return ""
	}
	small := image.NewRGBA(image.Rect(0, 0, w, 2*h))
	xdraw.ApproxBiLinear.Scale(small, small.Bounds(), img, img.Bounds(), draw.Src, nil)
	var b strings.Builder
	for y := 0; y < 2*h; y += 2 {
		for x := 0; x < w; x++ {
			top, bot := small.RGBAAt(x, y), small.RGBAAt(x, y+1)
			switch {
			case top.A < 128 && bot.A < 128:
				b.WriteString("\x1b[0m ")
			case bot.A < 128:
				fmt.Fprintf(&b, "\x1b[0;38;2;%d;%d;%dm▀", top.R, top.G, top.B)
			case top.A < 128:
				fmt.Fprintf(&b, "\x1b[0;38;2;%d;%d;%dm▄", bot.R, bot.G, bot.B)
			default:
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", top.R, top.G, top.B, bot.R, bot.G, bot.B)
			}
		}
		b.WriteString("\x1b[0m")
		if y+2 < 2*h {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Protocol is a way to show an image at full resolution.
type Protocol int

const (
	None Protocol = iota
	Kitty
	ITerm2
	Sixel
)

// Detect guesses the terminal's image protocol from its environment:
// kitty and Ghostty speak kitty's, iTerm2 and WezTerm iTerm2's, and
// terminals that say so in TERM, or are known to, sixel.
func Detect(env func(string) string) Protocol {
	term, prog := env("TERM"), env("TERM_PROGRAM")
	switch {
	case env("KITTY_WINDOW_ID") != "" || term == "xterm-kitty" || term == "xterm-ghostty" || prog == "ghostty":
		return Kitty
	case prog == "iTerm.app" || prog == "WezTerm" || env("LC_TERMINAL") == "iTerm2":
		return ITerm2
	case strings.Contains(term, "sixel") || prog == "foot" || strings.HasPrefix(term, "foot") || term == "mlterm" || term == "contour":
		return Sixel
	}
	return None
}

// Show writes img to w, at most cols cells wide, through p.
func Show(w io.Writer, p Protocol, img image.Image, cols int) error {
	switch p {
	case Kitty:
		return kitty(w, img)
	case ITerm2:
		return iterm2(w, img, cols)
	case Sixel:
		return sixel(w, img)
	}
	return errors.New("no image protocol")
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}

// kitty sends img as a PNG in 4096-byte base64 chunks.
func kitty(w io.Writer, img image.Image) error {
	data, err := encodePNG(img)
	if err != nil {
		return err
	}
	enc := base64.StdEncoding.EncodeToString(data)
	for first := true; len(enc) > 0; first = false {
		n := min(4096, len(enc))
		more := 0
		if n < len(enc) {
			more = 1
		}
		ctl := fmt.Sprintf("m=%d", more)
		if first {
			ctl = "a=T,f=100," + ctl
		}
		if _, err := fmt.Fprintf(w, "\x1b_G%s;%s\x1b\\", ctl, enc[:n]); err != nil {
			return err
		}
		enc = enc[n:]
	}
	_, err = io.WriteString(w, "\r\n")
	return err
}

// iterm2 sends img with iTerm2's inline file escape.
func iterm2(w io.Writer, img image.Image, cols int) error {
	data, err := encodePNG(img)
	if err != nil {
		return err
	}
	width := "auto"
	if cols > 0 && img.Bounds().Dx() > cols*8 {
		width = fmt.Sprint(cols)
	}
	_, err = fmt.Fprintf(w, "\x1b]1337;File=inline=1;size=%d;width=%s;preserveAspectRatio=1:%s\a\r\n",
		len(data), width, base64.StdEncoding.EncodeToString(data))
	return err
}

// sixel draws img in a 216-color palette, six rows of pixels at a time.
func sixel(w io.Writer, img image.Image) error {
	b := img.Bounds()
	pal := make(color.Palette, 0, 216)
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for bl := 0; bl < 6; bl++ {
				pal = append(pal, color.RGBA{uint8(r * 51), uint8(g * 51), uint8(bl * 51), 255})
			}
		}
	}
	p := image.NewPaletted(image.Rect(0, 0, b.Dx(), b.Dy()), pal)
	draw.FloydSteinberg.Draw(p, p.Bounds(), img, b.Min)

	var out bytes.Buffer
	fmt.Fprintf(&out, "\x1bPq\"1;1;%d;%d", b.Dx(), b.Dy())
	for i, c := range pal {
		r, g, bl, _ := c.RGBA()
		fmt.Fprintf(&out, "#%d;2;%d;%d;%d", i, r*100/0xffff, g*100/0xffff, bl*100/0xffff)
	}
	for y := 0; y < b.Dy(); y += 6 {
		var used [216]bool
		for x := 0; x < b.Dx(); x++ {
			for dy := 0; dy < 6 && y+dy < b.Dy(); dy++ {
				used[p.ColorIndexAt(x, y+dy)] = true
			}
		}
		for c := range used {
			if !used[c] {
				continue
			}
			fmt.Fprintf(&out, "#%d", c)
			for x := 0; x < b.Dx(); x++ {
				bits := 0
				for dy := 0; dy < 6 && y+dy < b.Dy(); dy++ {
					if int(p.ColorIndexAt(x, y+dy)) == c {
						bits |= 1 << dy
					}
				}
				out.WriteByte(byte(63 + bits))
			}
			out.WriteByte('$')
		}
		out.WriteByte('-')
	}
	out.WriteString("\x1b\\\r\n")
	_, err := w.Write(out.Bytes())
	return err
}
