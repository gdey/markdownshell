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
// exported variables the new env receives. Nil means all of them.
func (s *Stack) Get(ctx context.Context, name string, share []string) (*Env, error) {
	name = strings.Trim(name, "/")
	if e, ok := s.envs[name]; ok {
		return e, nil
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

// Filter keeps the variables named in share. Nil share keeps all of them;
// "*" in share also keeps all.
func Filter(vars map[string]string, share []string) map[string]string {
	if share == nil {
		return vars
	}
	out := map[string]string{}
	for _, k := range share {
		if k == "*" {
			return vars
		}
		if v, ok := vars[k]; ok {
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
