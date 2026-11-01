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

// MaxPlaces is how many files' window places are remembered.
const MaxPlaces = 100

// Place is where a file's window was and how big, in the screen's
// points. X and Y are its top left corner.
type Place struct {
	File   string `json:"file"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Size is a window's width and height.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
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
	// Theme is "light" or "dark"; empty follows the system.
	Theme string `json:"theme"`
	// Look is "tty" for the Teletype look, paper in the light theme and
	// phosphor in the dark; empty is the plain look.
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
	// Shell, if set, overrides the front matter's shell for every file.
	Shell string `json:"shell"`
	// HistoryRC is a file, such as ~/.zshrc, the shells read while the
	// History pane is on. Empty reads none.
	HistoryRC string `json:"historyRC"`
	// View is how a file opens: "rendered", "source" or "both".
	// Empty keeps the view the last tab had.
	View string `json:"view"`
	// StartFolder is the folder the start screen lists when the app
	// isn't started from a terminal. Empty means the last folder used.
	StartFolder string `json:"startFolder"`
	// TemplateFolder holds the markdown files New File… offers to start
	// from. Empty offers only a blank file.
	TemplateFolder string `json:"templateFolder"`
	// LastFolder is the folder of the file opened last.
	LastFolder string `json:"lastFolder"`
	// Recent are the files opened last, newest first.
	Recent []string `json:"recent"`
	// Places are where the windows of the files last shown alone in a
	// new window were, newest first.
	Places []Place `json:"places,omitempty"`
	// SettingsWindow is the size the Settings window was left at, or
	// nil for its default.
	SettingsWindow *Size `json:"settingsWindow,omitempty"`
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
	s.Places = slices.Clone(s.Places)
	if s.SettingsWindow != nil {
		sz := *s.SettingsWindow
		s.SettingsWindow = &sz
	}
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
