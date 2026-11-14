// Package trust remembers, for each document, whether its user let mdsh
// read the files the front matter's env.source names into its shells,
// so that mdsh, mdsh -i and mdsh-gui ask once per file, and again only
// when the list of files changes. The answers live in one JSON file,
// shared by every front end. A second store, at OutsidePath, keeps in
// the same way whether a document's includes and out= files may reach
// outside its folder, or hidden in it (see run.Outside).
package trust

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/gdey/markdownshell/run"
)

// Max is how many documents' answers are kept; the oldest go first.
const Max = 500

// Choice is the answer for one document.
type Choice struct {
	// File is the document's absolute path.
	File string `json:"file"`
	// Sources are the files approved or refused, as run.SourcePaths
	// gives them. A document that names other files is asked again.
	Sources []string `json:"sources"`
	// Allow is whether they may be read.
	Allow bool `json:"allow"`
	// At is when the answer was given.
	At time.Time `json:"at"`
}

type state struct {
	Files []Choice `json:"files"`
}

// Store keeps the answers in a JSON file. It reads the file on each
// lookup, so an answer given in one front end holds in the others.
type Store struct {
	path string
	mu   sync.Mutex
}

// DefaultPath is mdsh/sources.json in the user's config folder.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mdsh", "sources.json"), nil
}

// OutsidePath is mdsh/outside.json in the user's config folder, where
// the answers to whether a document may reach outside its root live.
func OutsidePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mdsh", "outside.json"), nil
}

// OpenOutside opens the store at OutsidePath, or returns nil, which
// remembers nothing, if there is no config folder.
func OpenOutside() *Store {
	p, err := OutsidePath()
	if err != nil {
		return nil
	}
	return Open(p)
}

// Open returns the store kept in path. The file is made on the first
// answer.
func Open(path string) *Store { return &Store{path: path} }

// OpenDefault opens the store at DefaultPath, or returns nil, which
// remembers nothing, if there is no config folder.
func OpenDefault() *Store {
	p, err := DefaultPath()
	if err != nil {
		return nil
	}
	return Open(p)
}

func (st *Store) read() (state, error) {
	var s state
	src, err := os.ReadFile(st.path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(src, &s)
}

// Lookup returns the answer given for file when it named sources, if
// there is one.
func (st *Store) Lookup(file string, sources []string) (allow, ok bool) {
	if st == nil || file == "" {
		return false, false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	s, err := st.read()
	if err != nil {
		return false, false
	}
	for _, c := range s.Files {
		if c.File == file {
			return c.Allow, slices.Equal(c.Sources, sources)
		}
	}
	return false, false
}

// Allowed lists the documents whose last answer allowed them, whatever
// their lists were then.
func (st *Store) Allowed() []string {
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	s, err := st.read()
	if err != nil {
		return nil
	}
	var files []string
	for _, c := range s.Files {
		if c.Allow {
			files = append(files, c.File)
		}
	}
	return files
}

// Forget drops the answer for file, so it is asked again.
func (st *Store) Forget(file string) error {
	return st.update(func(s *state) {
		s.Files = slices.DeleteFunc(s.Files, func(c Choice) bool { return c.File == file })
	})
}

// Remember keeps allow as the answer for file naming sources, in place
// of any earlier answer for file.
func (st *Store) Remember(file string, sources []string, allow bool) error {
	if file == "" {
		return nil
	}
	return st.update(func(s *state) {
		s.Files = slices.DeleteFunc(s.Files, func(c Choice) bool { return c.File == file })
		s.Files = slices.Insert(s.Files, 0, Choice{File: file, Sources: slices.Clone(sources), Allow: allow, At: time.Now().UTC()})
		if len(s.Files) > Max {
			s.Files = s.Files[:Max]
		}
	})
}

// update reads the store, changes it with f and writes it back.
func (st *Store) update(f func(*state)) error {
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	s, err := st.read()
	if err != nil {
		// A file that doesn't parse is started over rather than
		// leaving every document unable to remember.
		s = state{}
	}
	f(&s)
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	tmp := st.path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, st.path)
}

// Ask returns a run.Options.ApproveSources for the document at file
// that answers from the store when it can and otherwise asks ask, which
// may be nil when nobody can be asked. A Go or Skip answer from ask is
// remembered; Stop isn't, so the next run asks again.
func (st *Store) Ask(file string, ask func(paths []string) run.Answer) func(paths []string) run.Answer {
	return func(paths []string) run.Answer {
		if allow, ok := st.Lookup(file, paths); ok {
			if allow {
				return run.Go
			}
			return run.Skip
		}
		if ask == nil {
			return run.Stop
		}
		a := ask(paths)
		if a == run.Go || a == run.Skip {
			// Not being able to remember isn't a reason to refuse
			// what was just allowed.
			_ = st.Remember(file, paths, a == run.Go)
		}
		return a
	}
}
