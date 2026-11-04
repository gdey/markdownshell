package env

import (
	"context"
	"fmt"
	"slices"
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
	// OnStart, if set, is called as each env's shell starts, with the
	// share it was made with (nil for the top level, whose Config.Env
	// is already filtered) and the names of the variables it was given.
	OnStart func(e *Env, share, names []string)
	// Strict makes share lists pass only what they name, without the
	// Base set. It is for documents loaded from a URL, which must not get
	// even PATH or HOME unless they ask (see FilterStrict).
	Strict bool
	// Prepare, if set, is called before the stack starts its first
	// shell, with the Config every shell starts with, to change, such as
	// to set Init once someone has said it may run. If it fails, no
	// shell starts, Get returns its error, and the next Get calls it
	// again.
	Prepare  func(cfg *Config) error
	prepared bool
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
		if s.Prepare != nil && !s.prepared {
			if err := s.Prepare(&s.cfg); err != nil {
				return nil, err
			}
			s.prepared = true
		}
		sh, err := Start(s.cfg)
		if err != nil {
			return nil, err
		}
		e := &Env{Name: TopLevel, Shell: sh}
		s.envs[name] = e
		s.started(e, nil, s.cfg.Env)
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
	filter := Filter
	if s.Strict {
		filter = FilterStrict
	}
	cfg.Env = Environ(filter(vars, share))
	sh, err := Start(cfg)
	if err != nil {
		return nil, err
	}
	e := &Env{Name: name, Parent: parent, Shell: sh}
	s.envs[name] = e
	s.started(e, share, cfg.Env)
	return e, nil
}

func (s *Stack) started(e *Env, share, environ []string) {
	if s.OnStart == nil {
		return
	}
	names := make([]string, 0, len(environ))
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		names = append(names, k)
	}
	sort.Strings(names)
	s.OnStart(e, share, names)
}

// Close ends every shell in the stack.
func (s *Stack) Close() {
	for _, e := range s.envs {
		e.Shell.Close()
	}
}

// Base lists the variables every share list passes, unless it drops
// them by name (-PATH), so a block whose env is filtered can still find
// programs, its home folder and the temp folder, and print text in the
// user's language. None of them holds a secret: they say who the user
// is, where their files are and how to talk to the terminal, all of
// which a block can learn another way (id, ~, getpwuid). Names starting
// with BasePrefix (LC_ALL, LC_CTYPE, ...) are base too.
//
// Left out on purpose: anything that grants access rather than
// describing the user, such as SSH_AUTH_SOCK, DBUS_SESSION_BUS_ADDRESS,
// XDG_RUNTIME_DIR, DISPLAY and tokens. Those stay behind share.
var Base = []string{
	"PATH", "HOME", "USER", "LOGNAME", "SHELL",
	"TERM", "COLORTERM",
	"LANG", "LANGUAGE", "TZ", "TMPDIR",
	// macOS: the text encoding Core Foundation tools read; without it
	// some print mojibake for non-ASCII text.
	"__CF_USER_TEXT_ENCODING",
}

// BasePrefix marks the locale variables that are base along with Base.
const BasePrefix = "LC_"

// IsBase reports whether name is in the base set.
func IsBase(name string) bool {
	return strings.HasPrefix(name, BasePrefix) || slices.Contains(Base, name)
}

// Filter keeps the variables named in share, and the Base set. Nil share
// keeps all of them. "..." (or "*") in share keeps all, and a name
// prefixed with "-" is dropped even if "..." or the plain name also
// appears, so [..., -USER] keeps everything but USER. A base variable
// is dropped the same way: [-PATH] keeps the base set but PATH.
func Filter(vars map[string]string, share []string) map[string]string {
	return filter(vars, share, true)
}

// FilterStrict is Filter without the Base set: share keeps exactly the
// names it lists. Documents loaded from a URL (http, https, ftp, sftp)
// use it, so a script fetched from the internet sees none of the caller's
// environment it didn't name, PATH and HOME included.
func FilterStrict(vars map[string]string, share []string) map[string]string {
	return filter(vars, share, false)
}

func filter(vars map[string]string, share []string, base bool) map[string]string {
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
		if (all || keep[k] || base && IsBase(k)) && !drop[k] {
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
