package run

import (
	"strings"
	"time"

	"github.com/gdey/markdownshell/audit"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
)

// blockEnv is the name of the env b runs in, "" for the top level.
func blockEnv(b *doc.Block) string { return strings.Trim(b.Attrs["env"], "/") }

func (s *Session) logStart(b *doc.Block) {
	s.opts.Audit.Event("block.start", audit.Fields{"block": b.ID, "env": blockEnv(b)})
}

// logEnd logs the end of b, which started at start, with r.
func (s *Session) logEnd(b *doc.Block, r *Result, start time.Time) {
	if s.opts.Audit == nil {
		return
	}
	f := audit.Fields{"block": b.ID, "env": blockEnv(b), "exit": r.ExitStatus(), "ms": time.Since(start).Milliseconds()}
	if r.Exited {
		f["exited"] = true
	}
	if r.Err != nil {
		f["error"] = r.Err.Error()
	}
	s.opts.Audit.Event("block.end", f)
}

// logSkip logs that b didn't run, and why, with more fields if any.
func (s *Session) logSkip(b *doc.Block, reason string, more audit.Fields) {
	if s.opts.Audit == nil {
		return
	}
	f := audit.Fields{"block": b.ID, "env": blockEnv(b), "reason": reason}
	for k, v := range more {
		f[k] = v
	}
	s.opts.Audit.Event("block.skip", f)
}

// condSaw is the fields saying what a false run= condition saw.
func (s *Session) condSaw(c doc.Cond, lookup doc.Lookup) audit.Fields {
	f := audit.Fields{"cond": c.Text}
	if c.Ref != nil {
		v, _ := lookup(*c.Ref)
		f["saw"] = v
		if c.Ref.BlockID == "" && !c.Ref.Fact() {
			f["saw"] = s.logValue(c.Ref.Var, v)
		}
	}
	return f
}

// logValue is value, or what stands for it in the log if name is a
// secret variable that ctl set made.
func (s *Session) logValue(name, value string) any {
	if s.ctl != nil {
		s.ctl.mu.Lock()
		v, ok := s.ctl.vars[name]
		s.ctl.mu.Unlock()
		if ok && v.secret {
			return s.opts.Audit.Secret(value)
		}
	}
	return value
}

// logEnv logs an env's shell starting, with the names its share let
// through, or "all" when it has none.
func (s *Session) logEnv(e *env.Env, share, names []string) {
	if e.Parent == nil {
		share = s.d.Options.Env.Share
	}
	var shared any = "all"
	if share != nil {
		shared = names
	}
	f := audit.Fields{"env": e.Name, "shared": shared}
	if e.Parent != nil {
		f["parent"] = e.Parent.Name
	}
	s.opts.Audit.Event("env.start", f)
}
