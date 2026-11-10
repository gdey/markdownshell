package doc

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gdey/markdownshell/env"
)

// Options are the document-wide settings under the mdsh: key of the
// front matter.
type Options struct {
	// Shell runs sh and zsh blocks. Defaults to zsh when the document
	// has zsh blocks, else /bin/sh.
	Shell string `yaml:"shell"`
	// OnError is "continue" (default) or "stop".
	OnError string `yaml:"on_error"`
	// Step asks before running each block, as if every block had the
	// confirm attribute.
	Step bool `yaml:"step"`
	// Interpolate turns {{#block/env(VAR)}} placeholders on or off.
	// Defaults to on.
	Interpolate *bool `yaml:"interpolate"`
	// ShowStderr shows every block's stderr. Blocks can override it
	// with show-stderr=. By default stderr shows only for failed blocks.
	ShowStderr *bool `yaml:"show_stderr"`
	// Terminal is the size, in cells, of the terminal tty blocks run on
	// where mdsh draws it, as in the GUI. A zero cols fits the window;
	// a zero rows is 24.
	Terminal struct {
		Cols int `yaml:"cols"`
		Rows int `yaml:"rows"`
	} `yaml:"terminal"`
	Env struct {
		// Share lists the variables passed into the top-level env from
		// the caller, besides env.Base (except in a remote document),
		// and the default for sub-envs. Nil passes only env.Base; "..."
		// passes everything (see FullEnv).
		Share []string `yaml:"share"`
		// Source lists files sourced into every shell as it starts,
		// such as aliases and functions. A leading ~/ is the home
		// directory; other relative paths are relative to the document.
		Source []string `yaml:"source"`
	} `yaml:"env"`
}

// FullEnv reports whether env.share brings in the whole environment mdsh
// was started in, as "..." or "*" in the list does, and the names the
// list still leaves out with -NAME. Without it, blocks get only the base
// set (PATH, HOME, USER, ...) and the names the list gives; front ends
// show a banner on a file that brings in everything.
func (o Options) FullEnv() (full bool, except []string) {
	for _, k := range o.Env.Share {
		switch {
		case k == "..." || k == "*":
			full = true
		case strings.HasPrefix(k, "-"):
			except = append(except, k[1:])
		}
	}
	if !full {
		return false, nil
	}
	return true, except
}

// Shared lists the names env.share brings in from the environment mdsh
// was started in besides the base set (env.IsBase), in the list's
// order: the names a file asks for by name, such as GITHUB_TOKEN. "...",
// -NAME and base names aren't in it.
func (o Options) Shared() []string {
	var out []string
	for _, k := range o.Env.Share {
		if k == "..." || k == "*" || strings.HasPrefix(k, "-") || k == "" || env.IsBase(k) || slices.Contains(out, k) {
			continue
		}
		out = append(out, k)
	}
	return out
}

// ShareBanner is the line front ends show over a file whose env.share
// brings in more than the base set: the whole environment, naming what
// it leaves out, or the names it asks for. It is "" when the file gets
// only the base set. A file that names GITHUB_TOKEN gets a banner just
// as one with "..." does, so a secret never comes in unannounced
// (finding 2 of the v1.4.0 security review).
func (o Options) ShareBanner() string {
	if full, except := o.FullEnv(); full {
		text := "This file brings in all your environment variables"
		if len(except) > 0 {
			text += ", except " + strings.Join(except, ", ")
		}
		return text
	}
	names := o.Shared()
	if len(names) == 0 {
		return ""
	}
	return "This file brings in " + strings.Join(names, ", ") + " from your environment"
}

// Interpolating reports whether placeholders are filled in.
func (o Options) Interpolating() bool { return o.Interpolate == nil || *o.Interpolate }

// parseFrontMatter reads YAML front matter starting at pos, if any.
// Front matter mdsh can't read still covers its lines, with the error,
// and Options at their defaults.
func parseFrontMatter(src []byte, pos int) (Span, Options, error) {
	var opts Options
	end := lineEnd(src, pos)
	if string(bytes.TrimRight(src[pos:end], " \t\r\n")) != "---" {
		return Span{pos, pos}, opts, nil
	}
	body := end
	for p := body; p < len(src); {
		e := lineEnd(src, p)
		l := string(bytes.TrimRight(src[p:e], " \t\r\n"))
		if l == "---" || l == "..." {
			var fm struct {
				Mdsh Options `yaml:"mdsh"`
			}
			if err := yaml.Unmarshal(src[body:p], &fm); err != nil {
				return Span{pos, e}, opts, yamlError(err, lineNo(src, body)-1)
			}
			switch fm.Mdsh.OnError {
			case "", "continue", "stop":
			default:
				return Span{pos, e}, opts, fmt.Errorf("front matter: on_error must be continue or stop, got %q", fm.Mdsh.OnError)
			}
			if t := fm.Mdsh.Terminal; t.Cols < 0 || t.Rows < 0 || t.Cols > 1000 || t.Rows > 500 {
				return Span{pos, e}, opts, fmt.Errorf("front matter: terminal cols must be 0 to 1000 and rows 0 to 500, got %d by %d", t.Cols, t.Rows)
			}
			return Span{pos, e}, fm.Mdsh, nil
		}
		p = e
	}
	// No closing line: not front matter, just a thematic break.
	return Span{pos, pos}, opts, nil
}

// yamlLine is a line number in a YAML error.
var yamlLine = regexp.MustCompile(`\bline (\d+)\b`)

// yamlError is err, from reading the front matter's YAML, said as
// mdsh says errors, on one line: without YAML's "yaml: " and
// "unmarshal errors:", and with the lines it names counted from the top
// of the file, which is offset lines above the YAML's first line.
func yamlError(err error, offset int) error {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	msg = strings.TrimPrefix(msg, "unmarshal errors:\n")
	var parts []string
	for _, l := range strings.Split(msg, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	msg = yamlLine.ReplaceAllStringFunc(strings.Join(parts, "; "), func(m string) string {
		n, _ := strconv.Atoi(m[len("line "):])
		return "line " + strconv.Itoa(n+offset)
	})
	return fmt.Errorf("front matter: %s", msg)
}
