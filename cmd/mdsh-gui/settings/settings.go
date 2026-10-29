// Package settings keeps mdsh-gui's app-wide settings in a JSON file in
// the user's config folder.
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// MaxRecent is how many files Open Recent remembers.
const MaxRecent = 10

// Settings are the app-wide settings. The zero value is the default.
type Settings struct {
	// Autosave saves the file after each run.
	Autosave bool `json:"autosave"`
	// Shell, if set, overrides the front matter's shell for every file.
	Shell string `json:"shell"`
	// StartFolder is the folder the start screen lists when the app
	// isn't started from a terminal. Empty means the last folder used.
	StartFolder string `json:"startFolder"`
	// LastFolder is the folder of the file opened last.
	LastFolder string `json:"lastFolder"`
	// Recent are the files opened last, newest first.
	Recent []string `json:"recent"`
}

// Store reads and writes Settings.
type Store struct {
	path string

	mu sync.Mutex
	s  Settings
}

// DefaultPath is where the settings live: mdsh/gui.json in the user's
// config folder.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mdsh", "gui.json"), nil
}

// Open reads the settings in path. A missing file gives the defaults.
func Open(path string) (*Store, error) {
	st := &Store{path: path}
	src, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(src, &st.s); err != nil {
		return nil, err
	}
	return st, nil
}

// Get returns a copy of the settings.
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.s
	s.Recent = slices.Clone(s.Recent)
	return s
}

// Update changes the settings with f and writes them out.
func (st *Store) Update(f func(*Settings)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.s
	s.Recent = slices.Clone(s.Recent)
	f(&s)
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(st.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(st.path, append(out, '\n'), 0o644); err != nil {
		return err
	}
	st.s = s
	return nil
}

// Opened notes that file was opened: it goes to the top of Recent and
// its folder becomes LastFolder.
func (s *Settings) Opened(file string) {
	s.Recent = slices.DeleteFunc(s.Recent, func(f string) bool { return f == file })
	s.Recent = slices.Insert(s.Recent, 0, file)
	if len(s.Recent) > MaxRecent {
		s.Recent = s.Recent[:MaxRecent]
	}
	s.LastFolder = filepath.Dir(file)
}

// StartDir is the folder the start screen lists. Started from a
// terminal, that is the current folder cwd. Started from Finder or the
// Dock the current folder is /, so it is StartFolder, else LastFolder,
// else documents (~/Documents on macOS), else home. Folders that no
// longer exist are passed over.
func (s Settings) StartDir(cwd, home, documents string) string {
	if cwd != "" && cwd != "/" {
		return cwd
	}
	for _, dir := range []string{s.StartFolder, s.LastFolder, documents} {
		if isDir(dir) {
			return dir
		}
	}
	return home
}

func isDir(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
