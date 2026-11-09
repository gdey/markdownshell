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
	"time"
)

// MaxRecent is how many files Open Recent remembers.
const MaxRecent = 10

// MaxPlaces is how many files' window places are remembered.
const MaxPlaces = 100

// MaxFileViews is how many files' views are remembered.
const MaxFileViews = 100

// FileView is how a file was last shown: View is "rendered", "source"
// or "both", and History is whether the History pane was open.
type FileView struct {
	File    string `json:"file"`
	View    string `json:"view"`
	History bool   `json:"history,omitempty"`
}

// Place is where a file's window was and how big, in the screen's
// points. X and Y are its top left corner.
type Place struct {
	File   string `json:"file"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Settings are the app-wide settings. The zero value is the default.
type Settings struct {
	// Autosave saves the file after each run.
	Autosave bool `json:"autosave"`
	// Vim turns on Vim key bindings in the code editors.
	Vim bool `json:"vim"`
	// NoSpellCheck turns off spell checking of the prose in the code
	// editor, which is on by default.
	NoSpellCheck bool `json:"noSpellCheck"`
	// Neovim makes the source pane an embedded Neovim, when Vim is on
	// and a new enough nvim is found. Experimental.
	Neovim bool `json:"neovim"`
	// NeovimPath is the nvim to run; empty finds one.
	NeovimPath string `json:"neovimPath"`
	// NeovimClean starts Neovim without the user's config.
	NeovimClean bool `json:"neovimClean"`
	// NeovimOwnColors keeps the colorscheme from the user's Neovim
	// config, rather than the app's theme, which is the default.
	NeovimOwnColors bool `json:"neovimOwnColors"`
	// Theme is "light" or "dark"; empty follows the system.
	Theme string `json:"theme"`
	// Look is a look's id from its look.css in frontend/looks, such as
	// "tty" for Teletype; empty is the plain look.
	Look string `json:"look"`
	// TextSize is the text's size in percent, 80 to 150; 0 is the
	// look's own (100 for plain, 115 for Teletype).
	TextSize int `json:"textSize"`
	// UIFont and CodeFont are font families for the interface and for
	// code; empty is the look's own.
	UIFont   string `json:"uiFont"`
	CodeFont string `json:"codeFont"`
	// LineSpacing is "compact" or "relaxed"; empty is normal.
	LineSpacing string `json:"lineSpacing"`
	// NoSprockets, NoRules and NoShadows take the Teletype paper's
	// sprocket strips, its line rules and the shadows under cards off.
	NoSprockets bool `json:"noSprockets"`
	NoRules     bool `json:"noRules"`
	NoShadows   bool `json:"noShadows"`
	// Accent is a #rrggbb color for links, run buttons and selection;
	// empty is the look's own.
	Accent string `json:"accent"`
	// HistoryPane is the History pane's colors: "match" the window,
	// "invert" them (light under a dark window, dark under a light one)
	// or "dark" always. Empty is the look's own: dark for plain, match
	// for Teletype.
	HistoryPane string `json:"historyPane"`
	// HistoryFocus is how the History pane's prompt shows it has the
	// keyboard: "cursor" for a blinking block cursor where you type,
	// in place of the outline around the field. Empty is the outline.
	HistoryFocus string `json:"historyFocus"`
	// SecretShowSeconds is how long a block's secret stays shown after
	// its eye button shows it, and on the clipboard after it is copied.
	// 0 is SecretShowDefault.
	SecretShowSeconds int `json:"secretShowSeconds"`
	// Shell, if set, overrides the front matter's shell for every file.
	Shell string `json:"shell"`
	// HistoryRC is a file, such as ~/.zshrc, the shells read while the
	// History pane is on. Empty reads none.
	HistoryRC string `json:"historyRC"`
	// View is how a file opens: "rendered", "source" or "both".
	// Empty opens a file as it was last shown (see FileViews), and a
	// file not shown before in the rendered view.
	View string `json:"view"`
	// StartFolder is the folder the start screen lists when the app
	// isn't started from a terminal. Empty means the last folder used.
	StartFolder string `json:"startFolder"`
	// TemplateFolder holds the markdown files New File… offers to start
	// from. Empty offers only a blank file.
	TemplateFolder string `json:"templateFolder"`
	// SessionFolder is where New Session makes a file for the day and
	// Last Session finds the latest. Empty is ~/.mdsh/sessions.
	SessionFolder string `json:"sessionFolder"`
	// SessionTemplate is a markdown file a new session starts as a copy
	// of. Empty starts it with a heading for the date.
	SessionTemplate string `json:"sessionTemplate"`
	// LastFolder is the folder of the file opened last.
	LastFolder string `json:"lastFolder"`
	// Recent are the files opened last, newest first.
	Recent []string `json:"recent"`
	// Pinned are the files pinned to the top of the start screen, in
	// the order they were pinned.
	Pinned []string `json:"pinned,omitempty"`
	// Places are where the windows of the files last shown alone in a
	// new window were, newest first.
	Places []Place `json:"places,omitempty"`
	// FileViews are how the files shown last were left, newest first:
	// the view and whether the History pane was open.
	FileViews []FileView `json:"fileViews,omitempty"`
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

// Path is the file the settings live in.
func (st *Store) Path() string { return st.path }

// Get returns a copy of the settings.
func (st *Store) Get() Settings {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.s.clone()
}

// clone copies s, so that changing the copy leaves s as it was.
func (s Settings) clone() Settings {
	s.Recent = slices.Clone(s.Recent)
	s.Pinned = slices.Clone(s.Pinned)
	s.Places = slices.Clone(s.Places)
	s.FileViews = slices.Clone(s.FileViews)
	return s
}

// Update changes the settings with f and writes them out.
func (st *Store) Update(f func(*Settings)) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	s := st.s.clone()
	f(&s)
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := write(st.path, append(out, '\n')); err != nil {
		return err
	}
	st.s = s
	return nil
}

// write puts data in path, readable by the user only: the settings
// name recent files, the shell and the Neovim to run. It writes a
// temporary file and renames it over path, so a file left 0644 by an
// older version comes out 0600 and a crash never leaves half a file.
// A path that is a symlink (dotfiles kept elsewhere) is written through,
// so the link stays.
func write(path string, data []byte) error {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name()) // fails once renamed
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
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

// Pin pins file to the start screen, after the files pinned before it,
// or unpins it. It reports whether that changed anything.
func (s *Settings) Pin(file string, pin bool) bool {
	if slices.Contains(s.Pinned, file) == pin {
		return false
	}
	if pin {
		s.Pinned = append(s.Pinned, file)
	} else {
		s.Pinned = slices.DeleteFunc(s.Pinned, func(f string) bool { return f == file })
	}
	return true
}

// Place is where file's window was last, if that is remembered.
func (s Settings) Place(file string) (Place, bool) {
	i := slices.IndexFunc(s.Places, func(p Place) bool { return p.File == file })
	if i < 0 {
		return Place{}, false
	}
	return s.Places[i], true
}

// Placed remembers where p.File's window is: p goes to the top of
// Places, and the oldest are forgotten past MaxPlaces. It reports
// whether that changed anything.
func (s *Settings) Placed(p Place) bool {
	if len(s.Places) > 0 && s.Places[0] == p {
		return false
	}
	s.Places = slices.DeleteFunc(s.Places, func(q Place) bool { return q.File == p.File })
	s.Places = slices.Insert(s.Places, 0, p)
	if len(s.Places) > MaxPlaces {
		s.Places = s.Places[:MaxPlaces]
	}
	return true
}

// FileView is how file was last shown, if that is remembered.
func (s Settings) FileView(file string) (FileView, bool) {
	i := slices.IndexFunc(s.FileViews, func(v FileView) bool { return v.File == file })
	if i < 0 {
		return FileView{}, false
	}
	return s.FileViews[i], true
}

// Viewed remembers how v.File is shown: v goes to the top of FileViews,
// and the oldest are forgotten past MaxFileViews. It reports whether
// that changed anything.
func (s *Settings) Viewed(v FileView) bool {
	if len(s.FileViews) > 0 && s.FileViews[0] == v {
		return false
	}
	s.FileViews = slices.DeleteFunc(s.FileViews, func(w FileView) bool { return w.File == v.File })
	s.FileViews = slices.Insert(s.FileViews, 0, v)
	if len(s.FileViews) > MaxFileViews {
		s.FileViews = s.FileViews[:MaxFileViews]
	}
	return true
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

// SecretShowDefault is how long a secret stays shown, or on the
// clipboard, unless the settings say otherwise.
const SecretShowDefault = 60 * time.Second

// SecretShow is how long a secret stays shown, or on the clipboard.
func (s Settings) SecretShow() time.Duration {
	if s.SecretShowSeconds <= 0 {
		return SecretShowDefault
	}
	return time.Duration(s.SecretShowSeconds) * time.Second
}
