package main

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

// consoleEntry is one line of the app's Console pane, as mdsh-gui's
// ConsoleEntry. A "file" and a "line" attribute say where in a file the
// entry is about, rather than being part of its message.
type consoleEntry struct {
	Seq     int       `json:"seq"`
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	File    string    `json:"file,omitempty"`
	Line    int       `json:"line,omitempty"`
	// Fix names the fix for a warning about a file, from a "fix"
	// attribute, as notebook.FixWarning makes it.
	Fix string `json:"fix,omitempty"`
}

// staleEntry says a warning about file no longer applies: an edit fixed
// it. The app strikes out the entries with that file and message.
type staleEntry struct {
	File    string `json:"file"`
	Message string `json:"message"`
}

// consoleHandler sends what is logged at info and above to the app as
// "console" events, and passes every record on to next.
type consoleHandler struct {
	c      *conn
	next   slog.Handler
	seq    *seqCounter
	prefix string // attributes from WithAttrs, written out
	file   string
	line   int
	fix    string
}

type seqCounter struct {
	mu sync.Mutex
	n  int
}

func (s *seqCounter) next() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n++
	return s.n
}

func newConsoleHandler(c *conn, next slog.Handler) *consoleHandler {
	return &consoleHandler{c: c, next: next, seq: &seqCounter{}}
}

func (h *consoleHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelInfo || h.next.Enabled(ctx, l)
}

func (h *consoleHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelInfo {
		var b strings.Builder
		b.WriteString(r.Message)
		b.WriteString(h.prefix)
		file, line, fix := h.file, h.line, h.fix
		r.Attrs(func(a slog.Attr) bool {
			if !take(a, &file, &line, &fix) {
				writeAttr(&b, a)
			}
			return true
		})
		if file == "" && fix != "" {
			// Nothing to fix it in.
			writeAttr(&b, slog.String("fix", fix))
			fix = ""
		}
		t := r.Time
		if t.IsZero() {
			t = time.Now()
		}
		h.c.send(event{Event: "console", Data: consoleEntry{Seq: h.seq.next(), Time: t, Level: levelName(r.Level), Message: b.String(), File: file, Line: line, Fix: fix}})
	}
	if h.next.Enabled(ctx, r.Level) {
		return h.next.Handle(ctx, r)
	}
	return nil
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := *h
	n.next = h.next.WithAttrs(attrs)
	var b strings.Builder
	b.WriteString(h.prefix)
	for _, a := range attrs {
		if !take(a, &n.file, &n.line, &n.fix) {
			writeAttr(&b, a)
		}
	}
	n.prefix = b.String()
	return &n
}

// WithGroup is passed on; the console doesn't show groups.
func (h *consoleHandler) WithGroup(name string) slog.Handler {
	n := *h
	n.next = h.next.WithGroup(name)
	return &n
}

// take keeps a "file", "line" or "fix" attribute in file, line or fix.
func take(a slog.Attr, file *string, line *int, fix *string) bool {
	v := a.Value.Resolve()
	switch {
	case a.Key == "fix" && v.Kind() == slog.KindString && v.String() != "":
		*fix = v.String()
		return true
	case a.Key == "file" && v.Kind() == slog.KindString:
		*file = v.String()
		return true
	case a.Key == "line" && v.Kind() == slog.KindInt64:
		*line = int(v.Int64())
		return true
	}
	return false
}

// writeAttr writes a as " key=value", as slog's text handler would.
func writeAttr(b *strings.Builder, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	s := a.Value.Resolve().String()
	if s == "" || strings.ContainsAny(s, " \t\n\"=") {
		s = strconv.Quote(s)
	}
	b.WriteString(" " + a.Key + "=" + s)
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	default:
		return "info"
	}
}
