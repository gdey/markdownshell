package notebook

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/audit"
)

// AuditEvent is one entry of a file's event log, as the Events pane
// shows it: what the shells, the file and the blocks run in it did, and
// why.
type AuditEvent struct {
	// Seq counts up from 1, so a page can tell it missed one.
	Seq int `json:"seq"`
	// Time is when it happened, RFC 3339 with milliseconds, in UTC.
	Time string `json:"time"`
	// Event names it, such as session.start or block.end; see
	// docs/audit.md.
	Event string `json:"event"`
	// Text says what happened, in words.
	Text string `json:"text"`
	// Lost is set when what blocks set in a shell, its variables,
	// functions and aliases, went with it.
	Lost bool `json:"lost,omitempty"`
	// JSON is the event as the audit log writes it, one line.
	JSON string `json:"json"`
}

// eventsMax is how many events a log keeps; older ones go.
const eventsMax = 2000

// EventLog is a file's event log, kept by its tab so that it outlives
// the notebook, as when the file is reloaded. It writes in mdsh
// --audit's format (see package audit) and keeps the last eventsMax
// events for the Events pane. A nil *EventLog logs nothing.
type EventLog struct {
	log    *audit.Log
	notify func(AuditEvent)

	mu     sync.Mutex
	events []AuditEvent
}

// NewEventLog returns an empty log. notify, if set, is called with each
// new event; it must not block or call back into the log.
func NewEventLog(notify func(AuditEvent)) *EventLog {
	l := &EventLog{notify: notify}
	l.log = audit.New(eventWriter{l})
	return l
}

// Audit is the log for run.Options.Audit.
func (l *EventLog) Audit() *audit.Log {
	if l == nil {
		return nil
	}
	return l.log
}

// Event logs one event.
func (l *EventLog) Event(event string, f audit.Fields) {
	if l != nil {
		l.log.Event(event, f)
	}
}

// List returns the events kept, oldest first.
func (l *EventLog) List() []AuditEvent {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]AuditEvent(nil), l.events...)
}

// eventWriter takes the lines audit.Log writes, one per Write.
type eventWriter struct{ l *EventLog }

func (w eventWriter) Write(p []byte) (int, error) {
	line := strings.TrimSuffix(string(p), "\n")
	var f map[string]any
	if err := json.Unmarshal([]byte(line), &f); err != nil {
		// audit.Log writes JSON; anything else isn't an event.
		return len(p), nil
	}
	e := AuditEvent{JSON: line}
	if n, ok := f["seq"].(float64); ok {
		e.Seq = int(n)
	}
	e.Time, _ = f["t"].(string)
	e.Event, _ = f["event"].(string)
	for _, k := range []string{"v", "seq", "t", "event"} {
		delete(f, k)
	}
	e.Text, e.Lost = Describe(e.Event, f)
	w.l.mu.Lock()
	w.l.events = append(w.l.events, e)
	if len(w.l.events) > eventsMax {
		w.l.events = append(w.l.events[:0:0], w.l.events[len(w.l.events)-eventsMax:]...)
	}
	w.l.mu.Unlock()
	if w.l.notify != nil {
		w.l.notify(e)
	}
	return len(p), nil
}

// Why a notebook's shells start or end, for session.start and
// session.end.
const (
	whyOpen       = "open"
	whyReload     = "reload"
	whyRestart    = "restart"
	whyRunOptions = "run options"
	whyHistory    = "history pane"
	whySaveAs     = "save as"
	whyTTY        = "tty block"
	whyClose      = "close"
)

// reasons says each reason in words.
var reasons = map[string]string{
	whyOpen:       "the file was opened",
	whyReload:     "the file was reloaded from disk",
	whyRestart:    "Restart Shells",
	whyRunOptions: "the shell or the files the shells source changed",
	whyHistory:    "the History pane needs shells on a terminal",
	whySaveAs:     "Save As moved the file to another folder",
	whyTTY:        "the first tty block needs shells on a terminal",
	whyClose:      "the file was closed",
}

// Reason says why, a session.start or session.end reason, in words.
func Reason(why string) string {
	if r, ok := reasons[why]; ok {
		return r
	}
	return why
}

// Describe says what an audit event, with its fields besides v, seq, t
// and event, means, and whether what blocks set in a shell was lost
// with it.
func Describe(event string, f map[string]any) (text string, lost bool) {
	str := func(k string) string {
		switch v := f[k].(type) {
		case string:
			return v
		case nil:
			return ""
		case float64:
			return fmt.Sprint(v)
		default:
			b, _ := json.Marshal(v)
			return string(b)
		}
	}
	envName := func() string {
		if e := str("env"); e != "" {
			return "env " + e
		}
		return "the top level"
	}
	block := func() string {
		if b := str("block"); b != "" {
			return b
		}
		return "a block"
	}
	switch event {
	case "session.start":
		switch {
		case str("reason") == whyOpen:
			return "Shells ready: " + Reason(whyOpen) + ". Each starts with its first block.", false
		case f["lost"] == true:
			return "Shells started again: " + Reason(str("reason")) + ". What blocks set in the old ones is gone.", true
		}
		return "Shells started again: " + Reason(str("reason")) + ". No block had run in them yet.", false
	case "restart.needed":
		return "Shells kept: " + Reason(str("reason")) + ", which takes Restart Shells. Until you restart them, the shells and what blocks set in them stay as they are.", false
	case "args":
		return "Set the arguments, $1 and on, in the shells as they are.", false
	case "outputs.clear":
		return "Cleared all outputs. The shells keep what blocks set in them.", false
	case "session.end":
		return "Shells ended: " + Reason(str("reason")) + ".", false
	case "file.changed":
		return "The file changed on disk.", false
	case "file.reload":
		return "Reloaded the file from disk.", false
	case "file.save":
		return "Saved " + str("file") + ".", false
	case "env.start":
		if f["again"] == true {
			return "A new shell started for " + envName() + ", without what blocks set in the last one.", false
		}
		return "Shell started for " + envName() + ".", false
	case "env.end":
		switch str("reason") {
		case "timeout":
			return fmt.Sprintf("Block %s timed out, which ended the shell for %s. The next block there starts a new one.", block(), envName()), true
		case "stopped":
			return fmt.Sprintf("Stopping block %s ended the shell for %s. The next block there starts a new one.", block(), envName()), true
		case "killed":
			return fmt.Sprintf("You killed the shell for %s, as block %s wouldn't stop. The next block there starts a new one.", envName(), block()), true
		case "exit":
			return fmt.Sprintf("Block %s ended the shell for %s (status %s). The next block there starts a new one.", block(), envName(), str("exit")), true
		}
		return fmt.Sprintf("The shell for %s had ended (status %s), not by a block's run. A new one starts in its place.", envName(), str("exit")), true
	case "block.start":
		return "Block " + block() + " started.", false
	case "block.end":
		t := fmt.Sprintf("Block %s ended: status %s, %sms.", block(), str("exit"), str("ms"))
		switch e := str("error"); {
		case e == "context canceled" && f["exited"] != true:
			return fmt.Sprintf("Block %s was stopped (%sms). Its shell, and what blocks set there, stay.", block(), str("ms")), false
		case strings.HasPrefix(e, "timed out") && f["exited"] != true:
			return fmt.Sprintf("Block %s %s and was interrupted. Its shell, and what blocks set there, stay.", block(), e), false
		}
		if e := str("error"); e != "" {
			t += " " + e
		}
		return t, false
	case "block.skip":
		return fmt.Sprintf("Block %s didn't run (%s).", block(), str("reason")), false
	case "ctl":
		t := "mdsh ctl " + str("cmd")
		if b := str("block"); b != "" {
			t += " from block " + b
		}
		if e := str("error"); e != "" {
			return t + ": " + e, false
		}
		return t + ".", false
	case "var":
		return "Variable " + str("name") + " changed by mdsh ctl.", false
	}
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(event)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%s", k, str(k))
	}
	return b.String(), false
}
