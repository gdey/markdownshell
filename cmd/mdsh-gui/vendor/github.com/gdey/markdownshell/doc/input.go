package doc

import (
	"regexp"
	"slices"
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

// NoSave reports whether b carries the nosave attribute, or secret,
// which implies it: its value lives in the session only. Write-back
// keeps what the file holds after a runnable block, in its output
// copies and placeholders, and in a textbox or choice, instead of the
// run's output or what was typed or picked. See SetValue and
// OutputNoSave.
func (b *Block) NoSave() bool { return b.Attrs.Has("nosave") || b.Attrs.Has("secret") }

// OutputNoSave reports whether write-back keeps the output after b as
// the file has it: b is nosave, or an output fence attached to it is.
func (b *Block) OutputNoSave() bool { return b.NoSave() || b.outNoSave }

// SessionValue is the value SetValue gave nosave input b this session,
// and whether it has one. Without one, b's value is its body.
func (b *Block) SessionValue() (string, bool) {
	if b.session == nil {
		return "", false
	}
	return *b.session, true
}

// SetValue sets the value of nosave input b, and of every copy of it, for
// this session: a textbox's text, or the labels a choice has checked, one
// per line. The source is left alone, so saving keeps the body the file
// holds. It reports false, and sets nothing, when b isn't a nosave input.
func (d *Document) SetValue(b *Block, value string) bool {
	if !b.IsInput() || !b.NoSave() {
		return false
	}
	for _, c := range d.Mirrors(b) {
		v := value
		c.session = &v
	}
	return true
}

// SetChecks sets the picks of nosave choice b, and of every copy of it,
// for this session, as SetChoice does in the source: one bool per
// option. It reports false when b isn't a nosave choice.
func (d *Document) SetChecks(b *Block, checked []bool) bool {
	if !b.IsChoice() || !b.NoSave() {
		return false
	}
	var labels []string
	for i, o := range b.Choice().Options {
		if i < len(checked) && checked[i] {
			labels = append(labels, o.Label)
		}
	}
	return d.SetValue(b, strings.Join(labels, "\n"))
}

// InputFor is the textbox or choice that binds variable name, such as
// MDSH_VAR_host, with var=, or nil when none does.
func (d *Document) InputFor(name string) *Block {
	for _, b := range d.Blocks {
		if n, _, ok := b.Var(); ok && n == name {
			return b
		}
	}
	return nil
}

// VarPrefix starts the name of the variable an input's var= binds.
const VarPrefix = "MDSH_VAR_"

// Var is the variable input b binds with var=, such as MDSH_VAR_host,
// and its value: a textbox's text, or the labels a choice has checked,
// one per line. ok is false when b binds none, the name isn't valid or an
// earlier input binds it, including one b is a copy of.
func (b *Block) Var() (name, value string, ok bool) {
	v := b.Attrs["var"]
	if !b.IsInput() || !ValidVar(v) || b.VarTaken != "" || b.MirrorOf != "" {
		return "", "", false
	}
	return VarPrefix + v, b.Value(), true
}

// Value is what input b's var= holds, or would if it bound it.
func (b *Block) Value() string {
	if b.IsTextbox() {
		return b.Textbox().Text
	}
	var checked []string
	for _, o := range b.Choice().Options {
		if o.Checked {
			checked = append(checked, o.Label)
		}
	}
	return strings.Join(checked, "\n")
}

// controlDiff says how inputs a and b ask for different things, so b
// can't be a copy of a: "" when they are the same kind, with the same
// lines= or mode=, placeholder= and options, whatever is typed or
// checked.
func controlDiff(a, b *Block) string {
	switch {
	case a.Lang != b.Lang:
		return "it is a " + b.Lang + ", not a " + a.Lang
	case a.NoSave() != b.NoSave():
		return "nosave differs"
	case a.IsTextbox() && a.Textbox().Lines != b.Textbox().Lines:
		return "lines= differs"
	case a.IsTextbox():
		if a.Textbox().Placeholder != b.Textbox().Placeholder {
			return "placeholder= differs"
		}
		return ""
	}
	ca, cb := a.Choice(), b.Choice()
	switch {
	case ca.Mode != cb.Mode:
		return "mode= differs"
	case ca.Placeholder != cb.Placeholder:
		return "placeholder= differs"
	case !slices.EqualFunc(ca.Options, cb.Options, func(x, y Option) bool { return x.Label == y.Label }):
		return "its options differ"
	}
	return ""
}

// Mirrors is input b with every copy of it: the first input that binds
// its var= and those that mirror it, in order. It is just b when b has
// none.
func (d *Document) Mirrors(b *Block) []*Block {
	first := b.ID
	if b.MirrorOf != "" {
		first = b.MirrorOf
	}
	var out []*Block
	for _, c := range d.Blocks {
		if c.ID == first || (c.MirrorOf == first && c.IsInput()) {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		out = []*Block{b}
	}
	return out
}

// SetText returns d's source with textbox b's text replaced by text,
// and that of every copy of it.
func (d *Document) SetText(b *Block, text string) []byte {
	bodies := map[*Block]string{}
	for _, c := range d.Mirrors(b) {
		bodies[c] = text
	}
	return d.setBodies(bodies)
}

// MatchMirrors returns d's source with every copy of an input set to the
// value of the first, and whether that changed anything: the fix for
// the warning Parse gives when copies differ.
func (d *Document) MatchMirrors() ([]byte, bool) {
	bodies := map[*Block]string{}
	for _, b := range d.Blocks {
		if b.MirrorOf == "" {
			continue
		}
		first := d.Block(b.MirrorOf)
		if first == nil || b.Value() == first.Value() {
			continue
		}
		if b.IsTextbox() {
			bodies[b] = first.Textbox().Text
		} else {
			bodies[b] = choiceBody(b, checks(first))
		}
	}
	if len(bodies) == 0 {
		return d.Src, false
	}
	return d.setBodies(bodies), true
}

// checks is which of choice b's options are checked.
func checks(b *Block) []bool {
	var out []bool
	for _, o := range b.Choice().Options {
		out = append(out, o.Checked)
	}
	return out
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

// Textbox reads a textbox's attributes and text: the session's value for
// a nosave textbox that has one (see SetValue), else its body.
func (b *Block) Textbox() Textbox {
	n, _ := strconv.Atoi(b.Attrs["lines"])
	text := strings.TrimSuffix(b.Body, "\n")
	if v, ok := b.SessionValue(); ok {
		text = v
	}
	return Textbox{
		Text:        text,
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

// Choice reads a choice's attributes and options. A nosave choice with a
// session value (see SetValue) has the options that value names checked,
// whatever the body says.
func (b *Block) Choice() Choice {
	c := Choice{Mode: b.Attrs["mode"], Placeholder: b.Attrs["placeholder"]}
	if c.Mode != ChoiceRadio && c.Mode != ChoiceMulti {
		c.Mode = ChoiceDropdown
	}
	var picked map[string]bool
	if v, ok := b.SessionValue(); ok {
		picked = map[string]bool{}
		for _, label := range strings.Split(v, "\n") {
			if label != "" {
				picked[label] = true
			}
		}
	}
	for line := range strings.Lines(b.Body) {
		if m := optionLine.FindStringSubmatch(strings.TrimSuffix(line, "\n")); m != nil {
			checked := m[2] != " "
			if picked != nil {
				checked = picked[m[4]]
			}
			c.Options = append(c.Options, Option{Label: m[4], Checked: checked})
		}
	}
	return c
}

// SetChoice returns d's source with choice b's options checked as
// checked says, one bool per option, and those of every copy of it.
// Options past its end are unchecked.
func (d *Document) SetChoice(b *Block, checked []bool) []byte {
	bodies := map[*Block]string{}
	for _, c := range d.Mirrors(b) {
		bodies[c] = choiceBody(c, checked)
	}
	return d.setBodies(bodies)
}

// choiceBody is choice b's body with its options checked as checked
// says, leaving its other lines alone.
func choiceBody(b *Block, checked []bool) string {
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
	return body.String()
}

// SetBody returns d's source with b's body replaced by body, which gets
// a newline at its end if it has none, unless it is empty. The fence
// gets longer if a line of body would close it, and each line starts
// with b's container prefix.
func (d *Document) SetBody(b *Block, body string) []byte {
	return d.setBodies(map[*Block]string{b: body})
}

// setBodies returns d's source with the body of each block in bodies
// replaced, as SetBody replaces one.
func (d *Document) setBodies(bodies map[*Block]string) []byte {
	src := d.Src
	var out strings.Builder
	pos := 0
	for _, b := range d.Blocks {
		body, ok := bodies[b]
		if !ok {
			continue
		}
		out.Write(src[pos:b.Span.Start])
		out.WriteString(d.fenced(b, body))
		pos = b.Span.End
	}
	out.Write(src[pos:])
	return []byte(out.String())
}

// fenced is block b, fences and all, with body as its body.
func (d *Document) fenced(b *Block, body string) string {
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
	return out.String()
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

// InfoSpan is where b's info string, the part of its opening fence line
// after the backticks, is in d's source.
func (d *Document) InfoSpan(b *Block) Span {
	line := string(d.Src[b.Span.Start:lineEnd(d.Src, b.Span.Start)])
	at := strings.Index(line, b.Fence) + len(b.Fence)
	at += strings.Index(line[at:], b.Info)
	return Span{b.Span.Start + at, b.Span.Start + at + len(b.Info)}
}
