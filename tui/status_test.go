package tui

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdey/markdownshell/termstatus"
)

// statusReports decodes the OSC 7501 reports written to out as "state
// kind msg" lines, fields left out as -.
func statusReports(out *bytes.Buffer) []string {
	var got []string
	for _, s := range strings.Split(out.String(), "\x1b\\") {
		body, ok := strings.CutPrefix(s, "\x1b]7501;")
		if !ok {
			continue
		}
		f := map[string]string{"state": "-", "kind": "-", "msg": "-"}
		for _, pair := range strings.Split(body, ":") {
			k, v, _ := strings.Cut(pair, "=")
			if k == "msg" {
				b, _ := base64.StdEncoding.DecodeString(v)
				v = string(b)
			}
			f[k] = v
		}
		got = append(got, f["state"]+" "+f["kind"]+" "+f["msg"])
	}
	out.Reset()
	return got
}

func TestStatusFollowsRun(t *testing.T) {
	var out bytes.Buffer
	src := "```sh {#danger confirm=\"Really?\"}\necho boom\n```\n\n```sh {#bad}\nexit 3\n```\n"
	m, _ := newModel(t, src, Options{AutoSave: true, Status: termstatus.NewTracker(&out, "mdsh")})
	press(m, "enter")
	pump(t, m)
	if m.prompt == nil {
		t.Fatal("no prompt")
	}
	want := []string{"working - -", "blocked permission Run danger?"}
	if got := statusReports(&out); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("asking: got %v, want %v", got, want)
	}
	press(m, "r")
	pump(t, m)
	want = []string{"working - -", "working - danger", "done - -"}
	if got := statusReports(&out); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("ran: got %v, want %v", got, want)
	}
	// The next key says the result was seen.
	press(m, "j")
	if got := statusReports(&out); strings.Join(got, "\n") != "idle - -" {
		t.Errorf("seen: got %v", got)
	}
	press(m, "j")
	if got := statusReports(&out); len(got) != 0 {
		t.Errorf("idle again: got %v", got)
	}
	// A failing block ends in error.
	press(m, "enter")
	pump(t, m)
	got := statusReports(&out)
	if len(got) == 0 || got[len(got)-1] != "error - bad failed, exit 3" {
		t.Errorf("failed: got %v", got)
	}
}

// TestBlockStatusShows: a program in a background tty block's terminal
// reports what it is doing; the card's header says so, and the report
// goes up to the terminal mdsh -i runs in as a record under the block's
// name, cleared when the block ends.
func TestBlockStatusShows(t *testing.T) {
	var out bytes.Buffer
	src := "```sh {#deploy background tty}\nprintf '\\033]7501;state=blocked:kind=permission:app=terraform:msg=QXBwbHk/\\033\\\\'; read n\n```\n"
	m, _ := newModel(t, src, Options{Status: termstatus.NewTracker(&out, "mdsh")})
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	press(m, "enter")
	// The pane holds the report as soon as it arrives; the model takes
	// it in, and forwards it, on its message.
	until(t, m, "the report", func() bool { return strings.Contains(out.String(), "id=deploy") })
	if v := m.View(); !strings.Contains(v, "terraform waiting: permission Apply?") {
		t.Errorf("header doesn't show the status:\n%s", v)
	}
	seqs := strings.Split(out.String(), "\x1b\\")
	var child string
	for _, s := range seqs {
		if strings.Contains(s, "id=deploy") {
			child = s
		}
	}
	if !strings.Contains(child, "state=blocked:id=deploy:kind=permission:app=terraform") {
		t.Errorf("forwarded %q in %q", child, out.String())
	}
	// The Processes panel says so too. ctrl+g takes the keys from the
	// block first, and t gives them back.
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	press(m, "P")
	if v := m.View(); !strings.Contains(v, "◐ deploy") {
		t.Errorf("processes panel:\n%s", v)
	}
	// The row's text holds it; the view may cut the row short where
	// the shell's path is long, as macOS's temporary directory is.
	var rows []string
	for _, r := range m.procs {
		rows = append(rows, r.text)
	}
	if !strings.Contains(strings.Join(rows, "\n"), "waiting: permission Apply?") {
		t.Errorf("processes rows: %q", rows)
	}
	press(m, "P", "t")
	out.Reset()
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	until(t, m, "the block to end", func() bool { m.sess.Reap(); return !m.bg[0] && m.bgPanes[0] == nil })
	if !strings.Contains(out.String(), "\x1b]7501;state=clear:id=deploy\x1b\\") {
		t.Errorf("child not cleared: %q", out.String())
	}
	if len(m.forwarded) != 0 {
		t.Errorf("forwarded still %v", m.forwarded)
	}
}
