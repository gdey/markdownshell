// Package termnotify asks the terminal to show a desktop notification.
// Terminals never agreed on one sequence, so it picks one by what the
// terminal is: OSC 99 for kitty, OSC 9 for iTerm2, WezTerm and Ghostty,
// OSC 777 notify for foot, and a bell everywhere else, which most
// terminals turn into a badge, a bounce of the dock icon or a flash.
// Inside tmux the sequence is wrapped so tmux passes it on to the
// terminal it runs in, when tmux's allow-passthrough option lets it.
package termnotify

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Protocol is a way to ask for a notification.
type Protocol int

const (
	// Bell rings the terminal's bell, BEL.
	Bell Protocol = iota
	// OSC9 is iTerm2's OSC 9 ; text, which WezTerm and Ghostty take
	// too.
	OSC9
	// OSC99 is kitty's OSC 99, with a title and a body.
	OSC99
	// OSC777 is urxvt's OSC 777 ; notify ; title ; body, which foot
	// takes.
	OSC777
	// Off sends nothing.
	Off
)

func (p Protocol) String() string {
	switch p {
	case Bell:
		return "bell"
	case OSC9:
		return "osc9"
	case OSC99:
		return "osc99"
	case OSC777:
		return "osc777"
	case Off:
		return "off"
	}
	return "unknown"
}

// Parse reads a protocol's name, as MDSH_NOTIFY gives it.
func Parse(s string) (Protocol, bool) {
	for p := Bell; p <= Off; p++ {
		if strings.EqualFold(s, p.String()) {
			return p, true
		}
	}
	return Bell, false
}

// Detect picks the protocol for the terminal env describes. MDSH_NOTIFY
// overrides it: off, bell, osc9, osc99 or osc777. Inside tmux, where
// TERM and TERM_PROGRAM name tmux, use DetectTmux.
func Detect(env func(string) string) Protocol {
	if p, ok := Parse(env("MDSH_NOTIFY")); ok {
		return p
	}
	term, prog := env("TERM"), env("TERM_PROGRAM")
	switch {
	case env("KITTY_WINDOW_ID") != "" || term == "xterm-kitty":
		return OSC99
	case prog == "iTerm.app" || prog == "WezTerm" || prog == "ghostty" ||
		term == "xterm-ghostty" || env("LC_TERMINAL") == "iTerm2":
		return OSC9
	case prog == "foot" || strings.HasPrefix(term, "foot"):
		return OSC777
	}
	return Bell
}

// Tmux is what tmux says of the terminal its client runs in, and of the
// pane mdsh runs in.
type Tmux struct {
	// TermName is the client's TERM, such as xterm-kitty, and TermType
	// the terminal's own name and version, from XTVERSION, such as
	// "iTerm2 3.5.4" (tmux 3.3 and later).
	TermName, TermType string
	// Passthrough is the pane's allow-passthrough option: off, on or
	// all.
	Passthrough string
}

// DetectTmux picks the protocol inside tmux, from what tmux says of the
// terminal outside it. A notification needs passthrough; without it, a
// bell, which tmux marks on the window and passes on as its bell-action
// says.
func DetectTmux(env func(string) string, t Tmux) Protocol {
	if p, ok := Parse(env("MDSH_NOTIFY")); ok {
		return p
	}
	if t.Passthrough != "on" && t.Passthrough != "all" {
		return Bell
	}
	kind := strings.ToLower(t.TermType)
	switch {
	case t.TermName == "xterm-kitty" || strings.HasPrefix(kind, "kitty"):
		return OSC99
	case strings.HasPrefix(kind, "iterm2") || strings.HasPrefix(kind, "wezterm") ||
		strings.HasPrefix(kind, "ghostty") || t.TermName == "xterm-ghostty":
		return OSC9
	case strings.HasPrefix(kind, "foot") || strings.HasPrefix(t.TermName, "foot"):
		return OSC777
	}
	// tmux passes on the environment of the client that started its
	// server, so this can still name the terminal.
	return Detect(env)
}

// AskTmux asks the tmux server mdsh runs under about its client and
// pane. It gives the zero Tmux, which DetectTmux takes as no
// passthrough, if tmux doesn't answer within half a second.
func AskTmux(env func(string) string) Tmux {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	args := []string{"display-message", "-p"}
	if pane := env("TMUX_PANE"); pane != "" {
		args = append(args, "-t", pane)
	}
	args = append(args, "#{client_termname}\t#{client_termtype}\t#{allow-passthrough}")
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	if err != nil {
		return Tmux{}
	}
	f := strings.Split(strings.TrimRight(string(out), "\n"), "\t")
	if len(f) != 3 {
		return Tmux{}
	}
	return Tmux{TermName: f[0], TermType: f[1], Passthrough: f[2]}
}

// The longest title and body sent, in runes. Notifications show little
// more, and the protocols' own limits are larger.
const (
	MaxTitle = 64
	MaxBody  = 256
)

// Sequence is the escape sequence that asks, by p, for a notification
// with title and body. Control characters become spaces, and both are
// cut to MaxTitle and MaxBody. For Bell it is BEL, and for Off empty.
func Sequence(p Protocol, title, body string) string {
	title, body = clean(title, MaxTitle), clean(body, MaxBody)
	switch p {
	case Bell:
		return "\a"
	case OSC9:
		text := body
		if title != "" {
			text = title + ": " + body
		}
		// A ConEmu-aware terminal, such as Windows Terminal or
		// Ghostty, reads OSC 9 ; 4 ; ... as progress, and the like for
		// every number: never start with a digit.
		if text != "" && text[0] >= '0' && text[0] <= '9' {
			text = " " + text
		}
		return "\x1b]9;" + text + "\x1b\\"
	case OSC99:
		// Title then body, as one notification (i=), with the payloads
		// in base64 (e=1) so nothing in them is read as the sequence's.
		// o=unfocused: kitty shows nothing while its window has focus.
		const id = "mdsh"
		b64 := base64.StdEncoding.EncodeToString
		return "\x1b]99;i=" + id + ":d=0:o=unfocused:e=1:p=title;" + b64([]byte(title)) + "\x1b\\" +
			"\x1b]99;i=" + id + ":d=1:e=1:p=body;" + b64([]byte(body)) + "\x1b\\"
	case OSC777:
		// The title ends at the first semicolon.
		return "\x1b]777;notify;" + strings.ReplaceAll(title, ";", ",") + ";" + body + "\x1b\\"
	}
	return ""
}

// Passthrough wraps seq for tmux to pass on to the terminal outside it:
// DCS tmux ; seq ST, with each ESC in seq doubled.
func Passthrough(seq string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// clean turns control characters into spaces, and cuts s to max runes,
// marking the cut with an ellipsis.
func clean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// Notifier sends notifications to a terminal, and keeps the time of the
// user's last key, so a caller can notify only someone who has looked
// away. Its methods are safe to call from any goroutine, and on a nil
// Notifier do nothing.
type Notifier struct {
	w    io.Writer
	p    Protocol
	tmux bool
	now  func() time.Time

	mu   sync.Mutex
	seen time.Time
}

// New sends to w, the terminal, by p; with tmux set, wrapped for tmux to
// pass on. A nil w or Off gives a nil Notifier.
func New(w io.Writer, p Protocol, tmux bool) *Notifier {
	if w == nil || p == Off {
		return nil
	}
	n := &Notifier{w: w, p: p, tmux: tmux, now: time.Now}
	n.seen = n.now()
	return n
}

// ForTerminal is New for w, mdsh's own terminal, detected from the
// process's environment, and from tmux when it runs under tmux.
func ForTerminal(w io.Writer) *Notifier {
	if os.Getenv("TMUX") == "" {
		return New(w, Detect(os.Getenv), false)
	}
	return New(w, DetectTmux(os.Getenv, AskTmux(os.Getenv)), true)
}

// Protocol is what n sends by: Off for a nil Notifier.
func (n *Notifier) Protocol() Protocol {
	if n == nil {
		return Off
	}
	return n.p
}

// Seen notes a key from the user.
func (n *Notifier) Seen() {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.seen = n.now()
	n.mu.Unlock()
}

// Idle is how long since the user's last key, or since n was made.
func (n *Notifier) Idle() time.Duration {
	if n == nil {
		return 0
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.now().Sub(n.seen)
}

// Notify asks the terminal to show title and body.
func (n *Notifier) Notify(title, body string) {
	if n == nil {
		return
	}
	seq := Sequence(n.p, title, body)
	// tmux handles a bell itself: it marks the window and passes the
	// bell on.
	if n.tmux && n.p != Bell {
		seq = Passthrough(seq)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	io.WriteString(n.w, seq)
}
