// Package termstatus speaks the Program Status Protocol, OSC 7501: a
// program tells the terminal it runs in what it is doing (working,
// blocked on the user, done, failed), and the terminal shows that where
// it likes, such as in a tab, a notification or an inbox of running
// programs. mdsh uses it on both sides of the terminal: mdsh and mdsh -i
// report their own state to the terminal they run in, and the terminals
// mdsh draws itself (the History pane, tty blocks) take reports from the
// programs blocks run.
//
// The protocol is Mitchell Hashimoto's, revision 0.4 (2026-10-10):
// https://www.superlogical.com/rex/docs/build/program-status. A report
// is ESC ] 7501 ; key=value:key=value ESC \. Only state is required.
// Terminals that don't know the sequence ignore it, so a report costs
// nothing where it isn't understood.
package termstatus

import (
	"encoding/base64"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// State is what a program is doing.
type State string

const (
	// Idle is a program waiting for nothing and doing nothing, as an
	// editor at rest.
	Idle State = "idle"
	// Working is a program busy, with Progress if it knows how far
	// along it is.
	Working State = "working"
	// Done is a program that finished, and whose result nobody has
	// seen yet. The terminal keeps it after the program exits.
	Done State = "done"
	// Blocked is a program waiting on the user, and Kind says for
	// what. The terminal drops it when the program exits or the shell
	// prints its next prompt.
	Blocked State = "blocked"
	// Error is a program that failed and stopped. The terminal keeps
	// it after the program exits.
	Error State = "error"
	// Clear removes the record, and every record under it; with no ID,
	// every record the terminal has for the program.
	Clear State = "clear"
)

// Kind says what a blocked program waits for.
type Kind string

const (
	// Permission is a yes or no before something happens, as a confirm
	// block asks.
	Permission Kind = "permission"
	// Question is an answer typed or picked, as mdsh ctl ask asks.
	Question Kind = "question"
	// Auth is a credential: a password, a token, a login.
	Auth Kind = "auth"
)

// The protocol's limits. A report over any of them is thrown away whole
// by the terminal, so Sequence cuts a message to fit.
const (
	// MaxSequence is the length of a whole report, OSC to ST.
	MaxSequence = 4096
	// MaxMsg and MaxTitle are the decoded lengths of msg and title, in
	// bytes.
	MaxMsg   = 2048
	MaxTitle = 192
	// MaxApp is the length of an app name, and of one segment of an id.
	MaxApp = 32
	// MaxID is the length of a whole id, and MaxDepth how many segments
	// it may have.
	MaxID    = 128
	MaxDepth = 8
	// MaxRecords is how many records a terminal must keep at least;
	// past its own cap it drops the one updated longest ago.
	MaxRecords = 64
)

// NoProgress is Report.Progress for a program that doesn't know how far
// along it is.
const NoProgress = -1

// Report is one report: the whole state of the record ID names. Each
// report replaces the record, so a field left out is gone from it.
type Report struct {
	// ID names the record, as a path: "" is the root, the program
	// itself; "build/test" is a record under "build". Segments are
	// letters, digits and _ . + -, at most MaxApp bytes each, MaxDepth
	// deep and MaxID in all.
	ID string
	// State is required.
	State State
	// Kind says what a Blocked program waits for; it is dropped from
	// other states.
	Kind Kind
	// Progress is how far along a Working or Blocked program is, 0 to
	// 100, or NoProgress; it is dropped from other states.
	Progress int
	// App is a stable name for the program, such as "mdsh" or "cargo",
	// for the terminal to group and filter by and pick an icon for. It
	// is letters, digits and _ . + -, at most MaxApp bytes. A record
	// under one with an App inherits it.
	App string
	// Title is a short label, at most MaxTitle bytes.
	Title string
	// Msg is one line on what is happening or what is wanted, at most
	// MaxMsg bytes. The terminal may shorten it and reads nothing into
	// it.
	Msg string
}

// Query asks the terminal whether it speaks the protocol. One that does
// answers with the same sequence, listing the states and kinds it knows;
// one that doesn't answers nothing, so a program that asks should send
// CSI c right after and give up on the first reply.
const Query = "\x1b]7501;?\x1b\\"

// Sequence is r as the bytes to write to the terminal, or "" when r has
// no State, or an ID or App the protocol doesn't allow. Title and Msg
// are cut to fit, at a character boundary, and their control
// characters, which the terminal would refuse the whole report for,
// become spaces.
func (r Report) Sequence() string {
	if r.State == "" || !validID(r.ID) || (r.App != "" && !validName(r.App)) {
		return ""
	}
	var b strings.Builder
	b.WriteString("\x1b]7501;state=")
	b.WriteString(string(r.State))
	if r.ID != "" {
		b.WriteString(":id=")
		b.WriteString(r.ID)
	}
	if r.State == Blocked && r.Kind != "" {
		b.WriteString(":kind=")
		b.WriteString(string(r.Kind))
	}
	if (r.State == Working || r.State == Blocked) && r.Progress >= 0 {
		b.WriteString(":progress=")
		b.WriteString(strconv.Itoa(min(r.Progress, 100)))
	}
	if r.App != "" {
		b.WriteString(":app=")
		b.WriteString(r.App)
	}
	if t := line(r.Title, MaxTitle); t != "" {
		b.WriteString(":title=")
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(t)))
	}
	if m := line(r.Msg, MaxMsg); m != "" {
		b.WriteString(":msg=")
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(m)))
	}
	b.WriteString("\x1b\\")
	return b.String()
}

// line is s as one line of at most n bytes: control characters become
// spaces, runs of them one space, and the end is cut at a character.
func line(s string, n int) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for _, r := range s {
		if unicode.IsControl(r) || r == utf8.RuneError || unicode.Is(unicode.Cf, r) || r == ' ' || r == ' ' {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if len(out) > n {
		out = out[:n]
		for !utf8.ValidString(out) {
			out = out[:len(out)-1]
		}
		out = strings.TrimRight(out, " ")
	}
	return out
}

// validName reports whether s is an app name or an id segment.
func validName(s string) bool {
	if s == "" || len(s) > MaxApp {
		return false
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == '+', c == '-':
		default:
			return false
		}
	}
	return true
}

// validID reports whether id is a record path: "" or segments joined
// by /.
func validID(id string) bool {
	if id == "" {
		return true
	}
	if len(id) > MaxID {
		return false
	}
	segs := strings.Split(id, "/")
	if len(segs) > MaxDepth {
		return false
	}
	for _, s := range segs {
		if !validName(s) {
			return false
		}
	}
	return true
}

// Name makes s fit an id segment or an app name: anything but letters,
// digits and _ . + - becomes _, and it is cut to MaxApp bytes. An empty
// s is "_".
func Name(s string) string {
	var b strings.Builder
	for i := 0; i < len(s) && b.Len() < MaxApp; i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '.', c == '+', c == '-':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

// Tracker is a program's own state, the root record, reported to the
// terminal it runs in as it changes. Its methods may be called from any
// goroutine, and on a nil Tracker, or one made with a nil writer, they
// do nothing: a program with no terminal keeps the same calls.
type Tracker struct {
	w   io.Writer
	app string

	mu      sync.Mutex
	last    string // the sequence last written, so the same state isn't sent twice
	state   State
	working Report // the last Working report, for Unblocked
	// children are the child records reported, by id, each the
	// sequence last written for it.
	children map[string]string
}

// NewTracker reports to w, the program's terminal, as app. A nil w
// reports nothing.
func NewTracker(w io.Writer, app string) *Tracker {
	if w == nil {
		return nil
	}
	return &Tracker{w: w, app: app}
}

// report writes r, with the tracker's app, unless it was the last thing
// written.
func (t *Tracker) report(r Report) {
	r.App = t.app
	seq := r.Sequence()
	if seq == "" || seq == t.last {
		return
	}
	t.last = seq
	t.state = r.State
	io.WriteString(t.w, seq)
}

// Working says the program is busy with msg, progress percent of the
// way along, or NoProgress.
func (t *Tracker) Working(progress int, msg string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.working = Report{State: Working, Progress: progress, Msg: msg}
	t.report(t.working)
}

// Blocked says the program waits on the user for kind, msg saying what
// for. Unblocked goes back to what it was doing.
func (t *Tracker) Blocked(kind Kind, msg string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	r := Report{State: Blocked, Kind: kind, Msg: msg, Progress: NoProgress}
	if t.working.State == Working {
		r.Progress = t.working.Progress
	}
	t.report(r)
}

// Unblocked says the user answered: the program is working again, as it
// last reported before Blocked. Before any Working, it is Idle.
func (t *Tracker) Unblocked() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != Blocked {
		return
	}
	if t.working.State == Working {
		t.report(t.working)
	} else {
		t.report(Report{State: Idle})
	}
}

// Done says the program finished, msg saying how. The terminal keeps it
// after the program exits, until the user sees it.
func (t *Tracker) Done(msg string) { t.end(Done, msg) }

// Error says the program failed and stopped, msg saying how. The
// terminal keeps it after the program exits.
func (t *Tracker) Error(msg string) { t.end(Error, msg) }

func (t *Tracker) end(s State, msg string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.working = Report{}
	t.report(Report{State: s, Msg: msg})
}

// Idle says the program waits for nothing: the user is back, or there
// is nothing to do.
func (t *Tracker) Idle() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.working = Report{}
	t.report(Report{State: Idle})
}

// Settled reports whether the last report was Done or Error: the result
// is on show, and a sign of life from the user means it has been seen.
func (t *Tracker) Settled() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state == Done || t.state == Error
}

// Clear removes the program's records from the terminal, as when it
// quits with nothing to show.
func (t *Tracker) Clear() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.working = Report{}
	t.children = nil
	t.report(Report{State: Clear})
}

// Child reports r as a record under the program's own, id naming it:
// what a program the tracker's program runs is doing, as a block's
// program under mdsh. r's ID, if any, goes under id; its App is kept, so
// the terminal can tell whose record it is, and with none the terminal
// takes the tracker's. ClearChild takes the record back.
func (t *Tracker) Child(id string, r Report) {
	if t == nil || id == "" {
		return
	}
	if r.ID != "" {
		id += "/" + r.ID
	}
	r.ID = id
	seq := r.Sequence()
	if seq == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.children == nil {
		t.children = map[string]string{}
	}
	if t.children[id] == seq {
		return
	}
	t.children[id] = seq
	io.WriteString(t.w, seq)
}

// ClearChild removes the record id names, and those under it, as Child
// made them.
func (t *Tracker) ClearChild(id string) {
	if t == nil || id == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	had := false
	for k := range t.children {
		if k == id || strings.HasPrefix(k, id+"/") {
			delete(t.children, k)
			had = true
		}
	}
	if had {
		io.WriteString(t.w, Report{ID: id, State: Clear}.Sequence())
	}
}
