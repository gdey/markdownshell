package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termimg"
)

var (
	accent      = lipgloss.AdaptiveColor{Light: "#5A3FC0", Dark: "#B4A2FF"}
	runningCol  = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#E3B341"}
	okCol       = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	failCol     = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	dimCol      = lipgloss.AdaptiveColor{Light: "#6E7781", Dark: "#8B949E"}
	titleStyle  = lipgloss.NewStyle().Bold(true)
	dirtyStyle  = lipgloss.NewStyle().Foreground(runningCol)
	statusStyle = lipgloss.NewStyle().Foreground(dimCol)
	helpStyle   = lipgloss.NewStyle().Foreground(dimCol)
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(runningCol)
	stderrStyle = lipgloss.NewStyle().Foreground(failCol)
	savedStyle  = lipgloss.NewStyle().Foreground(dimCol)
	tagStyle    = lipgloss.NewStyle().Foreground(dimCol)
)

// refresh redraws the document into the viewport, noting where each
// block starts and ends.
func (m *Model) refresh() {
	if !m.ready {
		return
	}
	var buf strings.Builder
	m.tops, m.bots = map[int]int{}, map[int]int{}
	line, blank := 0, true
	add := func(s string) {
		s = strings.TrimRight(s, "\n")
		if s == "" {
			return
		}
		buf.WriteString(s)
		buf.WriteByte('\n')
		line += strings.Count(s, "\n") + 1
		blank = false
	}
	gap := func() {
		if !blank {
			buf.WriteByte('\n')
			line++
			blank = true
		}
	}
	pieces := render.Pieces(m.d, m.results, render.Options{Env: m.opts.Run.Environ})
	for n, p := range m.parts {
		if p.block < 0 {
			add(m.prose(pieces, n == 0, p))
			continue
		}
		gap()
		m.tops[p.block] = line
		add(m.block(p.block))
		m.bots[p.block] = line
		gap()
	}
	m.vp.SetContent(buf.String())
}

// prose draws part p's pieces, leaving out the front matter at the top.
func (m *Model) prose(pieces [][]byte, top bool, p part) string {
	src := string(bytes.Join(pieces[p.from:p.to], nil))
	if top {
		fm := m.d.Src[m.d.FrontMatter.Start:m.d.FrontMatter.End]
		src = strings.TrimPrefix(strings.TrimLeft(src, "\n"), string(fm))
	}
	return m.markdown(src)
}

// markdown draws md with glamour, once per text and width.
func (m *Model) markdown(md string) string {
	md = strings.TrimSpace(md)
	if s, ok := m.md[md]; ok {
		return s
	}
	out := md
	if md != "" {
		r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(m.opts.Style), glamour.WithWordWrap(max(m.width-4, 20)))
		if err == nil {
			if s, err := r.Render(md); err == nil {
				out = strings.Trim(s, "\n")
			}
		}
	}
	m.md[md] = out
	return out
}

// block draws block i: a header, its code in a box, and its output.
func (m *Model) block(i int) string {
	b := m.d.Blocks[i]
	r := m.results[i]
	selected := m.cur() == i
	running := m.active == i

	border := dimCol
	switch {
	case m.prompt != nil && m.prompt.block == b, running:
		border = runningCol
	case selected:
		border = accent
	}

	marker := "  "
	if selected {
		marker = lipgloss.NewStyle().Foreground(accent).Render("▶ ")
	}
	head := marker + titleStyle.Render(b.Lang)
	if b.IsInclude() {
		head += tagStyle.Render(" include " + b.Attrs["file"])
	} else {
		head += " " + lipgloss.NewStyle().Foreground(accent).Render("#"+b.ID)
	}
	if tags := tagsOf(b); tags != "" {
		head += " " + tagStyle.Render(tags)
	}
	if st := m.state(i, r); st != "" {
		head += "  " + st
	}

	body := strings.TrimRight(b.Body, "\n")
	if b.IsInclude() && r.Ran && r.Err == nil {
		body = strings.TrimRight(string(r.Stdout), "\n")
		if inc, _ := b.Include(); inc.Numbered {
			body = strings.TrimRight(render.NumberLines(body, max(inc.From, 1)), "\n")
		}
	}
	w := max(m.width-4, 10)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		MarginLeft(2).
		Width(w - 2).
		Render(clean(body))

	out := head + "\n" + box
	if o := m.output(i, r, w); o != "" {
		out += "\n" + lipgloss.NewStyle().MarginLeft(4).Render(o)
	}
	return out
}

// tagsOf lists the attributes worth seeing at a glance.
func tagsOf(b *doc.Block) string {
	var tags []string
	for _, a := range []string{"env", "out", "timeout", "run"} {
		if v, ok := b.Attrs[a]; ok && v != "" {
			tags = append(tags, a+"="+v)
		}
	}
	for _, a := range []string{"tty", "confirm", "skip"} {
		if b.Attrs.Has(a) {
			tags = append(tags, a)
		}
	}
	if inc, err := b.Include(); err == nil && b.IsInclude() {
		if inc.From > 0 || inc.To > 0 {
			tags = append(tags, fmt.Sprintf("lines %d-%d", max(inc.From, 1), inc.To))
		}
	}
	return strings.Join(tags, " ")
}

// state is the block's status for its header.
func (m *Model) state(i int, r *run.Result) string {
	switch {
	case m.active == i:
		return lipgloss.NewStyle().Foreground(runningCol).Render("running…")
	case m.prompt != nil && m.index[m.prompt.block] == i:
		return lipgloss.NewStyle().Foreground(runningCol).Render("waiting for an answer")
	case r.Ran && errors.Is(r.Err, context.Canceled):
		return lipgloss.NewStyle().Foreground(failCol).Render("✗ stopped")
	case r.Ran && r.Err != nil:
		return lipgloss.NewStyle().Foreground(failCol).Render("✗ " + r.Err.Error())
	case r.Ran && r.Failed():
		return lipgloss.NewStyle().Foreground(failCol).Render(fmt.Sprintf("✗ exit %d", r.ExitStatus()))
	case r.Ran && !r.Block.IsInclude():
		return lipgloss.NewStyle().Foreground(okCol).Render("✓")
	case r.Skipped && run.Runnable(r.Block):
		return tagStyle.Render("skipped")
	}
	return ""
}

// output is what shows under block i: live output while it runs, its
// result once it ran, or the output saved in the file.
func (m *Model) output(i int, r *run.Result, w int) string {
	wrap := lipgloss.NewStyle().Width(max(w-2, 10))
	if chunks, ok := m.live[i]; ok {
		var b strings.Builder
		for _, c := range chunks {
			s := clean(c.data)
			if c.stderr {
				s = stderrStyle.Render(s)
			}
			b.WriteString(s)
		}
		return wrap.Render(strings.TrimRight(b.String(), "\n"))
	}
	if r.Ran && !r.Block.IsInclude() {
		var parts []string
		if r.Image != "" {
			label := "image: "
			if run.OutKind(r.Block) == "recording" {
				label = "recording: "
			}
			parts = append(parts, savedStyle.Render(label+r.Image))
		} else if run.OutKind(r.Block) == "markdown" {
			if s := m.markdown(string(r.Stdout)); s != "" {
				parts = append(parts, s)
			}
		} else if s := strings.TrimRight(clean(string(r.Stdout)), "\n"); s != "" {
			parts = append(parts, s)
		}
		if s := strings.TrimRight(clean(string(r.Stderr)), "\n"); s != "" {
			parts = append(parts, stderrStyle.Render(s))
		}
		out := wrap.Render(strings.Join(parts, "\n"))
		if p := m.preview(r.Image, w-2); p != "" {
			out += "\n" + p
		}
		return out
	}
	b := m.d.Blocks[i]
	if b.HasOutput() {
		out := saved(m.d.Src[b.Output.Start:b.Output.End])
		if link := imageLink(out); link != "" {
			s := savedStyle.Render("saved output: " + link)
			if p := m.preview(link, w-2); p != "" {
				s += "\n" + p
			}
			return s
		}
		if run.OutKind(b) == "markdown" {
			return savedStyle.Render("saved output:") + "\n" + m.markdown(out)
		}
		return wrap.Render(savedStyle.Render("saved output:\n" + clean(out)))
	}
	return ""
}

// imageRE finds a markdown image link.
var imageRE = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)

// imageLink returns the target of the first image link in md, or "".
func imageLink(md string) string {
	if m := imageRE.FindStringSubmatch(md); m != nil {
		return m[1]
	}
	return ""
}

// imagePath resolves link, relative to the document, for termimg.Load.
func (m *Model) imagePath(link string) string {
	if link == "" || strings.HasPrefix(link, "data:") || filepath.IsAbs(link) {
		return link
	}
	return filepath.Join(filepath.Dir(m.opts.Run.File), filepath.FromSlash(link))
}

// preview draws the image at link as half blocks at most cols wide, or
// returns "" when previews are off or the image can't be read. Previews
// are cached by link, the file's time and the width.
func (m *Model) preview(link string, cols int) string {
	if !m.opts.Images || link == "" {
		return ""
	}
	path := m.imagePath(link)
	key := fmt.Sprintf("%s|%d", path, cols)
	if !strings.HasPrefix(path, "data:") {
		info, err := os.Stat(path)
		if err != nil {
			return ""
		}
		key += "|" + info.ModTime().String()
	}
	if s, ok := m.images[key]; ok {
		return s
	}
	s := ""
	if img, err := termimg.Load(path); err == nil {
		s = termimg.HalfBlocks(img, min(cols, previewCols), previewRows)
	}
	m.images[key] = s
	return s
}

// The largest preview, in cells.
const previewCols, previewRows = 64, 16

// saved is output saved in the file without its output fences and
// region markers.
func saved(src []byte) string {
	var lines []string
	for _, l := range strings.Split(string(src), "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") || strings.HasPrefix(t, "<!-- mdsh") {
			continue
		}
		lines = append(lines, l)
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// clean turns tabs into spaces and drops carriage returns and control
// characters that would upset the layout, keeping newlines and color
// escapes.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n', r == 0x1b:
			return r
		case r < 0x20, r == 0x7f:
			return -1
		}
		return r
	}, strings.ReplaceAll(s, "\t", "    "))
}
