package doc

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
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
	Env        struct {
		// Share lists the variables passed into the top-level env from
		// the caller, and the default for sub-envs. Nil means all.
		Share []string `yaml:"share"`
	} `yaml:"env"`
}

// Interpolating reports whether placeholders are filled in.
func (o Options) Interpolating() bool { return o.Interpolate == nil || *o.Interpolate }

// parseFrontMatter reads YAML front matter starting at pos, if any.
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
				return Span{}, opts, fmt.Errorf("front matter: %w", err)
			}
			switch fm.Mdsh.OnError {
			case "", "continue", "stop":
			default:
				return Span{}, opts, fmt.Errorf("front matter: on_error must be continue or stop, got %q", fm.Mdsh.OnError)
			}
			return Span{pos, e}, fm.Mdsh, nil
		}
		p = e
	}
	// No closing line: not front matter, just a thematic break.
	return Span{pos, pos}, opts, nil
}
