// Package nvim runs an embedded Neovim for the split view's source pane:
// nvim --embed with the ext_linegrid UI, editing a buffer kept in step
// with the notebook's text. The page draws the grid; this package passes
// its redraw events on and the page's keys back.
package nvim

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neovim/go-client/nvim"
)

// MinVersion is the oldest Neovim that works: 0.9 has no inline virtual
// text.
var MinVersion = Version{0, 10, 0}

// Version is a Neovim version.
type Version struct {
	Major, Minor, Patch int
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Less reports whether v is older than w.
func (v Version) Less(w Version) bool {
	if v.Major != w.Major {
		return v.Major < w.Major
	}
	if v.Minor != w.Minor {
		return v.Minor < w.Minor
	}
	return v.Patch < w.Patch
}

// candidates are where nvim usually is when it isn't on the PATH, as for
// an app opened from the macOS Finder, which doesn't get the shell's.
var candidates = []string{
	"/opt/homebrew/bin/nvim",
	"/usr/local/bin/nvim",
	"/opt/local/bin/nvim",
	"/usr/bin/nvim",
	"/snap/bin/nvim",
	"~/.local/bin/nvim",
	"~/.nix-profile/bin/nvim",
	`C:\Program Files\Neovim\bin\nvim.exe`,
}

// Found is a Neovim found by Find.
type Found struct {
	Path    string  `json:"path"`
	Version Version `json:"version"`
}

// ErrNotFound is Find's error when there is no nvim to use.
var ErrNotFound = errors.New("nvim not found")

// Find finds the nvim to use: path if it isn't empty, else nvim on the
// PATH, else the first of the usual places that has one. It is an error
// if that nvim is older than MinVersion.
func Find(path string) (Found, error) {
	if path == "" {
		path = look()
	}
	if path == "" {
		return Found{}, ErrNotFound
	}
	v, err := ReadVersion(path)
	if err != nil {
		return Found{Path: path}, err
	}
	f := Found{path, v}
	if v.Less(MinVersion) {
		return f, fmt.Errorf("%s is Neovim %s; mdsh-gui needs %s or newer", path, v, MinVersion)
	}
	return f, nil
}

func look() string {
	if p, err := exec.LookPath("nvim"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, c := range candidates {
		if rest, ok := strings.CutPrefix(c, "~/"); ok {
			if home == "" {
				continue
			}
			c = filepath.Join(home, rest)
		}
		if runtime.GOOS != "windows" && strings.HasPrefix(c, `C:\`) {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// ReadVersion runs nvim --version, which reads no config.
func ReadVersion(path string) (Version, error) {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return Version{}, fmt.Errorf("%s --version: %w", path, err)
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return parseVersion(line)
}

var versionLine = regexp.MustCompile(`^NVIM v(\d+)\.(\d+)\.(\d+)`)

// parseVersion reads the first line of nvim --version, such as
// "NVIM v0.11.4" or "NVIM v0.12.0-dev-1234+g5678".
func parseVersion(line string) (Version, error) {
	m := versionLine.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return Version{}, fmt.Errorf("not a Neovim version: %q", line)
	}
	var v Version
	v.Major, _ = strconv.Atoi(m[1])
	v.Minor, _ = strconv.Atoi(m[2])
	v.Patch, _ = strconv.Atoi(m[3])
	return v, nil
}

// StartTimeout is how long Start waits for Neovim and the user's config.
var StartTimeout = 5 * time.Second

//go:embed mdsh.lua
var mdshLua string

// Options are how to start an Editor.
type Options struct {
	Path       string
	Cols, Rows int
	// Name is the buffer's name, such as mdsh://notes.md.
	Name string
	Text string
	// Clean starts Neovim without the user's config.
	Clean bool
	// Dark sets 'background' to dark, else light, before the config
	// runs, which may set it again.
	Dark bool
	// LSP is the command that starts mdsh's language server, which the
	// buffer attaches to for completion, hover help and diagnostics.
	// Empty starts none.
	LSP []string

	// Redraw gets the redraw events up to each flush, as Neovim sent
	// them: each is the event's name and then one argument list for
	// each time it happened.
	Redraw func(events [][]any)
	// Changed gets the buffer's text a moment after it is edited.
	Changed func(text string)
	// Write gets the buffer's text for :w.
	Write func(text string)
	// Event gets what the buffer's mdsh side reports, such as the
	// cursor moving or :RunBlock: the event's name and its data as JSON.
	Event func(name, data string)
	// Exit is called once Neovim has quit, with why if it failed.
	Exit func(err error)
}

// Editor is one running Neovim.
type Editor struct {
	v *nvim.Nvim

	mu      sync.Mutex
	pending [][]any // redraw events since the last flush
	text    string  // the text last sent either way
}

// Start starts Neovim and attaches to it as its UI.
func Start(o Options) (*Editor, error) {
	bg := "light"
	if o.Dark {
		bg = "dark"
	}
	args := []string{"--embed", "--cmd", "set background=" + bg}
	if o.Clean {
		args = append(args, "--clean")
	}
	v, err := nvim.NewChildProcess(
		nvim.ChildProcessCommand(o.Path),
		nvim.ChildProcessArgs(args...),
		nvim.ChildProcessEnv(append(os.Environ(), "MDSH_GUI=1")),
		nvim.ChildProcessServe(false),
		nvim.ChildProcessLogf(log.Printf),
	)
	if err != nil {
		return nil, err
	}
	e := &Editor{v: v, text: o.Text}
	v.RegisterHandler("redraw", func(updates ...[]any) {
		e.mu.Lock()
		var flushes [][][]any
		for _, u := range updates {
			if len(u) > 0 && u[0] == "flush" {
				flushes = append(flushes, e.pending)
				e.pending = nil
				continue
			}
			e.pending = append(e.pending, u)
		}
		e.mu.Unlock()
		if o.Redraw != nil {
			for _, f := range flushes {
				o.Redraw(f)
			}
		}
	})
	v.RegisterHandler("mdsh_text", func(text string) {
		e.mu.Lock()
		e.text = text
		e.mu.Unlock()
		if o.Changed != nil {
			o.Changed(text)
		}
	})
	v.RegisterHandler("mdsh_write", func(text string) {
		e.mu.Lock()
		e.text = text
		e.mu.Unlock()
		if o.Write != nil {
			o.Write(text)
		}
	})
	v.RegisterHandler("mdsh_event", func(name, data string) {
		if o.Event != nil {
			o.Event(name, data)
		}
	})
	go func() {
		err := v.Serve()
		if o.Exit != nil {
			o.Exit(err)
		}
	}()
	fail := func(err error) (*Editor, error) {
		v.Close()
		return nil, err
	}
	// A config that stops at a prompt never answers; give up on it.
	setup := make(chan error, 1)
	go func() {
		opts := map[string]any{"rgb": true, "ext_linegrid": true}
		if err := v.AttachUI(max(o.Cols, 1), max(o.Rows, 1), opts); err != nil {
			setup <- fmt.Errorf("attaching to Neovim: %w", err)
			return
		}
		lspCmd := o.LSP
		if lspCmd == nil {
			lspCmd = []string{}
		}
		if err := v.ExecLua(mdshLua, nil, v.ChannelID(), o.Name, lines(o.Text), lspCmd); err != nil {
			setup <- fmt.Errorf("setting up Neovim: %w", err)
			return
		}
		setup <- nil
	}()
	select {
	case err := <-setup:
		if err != nil {
			return fail(err)
		}
	case <-time.After(StartTimeout):
		return fail(fmt.Errorf("Neovim didn't start within %v", StartTimeout))
	}
	return e, nil
}

// lines splits text into a buffer's lines. A last newline ends the last
// line rather than starting another.
func lines(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// Input types keys, in Neovim's key notation such as "<C-w>".
func (e *Editor) Input(keys string) error {
	_, err := e.v.Input(keys)
	return err
}

// Mouse passes on a mouse event: button is "left", "right", "middle" or
// "wheel"; action is "press", "drag" or "release", or for the wheel "up",
// "down", "left" or "right"; mod is like "C" or "S-A" or "".
func (e *Editor) Mouse(button, action, mod string, row, col int) error {
	return e.v.InputMouse(button, action, mod, 0, row, col)
}

// Paste pastes text as one undoable change.
func (e *Editor) Paste(text string) error {
	_, err := e.v.Paste(text, true, -1)
	return err
}

// Resize asks for a grid of cols by rows.
func (e *Editor) Resize(cols, rows int) error {
	return e.v.TryResizeUI(max(cols, 1), max(rows, 1))
}

// SetText replaces the buffer's text, as after a run writes its output.
// Text the buffer already has changes nothing.
func (e *Editor) SetText(text string) error {
	e.mu.Lock()
	same := text == e.text
	e.text = text
	e.mu.Unlock()
	if same {
		return nil
	}
	return e.v.ExecLua("mdsh.set_text(...)", nil, lines(text))
}

// ApplyEdits applies edits to the buffer: each replaces lines
// [Start, End), counted from 0, which must still hold Old, with New.
// Edits are in order and don't overlap. If any edit's lines changed, the
// whole text is replaced with text instead. Either way the buffer then
// has text, as far as mdsh-gui knows; typing since may follow.
func (e *Editor) ApplyEdits(text string, edits []Edit) error {
	e.mu.Lock()
	same := text == e.text
	e.text = text
	e.mu.Unlock()
	if same {
		return nil
	}
	var ok bool
	if err := e.v.ExecLua("return mdsh.apply(...)", &ok, edits); err != nil {
		return err
	}
	if ok {
		return nil
	}
	return e.v.ExecLua("mdsh.set_text(...)", nil, lines(text))
}

// Edit replaces lines [Start, End) of a buffer, counted from 0, which
// hold Old, with New.
type Edit struct {
	Start int      `msgpack:"start" json:"start"`
	End   int      `msgpack:"end" json:"end"`
	Old   []string `msgpack:"old" json:"old"`
	New   []string `msgpack:"new" json:"new"`
}

// Call calls the function called fn in the buffer's mdsh side with
// args, a JSON array, and returns what it returns, as JSON.
func (e *Editor) Call(fn, args string) (string, error) {
	if !callable.MatchString(fn) {
		return "", fmt.Errorf("no Neovim call %q", fn)
	}
	var out string
	err := e.v.ExecLua("return mdsh.call(...)", &out, fn, args)
	return out, err
}

var callable = regexp.MustCompile(`^[a-z_]+$`)

// Saved marks the buffer unchanged, once its text is written.
func (e *Editor) Saved() error {
	return e.v.ExecLua("mdsh.saved()", nil)
}

// Close quits Neovim.
func (e *Editor) Close() error {
	return e.v.Close()
}
