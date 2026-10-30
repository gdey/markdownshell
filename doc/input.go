package doc

import (
	"regexp"
	"strconv"
	"strings"
)

// TextboxLang is the language of a textbox, a block that holds text
// typed in an interactive view instead of code:
//
//	```textbox {#host lines=1 placeholder="Which host?" var=host}
//	db1.example.com
//	```
//
// The body is the text. It never runs.
const TextboxLang = "textbox"

// ChoiceLang is the language of a choice, a block whose body lists
// options as task list items, the checked ones chosen:
//
//	```choice {#env mode=radio var=env}
//	- [ ] staging
//	- [x] prod
//	```
//
// It never runs.
const ChoiceLang = "choice"

// The values of a choice's mode=.
const (
	ChoiceDropdown = "dropdown" // pick one from a list; the default
	ChoiceRadio    = "radio"    // pick one of a group of buttons
	ChoiceMulti    = "multi"    // check any number of boxes
)

// IsTextbox reports whether b is a textbox.
func (b *Block) IsTextbox() bool { return b.Lang == TextboxLang && !b.IsInclude() }

// IsChoice reports whether b is a choice.
func (b *Block) IsChoice() bool { return b.Lang == ChoiceLang && !b.IsInclude() }

// IsInput reports whether b is a textbox or a choice: a block someone
// fills in, not one that runs.
func (b *Block) IsInput() bool { return b.IsTextbox() || b.IsChoice() }

// VarPrefix starts the name of the variable an input's var= binds.
const VarPrefix = "MDSH_VAR_"

// Var is the variable input b binds with var=, such as MDSH_VAR_host,
// and its value: a textbox's text, or the labels a choice has checked,
// one per line. ok is false when b binds none, the name isn't valid or an
// earlier input binds it.
func (b *Block) Var() (name, value string, ok bool) {
	v := b.Attrs["var"]
	if !b.IsInput() || !ValidVar(v) || b.VarTaken != "" {
		return "", "", false
	}
	if b.IsTextbox() {
		return VarPrefix + v, b.Textbox().Text, true
	}
	var checked []string
	for _, o := range b.Choice().Options {
		if o.Checked {
			checked = append(checked, o.Label)
		}
	}
	return VarPrefix + v, strings.Join(checked, "\n"), true
}

var varName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// ValidVar reports whether name can follow VarPrefix in a variable.
func ValidVar(name string) bool { return varName.MatchString(name) }

// Textbox is what a textbox block asks for.
type Textbox struct {
	// Text is the body without its last newline.
	Text string
	// Lines is the most lines the text may have, at least 1.
	Lines int
	// Placeholder is shown while the text is empty, in interactive views
	// only.
	Placeholder string
}

// Textbox reads a textbox's attributes and text.
func (b *Block) Textbox() Textbox {
	n, _ := strconv.Atoi(b.Attrs["lines"])
	return Textbox{
		Text:        strings.TrimSuffix(b.Body, "\n"),
		Lines:       max(n, 1),
		Placeholder: b.Attrs["placeholder"],
	}
}

// Choice is what a choice block asks for.
type Choice struct {
	// Mode is ChoiceDropdown, ChoiceRadio or ChoiceMulti.
	Mode string
	// Options are the body's task list items, in order. Other lines are
	// left alone.
	Options []Option
	// Placeholder is what a dropdown shows while nothing is chosen.
	Placeholder string
}

// Option is one of a choice's options.
type Option struct {
	Label   string `json:"label"`
	Checked bool   `json:"checked"`
}

var optionLine = regexp.MustCompile(`^(\s*[-*+]\s+\[)([ xX])(\]\s+)(.*?)\s*$`)

// Choice reads a choice's attributes and options.
func (b *Block) Choice() Choice {
	c := Choice{Mode: b.Attrs["mode"], Placeholder: b.Attrs["placeholder"]}
	if c.Mode != ChoiceRadio && c.Mode != ChoiceMulti {
		c.Mode = ChoiceDropdown
	}
	for line := range strings.Lines(b.Body) {
		if m := optionLine.FindStringSubmatch(strings.TrimSuffix(line, "\n")); m != nil {
			c.Options = append(c.Options, Option{Label: m[4], Checked: m[2] != " "})
		}
	}
	return c
}

// SetChoice returns d's source with choice b's options checked as
// checked says, one bool per option. Options past its end are unchecked.
func (d *Document) SetChoice(b *Block, checked []bool) []byte {
	var body strings.Builder
	i := 0
	for line := range strings.Lines(b.Body) {
		m := optionLine.FindStringSubmatchIndex(line)
		if m == nil {
			body.WriteString(line)
			continue
		}
		mark := " "
		if i < len(checked) && checked[i] {
			mark = "x"
			if line[m[4]] == 'X' {
				mark = "X"
			}
		}
		i++
		body.WriteString(line[:m[4]] + mark + line[m[5]:])
	}
	return d.SetBody(b, body.String())
}

// SetBody returns d's source with b's body replaced by body, which gets
// a newline at its end if it has none, unless it is empty. The fence
// gets longer if a line of body would close it, and each line starts
// with b's container prefix.
func (d *Document) SetBody(b *Block, body string) []byte {
	src := d.Src
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	open := string(src[b.Span.Start:lineEnd(src, b.Span.Start)])
	closing := string(src[b.BodySpan.End:b.Span.End])
	if n := fenceRun(b.Fence[0], body); n > len(b.Fence) || closing == "" || !strings.HasSuffix(open, "\n") {
		head := b.Lead
		if head == "" {
			head = b.Prefix
		}
		f := strings.Repeat(b.Fence[:1], max(n, len(b.Fence)))
		open, closing = head+f+b.Info+"\n", b.Prefix+f+"\n"
	}
	var out strings.Builder
	out.Write(src[:b.Span.Start])
	out.WriteString(open)
	for line := range strings.Lines(body) {
		if b.Prefix != "" && strings.TrimSpace(line) == "" {
			out.WriteString(strings.TrimRight(b.Prefix, " "))
		} else {
			out.WriteString(b.Prefix)
		}
		out.WriteString(line)
	}
	out.WriteString(closing)
	out.Write(src[b.Span.End:])
	return []byte(out.String())
}

// fenceRun is the length of a fence of c that no line of s can close:
// at least 3, and longer than the longest run of c.
func fenceRun(c byte, s string) int {
	n, run := 3, 0
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			run = 0
			continue
		}
		if run++; run >= n {
			n = run + 1
		}
	}
	return n
}
