package notebook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// A diagram is a fenced d2 or svg block drawn as a picture. It holds the
// fenced block, which is drawn too, so the window can switch between the
// two. It's drawn when it's rendered, not when it's parsed. Without a
// picture, such as when d2 isn't installed or the diagram has a
// mistake, note says why and only the code shows.
type diagram struct {
	ast.BaseBlock
	lang        string
	body        []byte
	light, dark []byte
	note        string
}

var kindDiagram = ast.NewNodeKind("Diagram")

func (d *diagram) Kind() ast.NodeKind { return kindDiagram }

func (d *diagram) Dump(src []byte, level int) { ast.DumpHelper(d, src, level, nil, nil) }

// diagrams wraps fenced d2 and svg blocks in a diagram.
type diagrams struct{}

func (diagrams) Transform(root *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var fences []*ast.FencedCodeBlock
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if fc, ok := node.(*ast.FencedCodeBlock); ok && entering {
			if l := string(fc.Language(src)); l == "d2" || l == "svg" {
				fences = append(fences, fc)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, fc := range fences {
		var body bytes.Buffer
		for i := range fc.Lines().Len() {
			seg := fc.Lines().At(i)
			body.Write(seg.Value(src))
		}
		d := &diagram{lang: string(fc.Language(src)), body: body.Bytes()}
		parent := fc.Parent()
		parent.ReplaceChild(parent, fc, d)
		d.AppendChild(d, fc)
	}
}

// diagramRenderer draws a diagram as a figure: its pictures as images,
// so an SVG's scripts don't run and it can't load anything, then the
// fenced block and the note.
type diagramRenderer struct{}

func (diagramRenderer) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(kindDiagram, func(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		d := node.(*diagram)
		if !entering {
			if d.note != "" {
				_, _ = w.WriteString(`<p class="diagram-note">`)
				_, _ = w.Write(util.EscapeHTML([]byte(d.note)))
				_, _ = w.WriteString("</p>\n")
			}
			_, _ = w.WriteString("</figure>\n")
			return ast.WalkContinue, nil
		}
		if d.lang == "svg" {
			d.light = d.body
		} else {
			d.light, d.dark, d.note = drawD2(d.body)
		}
		_, _ = w.WriteString(`<figure class="diagram" data-lang="` + d.lang + `">`)
		if d.light != nil {
			_, _ = w.WriteString(`<div class="diagram-image">`)
			img := func(class string, svg []byte) {
				_, _ = w.WriteString(`<img` + class + ` alt="` + d.lang + ` diagram"` + svgWidth(svg) + ` src="data:image/svg+xml;base64,`)
				_, _ = w.WriteString(base64.StdEncoding.EncodeToString(svg))
				_, _ = w.WriteString(`">`)
			}
			if d.dark != nil {
				img(` class="light"`, d.light)
				img(` class="dark"`, d.dark)
			} else {
				img("", d.light)
			}
			_, _ = w.WriteString("</div>\n")
		}
		return ast.WalkContinue, nil
	})
}

var (
	svgTag   = regexp.MustCompile(`<svg\b[^>]*>`)
	svgAttr  = regexp.MustCompile(`\s(width|viewBox)\s*=\s*["']([^"']*)["']`)
	viewSize = regexp.MustCompile(`^\s*[-+.\d]+[\s,]+[-+.\d]+[\s,]+([.\d]+)[\s,]+[.\d]+\s*$`)
)

// svgWidth is a width attribute for an image of svg, from its viewBox
// when its root has no width of its own, as d2's hasn't. An image of an
// SVG without a width fills the line, so a small diagram would be drawn
// big.
func svgWidth(svg []byte) string {
	tag := svgTag.Find(svg)
	var box string
	for _, m := range svgAttr.FindAllSubmatch(tag, -1) {
		if string(m[1]) == "width" {
			return ""
		}
		box = string(m[2])
	}
	if m := viewSize.FindStringSubmatch(box); m != nil {
		if w, err := strconv.ParseFloat(m[1], 64); err == nil && w > 0 {
			return fmt.Sprintf(` width="%d"`, int(math.Ceil(w)))
		}
	}
	return ""
}

// d2Themes are the d2 themes for the light and dark looks: Neutral
// Default and Dark Mauve.
var d2Themes = [2]int64{0, 200}

// d2Timeout is how long d2 has to draw one diagram. A layout that takes
// longer is left to finish in the background, and its result kept.
var d2Timeout = 10 * time.Second

type d2Drawing struct {
	light, dark []byte
	note        string
}

// d2Cache keeps what d2 drew, by the diagram's source, so the window
// doesn't lay it out again for every redraw. When it fills it starts
// over.
var d2Cache = struct {
	sync.Mutex
	m map[[sha256.Size]byte]d2Drawing
}{m: map[[sha256.Size]byte]d2Drawing{}}

const d2CacheSize = 128

// d2Mu lets one diagram be drawn at a time, as the ruler that measures
// text isn't safe to share.
var d2Mu sync.Mutex

// d2Ruler measures text for d2's layouts. Making one loads its fonts,
// so it's made once, when the first diagram is drawn.
var d2Ruler = sync.OnceValues(textmeasure.NewRuler)

// d2Quiet keeps d2's logging out of the app's output.
var d2Quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// drawD2 draws the d2 source as SVG, in the light and dark themes, or
// says why it can't.
func drawD2(src []byte) (light, dark []byte, note string) {
	key := sha256.Sum256(src)
	d2Cache.Lock()
	got, ok := d2Cache.m[key]
	d2Cache.Unlock()
	if ok {
		return got.light, got.dark, got.note
	}
	done := make(chan d2Drawing, 1)
	go func() {
		got := d2Draw(string(src))
		d2Cache.Lock()
		if len(d2Cache.m) >= d2CacheSize {
			clear(d2Cache.m)
		}
		d2Cache.m[key] = got
		d2Cache.Unlock()
		done <- got
	}()
	select {
	case got = <-done:
		return got.light, got.dark, got.note
	case <-time.After(d2Timeout):
		return nil, nil, "d2 took too long to draw this diagram."
	}
}

// d2Draw lays out and draws src with the d2 library and its dagre
// layout.
func d2Draw(src string) (got d2Drawing) {
	d2Mu.Lock()
	defer d2Mu.Unlock()
	defer func() {
		// A layout bug shouldn't take the window down with it.
		if r := recover(); r != nil {
			got = d2Drawing{note: fmt.Sprintf("d2: %v", r)}
		}
	}()
	ruler, err := d2Ruler()
	if err != nil {
		return d2Drawing{note: "d2: " + err.Error()}
	}
	ctx := log.With(context.Background(), d2Quiet)
	pad := int64(20)
	opts := &d2svg.RenderOpts{Pad: &pad, ThemeID: &d2Themes[0]}
	diagram, _, err := d2lib.Compile(ctx, src, &d2lib.CompileOptions{
		Ruler: ruler,
		LayoutResolver: func(string) (d2graph.LayoutGraph, error) {
			return d2dagrelayout.DefaultLayout, nil
		},
	}, opts)
	if err != nil {
		return d2Drawing{note: "d2: " + d2Message(err.Error())}
	}
	var out [2][]byte
	for i := range d2Themes {
		opts.ThemeID = &d2Themes[i]
		if out[i], err = d2svg.Render(diagram, opts); err != nil {
			return d2Drawing{note: "d2: " + err.Error()}
		}
	}
	return d2Drawing{light: out[0], dark: out[1]}
}

// d2Message is d2's compile error made readable: the "-:" or ":" it puts
// before each line and column taken out.
func d2Message(msg string) string {
	var lines []string
	for l := range strings.Lines(msg) {
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(l, "-:")
		l = strings.TrimPrefix(l, ":")
		if l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

func diagramTransformer() parser.Option {
	return parser.WithASTTransformers(util.Prioritized(diagrams{}, 500))
}

func diagramRendererOption() renderer.Option {
	return renderer.WithNodeRenderers(util.Prioritized(diagramRenderer{}, 500))
}
