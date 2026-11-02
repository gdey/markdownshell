package notebook

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// FrontMatter is the document's front matter as the rendered view's form
// edits it: the keys mdsh and the start screen read, each in a field of
// its own, and the rest as YAML.
type FrontMatter struct {
	// Exists is whether the file has front matter at all.
	Exists bool   `json:"exists"`
	Title  string `json:"title"`
	// Description is the first of description, summary and abstract
	// the file has, the start screen's description; DescriptionKey is
	// which, or description when it has none.
	Description    string   `json:"description"`
	DescriptionKey string   `json:"descriptionKey"`
	Tags           []string `json:"tags"`
	// The rest are under mdsh:, as doc.Options reads them.
	Shell   string `json:"shell"`
	OnError string `json:"onError"` // "", "continue" or "stop"
	Step    bool   `json:"step"`
	// Interpolate and ShowStderr are "" when unset, else "on" or "off".
	Interpolate string `json:"interpolate"`
	ShowStderr  string `json:"showStderr"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
	// ShareAll is env.share left out, which shares every variable;
	// otherwise Share lists those shared, maybe none.
	ShareAll bool     `json:"shareAll"`
	Share    []string `json:"share"`
	Source   []string `json:"source"`
	// Other is the other top-level keys, as YAML, in the file's order.
	Other string `json:"other"`
}

// descKeys are the keys read for the start screen's description, as in
// library.DescriptionKeys, first match wins.
var descKeys = []string{"description", "summary", "abstract"}

// FrontMatter reads the document's front matter for the form.
func (n *Notebook) FrontMatter() (FrontMatter, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	root, err := n.frontMatterNode()
	if err != nil {
		return FrontMatter{}, err
	}
	return readFrontMatter(root, n.d.FrontMatter.End > n.d.FrontMatter.Start)
}

// frontMatterNode parses the front matter's YAML into the mapping it
// holds, an empty one when there is none.
func (n *Notebook) frontMatterNode() (*yaml.Node, error) {
	body := frontMatterBody(n.d.Src[n.d.FrontMatter.Start:n.d.FrontMatter.End])
	var d yaml.Node
	if err := yaml.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("front matter: %w", err)
	}
	if d.Kind == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	if len(d.Content) != 1 || d.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("front matter: it isn't a list of key: value lines")
	}
	return d.Content[0], nil
}

// frontMatterBody is the YAML between fm's --- lines.
func frontMatterBody(fm []byte) []byte {
	_, body, ok := bytes.Cut(fm, []byte("\n"))
	if !ok {
		return nil
	}
	body = bytes.TrimRight(body, "\r\n")
	if i := bytes.LastIndexByte(body, '\n'); i >= 0 {
		return body[:i+1]
	}
	return nil
}

func readFrontMatter(root *yaml.Node, exists bool) (FrontMatter, error) {
	f := FrontMatter{Exists: exists, DescriptionKey: descKeys[0], ShareAll: true}
	known := knownKeys(root)
	if v := scalar(root, "title"); v != nil {
		f.Title = v.Value
	}
	for _, k := range descKeys {
		if v := scalar(root, k); v != nil {
			f.Description, f.DescriptionKey = v.Value, k
			break
		}
	}
	if v := value(root, "tags"); v != nil {
		if tags, ok := list(v); ok {
			f.Tags = tags
		} else if v.Kind == yaml.ScalarNode {
			f.Tags = splitTags(v.Value)
		}
	}
	if m := value(root, "mdsh"); m != nil {
		var o struct {
			Shell       string `yaml:"shell"`
			OnError     string `yaml:"on_error"`
			Step        bool   `yaml:"step"`
			Interpolate *bool  `yaml:"interpolate"`
			ShowStderr  *bool  `yaml:"show_stderr"`
			Terminal    struct {
				Cols int `yaml:"cols"`
				Rows int `yaml:"rows"`
			} `yaml:"terminal"`
			Env struct {
				Share  []string `yaml:"share"`
				Source []string `yaml:"source"`
			} `yaml:"env"`
		}
		if err := m.Decode(&o); err != nil {
			return f, fmt.Errorf("front matter: mdsh: %w", err)
		}
		f.Shell, f.OnError, f.Step = o.Shell, o.OnError, o.Step
		f.Interpolate, f.ShowStderr = onOff(o.Interpolate), onOff(o.ShowStderr)
		f.Cols, f.Rows = o.Terminal.Cols, o.Terminal.Rows
		f.Source = o.Env.Source
		if env := value(m, "env"); env != nil && value(env, "share") != nil {
			f.ShareAll, f.Share = false, o.Env.Share
			if f.Share == nil {
				f.Share = []string{}
			}
		}
	}
	other := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if !known[root.Content[i].Value] {
			other.Content = append(other.Content, root.Content[i], root.Content[i+1])
		}
	}
	if len(other.Content) > 0 {
		s, err := encode(other)
		if err != nil {
			return f, err
		}
		f.Other = s
	}
	return f, nil
}

// knownKeys are the top-level keys of root the form has fields for:
// mdsh, and title, the description and tags when they hold what their
// field can show. Any other key is in Other.
func knownKeys(root *yaml.Node) map[string]bool {
	known := map[string]bool{"mdsh": true}
	if scalar(root, "title") != nil {
		known["title"] = true
	}
	for _, k := range descKeys {
		if scalar(root, k) != nil {
			known[k] = true
			break
		}
	}
	if v := value(root, "tags"); v != nil {
		if _, ok := list(v); ok || v.Kind == yaml.ScalarNode {
			known["tags"] = true
		}
	}
	return known
}

// SetFrontMatter writes f to the front matter, changing only the keys
// whose field differs from what the file has, so the rest keep their
// place. An empty field takes its key out, and front matter left with
// no keys is taken out too. It is a rendered-view edit, so Undo takes
// it back, and front matter mdsh refuses, such as on_error: maybe,
// comes back as its error and changes nothing.
func (n *Notebook) SetFrontMatter(f FrontMatter) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running || len(n.bg) > 0 {
		return ErrBusy
	}
	root, err := n.frontMatterNode()
	if err != nil {
		return err
	}
	was, err := readFrontMatter(root, true)
	if err != nil {
		return err
	}
	var changed []string
	note := func(what string) { changed = append(changed, what) }
	if f.Title != was.Title {
		setScalar(root, "title", f.Title)
		note("title")
	}
	if f.Description != was.Description {
		k := was.DescriptionKey
		if !slices.Contains(descKeys, k) {
			k = descKeys[0]
		}
		setScalar(root, k, f.Description)
		note("description")
	}
	if tags := cleanTags(f.Tags); !slices.Equal(tags, was.Tags) {
		setList(root, "tags", tags, false)
		note("tags")
	}
	mdsh := func() *yaml.Node { return child(root, "mdsh") }
	if f.Shell = strings.TrimSpace(f.Shell); f.Shell != was.Shell {
		setScalar(mdsh(), "shell", f.Shell)
		note("shell")
	}
	if f.OnError != was.OnError {
		setScalar(mdsh(), "on_error", f.OnError)
		note("on error")
	}
	if f.Step != was.Step {
		setScalar(mdsh(), "step", map[bool]string{true: "true"}[f.Step])
		note("step")
	}
	if f.Interpolate != was.Interpolate {
		setScalar(mdsh(), "interpolate", boolText(f.Interpolate))
		note("placeholders")
	}
	if f.ShowStderr != was.ShowStderr {
		setScalar(mdsh(), "show_stderr", boolText(f.ShowStderr))
		note("show stderr")
	}
	if f.Cols != was.Cols || f.Rows != was.Rows {
		t := child(mdsh(), "terminal")
		setScalar(t, "cols", count(f.Cols))
		setScalar(t, "rows", count(f.Rows))
		note("terminal size")
	}
	share := cleanList(f.Share)
	if f.ShareAll != was.ShareAll || !f.ShareAll && !slices.Equal(share, was.Share) {
		env := child(mdsh(), "env")
		if f.ShareAll {
			del(env, "share")
		} else {
			setList(env, "share", share, true)
		}
		note("shared variables")
	}
	if src := cleanList(f.Source); !slices.Equal(src, was.Source) {
		setList(child(mdsh(), "env"), "source", src, false)
		note("sourced files")
	}
	if strings.TrimSpace(f.Other) != strings.TrimSpace(was.Other) {
		if err := setOther(root, f.Other); err != nil {
			return err
		}
		note("other keys")
	}
	if len(changed) == 0 {
		return nil
	}
	prune(root, "mdsh", "terminal", "env")
	src, err := n.withFrontMatter(root)
	if err != nil {
		return err
	}
	what := "edit front matter"
	if len(changed) == 1 {
		what = "change " + changed[0]
	}
	return n.change(what, "", src, nil)
}

// withFrontMatter is the file with root as its front matter, or none if
// root has no keys. Front matter the file didn't have goes at the top,
// after any #! line.
func (n *Notebook) withFrontMatter(root *yaml.Node) ([]byte, error) {
	var fm []byte
	if len(root.Content) > 0 {
		s, err := encode(root)
		if err != nil {
			return nil, err
		}
		fm = []byte("---\n" + s + "---\n")
	}
	at := n.d.FrontMatter
	src := n.d.Src
	return slices.Concat(src[:at.Start], fm, src[at.End:]), nil
}

// setOther replaces the keys the form has no field for with those in
// text, which must be key: value lines. A key kept keeps its place, one
// taken out goes, and a new one is added at the end.
func setOther(root *yaml.Node, text string) error {
	var d yaml.Node
	if err := yaml.Unmarshal([]byte(text), &d); err != nil {
		return fmt.Errorf("other keys: %w", err)
	}
	other := &yaml.Node{Kind: yaml.MappingNode}
	if d.Kind != 0 {
		if len(d.Content) != 1 || d.Content[0].Kind != yaml.MappingNode {
			return errors.New("other keys: write them as key: value lines")
		}
		other = d.Content[0]
	}
	known := knownKeys(root)
	for i := 0; i+1 < len(other.Content); i += 2 {
		if k := other.Content[i].Value; known[k] {
			return fmt.Errorf("other keys: %s has a field of its own", k)
		}
	}
	keep := root.Content[:0:0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i]
		if known[k.Value] {
			keep = append(keep, k, root.Content[i+1])
			continue
		}
		if j := keyAt(other, k.Value); j >= 0 {
			keep = append(keep, other.Content[j], other.Content[j+1])
		}
	}
	for i := 0; i+1 < len(other.Content); i += 2 {
		if keyAt(root, other.Content[i].Value) < 0 {
			keep = append(keep, other.Content[i], other.Content[i+1])
		}
	}
	root.Content = keep
	return nil
}

// encode writes m as YAML, two spaces to a level.
func encode(m *yaml.Node) (string, error) {
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(m); err != nil {
		return "", err
	}
	if err := e.Close(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// keyAt is the index of key k in mapping m, or -1.
func keyAt(m *yaml.Node, k string) int {
	if m == nil || m.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == k {
			return i
		}
	}
	return -1
}

// value is the value of key k in mapping m, or nil.
func value(m *yaml.Node, k string) *yaml.Node {
	if i := keyAt(m, k); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

// scalar is the value of key k in m if it is a scalar, else nil.
func scalar(m *yaml.Node, k string) *yaml.Node {
	if v := value(m, k); v != nil && v.Kind == yaml.ScalarNode && v.Tag != "!!null" {
		return v
	}
	return nil
}

// list is v's items, if it is a list of scalars.
func list(v *yaml.Node) ([]string, bool) {
	if v.Kind != yaml.SequenceNode {
		return nil, false
	}
	out := []string{}
	for _, x := range v.Content {
		if x.Kind != yaml.ScalarNode {
			return nil, false
		}
		out = append(out, x.Value)
	}
	return out, true
}

// child is the mapping under key k of m, made at the end if it isn't
// there. A value there that isn't a mapping is replaced.
func child(m *yaml.Node, k string) *yaml.Node {
	if v := value(m, k); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	c := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	set(m, k, c)
	return c
}

// set puts v under key k of m, in its place if k is there, else at the
// end.
func set(m *yaml.Node, k string, v *yaml.Node) {
	if i := keyAt(m, k); i >= 0 {
		// Keep a comment that was on the line.
		v.LineComment = m.Content[i+1].LineComment
		m.Content[i+1] = v
		return
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, v)
}

// del takes key k out of m.
func del(m *yaml.Node, k string) {
	if i := keyAt(m, k); i >= 0 {
		m.Content = slices.Delete(m.Content, i, i+2)
	}
}

// setScalar sets key k of m to s, or takes it out when s is empty.
// Numbers and true and false are written as they are; other text is
// quoted when YAML would read it as something else.
func setScalar(m *yaml.Node, k, s string) {
	if s == "" {
		del(m, k)
		return
	}
	v := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
	if _, err := strconv.Atoi(s); err == nil {
		v.Tag = "!!int"
	} else if s == "true" || s == "false" {
		v.Tag = "!!bool"
	}
	if strings.Contains(s, "\n") {
		v.Style = yaml.LiteralStyle
	}
	set(m, k, v)
}

// setList sets key k of m to the list items, one line, or takes it out
// when it is empty, unless keepEmpty, as for env.share, where [] shares
// nothing.
func setList(m *yaml.Node, k string, items []string, keepEmpty bool) {
	if len(items) == 0 && !keepEmpty {
		del(m, k)
		return
	}
	v := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle, Content: []*yaml.Node{}}
	for _, s := range items {
		v.Content = append(v.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s})
	}
	set(m, k, v)
}

// prune takes out the mappings under path that are left empty, deepest
// first: mdsh's terminal and env, then mdsh.
func prune(root *yaml.Node, top string, under ...string) {
	m := value(root, top)
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for _, k := range under {
		if v := value(m, k); v != nil && v.Kind == yaml.MappingNode && len(v.Content) == 0 {
			del(m, k)
		}
	}
	if len(m.Content) == 0 {
		del(root, top)
	}
}

func onOff(b *bool) string {
	switch {
	case b == nil:
		return ""
	case *b:
		return "on"
	}
	return "off"
}

func boolText(s string) string {
	switch s {
	case "on":
		return "true"
	case "off":
		return "false"
	}
	return ""
}

func count(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// splitTags splits one string of tags at commas and spaces.
func splitTags(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
}

// cleanTags are tags as typed, each split at commas and spaces, with a
// leading # dropped and repeats left out.
func cleanTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		for _, t := range splitTags(t) {
			if t = strings.TrimPrefix(t, "#"); t != "" && !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	if out == nil {
		out = []string{}
	}
	return out
}

// cleanList is items trimmed, without empty ones.
func cleanList(items []string) []string {
	out := []string{}
	for _, s := range items {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
