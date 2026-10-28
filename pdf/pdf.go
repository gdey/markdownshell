// Package pdf lays out markdown as a PDF, so a run document can be read
// or shared as a printed page. It is pure Go: markdown is parsed with
// goldmark and drawn with fpdf in the Go fonts.
package pdf

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/termrec"
	"github.com/gdey/markdownshell/textimg"
	"github.com/go-pdf/fpdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	bodySize = 10.5 // pt
	codeSize = 9    // pt
	margin   = 20   // mm
	indentW  = 6    // mm per list or quote level
)

// exitRE finds an output fence's exit status.
var exitRE = regexp.MustCompile(`\bexit=(\d+)`)

// headingSizes are the font sizes of heading levels 1 to 6, in pt.
var headingSizes = [...]float64{20, 16, 13.5, 12, bodySize, bodySize}

// Options control how Render lays out the page.
type Options struct {
	// Dir is where relative image paths are found. Empty means the
	// working directory.
	Dir string
	// Title is the PDF's title metadata.
	Title string
}

// Render lays out md as a PDF. Front matter is left out. Images that
// can't be drawn (remote, SVG, missing) show as their alt text.
func Render(md []byte, opts Options) ([]byte, error) {
	md = stripFrontMatter(md)
	gm := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root := gm.Parser().Parse(text.NewReader(md))

	f := fpdf.New("P", "mm", "A4", "")
	f.SetMargins(margin, margin, margin)
	f.SetAutoPageBreak(true, margin)
	f.SetTitle(opts.Title, true)
	f.SetCreator("mdsh", true)
	f.AddUTF8FontFromBytes("sans", "", goregular.TTF)
	f.AddUTF8FontFromBytes("sans", "B", gobold.TTF)
	f.AddUTF8FontFromBytes("sans", "I", goitalic.TTF)
	f.AddUTF8FontFromBytes("sans", "BI", gobolditalic.TTF)
	f.AddUTF8FontFromBytes("mono", "", gomono.TTF)
	f.AddUTF8FontFromBytes("mono", "B", gomonobold.TTF)
	f.SetFooterFunc(func() {
		f.SetY(-margin / 2)
		f.SetFont("sans", "", 8)
		f.SetTextColor(140, 140, 140)
		f.CellFormat(0, 4, fmt.Sprint(f.PageNo()), "", 0, "C", false, 0, "")
	})
	f.AddPage()

	r := &renderer{f: f, src: md, dir: opts.Dir, left: margin}
	r.blocks(root)
	if err := f.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type renderer struct {
	f   *fpdf.Fpdf
	src []byte
	dir string

	// Inline state: the font family, style flags and size in use.
	mono, bold, italic, strike bool
	size                       float64
	link                       string
	// color is the text color for the block being drawn.
	color [3]int

	// left is the current left edge, which lists and quotes push in.
	left   float64
	images int
}

func (r *renderer) setFont() {
	family, style := "sans", ""
	if r.mono {
		family = "mono"
	}
	if r.bold {
		style += "B"
	}
	if r.italic && !r.mono {
		style += "I"
	}
	if r.strike {
		style += "S"
	}
	if r.link != "" {
		style += "U"
	}
	size := r.size
	if r.mono {
		size *= 0.92
	}
	r.f.SetFont(family, style, size)
	if r.link != "" {
		r.f.SetTextColor(20, 90, 180)
	} else {
		r.f.SetTextColor(r.color[0], r.color[1], r.color[2])
	}
}

func (r *renderer) lineHeight() float64 { return r.size * 0.3528 * 1.45 }

func (r *renderer) setLeft(x float64) {
	r.left = x
	r.f.SetLeftMargin(x)
	r.f.SetX(x)
}

// gap adds vertical space before a block, except at the top of a page.
func (r *renderer) gap(mm float64) {
	if r.f.GetY() > margin+0.1 {
		r.f.Ln(mm)
	}
	r.f.SetX(r.left)
}

func (r *renderer) contentWidth() float64 {
	w, _ := r.f.GetPageSize()
	return w - margin - r.left
}

func (r *renderer) blocks(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.block(c)
	}
}

func (r *renderer) block(n ast.Node) {
	r.size, r.color = bodySize, [3]int{30, 30, 30}
	switch n := n.(type) {
	case *ast.Heading:
		level := min(max(n.Level, 1), 6)
		r.gap(r.lineHeight() * 0.9)
		r.size, r.bold = headingSizes[level-1], true
		r.f.Bookmark(plain(n, r.src), level-1, -1)
		r.inlines(n)
		r.bold = false
		r.endLine()
		if level <= 2 {
			r.rule(200)
		}
	case *ast.Paragraph:
		r.gap(r.lineHeight() * 0.5)
		r.inlines(n)
		r.endLine()
	case *ast.TextBlock:
		r.inlines(n)
		r.endLine()
	case *ast.FencedCodeBlock:
		r.gap(r.lineHeight() * 0.5)
		r.code(string(n.Language(r.src)), n.Info, lines(n, r.src))
	case *ast.CodeBlock:
		r.gap(r.lineHeight() * 0.5)
		r.code("", nil, lines(n, r.src))
	case *ast.Blockquote:
		r.gap(r.lineHeight() * 0.5)
		left, top, page := r.left, r.f.GetY(), r.f.PageNo()
		r.setLeft(left + indentW)
		r.blocks(n)
		if r.f.PageNo() == page {
			r.f.SetDrawColor(200, 200, 200)
			r.f.SetLineWidth(0.8)
			r.f.Line(left+1.5, top+1, left+1.5, r.f.GetY())
		}
		r.setLeft(left)
	case *ast.List:
		r.gap(r.lineHeight() * 0.3)
		r.list(n)
	case *ast.ThematicBreak:
		r.gap(r.lineHeight() * 0.5)
		r.rule(180)
	case *east.Table:
		r.gap(r.lineHeight() * 0.5)
		r.table(n)
	case *ast.HTMLBlock:
		// Raw HTML, including mdsh's output region markers, has no
		// place on a printed page.
	default:
		r.blocks(n)
	}
}

// endLine finishes the line the inline text ended on.
func (r *renderer) endLine() {
	r.f.Ln(r.lineHeight())
	r.f.SetX(r.left)
}

func (r *renderer) rule(gray int) {
	y := r.f.GetY() + 1
	w, _ := r.f.GetPageSize()
	r.f.SetDrawColor(gray, gray, gray)
	r.f.SetLineWidth(0.3)
	r.f.Line(r.left, y, w-margin, y)
	r.f.SetY(y + 1.5)
	r.f.SetX(r.left)
}

func (r *renderer) list(n *ast.List) {
	left := r.left
	num := n.Start
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		r.size, r.color = bodySize, [3]int{30, 30, 30}
		marker := "•"
		if n.IsOrdered() {
			marker = fmt.Sprintf("%d%c", num, n.Marker)
			num++
		}
		if cb, ok := taskBox(item); ok {
			// The Go fonts have no ballot box glyphs.
			marker = "[ ]"
			if cb {
				marker = "[x]"
			}
		}
		if !n.IsTight && item != n.FirstChild() {
			r.f.Ln(r.lineHeight() * 0.3)
		}
		r.setFont()
		r.f.SetX(left)
		r.f.CellFormat(indentW, r.lineHeight(), marker, "", 0, "L", false, 0, "")
		if r.f.GetX() > left+indentW {
			r.f.Write(r.lineHeight(), " ")
		}
		r.setLeft(left + indentW)
		r.f.SetX(left + indentW)
		first := true
		for c := item.FirstChild(); c != nil; c = c.NextSibling() {
			if first {
				// The first paragraph starts on the marker's line.
				if p, ok := c.(*ast.Paragraph); ok {
					r.inlines(p)
					r.endLine()
					first = false
					continue
				}
				if _, ok := c.(*ast.TextBlock); ok {
					r.block(c)
					first = false
					continue
				}
				r.endLine()
			}
			first = false
			r.block(c)
		}
		if first {
			r.endLine()
		}
		r.setLeft(left)
	}
}

func taskBox(item ast.Node) (checked, ok bool) {
	c := item.FirstChild()
	if c == nil {
		return false, false
	}
	if cb, ok := c.FirstChild().(*east.TaskCheckBox); ok {
		return cb.IsChecked, true
	}
	return false, false
}

func (r *renderer) inlines(n ast.Node) {
	r.setFont()
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.inline(c)
	}
}

func (r *renderer) write(s string) {
	if s == "" {
		return
	}
	r.setFont()
	if r.link != "" {
		r.f.WriteLinkString(r.lineHeight(), s, r.link)
	} else {
		r.f.Write(r.lineHeight(), s)
	}
}

func (r *renderer) inline(n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		r.write(string(n.Segment.Value(r.src)))
		if n.HardLineBreak() {
			r.endLine()
		} else if n.SoftLineBreak() {
			r.write(" ")
		}
	case *ast.String:
		r.write(string(n.Value))
	case *ast.CodeSpan:
		r.mono = true
		r.write(plain(n, r.src))
		r.mono = false
	case *ast.Emphasis:
		was := r.bold
		if n.Level >= 2 {
			r.bold = true
		} else {
			r.italic = !r.italic
		}
		r.children(n)
		if n.Level >= 2 {
			r.bold = was
		} else {
			r.italic = !r.italic
		}
	case *east.Strikethrough:
		r.strike = true
		r.children(n)
		r.strike = false
	case *ast.Link:
		r.link = string(n.Destination)
		r.children(n)
		r.link = ""
	case *ast.AutoLink:
		r.link = string(n.URL(r.src))
		r.write(string(n.Label(r.src)))
		r.link = ""
	case *ast.Image:
		r.image(n)
	case *east.TaskCheckBox, *ast.RawHTML:
		// The box is drawn as the list marker; inline HTML is dropped.
	default:
		r.children(n)
	}
}

func (r *renderer) children(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		r.inline(c)
	}
}

// code draws a code block as a shaded box. Output blocks get a
// terminal-like dark box, and stderr a red one.
func (r *renderer) code(lang string, info *ast.Text, body []string) {
	bg, fg := [3]int{244, 244, 246}, [3]int{30, 30, 30}
	label := lang
	infoText := ""
	if info != nil {
		infoText = strings.TrimSpace(string(info.Segment.Value(r.src)))
		label = infoText
	}
	if lang == "output" {
		bg, fg = [3]int{30, 30, 30}, [3]int{212, 212, 212}
		if strings.Contains(label, "stream=stderr") {
			bg, fg = [3]int{60, 20, 20}, [3]int{255, 200, 200}
		}
		// Only a failing status is worth a caption.
		label = ""
		if m := exitRE.FindStringSubmatch(infoText); m != nil && m[1] != "0" {
			label = "exit " + m[1]
		}
		if len(body) == 1 && body[0] == "" {
			body = nil
		}
	}
	// gutter is the length of the line numbers printing put in front of a
	// number_on include's lines.
	gutter := 0
	if _, attrs := doc.ParseInfo(infoText); attrs.Has(doc.IncludeAttr) {
		if inc, err := (&doc.Block{Attrs: attrs}).Include(); err == nil {
			label = includeLabel(inc)
			if inc.Numbered {
				gutter = numberedGutter(body, max(inc.From, 1))
			}
		}
	}
	r.mono, r.size = true, codeSize/0.92
	r.setFont()
	lh := codeSize * 0.3528 * 1.35
	w := r.contentWidth()
	r.f.SetFillColor(bg[0], bg[1], bg[2])
	r.f.SetCellMargin(2.5)
	if label != "" {
		r.f.SetTextColor(150, 150, 150)
		r.f.SetFont("mono", "", codeSize*0.85)
		r.f.CellFormat(w, lh, label, "", 1, "L", true, 0, "")
		r.setFont()
	}
	r.f.SetTextColor(fg[0], fg[1], fg[2])
	r.f.CellFormat(w, 1.5, "", "", 1, "L", true, 0, "")
	gw := 0.0
	if gutter > 0 {
		gw = r.f.GetStringWidth(strings.Repeat("0", gutter)) + 2.5
	}
	for _, line := range body {
		num := ""
		if gutter > 0 {
			num, line = strings.TrimSpace(line[:gutter]), line[gutter:]
		}
		for i, part := range r.wrapMono(line, w-5-gw) {
			r.f.SetX(r.left)
			if gutter > 0 {
				if i > 0 {
					num = ""
				}
				r.f.SetTextColor(150, 150, 150)
				r.f.CellFormat(gw, lh, num, "", 0, "R", true, 0, "")
				r.f.SetTextColor(fg[0], fg[1], fg[2])
			}
			r.f.CellFormat(w-gw, lh, part, "", 1, "L", true, 0, "")
		}
	}
	r.f.SetX(r.left)
	r.f.CellFormat(w, 1.5, "", "", 1, "L", true, 0, "")
	r.f.SetCellMargin(1)
	r.f.SetX(r.left)
	r.mono = false
}

// includeLabel captions an include block with the lines it shows.
func includeLabel(inc doc.Include) string {
	switch {
	case inc.From > 1 && inc.To > 0:
		return fmt.Sprintf("%s, lines %d–%d", inc.File, inc.From, inc.To)
	case inc.From > 1:
		return fmt.Sprintf("%s, from line %d", inc.File, inc.From)
	case inc.To > 0:
		return fmt.Sprintf("%s, lines 1–%d", inc.File, inc.To)
	}
	return inc.File
}

// numberedGutter returns the length of the "%*d  " line numbers that
// printing put in front of each line, counting from first, or 0 if any
// line lacks them.
func numberedGutter(lines []string, first int) int {
	w := len(strconv.Itoa(first + len(lines) - 1))
	for i, line := range lines {
		want := fmt.Sprintf("%*d  ", w, first+i)
		if !strings.HasPrefix(line, want) {
			return 0
		}
	}
	return w + 2
}

// wrapMono breaks a line of monospace text to fit w.
func (r *renderer) wrapMono(line string, w float64) []string {
	line = strings.ReplaceAll(line, "\t", "    ")
	cw := r.f.GetStringWidth("M")
	n := max(int(w/cw), 1)
	runes := []rune(line)
	if len(runes) <= n {
		return []string{line}
	}
	var out []string
	for len(runes) > n {
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return append(out, string(runes))
}

func (r *renderer) table(t *east.Table) {
	var rows [][]string
	var header int
	for row := t.FirstChild(); row != nil; row = row.NextSibling() {
		if _, ok := row.(*east.TableHeader); ok {
			header++
		}
		var cells []string
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			cells = append(cells, plain(c, r.src))
		}
		rows = append(rows, cells)
	}
	cols := len(t.Alignments)
	if cols == 0 {
		return
	}
	w := r.contentWidth() / float64(cols)
	lh := r.lineHeight()
	r.f.SetDrawColor(190, 190, 190)
	r.f.SetLineWidth(0.2)
	r.f.SetCellMargin(1.5)
	for i, cells := range rows {
		r.bold = i < header
		r.setFont()
		split := make([][]string, cols)
		h := 1
		for j := range cols {
			s := ""
			if j < len(cells) {
				s = cells[j]
			}
			split[j] = r.f.SplitText(s, w-3)
			if len(split[j]) == 0 {
				split[j] = []string{""}
			}
			h = max(h, len(split[j]))
		}
		rowH := float64(h)*lh + 1
		_, pageH := r.f.GetPageSize()
		if r.f.GetY()+rowH > pageH-margin {
			r.f.AddPage()
		}
		y := r.f.GetY()
		for j := range cols {
			x := r.left + float64(j)*w
			if i < header {
				r.f.SetFillColor(240, 240, 242)
				r.f.Rect(x, y, w, rowH, "FD")
			} else {
				r.f.Rect(x, y, w, rowH, "D")
			}
			align := "L"
			switch t.Alignments[j] {
			case east.AlignCenter:
				align = "C"
			case east.AlignRight:
				align = "R"
			}
			for k, s := range split[j] {
				r.f.SetXY(x, y+0.5+float64(k)*lh)
				r.f.CellFormat(w, lh, s, "", 0, align, false, 0, "")
			}
		}
		r.f.SetXY(r.left, y+rowH)
	}
	r.bold = false
	r.f.SetCellMargin(1)
}

// image draws an image on its own lines, scaled to fit the page.
func (r *renderer) image(n *ast.Image) {
	alt := plain(n, r.src)
	data, typ := r.load(string(n.Destination))
	if typ == "SVG" {
		// fpdf can't draw SVG, but mdsh's own screenshots are just text.
		if txt, ok := textimg.FromSVG(data); ok {
			if r.f.GetX() > r.left+0.1 {
				r.endLine()
			}
			size := r.size
			r.code("output", nil, textimg.Lines(txt))
			r.size = size
			return
		}
		data = nil
	}
	if data == nil {
		was := r.italic
		r.italic = true
		r.write("[image: " + alt + "]")
		r.italic = was
		return
	}
	r.images++
	name := fmt.Sprintf("img%d", r.images)
	info := r.f.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: typ}, bytes.NewReader(data))
	if r.f.Err() {
		r.f.ClearError()
		r.write("[image: " + alt + "]")
		return
	}
	// fpdf sizes images at 72 dpi; 144 suits mdsh's 2x screenshots and
	// most plots.
	w, h := info.Extent()
	w, h = w/2, h/2
	if maxW := r.contentWidth(); w > maxW {
		w, h = maxW, h*maxW/w
	}
	_, pageH := r.f.GetPageSize()
	if maxH := pageH - 2*margin; h > maxH {
		w, h = w*maxH/h, maxH
	}
	if r.f.GetX() > r.left+0.1 {
		r.endLine()
	}
	if r.f.GetY()+h > pageH-margin {
		r.f.AddPage()
	}
	r.f.ImageOptions(name, r.left, r.f.GetY(), w, h, true, fpdf.ImageOptions{ImageType: typ}, 0, "")
	r.f.SetX(r.left)
}

// load reads an image from a data: URI or a local path, and names its
// type. It returns nil for anything that isn't PNG, JPEG, GIF or SVG. A
// GIF becomes a PNG of one frame: the last, or the one a #frame=N
// fragment names, counting from 1, so an animation shows how it ends.
func (r *renderer) load(dest string) ([]byte, string) {
	dest, frag, _ := strings.Cut(dest, "#")
	data, typ := r.read(dest)
	if typ != "GIF" {
		return data, typ
	}
	n := 0
	if v, ok := strings.CutPrefix(frag, "frame="); ok {
		n, _ = strconv.Atoi(v)
	}
	img, err := termrec.Frame(data, n)
	if err != nil {
		return nil, ""
	}
	var b bytes.Buffer
	if err := png.Encode(&b, double(img)); err != nil {
		return nil, ""
	}
	return b.Bytes(), "PNG"
}

// double scales img up 2x, pixel for pixel. Images are laid out at half
// size, to suit mdsh's 2x screenshots, and terminal recordings are 1x.
func double(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, 2*b.Dx(), 2*b.Dy()))
	for y := range out.Bounds().Dy() {
		for x := range out.Bounds().Dx() {
			out.Set(x, y, img.At(b.Min.X+x/2, b.Min.Y+y/2))
		}
	}
	return out
}

// read reads an image from a data: URI or a local path, and names its
// type.
func (r *renderer) read(dest string) ([]byte, string) {
	var data []byte
	if rest, ok := strings.CutPrefix(dest, "data:"); ok {
		meta, payload, ok := strings.Cut(rest, ",")
		if !ok || !strings.HasSuffix(meta, ";base64") {
			return nil, ""
		}
		var err error
		if data, err = base64.StdEncoding.DecodeString(payload); err != nil {
			return nil, ""
		}
	} else {
		if u, err := url.Parse(dest); err != nil || u.Scheme != "" {
			return nil, ""
		} else {
			dest = u.Path
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(r.dir, filepath.FromSlash(dest))
		}
		var err error
		if data, err = os.ReadFile(dest); err != nil {
			return nil, ""
		}
	}
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG")):
		return data, "PNG"
	case bytes.HasPrefix(data, []byte("\xff\xd8")):
		return data, "JPG"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return data, "GIF"
	case bytes.Contains(data[:min(len(data), 512)], []byte("<svg")):
		return data, "SVG"
	}
	return nil, ""
}

// plain is the text inside n, without markup.
func plain(n ast.Node, src []byte) string {
	var b strings.Builder
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch c := c.(type) {
			case *ast.Text:
				b.Write(c.Segment.Value(src))
				if c.SoftLineBreak() || c.HardLineBreak() {
					b.WriteByte(' ')
				}
			case *ast.String:
				b.Write(c.Value)
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return b.String()
}

func lines(n ast.Node, src []byte) []string {
	var b strings.Builder
	segs := n.Lines()
	for i := 0; i < segs.Len(); i++ {
		s := segs.At(i)
		b.Write(s.Value(src))
	}
	return strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
}

// stripFrontMatter drops YAML front matter at the start of md.
func stripFrontMatter(md []byte) []byte {
	if !bytes.HasPrefix(md, []byte("---\n")) && !bytes.HasPrefix(md, []byte("---\r\n")) {
		return md
	}
	pos := bytes.IndexByte(md, '\n') + 1
	for pos < len(md) {
		end := bytes.IndexByte(md[pos:], '\n')
		if end < 0 {
			end = len(md) - pos
		} else {
			end++
		}
		line := strings.TrimRight(string(md[pos:pos+end]), "\r\n")
		pos += end
		if line == "---" || line == "..." {
			return md[pos:]
		}
	}
	return md
}
