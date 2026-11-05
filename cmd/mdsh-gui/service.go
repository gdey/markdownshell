package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/fonts"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/looks"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/session"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/shellrc"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/library"
	"github.com/gdey/markdownshell/report"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termrec"
	"github.com/gdey/markdownshell/writable"
)

// This file holds the methods the frontend calls.

// Doc is what the frontend shows about the open file.
type Doc struct {
	File    string `json:"file"`
	Dirty   bool   `json:"dirty"`
	Changed bool   `json:"changed"`
	Running bool   `json:"running"` // a run is going on
	// Fresh is set the first time Doc is asked for after a file opens
	// in the tab, but not after it is reloaded, so the page can show
	// the view the settings open files in.
	Fresh bool `json:"fresh"`
	// OpenHistory is set the first time Doc is asked for after a
	// session opens in the tab, so the page opens the History pane and
	// focuses it.
	OpenHistory bool                `json:"openHistory"`
	Run         notebook.RunOptions `json:"run"`
	// Args is Run.Args as one line, quoted as a shell would need.
	Args string `json:"args"`
	// History is set while the History pane is on for the file.
	History bool `json:"history"`
	// TermCols and TermRows are the terminal size the front matter asks
	// for, 0 where it doesn't.
	TermCols int `json:"termCols"`
	TermRows int `json:"termRows"`
	// EnvBanner, if set, says that the front matter brings in the whole
	// environment the app started in (env.share with ...).
	EnvBanner string `json:"envBanner"`
	// LastView is how the file was last shown, "rendered", "source" or
	// "both", or "" if that isn't remembered. LastHistory is whether
	// its History pane was open then.
	LastView    string `json:"lastView"`
	LastHistory bool   `json:"lastHistory"`
}

// Doc describes the open file. File is "" while the start screen shows.
func (a *App) Doc(ctx context.Context) (Doc, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return Doc{}, err
	}
	n, err := t.notebook()
	if err != nil {
		return Doc{}, nil
	}
	t.mu.Lock()
	running, fresh, history := t.running, t.fresh, t.openHistory
	t.fresh, t.openHistory = false, false
	t.mu.Unlock()
	ro := n.RunOptions()
	cols, rows := n.TerminalSize()
	last, _ := a.settings.Get().FileView(n.File())
	return Doc{File: n.File(), Dirty: n.Dirty(), Changed: n.Changed(), Running: running, Fresh: fresh, OpenHistory: history, Run: ro, Args: run.JoinArgs(ro.Args), History: n.History(), TermCols: cols, TermRows: rows, EnvBanner: n.EnvBanner(), LastView: last.View, LastHistory: last.History}, nil
}

// Live is what the run going on in the tab has sent so far, for a page
// that starts partway through it.
func (a *App) Live(ctx context.Context) ([]liveEvent, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.live), nil
}

// Cells is the document laid out for drawing.
func (a *App) Cells(ctx context.Context) ([]notebook.Cell, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return nil, err
	}
	return n.Cells(), nil
}

// RunBlock runs block i alone.
func (a *App) RunBlock(ctx context.Context, i int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if err := a.beforeRun(t, n); err != nil {
		return err
	}
	err = t.run(func() error { return n.RunBlock(i) })
	a.afterRun(t, n)
	return err
}

// RunFrom runs block i and the blocks after it.
func (a *App) RunFrom(ctx context.Context, i int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if err := a.beforeRun(t, n); err != nil {
		return err
	}
	err = t.run(func() error { return n.RunFrom(i) })
	a.afterRun(t, n)
	return err
}

// RunSection runs blocks from up to but not including to, a heading's
// section, as Run all runs them.
func (a *App) RunSection(ctx context.Context, from, to int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if err := a.beforeRun(t, n); err != nil {
		return err
	}
	err = t.run(func() error { return n.RunRange(from, to) })
	a.afterRun(t, n)
	return err
}

// beforeRun, with autosave on and a read-only file, asks where to save a
// copy before the run, which then goes on in the copy. Cancelling stops
// this run; the next one goes ahead without asking, and without saving.
func (a *App) beforeRun(t *docTab, n *notebook.Notebook) error {
	file := n.File()
	if !a.settings.Get().Autosave || !t.askReadOnly(file) {
		return nil
	}
	name := filepath.Base(file)
	ok, err := a.saveAs(t, n, name+" is read-only. Save a copy to run it with autosave")
	if err != nil || ok {
		return err
	}
	t.runReadOnly(file)
	return fmt.Errorf("not run: %s is read-only; run again to run without saving", name)
}

// afterRun marks the title and saves if autosave is on. It doesn't
// save over a file that changed on disk; the banner asks about that.
func (a *App) afterRun(t *docTab, n *notebook.Notebook) {
	if a.settings.Get().Autosave && n.Dirty() && !n.Changed() {
		if err := n.Save(); err == nil {
			t.saved()
			return
		}
	}
	t.changed()
}

// HasOutput reports whether the file has output for ClearOutputs to
// take out, so Clear All Outputs is only offered when it would do
// something.
func (a *App) HasOutput(ctx context.Context) (bool, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return false, err
	}
	return n.HasOutput(), nil
}

// ClearOutputs takes every block's output out of the document and
// starts its shells again.
func (a *App) ClearOutputs(ctx context.Context) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.ClearOutputs()
	t.changed()
	return err
}

// RestartShells starts the file's shells again, as if nothing had run,
// without changing the file. The page asks the user first: it is the
// one way the app ends shells that blocks set things up in.
func (a *App) RestartShells(ctx context.Context) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.Restart()
	t.changed()
	return err
}

// KillShell ends the shell the running block runs in, at once, for a
// block that Stop didn't stop, once the user has said to. What blocks
// set in that shell is gone. It reports whether a block was running.
func (a *App) KillShell(ctx context.Context) bool {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return false
	}
	return n.Kill()
}

// Stop stops the running block, answering stop if a block is waiting
// to be confirmed. The block is interrupted, as ctrl+c would, and its
// shell lives on.
func (a *App) Stop(ctx context.Context) {
	t, err := a.tab(ctx)
	if err != nil {
		return
	}
	select {
	case t.answers <- run.Stop:
	default:
	}
	if n, err := t.notebook(); err == nil {
		n.Stop()
	}
}

// Answer replies to the question a confirm event asked: "run", "skip"
// or "stop".
func (a *App) Answer(ctx context.Context, reply string) error {
	t, err := a.tab(ctx)
	if err != nil {
		return err
	}
	ans, ok := map[string]run.Answer{"run": run.Go, "skip": run.Skip, "stop": run.Stop}[reply]
	if !ok {
		return fmt.Errorf("unknown answer %q", reply)
	}
	select {
	case t.answers <- ans:
		return nil
	default:
		return errors.New("nothing is waiting for an answer")
	}
}

// Open asks for markdown files and opens them: the first in this tab if
// it shows the start screen, the rest in new tabs.
func (a *App) Open(ctx context.Context) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	files, err := a.openDialog(w.win)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := a.openIn(w, f); err != nil {
			return err
		}
	}
	return nil
}

// NewFile asks where to make a new markdown file and opens it: in this
// tab if it shows the start screen, else in a new tab. The file is a
// copy of template, or a heading if template is "".
func (a *App) NewFile(ctx context.Context, template string) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	return a.newFile(w, template)
}

// Templates lists the markdown files in the templates folder, for New
// File… to start from. With no templates folder set it lists none.
func (a *App) Templates() ([]library.File, error) {
	dir := a.settings.Get().TemplateFolder
	if dir == "" {
		return nil, nil
	}
	l, err := library.List(dir)
	return l.Files, err
}

// NewSession opens the day's session, from the start screen: a file
// named for the date in the sessions folder, made from the session
// template if it isn't there yet, or else set to run in your login
// shell. It opens with the History pane open
// and focused.
func (a *App) NewSession(ctx context.Context) error {
	s := a.settings.Get()
	file, _, err := session.Today(a.sessionDir(), s.SessionTemplate, session.LoginShell(os.Getenv), time.Now())
	if err != nil {
		return fmt.Errorf("new session: %w", err)
	}
	return a.openSession(ctx, file)
}

// LastSession opens the file in the sessions folder changed last, as
// NewSession opens a session.
func (a *App) LastSession(ctx context.Context) error {
	file, err := session.Last(a.sessionDir())
	if err != nil {
		return fmt.Errorf("last session: %w", err)
	}
	if file == "" {
		return errors.New("no sessions yet: New Session makes one")
	}
	return a.openSession(ctx, file)
}

// sessionDir is the sessions folder the settings name.
func (a *App) sessionDir() string {
	home, _ := os.UserHomeDir()
	return session.Dir(a.settings.Get().SessionFolder, home)
}

// openSession opens file as openIn does, and has its tab open the
// History pane and focus it: a tab opening the file asks Doc, which
// says so once, and a tab that has it open already is told.
func (a *App) openSession(ctx context.Context, file string) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	if t := a.openTab(file); t != nil {
		a.focusTab(t)
		t.emit("menu", "history-open")
		return nil
	}
	if err := a.openIn(w, file); err != nil {
		return err
	}
	if t := a.openTab(file); t != nil {
		t.mu.Lock()
		t.openHistory = true
		t.mu.Unlock()
	}
	return nil
}

// OpenPath opens file, from the start screen.
func (a *App) OpenPath(ctx context.Context, file string) error {
	t, err := a.tab(ctx)
	if err != nil {
		return err
	}
	if open := a.openTab(file); open != nil {
		a.focusTab(open)
		return nil
	}
	return t.open(file, a.runOptions())
}

// Save writes the output into the file.
func (a *App) Save(ctx context.Context) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if errors.Is(checkWritable(n.File()), writable.ErrReadOnly) {
		name := filepath.Base(n.File())
		ok, err := a.saveAs(t, n, name+" is read-only. Save a copy with its output")
		if err == nil && !ok {
			err = fmt.Errorf("not saved: %s is read-only", name)
		}
		return err
	}
	if err := n.Save(); err != nil {
		return err
	}
	t.saved()
	return nil
}

// SaveAs asks where to save the file with its output, and from then on
// the tab shows the new file. It returns false if the user cancels.
func (a *App) SaveAs(ctx context.Context) (bool, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return false, err
	}
	return a.saveAs(t, n, "Save a copy with its output")
}

// checkWritable reports whether a file can be saved; tests replace it.
var checkWritable = writable.Check

// saveAs asks, with message, where to save n with its output, and saves
// it there. It returns false if the user cancels.
func (a *App) saveAs(t *docTab, n *notebook.Notebook, message string) (bool, error) {
	w := t.window()
	if w == nil {
		return false, errNoFile
	}
	file, err := a.app.Dialog.SaveFile().
		SetMessage(message).
		SetDirectory(filepath.Dir(n.File())).
		SetFilename(filepath.Base(n.File())).
		AddFilter("Markdown", "*.md;*.markdown").
		AttachToWindow(w.win).
		PromptForSingleSelection()
	if err != nil || file == "" {
		return false, err
	}
	if err := n.SaveAs(file); err != nil {
		return false, err
	}
	t.saved()
	a.opened(n.File())
	return true, nil
}

// Reload reads the file from disk again, dropping output not saved.
// The shells keep what blocks set in them, unless the front matter
// changed the shell, env.share or env.source.
func (a *App) Reload(ctx context.Context) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if _, err := n.Reload(); err != nil {
		return err
	}
	t.mu.Lock()
	t.warned = false
	t.mu.Unlock()
	t.changed()
	t.emit("opened", n.File())
	return nil
}

// Events is the event log of the file this window shows: its shells
// starting and ending, and why, blocks running, the file changing on
// disk and mdsh ctl calls, oldest first. New events come as "audit"
// events.
func (a *App) Events(ctx context.Context) ([]notebook.AuditEvent, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return nil, err
	}
	return t.eventLog().List(), nil
}

// SetRunOptions sets the arguments (one line, split as a shell would),
// --only, --step and --stop-on-error for this file.
func (a *App) SetRunOptions(ctx context.Context, args, only string, step, stopOnError bool) error {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	list, err := run.SplitArgs(args)
	if err != nil {
		return fmt.Errorf("arguments: %w", err)
	}
	ro := n.RunOptions()
	ro.Args, ro.Only, ro.Step, ro.StopOnError = list, only, step, stopOnError
	_, err = n.SetRunOptions(ro)
	return err
}

// SetHistory turns the History pane on or off for the file. Turning it
// on needs shells on a terminal: if theirs have none and blocks have
// run in them, it starts them again only with restart, the user having
// said so, and otherwise fails with notebook.ErrRestartNeeded. It
// reports whether they started again. The shells read the rc file
// Settings names as it is now.
func (a *App) SetHistory(ctx context.Context, on, restart bool) (bool, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return false, err
	}
	n.SetHistoryRC(a.settings.Get().HistoryRC)
	return n.SetHistory(on, restart)
}

// HistoryEnter takes what was typed in the History pane: a note is added
// to the file as prose, and a command is added as a block and run.
func (a *App) HistoryEnter(ctx context.Context, input string) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	if err := a.beforeRun(t, n); err != nil {
		return err
	}
	err = t.run(func() error { return n.Enter(input) })
	a.afterRun(t, n)
	return err
}

// HistoryComplete reports whether input is a whole command, so enter
// runs it, rather than starting another line.
func (a *App) HistoryComplete(ctx context.Context, input string) bool {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return true
	}
	return n.Complete(input)
}

// HistoryCompletions completes the word at the end of before, what is
// typed in the History pane up to the cursor, for Tab.
func (a *App) HistoryCompletions(ctx context.Context, before string) (notebook.Completion, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Completion{}, err
	}
	return n.Completions(before)
}

// HistoryPrompt is the shell's PS1 and PS2, expanded, for the History
// pane's prompt, at the pane's size, cols by rows.
func (a *App) HistoryPrompt(ctx context.Context, cols, rows int) (notebook.Prompt, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Prompt{}, err
	}
	return n.PromptAt(cols, rows)
}

// Start is what the start screen lists.
type Start struct {
	Dir         string               `json:"dir"`
	Home        string               `json:"home"` // for showing paths as ~/…
	Files       []library.File       `json:"files"`
	Collections []library.Collection `json:"collections"`
	Full        bool                 `json:"full"` // more files than listed
	Recent      []string             `json:"recent"`
	// Pinned are the pinned files that still exist, shown above Recent.
	Pinned []string `json:"pinned"`
	// LastSession is the file Last Session opens, or "" if there are no
	// sessions yet.
	LastSession string `json:"lastSession"`
	// Writable are the listed folders, by rel, whose .mdsh.yaml the
	// folder settings can save.
	Writable []string `json:"writable"`
}

// StartScreen lists the markdown files under the start folder, and the
// recent files that still exist.
func (a *App) StartScreen() (Start, error) {
	dir := a.startDir()
	l, err := library.List(dir)
	if err != nil {
		return Start{}, err
	}
	// The window may load the collections' images, and nothing else
	// outside the open documents' folders.
	images := map[string]bool{}
	for i := range l.Collections {
		c := &l.Collections[i]
		if c.IconPath != "" {
			images[c.IconPath] = true
			c.IconURL = notebook.DocURL(c.IconPath)
		}
		if c.Background != "" {
			images[c.Background] = true
			c.BackgroundURL = notebook.DocURL(c.Background)
		}
	}
	folders, writable := startFolders(dir, l)
	a.mu.Lock()
	a.startImages = images
	a.startFolders = folders
	a.mu.Unlock()
	s := a.settings.Get()
	home, _ := os.UserHomeDir()
	last, _ := session.Last(a.sessionDir())
	return Start{Dir: dir, Home: home, Files: l.Files, Collections: l.Collections, Full: l.Full, Recent: existing(s.Recent), Pinned: existing(s.Pinned), LastSession: last, Writable: writable}, nil
}

// existing are the files that still exist. The others stay in the
// settings, so a file that comes back, such as on a drive plugged in
// again, shows again.
func existing(files []string) []string {
	var out []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// Pin pins file to the top of the start screen, or unpins it.
func (a *App) Pin(file string, pin bool) error {
	if !filepath.IsAbs(file) {
		return fmt.Errorf("pin %q: not an absolute path", file)
	}
	file = filepath.Clean(file)
	return a.settings.Update(func(s *settings.Settings) { s.Pin(file, pin) })
}

// Settings are the app-wide settings.
func (a *App) Settings() settings.Settings { return a.settings.Get() }

// SetSettings saves the settings edited in Settings (see
// edited). Open windows hear of it with a "settings" event.
func (a *App) SetSettings(s settings.Settings) error {
	err := a.settings.Update(func(cur *settings.Settings) { edited(cur, s) })
	if err == nil {
		a.app.Event.Emit("settings", a.settings.Get())
	}
	return err
}

// edited copies into cur what Settings edits from s:
// autosave, Vim mode, spell checking, theme, the look and its options,
// shell, the History pane's rc file, the view files open in, the start
// folder, the templates folder, and the sessions folder and template. The rest, such as Recent, stays as
// it is. Values the window can't have sent are dropped.
func edited(cur *settings.Settings, s settings.Settings) {
	cur.Autosave, cur.Vim, cur.NoSpellCheck, cur.Shell, cur.StartFolder = s.Autosave, s.Vim, s.NoSpellCheck, s.Shell, s.StartFolder
	cur.TemplateFolder = s.TemplateFolder
	cur.SessionFolder, cur.SessionTemplate = strings.TrimSpace(s.SessionFolder), strings.TrimSpace(s.SessionTemplate)
	cur.HistoryRC = strings.TrimSpace(s.HistoryRC)
	cur.Neovim, cur.NeovimPath, cur.NeovimClean = s.Neovim, strings.TrimSpace(s.NeovimPath), s.NeovimClean
	cur.NeovimOwnColors = s.NeovimOwnColors
	cur.Theme = ""
	if s.Theme == "light" || s.Theme == "dark" {
		cur.Theme = s.Theme
	}
	cur.Look = ""
	if looks.Valid(allLooks, s.Look) {
		cur.Look = s.Look
	}
	cur.TextSize = 0
	if s.TextSize != 0 {
		cur.TextSize = min(max(s.TextSize, 80), 150)
	}
	cur.UIFont, cur.CodeFont = fontName(s.UIFont), fontName(s.CodeFont)
	cur.LineSpacing = ""
	if s.LineSpacing == "compact" || s.LineSpacing == "relaxed" {
		cur.LineSpacing = s.LineSpacing
	}
	cur.NoSprockets, cur.NoRules, cur.NoShadows = s.NoSprockets, s.NoRules, s.NoShadows
	cur.Accent = ""
	if hexColor.MatchString(s.Accent) {
		cur.Accent = strings.ToLower(s.Accent)
	}
	cur.HistoryPane = ""
	if s.HistoryPane == "match" || s.HistoryPane == "invert" || s.HistoryPane == "dark" {
		cur.HistoryPane = s.HistoryPane
	}
	cur.HistoryFocus = ""
	if s.HistoryFocus == "cursor" {
		cur.HistoryFocus = s.HistoryFocus
	}
	cur.View = ""
	if s.View == "rendered" || s.View == "source" || s.View == "both" {
		cur.View = s.View
	}
}

// ZshWrapper is whether the user's .zshrc, and .bashrc if they have one,
// has the mdsh function that gives blocks the calling shell's aliases
// and functions, and loads mdsh's completion.
type ZshWrapper struct {
	Path     string `json:"path"`     // the .zshrc
	BashPath string `json:"bashPath"` // the .bashrc, or "" when there is none
	Home     string `json:"home"`     // for showing Path as ~/…
	On       bool   `json:"on"`
}

// ZshWrapperState reads the user's .zshrc and .bashrc for the mdsh
// function, bringing a block from an older mdsh-gui up to date so it
// loads completion too.
func (a *App) ZshWrapperState() (ZshWrapper, error) {
	zsh, err := shellrc.Zsh.Path()
	if err != nil {
		return ZshWrapper{}, err
	}
	home, _ := os.UserHomeDir()
	w := ZshWrapper{Path: zsh, Home: home}
	if err := shellrc.Zsh.Update(zsh); err != nil {
		return w, err
	}
	if w.On, err = shellrc.Zsh.Installed(zsh); err != nil {
		return w, err
	}
	bash, err := shellrc.Bash.Path()
	if err != nil {
		return w, err
	}
	if _, err := os.Stat(bash); err != nil {
		return w, nil
	}
	w.BashPath = bash
	if err := shellrc.Bash.Update(bash); err != nil {
		return w, err
	}
	on, err := shellrc.Bash.Installed(bash)
	w.On = w.On || on
	return w, err
}

// SetZshWrapper adds the mdsh function to the user's .zshrc, and to
// their .bashrc if they have one, or takes it out of both, and returns
// the new state. It doesn't make a .bashrc for someone who doesn't use
// bash.
func (a *App) SetZshWrapper(on bool) (ZshWrapper, error) {
	zsh, err := shellrc.Zsh.Path()
	if err != nil {
		return ZshWrapper{}, err
	}
	if err := shellrc.Zsh.Set(zsh, on); err != nil {
		return ZshWrapper{}, err
	}
	bash, err := shellrc.Bash.Path()
	if err != nil {
		return ZshWrapper{}, err
	}
	if _, err := os.Stat(bash); err == nil {
		if err := shellrc.Bash.Set(bash, on); err != nil {
			return ZshWrapper{}, err
		}
	}
	return a.ZshWrapperState()
}

// ChooseFolder asks for a folder, with title, starting in dir or the
// start folder, and returns it, or "" if none was picked.
func (a *App) ChooseFolder(ctx context.Context, title, dir string) (string, error) {
	if dir == "" {
		dir = a.startDir()
	}
	d := a.app.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle(title).
		SetDirectory(dir)
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// ChooseFile asks for a markdown file, with title, starting in dir or
// the start folder, and returns it, or "" if none was picked.
func (a *App) ChooseFile(ctx context.Context, title, dir string) (string, error) {
	if dir == "" {
		dir = a.startDir()
	}
	d := a.app.Dialog.OpenFile().
		CanChooseFiles(true).
		SetTitle(title).
		SetDirectory(dir).
		AddFilter("Markdown", "*.md;*.markdown")
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

// NewWindow opens a window with the start screen.
func (a *App) NewWindow() { a.newWindow(nil) }

// BlockAttrs describes the block attributes, for completion, hover help
// and the attribute form.
func (a *App) BlockAttrs() []doc.AttrSpec { return doc.BlockAttrs }

// Names are the block ids and envs in the file, for completion.
func (a *App) Names(ctx context.Context) (notebook.Names, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Names{}, err
	}
	return n.Names(), nil
}

// CheckInfos checks the attributes in fences' info strings. The source
// pane asks about every new one in one call: a long document has
// thousands, and asking about each in turn took seconds.
func (a *App) CheckInfos(infos []string) [][]notebook.Problem {
	out := make([][]notebook.Problem, len(infos))
	for i, info := range infos {
		out[i] = notebook.CheckInfo(info)
	}
	return out
}

// SourceBlocks lists the blocks in the source pane's text, for
// completing and checking depends=.
func (a *App) SourceBlocks(text string) []notebook.SourceBlock { return notebook.SourceBlocks(text) }

// Text is the whole file, for the split view's source pane.
func (a *App) Text(ctx context.Context) (string, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	return n.Text(), nil
}

// SetText replaces the whole file, as typed in the source pane, if it
// is still base, and reports whether it did. When it isn't, the pane
// reads the file again, keeps what was typed and tries again.
func (a *App) SetText(ctx context.Context, base, text string) (bool, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return false, err
	}
	ok, err := n.SetText(base, text)
	if ok {
		t.changed()
	}
	return ok, err
}

// SetTask checks or unchecks task list item task, as numbered by the
// cells' tasks, in the file.
func (a *App) SetTask(ctx context.Context, task int, done bool) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetTask(task, done)
	t.changed()
	return err
}

// SetFolds writes folds made by hand in the rendered view to the file,
// as collapse attributes and mdsh:collapse lines, so it opens with them.
func (a *App) SetFolds(ctx context.Context, folds []notebook.Fold) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetFolds(folds)
	t.changed()
	return err
}

// SetCastFrame writes frame=n, where a recording's player was left, on
// tty block i, or with i < 0 on the cast fence holding body. Undo takes
// it back.
func (a *App) SetCastFrame(ctx context.Context, i int, body string, frame int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetCastFrame(i, body, frame)
	t.changed()
	return err
}

// ReplaceCast swaps tty block i's recording, in its output, for the
// text of frame n (as "text") or an animated GIF of it (as "gif"). Undo
// puts the recording back.
func (a *App) ReplaceCast(ctx context.Context, i int, as string, frame int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.ReplaceCast(i, as, frame)
	t.changed()
	return err
}

// clipboardText is the text clipboard, as Wails gives it.
type clipboardText interface {
	SetText(string) bool
	Text() (string, bool)
}

// CopySecret puts text, which may hold a secret, on the clipboard, kept
// out of clipboard history where the system allows, and takes it off
// again after the settings' SecretShow unless something else was
// copied.
func (a *App) CopySecret(text string) error {
	return copySecret(text, a.app.Clipboard, a.settings.Get().SecretShow())
}

// CopyCastGIF puts an animated GIF of cast, an asciicast v2 recording, on
// the clipboard.
func (a *App) CopyCastGIF(cast string) error {
	rec, err := termrec.ReadCast(strings.NewReader(cast))
	if err != nil {
		return err
	}
	data, err := termrec.GIF(rec)
	if err != nil {
		return err
	}
	return copyGIF(data)
}

// SetDepends sets block i's depends= to the blocks deps names, by index,
// or takes it out when deps is empty. Undo takes it back.
func (a *App) SetDepends(ctx context.Context, i int, deps []int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetDepends(i, deps)
	t.changed()
	return err
}

// FrontMatter reads the file's front matter for the rendered view's
// front matter form.
func (a *App) FrontMatter(ctx context.Context) (notebook.FrontMatter, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.FrontMatter{}, err
	}
	return n.FrontMatter()
}

// SetFrontMatter writes the front matter form's fields to the file,
// changing only the keys whose field changed. Undo takes it back.
func (a *App) SetFrontMatter(ctx context.Context, f notebook.FrontMatter) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetFrontMatter(f)
	t.changed()
	return err
}

// SetTextbox replaces the text of textbox block i, as typed in it.
func (a *App) SetTextbox(ctx context.Context, i int, text string) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetTextbox(i, text)
	t.changed()
	return err
}

// MoveBlock moves top-level block i up (dir < 0) or down past the next
// cell, or with byBlock past the next block. Undo takes it back.
func (a *App) MoveBlock(ctx context.Context, i, dir int, byBlock bool) (notebook.Move, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Move{}, err
	}
	m, err := n.MoveBlock(i, dir, byBlock)
	t.changed()
	return m, err
}

// DeleteBlock takes top-level block i, with its output and tape, out of
// the file. Undo puts it back.
func (a *App) DeleteBlock(ctx context.Context, i int) (notebook.Deleted, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Deleted{}, err
	}
	del, err := n.DeleteBlock(i)
	t.changed()
	return del, err
}

// ClearOutput takes block i's output out of the file and forgets its
// run. Undo puts it back.
func (a *App) ClearOutput(ctx context.Context, i int) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.ClearOutput(i)
	t.changed()
	return err
}

// MoveTo moves the block or heading's section that starts at from to
// at, a cell's From or -1 for the end, as dragging does.
func (a *App) MoveTo(ctx context.Context, from, at int) (notebook.Move, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Move{}, err
	}
	m, err := n.MoveTo(from, at)
	t.changed()
	return m, err
}

// PlanMove works out MoveBlock's move without making it, for the source
// view to make as its own edit.
func (a *App) PlanMove(ctx context.Context, i, dir int, byBlock bool) (notebook.Move, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Move{}, err
	}
	m, err := n.PlanMove(i, dir, byBlock)
	t.changed()
	return m, err
}

// BlockAt is the top-level block on line of the file, or -1.
func (a *App) BlockAt(ctx context.Context, line int) (int, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return -1, err
	}
	return n.BlockAt(line), nil
}

// Undo takes back the rendered view's last edit, such as checking a task,
// and says what it was: "" when there is none.
func (a *App) Undo(ctx context.Context) (string, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	what, err := n.Undo()
	t.changed()
	return what, err
}

// Redo does again what Undo took back, and says what it was.
func (a *App) Redo(ctx context.Context) (string, error) {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	what, err := n.Redo()
	t.changed()
	return what, err
}

// SetChoice checks the options of choice block i as picked in it.
func (a *App) SetChoice(ctx context.Context, i int, checked []bool) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.SetChoice(i, checked)
	t.changed()
	return err
}

// View is what a tab shows, for the View menu's check marks.
type View struct {
	Split    bool `json:"split"`    // the source pane
	Rendered bool `json:"rendered"` // the rendered view
	Outline  bool `json:"outline"`
	Code     bool `json:"code"`
	Output   bool `json:"output"`
	History  bool `json:"history"` // the History pane
	Console  bool `json:"console"` // the Console pane
}

// defaultView is what a new tab shows until its page says.
var defaultView = View{Rendered: true, Code: true, Output: true}

// SetView notes what the calling tab shows.
func (a *App) SetView(ctx context.Context, v View) {
	t, err := a.tab(ctx)
	if err != nil {
		return
	}
	t.mu.Lock()
	t.view = v
	t.mu.Unlock()
	if a.currentTab() == t {
		a.showView(v)
	}
}

// RememberView notes how file is shown, for the next time it opens:
// view is "rendered", "source" or "both", and history is whether the
// History pane is open.
func (a *App) RememberView(file, view string, history bool) error {
	if view != "rendered" && view != "source" && view != "both" {
		return fmt.Errorf("remember view: unknown view %q", view)
	}
	if !filepath.IsAbs(file) {
		return fmt.Errorf("remember view %q: not an absolute path", file)
	}
	v := settings.FileView{File: filepath.Clean(file), View: view, History: history}
	if last, ok := a.settings.Get().FileView(v.File); ok && last == v {
		return nil
	}
	return a.settings.Update(func(s *settings.Settings) { s.Viewed(v) })
}

// ReportStart is what the Report dialog starts with.
type ReportStart struct {
	Ran   int `json:"ran"`   // runnable blocks that ran in this tab
	Total int `json:"total"` // runnable blocks
	// Dest is where a report goes unless another file is picked: beside
	// the document, named after it, without an extension.
	Dest string `json:"dest"`
	Home string `json:"home"` // for showing paths as ~/…
}

// ReportStart reads what the Report dialog shows when it opens.
func (a *App) ReportStart(ctx context.Context) (ReportStart, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return ReportStart{}, err
	}
	ran, total := n.Ran()
	file := n.File()
	home, _ := os.UserHomeDir()
	return ReportStart{Ran: ran, Total: total, Dest: strings.TrimSuffix(file, filepath.Ext(file)), Home: home}, nil
}

// ChooseReport asks where to save a report in format, starting at dest,
// and returns the file, or "" if none was picked.
func (a *App) ChooseReport(ctx context.Context, format report.Format, dest string) (string, error) {
	t, _, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	w := t.window()
	if w == nil {
		return "", errNoFile
	}
	name := map[report.Format]string{report.PDF: "PDF", report.HTML: "HTML", report.Markdown: "Markdown"}[format]
	return a.app.Dialog.SaveFile().
		SetMessage("Save the report").
		SetDirectory(filepath.Dir(dest)).
		SetFilename(filepath.Base(dest)).
		AddFilter(name, "*"+format.Ext()).
		AttachToWindow(w.win).
		PromptForSingleSelection()
}

// ReportCommand is the mdsh command that makes the report r describes.
func (a *App) ReportCommand(ctx context.Context, r notebook.Report) (string, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	return n.ReportCommand(r, home), nil
}

// CreateReport writes the report r describes.
func (a *App) CreateReport(ctx context.Context, r notebook.Report) error {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	return n.WriteReport(r)
}

// Reveal shows file in the file manager.
func (a *App) Reveal(file string) error {
	return a.app.Env.OpenFileManager(file, true)
}

// Type sends keys typed in the terminal pane to the tty block running in
// it.
func (a *App) Type(ctx context.Context, keys string) {
	if _, n, err := a.notebook(ctx); err == nil {
		n.Type(keys)
	}
}

// SetTerminalSize sets the terminal pane's size in cells, which the
// window works out from its width.
func (a *App) SetTerminalSize(ctx context.Context, cols, rows int) {
	if _, n, err := a.notebook(ctx); err == nil {
		n.SetTerminalSize(cols, rows)
	}
}

// Tabs are the calling window's tabs, for its tab bar.
func (a *App) Tabs(ctx context.Context) (Tabs, error) {
	w, err := a.window(ctx)
	if err != nil {
		return Tabs{}, err
	}
	return w.state(), nil
}

// NewTab opens a tab with the start screen in the calling window, after
// the tab in front.
func (a *App) NewTab(ctx context.Context) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	a.newTabIn(w)
	return nil
}

// SelectTab brings tab id to the front of its window.
func (a *App) SelectTab(id int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	a.selectTab(t)
	return nil
}

// CloseTab closes tab id, asking first if it has unsaved output.
func (a *App) CloseTab(id int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	go a.closeTab(t)
	return nil
}

// DragTab notes that tab id is being dragged, so the window it is
// dropped on can take it.
func (a *App) DragTab(id int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.dragging = t
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", true)
	return nil
}

// DropTab puts the tab being dragged in the calling window, at index i
// of its tab bar. It does nothing if no tab is being dragged.
func (a *App) DropTab(ctx context.Context, i int) error {
	w, err := a.window(ctx)
	if err != nil {
		return err
	}
	a.mu.Lock()
	t := a.dragging
	a.dragging = nil
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", false)
	if t != nil {
		a.moveTab(t, w, i)
		w.win.Focus()
	}
	return nil
}

// EndDrag ends dragging tab id. If no window took it and it was let go
// outside its window, at x, y on the screen, it gets a window of its own
// there.
func (a *App) EndDrag(id int, outside bool, x, y int) error {
	t, err := a.tabByID(id)
	if err != nil {
		return err
	}
	a.mu.Lock()
	dropped := a.dragging != t
	a.dragging = nil
	w := a.tabWindow[t]
	alone := w != nil && len(w.tabs) == 1
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", false)
	if dropped || !outside || alone {
		return nil
	}
	a.detach(t, x, y)
	return nil
}

// DetachTab gives the tab being dragged a window of its own, at x, y on
// the screen, if it shares its window. It does nothing if no tab is being
// dragged.
func (a *App) DetachTab(x, y int) error {
	a.mu.Lock()
	t := a.dragging
	a.dragging = nil
	w := a.tabWindow[t]
	alone := w == nil || len(w.tabs) == 1
	a.mu.Unlock()
	a.app.Event.Emit("tab-drag", false)
	if t != nil && !alone {
		a.detach(t, x, y)
	}
	return nil
}

// detach moves t to a new window with its tab bar at x, y.
func (a *App) detach(t *docTab, x, y int) {
	nw := a.newWindow(t)
	nw.win.SetPosition(max(0, x-120), max(0, y-16))
	nw.win.Focus()
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// fontName is a font family name Settings can save; see fonts.Name.
func fontName(name string) string { return fonts.Name(name) }

// Fonts lists the font families installed on this computer, for the
// Look tab's font menus.
func (a *App) Fonts() fonts.Fonts { return fonts.List() }
