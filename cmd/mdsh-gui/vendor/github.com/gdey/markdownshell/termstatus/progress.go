package termstatus

import (
	"io"
	"strconv"
	"time"
)

// Two older signals follow a Tracker's state, for the terminals that
// don't speak OSC 7501 yet:
//
//   - a progress bar, ConEmu's OSC 9;4, which Ghostty, Windows Terminal,
//     VS Code and ConEmu show on the tab or the taskbar. iTerm2, WezTerm
//     and others read any OSC 9 as a notification, so it is sent only
//     where ProgressTerminal says it is understood.
//   - the window title, OSC 0: the file's name, marked while it runs,
//     waits or has failed. Every terminal knows it. The title the
//     terminal had is pushed on its title stack first (CSI 22;0 t) and
//     popped when the program exits (CSI 23;0 t); a terminal without
//     the stack keeps mdsh's title until something else sets one.

// The progress bar's states, OSC 9 ; 4 ; state ; percent.
const (
	progressHide          = 0
	progressNormal        = 1
	progressError         = 2
	progressIndeterminate = 3
	progressPaused        = 4
)

// RefreshEvery is how often the progress bar is sent again while the
// program works or waits: Ghostty hides a bar it hasn't heard of for 15
// seconds.
const RefreshEvery = 10 * time.Second

// ProgressTerminal reports whether the terminal the environment
// describes shows OSC 9;4 progress rather than reading it as a
// notification.
func ProgressTerminal(getenv func(string) string) bool {
	switch getenv("TERM_PROGRAM") {
	case "ghostty", "vscode":
		return true
	case "tmux":
		// tmux drops an OSC it doesn't know, and the variables below
		// are the outer terminal's, from before tmux started.
		return false
	}
	return getenv("TERM") == "xterm-ghostty" || getenv("WT_SESSION") != "" || getenv("ConEmuPID") != ""
}

// ProgressSequence is r as an OSC 9;4 progress bar: working with
// progress is a bar at that percent, working without is the busy bar,
// blocked is the paused (Windows Terminal: warning) bar, error is a
// full red one, and anything else hides the bar.
func ProgressSequence(r Report) string {
	state, pct := progressHide, 0
	switch r.State {
	case Working:
		state, pct = progressNormal, r.Progress
		if r.Progress < 0 {
			state, pct = progressIndeterminate, 0
		}
	case Blocked:
		state, pct = progressPaused, max(r.Progress, 0)
	case Error:
		state, pct = progressError, 100
	}
	return "\x1b]9;4;" + strconv.Itoa(state) + ";" + strconv.Itoa(min(pct, 100)) + "\x1b\\"
}

// TitleText is the window title for the file name while the program,
// app, is in r's state: "[42%] deploy.md - mdsh" while working,
// "[waiting]" while blocked, "[failed]" after an error, "[done]" until
// the result is seen. With no name it is the app alone.
func TitleText(app, name string, r Report) string {
	t := app
	if name != "" {
		t = name + " - " + app
	}
	switch r.State {
	case Working:
		if r.Progress >= 0 {
			return "[" + strconv.Itoa(min(r.Progress, 100)) + "%] " + t
		}
		return "[running] " + t
	case Blocked:
		return "[waiting] " + t
	case Error:
		return "[failed] " + t
	case Done:
		return "[done] " + t
	}
	return t
}

// TitleSequence is the OSC 0 that sets the window title and icon name
// to title, made one line without control characters, at most MaxTitle
// bytes.
func TitleSequence(title string) string {
	return "\x1b]0;" + line(title, MaxTitle) + "\x1b\\"
}

const (
	pushTitle = "\x1b[22;0t"
	popTitle  = "\x1b[23;0t"
)

// WithProgress has t also show a progress bar (OSC 9;4) when on, as
// ProgressTerminal says for the program's terminal. It returns t, and
// is called before any report.
func (t *Tracker) WithProgress(on bool) *Tracker {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress = on
	if t.refreshEvery == 0 {
		t.refreshEvery = RefreshEvery
	}
	return t
}

// Title has the window title follow t's state, for the file name, or
// for the program alone when name is "". The first call saves the
// terminal's own title, which Exit and Clear put back.
func (t *Tracker) Title(name string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.titled {
		t.titled = true
		io.WriteString(t.w, pushTitle)
	}
	t.titleName = name
	t.follow()
}

// Exit drops what lasts only while the program runs: the progress bar
// goes and the terminal's own title comes back. The OSC 7501 record
// stays, as the protocol keeps done and error after the program ends.
func (t *Tracker) Exit() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.exit()
}

func (t *Tracker) exit() {
	if t.refresh != nil {
		t.refresh.Stop()
		t.refresh = nil
	}
	if t.progress && t.lastProgress != "" {
		if hide := ProgressSequence(Report{State: Idle}); t.lastProgress != hide {
			io.WriteString(t.w, hide)
		}
		t.lastProgress = ""
	}
	if t.titled {
		io.WriteString(t.w, popTitle)
		t.titled, t.lastTitle = false, ""
	}
}

// follow brings the progress bar and title in line with t.cur, writing
// only what changed. t.mu is held.
func (t *Tracker) follow() {
	if t.progress && t.cur.State != "" {
		// A bar never shown needn't be hidden.
		seq := ProgressSequence(t.cur)
		if seq != t.lastProgress && (t.lastProgress != "" || seq != ProgressSequence(Report{State: Idle})) {
			t.lastProgress = seq
			io.WriteString(t.w, seq)
		}
		t.keepProgress()
	}
	if t.titled {
		if seq := TitleSequence(TitleText(t.app, t.titleName, t.cur)); seq != t.lastTitle {
			t.lastTitle = seq
			io.WriteString(t.w, seq)
		}
	}
}

// keepProgress sends the bar again every refreshEvery while the program
// works or waits, so a terminal that times bars out keeps showing it.
// t.mu is held.
func (t *Tracker) keepProgress() {
	live := t.cur.State == Working || t.cur.State == Blocked
	switch {
	case live && t.refresh == nil:
		t.refresh = time.AfterFunc(t.refreshEvery, t.resend)
	case !live && t.refresh != nil:
		t.refresh.Stop()
		t.refresh = nil
	}
}

func (t *Tracker) resend() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.refresh == nil {
		return // stopped since
	}
	t.refresh = nil
	if t.cur.State == Working || t.cur.State == Blocked {
		io.WriteString(t.w, t.lastProgress)
		t.keepProgress()
	}
}
