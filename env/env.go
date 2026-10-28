package env

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// TopLevel is the name of the env blocks run in when they set no env=.
const TopLevel = ""

// Env is a named shell stacked on its parent.
type Env struct {
	Name   string // full name, such as "build/test"; "" for the top level
	Parent *Env
	Shell  *Shell
}

// Stack creates envs on first use and keeps them for reuse.
type Stack struct {
	cfg  Config
	envs map[string]*Env
}

// NewStack returns a stack whose top-level env starts with cfg.
func NewStack(cfg Config) *Stack {
	return &Stack{cfg: cfg, envs: map[string]*Env{}}
}

// Get returns the env called name, creating it and any missing parents.
// share is used only when the env is created: it lists the parent's
// exported variables the new env receives. Nil means all of them. An env
// whose shell has exited is created again, with a fresh shell.
func (s *Stack) Get(ctx context.Context, name string, share []string) (*Env, error) {
	name = strings.Trim(name, "/")
	if e, ok := s.envs[name]; ok {
		if !e.Shell.Exited() {
			return e, nil
		}
		e.Shell.Close()
		delete(s.envs, name)
	}
	if name == TopLevel {
		sh, err := Start(s.cfg)
		if err != nil {
			return nil, err
		}
		e := &Env{Name: TopLevel, Shell: sh}
		s.envs[name] = e
		return e, nil
	}

	parentName := ""
	if i := strings.LastIndex(name, "/"); i >= 0 {
		parentName = name[:i]
	}
	// Missing intermediate envs inherit everything from their parent.
	parent, err := s.Get(ctx, parentName, nil)
	if err != nil {
		return nil, err
	}
	vars, err := parent.Shell.Exported(ctx)
	if err != nil {
		return nil, fmt.Errorf("env %q: %w", name, err)
	}
	cfg := s.cfg
	cfg.Env = Environ(Filter(vars, share))
	sh, err := Start(cfg)
	if err != nil {
		return nil, err
	}
	e := &Env{Name: name, Parent: parent, Shell: sh}
	s.envs[name] = e
	return e, nil
}

// Close ends every shell in the stack.
func (s *Stack) Close() {
	for _, e := range s.envs {
		e.Shell.Close()
	}
}

// Filter keeps the variables named in share. Nil share keeps all of them.
// "..." (or "*") in share keeps all, and a name prefixed with "-" is
// dropped even if "..." or the plain name also appears, so
// [..., -USER] keeps everything but USER.
func Filter(vars map[string]string, share []string) map[string]string {
	if share == nil {
		return vars
	}
	all := false
	keep := map[string]bool{}
	drop := map[string]bool{}
	for _, k := range share {
		switch {
		case k == "..." || k == "*":
			all = true
		case strings.HasPrefix(k, "-"):
			drop[k[1:]] = true
		default:
			keep[k] = true
		}
	}
	out := map[string]string{}
	for k, v := range vars {
		if (all || keep[k]) && !drop[k] {
			out[k] = v
		}
	}
	return out
}

// Environ turns vars into sorted KEY=value pairs.
func Environ(vars map[string]string) []string {
	out := make([]string, 0, len(vars))
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// Vars turns KEY=value pairs into a map.
func Vars(environ []string) map[string]string {
	out := map[string]string{}
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	return out
}
