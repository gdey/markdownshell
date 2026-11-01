package main

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The console keeps what the app logs, errors from calls the frontend
// makes included, for View › Console to show. Everything logged through
// the Wails logger or slog's default logger at info or above lands here,
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

// ConsoleEntry is one line of the console.
type ConsoleEntry struct {
	Seq     int       `json:"seq"` // counts up from 1, so a page can tell it missed one
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`        // the message, then its attributes as key=value
	File    string    `json:"file,omitempty"` // the file the entry is about, from a "file" attribute
	Line    int       `json:"line,omitempty"` // the line in File, from a "line" attribute; 0 if none
}

// consoleMax is how many entries the console keeps; older ones go.
const consoleMax = 1000

// console is the log the Console pane shows.
type console struct {
	mu      sync.Mutex
	entries []ConsoleEntry
	seq     int
	// out is where new entries go to be sent to the windows, by a
	// goroutine of their own: an entry can be logged from deep inside
	// Wails, where sending an event then and there could deadlock.
	out chan ConsoleEntry
}

func newConsole() *console {
	return &console{out: make(chan ConsoleEntry, 256)}
}

// add adds e, numbering it.
func (c *console) add(e ConsoleEntry) ConsoleEntry {
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

// list returns the entries kept.
func (c *console) list() []ConsoleEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]ConsoleEntry(nil), c.entries...)
}

// clear forgets the entries. Seq keeps counting.
func (c *console) clear() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// send calls emit for each new entry, until the app ends.
func (c *console) send(ctx context.Context, emit func(ConsoleEntry)) {
	for {
		select {
		case e := <-c.out:
			emit(e)
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
	c      *console
	prefix string // the attributes and groups from WithAttrs and WithGroup, formatted
	group  string // the open group, as "a.b." for keys
	where  where  // the file and line from WithAttrs
}

// where is the place in a file an entry is about.
type where struct {
	file string
	line int
}

// take keeps a if it is a top-level "file" or "line" attribute, and
// reports whether it did.
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

func newConsoleHandler(next slog.Handler, c *console) *consoleHandler {
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
		t := r.Time
		if t.IsZero() {
			t = time.Now()
		}
		h.c.add(ConsoleEntry{Time: t, Level: levelName(r.Level), Message: b.String(), File: w.file, Line: w.line})
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
// as "console" events, and a "console-cleared" event says it was cleared.
func (a *App) Console() []ConsoleEntry {
	return a.console.list()
}

// ClearConsole empties the console, in every window.
func (a *App) ClearConsole() {
	a.console.clear()
	a.app.Event.Emit("console-cleared")
}
