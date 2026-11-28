package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/termnotify"
)

// notifications are the OSC 777 notifications written to out, as
// "title|body" lines.
func notifications(out *bytes.Buffer) []string {
	var got []string
	for _, s := range strings.Split(out.String(), "\x1b\\") {
		if body, ok := strings.CutPrefix(s, "\x1b]777;notify;"); ok {
			got = append(got, strings.Replace(body, ";", "|", 1))
		}
	}
	out.Reset()
	return got
}

func TestNotify(t *testing.T) {
	defer func(d time.Duration) { notifyAfter = d }(notifyAfter)
	var out bytes.Buffer
	src := "```sh {#danger confirm=\"Really?\"}\necho boom\n```\n\n```sh {#bad}\nexit 3\n```\n"
	m, _ := newModel(t, src, Options{AutoSave: true, Notify: termnotify.New(&out, termnotify.OSC777, false)})
	name := m.notifyTitle()

	// Watched: a prompt right after a key, and a quick run, notify
	// nobody.
	notifyAfter = time.Hour
	press(m, "enter")
	pump(t, m)
	if m.prompt == nil {
		t.Fatal("no prompt")
	}
	press(m, "r")
	pump(t, m)
	if got := notifications(&out); len(got) != 0 {
		t.Errorf("watched: got %v", got)
	}

	// Looked away: both notify, once each.
	notifyAfter = 0
	press(m, "enter")
	pump(t, m)
	m.statusAsking() // asked again: no second notification
	press(m, "r")
	pump(t, m)
	want := []string{name + "|Run danger?", name + "|Finished in 0s"}
	if got := notifications(&out); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("away: got %q, want %q", got, want)
	}

	// A failure says which block.
	press(m, "j")
	press(m, "j")
	press(m, "enter")
	pump(t, m)
	want = []string{name + "|bad failed, exit 3"}
	if got := notifications(&out); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("failed: got %q, want %q", got, want)
	}
}
