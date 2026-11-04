package run

import (
	"encoding/json"

	"github.com/gdey/markdownshell/audit"
	"sort"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/ctl"
	"github.com/gdey/markdownshell/doc"
)

// Progress is what a block reported with mdsh ctl status. Step and Of
// are 0 when it gave no count.
type Progress struct {
	Text     string
	Step, Of int
}

// ctlVar is a variable mdsh ctl set made.
type ctlVar struct {
	value  string
	secret bool
}

// ctlState is what calls to the control socket change. Calls come in on
// the server's goroutine while blocks run, so it has a lock of its own.
type ctlState struct {
	srv *ctl.Server

	mu    sync.Mutex
	vars  map[string]ctlVar // by full name, MDSH_VAR_ and the name
	unset map[string]bool   // names unset after a set, to unset in shells
}

// startCtl starts the control socket. If the system can't make one,
// blocks run without $MDSH_SOCK and mdsh ctl says it isn't running under
// mdsh.
func (s *Session) startCtl() {
	s.ctl = &ctlState{vars: map[string]ctlVar{}, unset: map[string]bool{}}
	srv, err := ctl.Listen(s.handleLogged)
	if err != nil {
		return
	}
	s.ctl.srv = srv
}

// closeCtl stops the control socket and removes it.
func (s *Session) closeCtl() {
	if s.ctl != nil && s.ctl.srv != nil {
		s.ctl.srv.Close()
		s.ctl.srv = nil
	}
}

// issue gives b a token for this run of it, and returns the prelude that
// exports the socket, the token and the variables ctl set has made, and
// a function that stops the token working.
func (s *Session) issue(b *doc.Block) (pre string, revoke func()) {
	pre = prelude(s.d, b, s.opts)
	if s.ctl == nil || s.ctl.srv == nil {
		return pre, func() {}
	}
	token := s.ctl.srv.Issue(b)
	return s.ctlPrelude(token) + pre, func() { s.ctl.srv.Revoke(token) }
}

// ctlPrelude exports $MDSH_SOCK, $MDSH_TOKEN and the variables ctl set
// has made, and unsets those ctl unset removed. It is one line.
func (s *Session) ctlPrelude(token string) string {
	c := s.ctl
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	for _, name := range sortedKeys(c.unset) {
		b.WriteString("unset " + name + "; ")
	}
	names := []string{ctl.SockVar, ctl.TokenVar}
	b.WriteString(ctl.SockVar + "=" + quote(c.srv.Path()) + " " + ctl.TokenVar + "=" + quote(token))
	for _, name := range sortedKeys(c.vars) {
		names = append(names, name)
		b.WriteString(" " + name + "=" + oneLine(c.vars[name].value))
	}
	b.WriteString("; export " + strings.Join(names, " ") + "; ")
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ctlLookup is lookup with the variables ctl set has made, as they are
// when a ref is read.
func (s *Session) ctlLookup(lookup doc.Lookup) doc.Lookup {
	return func(ref doc.Ref) (string, bool) {
		if ref.BlockID == "" && !ref.Fact() && s.ctl != nil && !s.inputBinds(ref.Var) {
			s.ctl.mu.Lock()
			v, ok := s.ctl.vars[ref.Var]
			gone := s.ctl.unset[ref.Var]
			s.ctl.mu.Unlock()
			if ok {
				return v.value, true
			}
			if gone {
				return "", true
			}
		}
		return lookup(ref)
	}
}

// inputBinds reports whether a textbox or choice binds name with var=.
func (s *Session) inputBinds(name string) bool { return s.inputFor(name) != nil }

// inputFor is the textbox or choice that binds name with var=, or nil.
func (s *Session) inputFor(name string) *doc.Block { return s.d.InputFor(name) }

// handleCtl answers a call from a block.
func (s *Session) handleCtl(caller any, req ctl.Request) ctl.Response {
	b, _ := caller.(*doc.Block)
	switch req.Cmd {
	case "set":
		var a ctl.SetArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return ctl.Refuse("set: %v", err)
		}
		name, resp, ok := s.ctlName("set", a.Name)
		if !ok {
			return resp
		}
		if in := s.inputFor(name); in != nil {
			if !in.NoSave() {
				return ctl.Refuse("set: $%s belongs to an input in the document; change the input instead", name)
			}
			// A nosave input's value is the session's, so a block may
			// fill it; the file keeps the input's body.
			old := s.varNow(name)
			s.d.SetValue(in, a.Value)
			s.logVar(b, name, old, &ctlVar{value: a.Value, secret: a.Secret})
			return ctl.Done("")
		}
		old := s.varNow(name)
		s.ctl.mu.Lock()
		s.ctl.vars[name] = ctlVar{value: a.Value, secret: a.Secret}
		delete(s.ctl.unset, name)
		s.ctl.mu.Unlock()
		s.logVar(b, name, old, &ctlVar{value: a.Value, secret: a.Secret})
		return ctl.Done("")
	case "unset":
		var a ctl.NameArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return ctl.Refuse("unset: %v", err)
		}
		name, resp, ok := s.ctlName("unset", a.Name)
		if !ok {
			return resp
		}
		old := s.varNow(name)
		s.ctl.mu.Lock()
		_, ok = s.ctl.vars[name]
		if ok {
			delete(s.ctl.vars, name)
			s.ctl.unset[name] = true
		}
		s.ctl.mu.Unlock()
		if ok {
			s.logVar(b, name, old, nil)
		}
		return ctl.Done("")
	case "get":
		var a ctl.NameArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return ctl.Refuse("get: %v", err)
		}
		name, resp, ok := s.ctlName("get", a.Name)
		if !ok {
			return resp
		}
		v, _ := s.ctlLookup(s.baseLookup)(doc.Ref{Var: name})
		return ctl.Done(v)
	case "status":
		var a ctl.StatusArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			return ctl.Refuse("status: %v", err)
		}
		if a.Step < 0 || a.Of < 0 || a.Of > 0 && a.Step > a.Of {
			return ctl.Refuse("status: --step %d --of %d is not a count", a.Step, a.Of)
		}
		s.emit(Event{Kind: Status, Block: b, Progress: &Progress{Text: a.Text, Step: a.Step, Of: a.Of}})
		return ctl.Done("")
	}
	return ctl.Refuse("unknown command %q", req.Cmd)
}

// ctlName checks a variable's name from a call and returns it with
// MDSH_VAR_ in front.
func (s *Session) ctlName(cmd, name string) (string, ctl.Response, bool) {
	if !doc.ValidVar(name) {
		return "", ctl.Refuse("%s: %q is not a variable name: use letters, digits and _", cmd, name), false
	}
	return doc.VarPrefix + name, ctl.Response{}, true
}

// handleLogged answers a call and logs it.
func (s *Session) handleLogged(caller any, req ctl.Request) ctl.Response {
	resp := s.handleCtl(caller, req)
	if s.opts.Audit == nil {
		return resp
	}
	f := audit.Fields{"cmd": req.Cmd, "ok": resp.OK, "code": resp.Code}
	if b, ok := caller.(*doc.Block); ok {
		f["block"], f["env"] = b.ID, blockEnv(b)
	}
	var args map[string]any
	if json.Unmarshal(req.Args, &args) == nil {
		if secret, _ := args["secret"].(bool); secret {
			v, _ := args["value"].(string)
			args["value"] = s.opts.Audit.Secret(v)
		}
		f["args"] = args
	}
	if resp.Error != "" {
		f["error"] = resp.Error
	}
	s.opts.Audit.Event("ctl", f)
	return resp
}

// ctlOld is a variable as it was before a change, for the log.
type ctlOld struct {
	value  string
	secret bool
	set    bool // whether it had a value: from ctl, an input or the caller
}

// varNow is name as it is now, before a change.
func (s *Session) varNow(name string) ctlOld {
	s.ctl.mu.Lock()
	v, ok := s.ctl.vars[name]
	s.ctl.mu.Unlock()
	if ok {
		return ctlOld{v.value, v.secret, true}
	}
	value, _ := s.ctlLookup(s.baseLookup)(doc.Ref{Var: name})
	return ctlOld{value: value, set: value != ""}
}

// logVar logs that name changed from old to now, nil when unset, by a
// call from b.
func (s *Session) logVar(b *doc.Block, name string, old ctlOld, now *ctlVar) {
	l := s.opts.Audit
	if l == nil {
		return
	}
	show := func(value string, secret bool) any {
		if secret {
			return l.Secret(value)
		}
		return value
	}
	f := audit.Fields{"name": name, "by": "ctl", "old": nil, "new": nil}
	if b != nil {
		f["block"], f["env"] = b.ID, blockEnv(b)
	}
	if old.set {
		f["old"] = show(old.value, old.secret)
	}
	if now != nil {
		f["new"] = show(now.value, now.secret)
	}
	l.Event("var", f)
}
