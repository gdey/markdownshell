package termnotify

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func envOf(kv ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return func(k string) string { return m[k] }
}

func TestDetect(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want Protocol
	}{
		{"kitty by window", []string{"KITTY_WINDOW_ID", "1", "TERM", "xterm-256color"}, OSC99},
		{"kitty by TERM", []string{"TERM", "xterm-kitty"}, OSC99},
		{"iTerm2", []string{"TERM_PROGRAM", "iTerm.app"}, OSC9},
		{"iTerm2 over ssh", []string{"LC_TERMINAL", "iTerm2"}, OSC9},
		{"WezTerm", []string{"TERM_PROGRAM", "WezTerm"}, OSC9},
		{"Ghostty", []string{"TERM", "xterm-ghostty"}, OSC9},
		{"foot", []string{"TERM", "foot-extra"}, OSC777},
		{"Terminal.app", []string{"TERM_PROGRAM", "Apple_Terminal"}, Bell},
		{"unknown", nil, Bell},
		{"override", []string{"TERM", "xterm-kitty", "MDSH_NOTIFY", "off"}, Off},
		{"override case", []string{"MDSH_NOTIFY", "OSC777"}, OSC777},
		{"bad override is ignored", []string{"TERM", "xterm-kitty", "MDSH_NOTIFY", "loud"}, OSC99},
	} {
		if got := Detect(envOf(tc.env...)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDetectTmux(t *testing.T) {
	tmuxEnv := []string{"TERM", "tmux-256color", "TERM_PROGRAM", "tmux"}
	for _, tc := range []struct {
		name string
		env  []string
		t    Tmux
		want Protocol
	}{
		{"no passthrough", tmuxEnv, Tmux{TermName: "xterm-kitty", Passthrough: "off"}, Bell},
		{"tmux didn't answer", tmuxEnv, Tmux{}, Bell},
		{"kitty", tmuxEnv, Tmux{TermName: "xterm-kitty", Passthrough: "on"}, OSC99},
		{"iTerm2 by XTVERSION", tmuxEnv, Tmux{TermName: "xterm-256color", TermType: "iTerm2 3.5.4", Passthrough: "all"}, OSC9},
		{"WezTerm", tmuxEnv, Tmux{TermType: "WezTerm 20240203", Passthrough: "on"}, OSC9},
		{"ghostty", tmuxEnv, Tmux{TermName: "xterm-ghostty", Passthrough: "on"}, OSC9},
		{"foot", tmuxEnv, Tmux{TermName: "foot", Passthrough: "on"}, OSC777},
		{"server's environment", append(tmuxEnv, "LC_TERMINAL", "iTerm2"), Tmux{TermName: "xterm-256color", Passthrough: "on"}, OSC9},
		{"unknown", tmuxEnv, Tmux{TermName: "xterm-256color", Passthrough: "on"}, Bell},
		{"override", append(tmuxEnv, "MDSH_NOTIFY", "osc9"), Tmux{}, OSC9},
	} {
		if got := DetectTmux(envOf(tc.env...), tc.t); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestSequence(t *testing.T) {
	for _, tc := range []struct {
		name        string
		p           Protocol
		title, body string
		want        string
	}{
		{"bell", Bell, "a", "b", "\a"},
		{"off", Off, "a", "b", ""},
		{"osc 9", OSC9, "deploy.md", "Run build?", "\x1b]9;deploy.md: Run build?\x1b\\"},
		{"osc 9 body only", OSC9, "", "hi", "\x1b]9;hi\x1b\\"},
		{"osc 9 never starts with a digit", OSC9, "", "4;1 done", "\x1b]9; 4;1 done\x1b\\"},
		{"osc 99", OSC99, "deploy.md", "Run build?",
			"\x1b]99;i=mdsh:d=0:o=unfocused:e=1:p=title;" + b64("deploy.md") + "\x1b\\" +
				"\x1b]99;i=mdsh:d=1:e=1:p=body;" + b64("Run build?") + "\x1b\\"},
		{"osc 777", OSC777, "a;b", "c;d", "\x1b]777;notify;a,b;c;d\x1b\\"},
		{"control characters", OSC9, "t\x1b]0;x\a", "a\nb\x9c", "\x1b]9;t ]0;x: a b\x1b\\"},
	} {
		if got := Sequence(tc.p, tc.title, tc.body); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSequenceCuts(t *testing.T) {
	seq := Sequence(OSC777, strings.Repeat("t", 100), strings.Repeat("é", 300))
	_, rest, _ := strings.Cut(seq, "notify;")
	title, body, _ := strings.Cut(strings.TrimSuffix(rest, "\x1b\\"), ";")
	if n := len([]rune(title)); n != MaxTitle || !strings.HasSuffix(title, "…") {
		t.Errorf("title is %d runes: %q", n, title)
	}
	if n := len([]rune(body)); n != MaxBody || !strings.HasSuffix(body, "…") {
		t.Errorf("body is %d runes", n)
	}
}

func TestPassthrough(t *testing.T) {
	got := Passthrough("\x1b]9;hi\x1b\\")
	want := "\x1bPtmux;\x1b\x1b]9;hi\x1b\x1b\\\x1b\\"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNotifier(t *testing.T) {
	var buf bytes.Buffer
	n := New(&buf, OSC9, true)
	clock := time.Unix(1000, 0)
	n.now = func() time.Time { return clock }
	n.Seen()
	clock = clock.Add(12 * time.Second)
	if got := n.Idle(); got != 12*time.Second {
		t.Errorf("idle %v", got)
	}
	n.Notify("f.md", "done")
	if want := Passthrough("\x1b]9;f.md: done\x1b\\"); buf.String() != want {
		t.Errorf("wrote %q, want %q", buf.String(), want)
	}

	// tmux rings its own bell: not wrapped.
	buf.Reset()
	New(&buf, Bell, true).Notify("f.md", "done")
	if buf.String() != "\a" {
		t.Errorf("bell under tmux wrote %q", buf.String())
	}

	// Off, or no terminal, is a nil Notifier, whose methods do nothing.
	for _, n := range []*Notifier{New(&buf, Off, false), New(nil, OSC9, false)} {
		if n != nil {
			t.Fatal("want a nil Notifier")
		}
		n.Seen()
		n.Notify("a", "b")
		if n.Idle() != 0 || n.Protocol() != Off {
			t.Error("nil Notifier")
		}
	}
}
