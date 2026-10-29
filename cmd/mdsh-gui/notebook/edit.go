package notebook

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// Source is block i's text, opening fence to closing fence, as the block
// editor shows it. SetBlock takes it back.
func (n *Notebook) Source(i int) (string, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if i < 0 || i >= len(n.d.Blocks) {
		return "", fmt.Errorf("no block %d", i)
	}
	return n.d.BlockText(n.d.Blocks[i]), nil
}

// SetBlock replaces block i with text, as Source gives it. The block's
// output stays until it runs again, as in mdsh -i.
func (n *Notebook) SetBlock(i int, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if i < 0 || i >= len(n.d.Blocks) {
		return fmt.Errorf("no block %d", i)
	}
	if err := n.d.SetBlock(n.d.Blocks[i], text); err != nil {
		return err
	}
	n.dirty = true
	return n.sync()
}

// AddBlock adds a block, written as text with its fences, to the end of
// the document and returns its index.
func (n *Notebook) AddBlock(text string) (int, error) {
	nd, err := doc.Parse([]byte(text))
	if err != nil {
		return 0, err
	}
	if len(nd.Blocks) != 1 || strings.TrimSpace(string(nd.Src[nd.Blocks[0].Span.End:])) != "" || strings.TrimSpace(string(nd.Src[:nd.Blocks[0].Span.Start])) != "" {
		return 0, errors.New("write one fenced block")
	}
	nb := nd.Blocks[0]
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return 0, ErrBusy
	}
	body := nb.Body
	if strings.TrimSpace(body) == "" {
		return 0, errors.New("the block is empty")
	}
	saved := n.d.Src
	b, err := n.d.AppendBlock(nb.Lang, body)
	if err != nil {
		return 0, err
	}
	// AppendBlock writes only the language; SetBlock adds the id and the
	// other attributes as written.
	if err := n.d.SetBlock(b, text); err != nil {
		n.undoAppend(saved)
		return 0, err
	}
	n.dirty = true
	return len(n.d.Blocks) - 1, n.sync()
}

// undoAppend drops the block AppendBlock just added.
func (n *Notebook) undoAppend(src []byte) {
	n.d.Src = src
	n.d.Blocks = n.d.Blocks[:len(n.d.Blocks)-1]
}

// sync catches the session up with an edited document. A document that
// gains its first tty block needs new shells.
func (n *Notebook) sync() error {
	for i := len(n.results); i < len(n.d.Blocks); i++ {
		n.results = append(n.results, &run.Result{Block: n.d.Blocks[i]})
	}
	for i, b := range n.d.Blocks {
		n.index[b] = i
	}
	err := n.sess.Sync()
	if !errors.Is(err, run.ErrRestart) {
		return err
	}
	sess, err := n.newSession(n.opts.Run)
	if err != nil {
		return err
	}
	n.sess.Close()
	n.sess = sess
	return nil
}

// Names are what attribute completion offers: block ids, for conditions,
// and env names.
type Names struct {
	IDs  []string `json:"ids"`
	Envs []string `json:"envs"`
}

// Names lists the document's block ids and envs.
func (n *Notebook) Names() Names {
	n.mu.Lock()
	defer n.mu.Unlock()
	var names Names
	for _, b := range n.d.Blocks {
		if b.IsInclude() || b.IsTape() {
			continue
		}
		names.IDs = append(names.IDs, b.ID)
		if e := b.Attrs["env"]; e != "" && !slices.Contains(names.Envs, e) {
			names.Envs = append(names.Envs, e)
		}
	}
	slices.Sort(names.Envs)
	return names
}

// Problem is something wrong in a fence's info string, for the block
// editor to underline. From and To are byte offsets in the info string.
type Problem struct {
	From     int    `json:"from"`
	To       int    `json:"to"`
	Severity string `json:"severity"` // error or warning
	Message  string `json:"message"`
}

// CheckInfo checks the attributes in a fence's info string, such as
// sh {#build timeout=5m}, against doc.BlockAttrs.
func CheckInfo(info string) []Problem {
	in := doc.SplitInfo(info)
	include := false
	for _, t := range in.Tokens {
		if t == doc.IncludeAttr {
			include = true
		}
	}
	var problems []Problem
	at := len(in.Lang)
	for _, tok := range in.Tokens {
		from := strings.Index(info[at:], tok)
		if from < 0 {
			continue
		}
		from += at
		at = from + len(tok)
		add := func(severity, format string, args ...any) {
			problems = append(problems, Problem{From: from, To: at, Severity: severity, Message: fmt.Sprintf(format, args...)})
		}
		if strings.HasPrefix(tok, ".") {
			continue
		}
		key, value, set := strings.Cut(tok, "=")
		if strings.HasPrefix(tok, "#") {
			key, value, set = "#", tok[1:], true
		}
		spec, ok := doc.LookupAttr(key)
		switch {
		case !ok:
			add("warning", "mdsh doesn't know %s and leaves it as it is", key)
			continue
		case include && !spec.Include:
			if key == "#" {
				add("error", "an include block has no name")
			} else {
				add("error", "an include block never runs, so it can't have %s", key)
			}
			continue
		case !include && spec.Include:
			add("warning", "%s only means something with <include", key)
			continue
		}
		if err := spec.Check(unquote(value), set); err != nil {
			add("error", "%s", err)
		}
	}
	return problems
}

// unquote matches doc's reading of "quoted" values.
func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return strings.ReplaceAll(v[1:len(v)-1], `\"`, `"`)
	}
	return v
}

// EditInfo sets or removes one attribute in a fence's info string,
// keeping the others and their order. bare writes it without a value.
func EditInfo(info, key, value string, bare, remove bool) string {
	in := doc.SplitInfo(info)
	if remove {
		in.Del(key)
	} else {
		in.Set(key, value, bare)
	}
	return in.String()
}

// SetLang changes the language in a fence's info string.
func SetLang(info, lang string) string {
	in := doc.SplitInfo(info)
	in.Lang = lang
	return in.String()
}
