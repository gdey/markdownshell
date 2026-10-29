// Package report turns a run into a file to share: the printed markdown,
// a standalone HTML page or a PDF. The CLI's --pdf and --html and the
// GUI's Report dialog both use it, so they make the same reports.
package report

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/pdf"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

// Format is a kind of report.
type Format string

const (
	// Markdown is what mdsh prints: placeholders as values, mdsh:if
	// sections decided, and no #! line. It is what mdsh -n -o saves.
	Markdown Format = "markdown"
	// HTML is a standalone page, with local images inside it.
	HTML Format = "html"
	// PDF is what mdsh --pdf writes.
	PDF Format = "pdf"
)

// Formats lists every format, in the order a menu shows them.
var Formats = []Format{PDF, HTML, Markdown}

// Ext is the format's usual file extension, with its dot.
func (f Format) Ext() string {
	switch f {
	case HTML:
		return ".html"
	case PDF:
		return ".pdf"
	}
	return ".md"
}

// FormatOf names the format a file's extension asks for.
func FormatOf(path string) (Format, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return Markdown, true
	case ".html", ".htm":
		return HTML, true
	case ".pdf":
		return PDF, true
	}
	return "", false
}

// Options control what a report shows.
type Options struct {
	Format Format
	// HideCode leaves out runnable blocks, as --hide-code does.
	HideCode bool
	// HideOutput leaves out output, as --hide-output does.
	HideOutput bool
	// Env is the caller's environment, for {{$VAR}} placeholders and
	// conditions; see render.Options.
	Env []string
	// Dir is where relative image links are read from, for HTML and
	// PDF. Empty means the working directory.
	Dir string
	// Title is the report's title. Empty means none for a PDF and
	// "mdsh report" for an HTML page.
	Title string
}

// Render makes a report of d with results, matched to its blocks as
// render.Markdown matches them. Blocks without a result show the output
// saved in the file, so run.Includes(d, ...) as results reports the file
// as it was last saved, running nothing.
func Render(d *doc.Document, results []*run.Result, opts Options) ([]byte, error) {
	md := render.Markdown(d, results, render.Options{HideCode: opts.HideCode, HideOutput: opts.HideOutput, Env: opts.Env})
	switch opts.Format {
	case Markdown, "":
		return md, nil
	case HTML:
		return page(md, opts)
	case PDF:
		out, err := pdf.Render(md, pdf.Options{Dir: opts.Dir, Title: opts.Title})
		if err != nil {
			return nil, fmt.Errorf("pdf: %w", err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown report format %q", opts.Format)
}

// Title is a report title for file: its name without the extension.
func Title(file string) string {
	if file == "" {
		return ""
	}
	return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

// stripFrontMatter drops YAML front matter at the start of md, as
// pdf.Render does.
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
