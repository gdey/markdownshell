package main

import (
	"errors"
	"fmt"
	"github.com/gdey/markdownshell/audit"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gdey/markdownshell/cmd/mdsh-gui/notebook"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/session"
	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
	"github.com/gdey/markdownshell/collection"
	"github.com/gdey/markdownshell/library"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/writable"
)

// watchEvery is how often an open document's file is checked for
// changes on disk, as mdsh-gui does.
const watchEvery = 2 * time.Second

// errNoDoc is the error for a call naming a document that isn't open.
var errNoDoc = errors.New("no such document")

// bridge holds the documents the app has open and the app's settings.
type bridge struct {
	c        *conn
	settings *settings.Store

	// openMu makes opening a file one at a time, so two opens of the
	// same file can't both miss it in docs and open it twice.
	openMu sync.Mutex

	mu     sync.Mutex
	docs   map[int]*document
	nextID int
	// folders are the folders the start screen last listed, by rel,
	// with their absolute paths: the only ones the folder settings
	// dialog may read or write.
	folders map[string]string

	// casts are the recordings the cast player loaded.
	casts casts
}

func newBridge(c *conn, st *settings.Store) *bridge {
	return &bridge{c: c, settings: st, docs: map[int]*document{}}
}

// document is one open file: a notebook with its shells. It is
// mdsh-gui's docTab without the window.
type document struct {
	b       *bridge
	id      int
	n       *notebook.Notebook
	answers chan run.Answer
	done    chan struct{}

	mu          sync.Mutex
	running     bool
	asking      bool // a confirm event waits for an answer
	warned      bool // file-changed was sent for the file on disk
	roRun       string
	openHistory bool

	// events is the file's event log, kept through reloads, and auditQ
	// takes its new events to sendAudit.
	events *notebook.EventLog
	auditQ chan notebook.AuditEvent
}

// sendAudit sends the app each new event of the document's log, as an
// "audit" event, until the document closes. It runs on its own
// goroutine, since events are logged with the notebook's lock held.
func (d *document) sendAudit() {
	for {
		select {
		case <-d.done:
			return
		case e := <-d.auditQ:
			d.emit("audit", e)
		}
	}
}

// emit sends the app an event about the document.
func (d *document) emit(name string, data any) {
	d.b.c.send(event{Event: name, Doc: d.id, Data: data})
}

// nb is the document's notebook, which Reload replaces.
func (d *document) nb() *notebook.Notebook {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.n
}

func (b *bridge) doc(id int) (*document, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d, ok := b.docs[id]
	if !ok {
		return nil, fmt.Errorf("%w: %d", errNoDoc, id)
	}
	return d, nil
}

// byFile is the document that has file open, or nil.
func (b *bridge) byFile(file string) *document {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, d := range b.docs {
		if d.nb().File() == file {
			return d
		}
	}
	return nil
}

// runOptions are the run options a newly opened file starts with: the
// shell from Settings, and the History pane's rc file.
func (b *bridge) runOptions() notebook.RunOptions {
	s := b.settings.Get()
	return notebook.RunOptions{Shell: s.Shell, HistoryRC: s.HistoryRC}
}

// open opens file as a new document and returns its id. A file already
// open returns the document that has it.
func (b *bridge) open(file string) (int, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return 0, err
	}
	b.openMu.Lock()
	defer b.openMu.Unlock()
	if d := b.byFile(abs); d != nil {
		return d.id, nil
	}
	d := &document{b: b, answers: make(chan run.Answer, 1), done: make(chan struct{}), auditQ: make(chan notebook.AuditEvent, 256)}
	d.events = notebook.NewEventLog(func(e notebook.AuditEvent) {
		select {
		case d.auditQ <- e:
		default:
			// The app sees the gap in Seq and reads the log again.
		}
	})
	b.mu.Lock()
	b.nextID++
	d.id = b.nextID
	b.mu.Unlock()
	n, err := notebook.Open(abs, d.options(b.runOptions()))
	if err != nil {
		return 0, err
	}
	d.n = n
	b.mu.Lock()
	b.docs[d.id] = d
	b.mu.Unlock()
	b.opened(n.File())
	go d.watch()
	go d.sendAudit()
	return d.id, nil
}

func (d *document) options(ro notebook.RunOptions) notebook.Options {
	return notebook.Options{
		Run:     ro,
		Events:  d.events,
		OnEvent: func(e notebook.Event) { d.emit("block", e) },
		Confirm: func(block int, q string) run.Answer {
			return d.ask(map[string]any{"block": block, "question": q})
		},
		Depend: func(block, of int, q string) run.Answer {
			return d.ask(map[string]any{"block": block, "of": of, "question": q})
		},
		// A warning an edit fixed goes stale in the console.
		Resolved: func(file, warning string) {
			d.b.c.send(event{Event: "console-stale", Data: staleEntry{File: file, Message: warning}})
		},
	}
}

// reload reads the document's file again, dropping output not saved.
// The shells keep what blocks set in them, unless the front matter
// changed the shell, env.share or env.source, which waits for Restart
// Shells.
func (d *document) reload() error {
	// A run waiting on a question is stopped first, so the next answer
	// can't resume it on the file read again, and the reload waits a
	// moment for it to end. A block running otherwise fails it, as in
	// mdsh-gui.
	stopped := d.reply(run.Stop)
	for tries := 0; ; tries++ {
		_, err := d.nb().Reload()
		if err == nil {
			break
		}
		if !stopped || !errors.Is(err, notebook.ErrBusy) || tries >= 500 {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	d.mu.Lock()
	d.warned = false
	d.mu.Unlock()
	d.emit("opened", d.nb().File())
	return nil
}

// opened notes in the settings that file was opened, for Recent.
func (b *bridge) opened(file string) {
	b.settings.Update(func(s *settings.Settings) { s.Opened(file) })
}

// ask sends a confirm event and waits for the app's answer.
func (d *document) ask(q map[string]any) run.Answer {
	d.mu.Lock()
	d.asking = true
	d.mu.Unlock()
	d.emit("confirm", q)
	select {
	case a := <-d.answers:
		return a
	case <-d.done:
		d.mu.Lock()
		d.asking = false
		d.mu.Unlock()
		return run.Stop
	}
}

// reply answers the question ask is waiting on. It reports false when
// nothing is waiting. The answer may come before ask is ready for it,
// as the app can answer as soon as it reads the event, so answers holds
// one.
func (d *document) reply(a run.Answer) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.asking {
		return false
	}
	d.asking = false
	d.answers <- a
	return true
}

// run runs f, a run of the document, and says when it is over with a
// "ran" event.
func (d *document) run(f func() error) error {
	d.mu.Lock()
	d.running = true
	d.mu.Unlock()
	err := f()
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()
	d.emit("ran", nil)
	return err
}

// beforeRun, with autosave on and a read-only file, refuses the first
// run, as mdsh-gui does, so the app can offer to save a copy; the next
// run goes ahead without saving.
func (d *document) beforeRun() error {
	file := d.nb().File()
	if !d.b.settings.Get().Autosave {
		return nil
	}
	d.mu.Lock()
	skip := d.roRun == file
	d.mu.Unlock()
	if skip || !errors.Is(writable.Check(file), writable.ErrReadOnly) {
		return nil
	}
	d.mu.Lock()
	d.roRun = file
	d.mu.Unlock()
	return fmt.Errorf("not run: %s is read-only; save a copy, or run again to run without saving", filepath.Base(file))
}

// afterRun saves if autosave is on, unless the file changed on disk.
func (d *document) afterRun() {
	if d.b.settings.Get().Autosave && d.nb().Dirty() && !d.nb().Changed() {
		if err := d.nb().Save(); err == nil {
			d.saved()
		}
	}
}

func (d *document) saved() {
	d.mu.Lock()
	d.warned = false
	d.mu.Unlock()
	d.emit("saved", d.nb().File())
}

// watch tells the app once when the file changes on disk, until it is
// saved or opened again.
func (d *document) watch() {
	tick := time.NewTicker(watchEvery)
	defer tick.Stop()
	for {
		select {
		case <-d.done:
			return
		case <-tick.C:
		}
		d.mu.Lock()
		n, warned := d.n, d.warned // under d.mu
		d.mu.Unlock()
		if warned || !n.Changed() {
			continue
		}
		d.mu.Lock()
		d.warned = true
		d.mu.Unlock()
		d.events.Event("file.changed", audit.Fields{"file": n.File()})
		d.emit("file-changed", n.File())
	}
}

// close ends the document's shells.
func (b *bridge) close(id int) error {
	b.mu.Lock()
	d, ok := b.docs[id]
	delete(b.docs, id)
	b.mu.Unlock()
	if !ok {
		return errNoDoc
	}
	close(d.done)
	d.nb().Close()
	return nil
}

func (b *bridge) closeAll() {
	b.mu.Lock()
	ids := make([]int, 0, len(b.docs))
	for id := range b.docs {
		ids = append(ids, id)
	}
	b.mu.Unlock()
	for _, id := range ids {
		b.close(id)
	}
}

// docInfo is what the app shows about an open document; mdsh-gui's Doc.
type docInfo struct {
	ID          int                 `json:"id"`
	File        string              `json:"file"`
	Dirty       bool                `json:"dirty"`
	Changed     bool                `json:"changed"`
	Running     bool                `json:"running"`
	OpenHistory bool                `json:"openHistory"`
	Run         notebook.RunOptions `json:"run"`
	Args        string              `json:"args"`
	History     bool                `json:"history"`
	HasOutput   bool                `json:"hasOutput"`
	TermCols    int                 `json:"termCols"`
	TermRows    int                 `json:"termRows"`
	EnvBanner   string              `json:"envBanner"`
	// LastView is how the file was last shown, "rendered", "source" or
	// "both", or "" if that isn't remembered. LastHistory is whether
	// its History pane was open then.
	LastView    string `json:"lastView"`
	LastHistory bool   `json:"lastHistory"`
}

func (d *document) info() docInfo {
	d.mu.Lock()
	running, history := d.running, d.openHistory
	d.openHistory = false
	d.mu.Unlock()
	ro := d.nb().RunOptions()
	cols, rows := d.nb().TerminalSize()
	n := d.nb()
	last, _ := d.b.settings.Get().FileView(n.File())
	return docInfo{ID: d.id, File: n.File(), Dirty: n.Dirty(), Changed: n.Changed(), Running: running, OpenHistory: history, Run: ro, Args: run.JoinArgs(ro.Args), History: n.History(), HasOutput: n.HasOutput(), TermCols: cols, TermRows: rows, EnvBanner: n.EnvBanner(), LastView: last.View, LastHistory: last.History}
}

// start is what the start screen lists; mdsh-gui's Start.
type start struct {
	Dir         string               `json:"dir"`
	Home        string               `json:"home"`
	Files       []library.File       `json:"files"`
	Collections []library.Collection `json:"collections"`
	Full        bool                 `json:"full"`
	Recent      []string             `json:"recent"`
	Pinned      []string             `json:"pinned"`
	LastSession string               `json:"lastSession"`
	// Writable are the listed folders, by rel, whose .mdsh.yaml the
	// folder settings can save.
	Writable []string `json:"writable"`
}

// startDir is dir if set, else the start folder the settings give.
func (b *bridge) startDir(dir string) string {
	if dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	docs := ""
	if runtime.GOOS == "darwin" {
		docs = filepath.Join(home, "Documents")
	}
	return b.settings.Get().StartDir(cwd, home, docs)
}

func (b *bridge) startScreen(dir string) (start, error) {
	dir = b.startDir(dir)
	l, err := library.List(dir)
	if err != nil {
		return start{}, err
	}
	// The app loads a collection's images from file: URLs.
	for i := range l.Collections {
		c := &l.Collections[i]
		if c.IconPath != "" {
			c.IconURL = (&url.URL{Scheme: "file", Path: c.IconPath}).String()
		}
		if c.Background != "" {
			c.BackgroundURL = (&url.URL{Scheme: "file", Path: c.Background}).String()
		}
	}
	s := b.settings.Get()
	home, _ := os.UserHomeDir()
	last, _ := session.Last(b.sessionDir())
	return start{Dir: dir, Home: home, Files: l.Files, Collections: l.Collections, Full: l.Full, Recent: existing(s.Recent), Pinned: existing(s.Pinned), LastSession: last, Writable: b.writableFolders(dir, l)}, nil
}

// writableFolders are the folders a start screen listing of dir shows,
// by rel, whose .mdsh.yaml can be saved, in path order, as mdsh-gui's
// startFolders works them out.
func (b *bridge) writableFolders(dir string, l library.Listing) []string {
	folders := map[string]string{}
	var out []string
	add := func(rel string) {
		if _, ok := folders[rel]; ok {
			return
		}
		abs := filepath.Join(dir, filepath.FromSlash(rel))
		folders[rel] = abs
		if collection.Writable(abs) {
			out = append(out, rel)
		}
	}
	for _, c := range l.Collections {
		add(c.Rel)
	}
	for _, f := range l.Files {
		if d := path.Dir(f.Rel); d != "." {
			add(d)
		} else {
			add("")
		}
	}
	slices.Sort(out)
	b.mu.Lock()
	b.folders = folders
	b.mu.Unlock()
	return out
}

// folder is the absolute path of rel, a folder the start screen lists.
func (b *bridge) folder(rel string) (string, error) {
	b.mu.Lock()
	abs, ok := b.folders[rel]
	b.mu.Unlock()
	if !ok {
		return "", errors.New("not a folder the start screen lists")
	}
	return abs, nil
}

func (b *bridge) sessionDir() string {
	home, _ := os.UserHomeDir()
	return session.Dir(b.settings.Get().SessionFolder, home)
}

// existing are the files that still exist.
func existing(files []string) []string {
	var out []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// createFile writes a new markdown file, as mdsh-gui's New File… does:
// a copy of template, or a heading named for the file.
func (b *bridge) createFile(file, template string) (string, error) {
	file, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	if filepath.Ext(file) == "" {
		file += ".md"
	}
	if b.byFile(file) != nil {
		return "", fmt.Errorf("%s is open; close it first", filepath.Base(file))
	}
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	src := []byte("# " + title + "\n")
	if template != "" {
		var err error
		if src, err = os.ReadFile(template); err != nil {
			return "", err
		}
	}
	return file, os.WriteFile(file, src, 0o644)
}

// edited copies into cur what the Settings window edits from s, as
// mdsh-gui's edited does, dropping values it can't have sent.
func edited(cur *settings.Settings, s settings.Settings) {
	cur.Autosave, cur.Vim, cur.NoSpellCheck, cur.Shell, cur.StartFolder = s.Autosave, s.Vim, s.NoSpellCheck, s.Shell, s.StartFolder
	cur.TemplateFolder = s.TemplateFolder
	cur.SessionFolder, cur.SessionTemplate = strings.TrimSpace(s.SessionFolder), strings.TrimSpace(s.SessionTemplate)
	cur.HistoryRC = strings.TrimSpace(s.HistoryRC)
	cur.Theme = oneOf(s.Theme, "light", "dark")
	cur.Look = oneOf(s.Look, lookIDs...)
	cur.TextSize = 0
	if s.TextSize != 0 {
		cur.TextSize = min(max(s.TextSize, 80), 150)
	}
	cur.UIFont, cur.CodeFont = strings.TrimSpace(s.UIFont), strings.TrimSpace(s.CodeFont)
	cur.LineSpacing = oneOf(s.LineSpacing, "compact", "relaxed")
	cur.NoSprockets, cur.NoRules, cur.NoShadows = s.NoSprockets, s.NoRules, s.NoShadows
	cur.Accent = ""
	if isHexColor(s.Accent) {
		cur.Accent = strings.ToLower(s.Accent)
	}
	cur.HistoryPane = oneOf(s.HistoryPane, "match", "invert", "dark")
	cur.View = oneOf(s.View, "rendered", "source", "both")
}

// lookIDs are the looks mdsh-swift draws, besides Plain's "", by their
// ids in mdsh-gui's look.css files (looks_test.go checks them).
var lookIDs = []string{"tty", "blueprint", "gum", "witch"}

func oneOf(s string, allowed ...string) string {
	if slices.Contains(allowed, s) {
		return s
	}
	return ""
}

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// answers maps the app's replies to a confirm event.
var answers = map[string]run.Answer{"run": run.Go, "skip": run.Skip, "stop": run.Stop}
