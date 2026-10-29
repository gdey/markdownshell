package notebook

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// sync catches the session up with an edited document. Blocks that
// kept their identity keep their results. A document that gains its
// first tty block needs new shells.
func (n *Notebook) sync() error {
	old := make(map[*doc.Block]*run.Result, len(n.results))
	for _, r := range n.results {
		old[r.Block] = r
	}
	n.results = make([]*run.Result, len(n.d.Blocks))
	clear(n.index)
	for i, b := range n.d.Blocks {
		n.index[b] = i
		if n.results[i] = old[b]; n.results[i] == nil {
			n.results[i] = &run.Result{Block: b}
		}
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
