package ui

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdey/markdownshell/notebook"
)

// The console keeps what the app logs, errors from calls the frontend
// makes included, for View › Console to show. Everything logged through
// the host's logger or slog's default logger at info or above lands here,
// as well as wherever it went before (the terminal, in a dev build).
//
// To report something to the console, log it with slog:
//
//	slog.Warn("block git-cleanup: depends= names unknown block", "file", path, "line", 96)
//
// A "file" attribute, the file's absolute path, and a "line" attribute
// say where in a file the entry is about. They become the entry's File
// and Line rather than part of its message, and a click on the entry in
// a window showing that file takes its source pane to the line.
//
// A "fix" attribute on a warning about a file names the fix for it, as
// notebook.FixWarning makes it, and the console offers it. A warning
// that later no longer applies stays, struck through: see resolve.

// ConsoleEntry is one line of the console.
type ConsoleEntry struct {
	Seq     int       `json:"seq"` // counts up from 1, so a page can tell it missed one
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`        // the message, then its attributes as key=value
	File    string    `json:"file,omitempty"` // the file the entry is about, from a "file" attribute
	Line    int       `json:"line,omitempty"` // the line in File, from a "line" attribute; 0 if none
	Fix     string    `json:"fix,omitempty"`  // names the fix for it, from a "fix" attribute
	// Stale says it no longer applies: an edit fixed what it warned of.
	Stale bool `json:"stale,omitempty"`
}

// consoleMax is how many entries the console keeps; older ones go.
const consoleMax = 1000

// Console is the log the Console pane shows.
type Console struct {
	mu      sync.Mutex
	entries []ConsoleEntry
	seq     int
	// out is where new entries go to be sent to the windows, by a
	// goroutine of their own: an entry can be logged from deep inside
	// Wails, where sending an event then and there could deadlock.
	out chan ConsoleEntry
	// stale is the entries going stale, by Seq, to be sent the same way.
	stale chan []int
}

// NewConsole makes an empty console. Log to it through a handler from
// NewConsoleHandler, and have Send take what it gets to the windows.
func NewConsole() *Console {
	return &Console{out: make(chan ConsoleEntry, 256), stale: make(chan []int, 64)}
}

// add adds e, numbering it.
func (c *Console) add(e ConsoleEntry) ConsoleEntry {
	c.mu.Lock()
	c.seq++
	e.Seq = c.seq
	c.entries = append(c.entries, e)
	if len(c.entries) > consoleMax {
		c.entries = append(c.entries[:0:0], c.entries[len(c.entries)-consoleMax:]...)
	}
	c.mu.Unlock()
	select {
	case c.out <- e:
	default:
		// The pages see the gap in Seq and read the console again.
	}
	return e
}

// resolve marks the entries about file whose message is message stale,
// as when an edit fixes what a warning warned of, and returns their Seqs.
func (c *Console) resolve(file, message string) []int {
	c.mu.Lock()
	var seqs []int
	for i := range c.entries {
		e := &c.entries[i]
		if !e.Stale && e.File == file && e.Message == message {
			e.Stale = true
			seqs = append(seqs, e.Seq)
		}
	}
	c.mu.Unlock()
	if len(seqs) > 0 {
		select {
		case c.stale <- seqs:
		default:
			// Dropped; the pages read the console again when they open.
		}
	}
	return seqs
}

// list returns the entries kept.
func (c *Console) list() []ConsoleEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ConsoleEntry(nil), c.entries...)
}

// clear forgets the entries. Seq keeps counting.
func (c *Console) clear() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// Send calls emit for each new entry, and stale with the Seqs of the
// entries that go stale, until the app ends.
func (c *Console) Send(ctx context.Context, emit func(ConsoleEntry), stale func([]int)) {
	for {
		select {
		case e := <-c.out:
			emit(e)
		case seqs := <-c.stale:
			stale(seqs)
		case <-ctx.Done():
			return
		}
	}
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	}
	return "debug"
}

// consoleHandler is a slog.Handler that adds records at info and above
// to a console, and hands every record to next as well.
type consoleHandler struct {
	next   slog.Handler
	c      *Console
	prefix string // the attributes and groups from WithAttrs and WithGroup, formatted
	group  string // the open group, as "a.b." for keys
	where  where  // the file and line from WithAttrs
}

// where is the place in a file an entry is about, and its fix.
type where struct {
	file string
	line int
	fix  string
}

// take keeps a if it is a top-level "file", "line" or "fix" attribute,
// and reports whether it did.
func (w *where) take(group string, a slog.Attr) bool {
	if group != "" {
		return false
	}
	v := a.Value.Resolve()
	switch a.Key {
	case "file":
		if v.Kind() == slog.KindString && v.String() != "" {
			w.file = v.String()
			return true
		}
	case "fix":
		if v.Kind() == slog.KindString && v.String() != "" {
			w.fix = v.String()
			return true
		}
	case "line":
		switch v.Kind() {
		case slog.KindInt64:
			w.line = int(v.Int64())
			return true
		case slog.KindUint64:
			w.line = int(v.Uint64())
			return true
		}
	}
	return false
}

// NewConsoleHandler is a handler that adds what is logged at info or
// above to c and passes everything on to next.
func NewConsoleHandler(next slog.Handler, c *Console) slog.Handler {
	return &consoleHandler{next: next, c: c}
}

func (h *consoleHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelInfo || h.next.Enabled(ctx, l)
}

// noise reports whether r is too common to show: the asset server notes
// every file a page loads.
func noise(r slog.Record) bool {
	return r.Level < slog.LevelWarn && strings.HasPrefix(r.Message, "[AssetFileServerFS] ")
}

func (h *consoleHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelInfo && !noise(r) {
		var b strings.Builder
		b.WriteString(r.Message)
		b.WriteString(h.prefix)
		w := h.where
		r.Attrs(func(a slog.Attr) bool {
			if !w.take(h.group, a) {
				writeAttr(&b, h.group, a)
			}
			return true
		})
		if w.file == "" && w.line != 0 {
			// A line in no file in particular is just text.
			writeAttr(&b, "", slog.Int("line", w.line))
			w.line = 0
		}
		if w.file == "" && w.fix != "" {
			// Nothing to fix it in.
			writeAttr(&b, "", slog.String("fix", w.fix))
			w.fix = ""
		}
		t := r.Time
		if t.IsZero() {
			t = time.Now()
		}
		h.c.add(ConsoleEntry{Time: t, Level: levelName(r.Level), Message: b.String(), File: w.file, Line: w.line, Fix: w.fix})
	}
	if h.next.Enabled(ctx, r.Level) {
		return h.next.Handle(ctx, r)
	}
	return nil
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var b strings.Builder
	b.WriteString(h.prefix)
	w := h.where
	for _, a := range attrs {
		if !w.take(h.group, a) {
			writeAttr(&b, h.group, a)
		}
	}
	return &consoleHandler{next: h.next.WithAttrs(attrs), c: h.c, prefix: b.String(), group: h.group, where: w}
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &consoleHandler{next: h.next.WithGroup(name), c: h.c, prefix: h.prefix, group: h.group + name + ".", where: h.where}
}

// writeAttr writes a as " key=value", as slog's text handler would.
func writeAttr(b *strings.Builder, group string, a slog.Attr) {
	v := a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return
	}
	if v.Kind() == slog.KindGroup {
		g := group
		if a.Key != "" {
			g += a.Key + "."
		}
		for _, ga := range v.Group() {
			writeAttr(b, g, ga)
		}
		return
	}
	b.WriteString(" ")
	b.WriteString(group)
	b.WriteString(a.Key)
	b.WriteString("=")
	s := v.String()
	if v.Kind() == slog.KindTime {
		s = v.Time().Format(time.RFC3339)
	}
	if s == "" || strings.ContainsAny(s, " \t\n\"=") {
		s = strconv.Quote(s)
	}
	b.WriteString(s)
}

// Console returns what the console holds, oldest first. New entries come
// as "console" events, a "console-stale" event names by Seq the entries
// that no longer apply, and a "console-cleared" event says it was
// cleared.
func (a *App) Console() []ConsoleEntry {
	return a.console.list()
}

// DescribeFix says what FixWarning would do for warning, a console
// entry's message, in the file this window shows.
func (a *App) DescribeFix(ctx context.Context, warning string) (notebook.Fix, error) {
	_, n, err := a.notebook(ctx)
	if err != nil {
		return notebook.Fix{}, err
	}
	return n.DescribeFix(warning)
}

// FixWarning makes the fix a console entry offers for warning, its
// message, in the file this window shows. Undo takes it back.
func (a *App) FixWarning(ctx context.Context, warning string) error {
	t, n, err := a.notebook(ctx)
	if err != nil {
		return err
	}
	err = n.FixWarning(warning)
	t.changed()
	return err
}

// ClearConsole empties the console, in every window.
func (a *App) ClearConsole() {
	a.console.clear()
	a.host.EmitAll("console-cleared", nil)
}
