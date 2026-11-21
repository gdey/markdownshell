package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/notebook"
	"github.com/gdey/markdownshell/settings"
)

func TestProcsText(t *testing.T) {
	got := procsText([]env.ShellProcs{
		{PID: 10, Secs: 302, Children: []*env.Proc{
			{PID: 11, Command: "sleep 100", Secs: 59, Children: []*env.Proc{{PID: 12, Command: "cat", Secs: 3}}},
			{PID: 13, Command: "python3 -m http.server", Secs: 3700, Block: "server"},
		}},
		{Env: "db", PID: 20, Secs: 90000},
	})
	want := "10 shell  (5m 02s)\n" +
		"  11 sleep 100  (59s)\n" +
		"    12 cat  (3s)\n" +
		"  13 [background block server] python3 -m http.server  (1h 01m)\n" +
		"20 env db  (1d 1h) · nothing running under it\n"
	if got != want {
		t.Errorf("procsText:\n%s\nwant:\n%s", got, want)
	}
}

// A line repeated back to back is one line, with a count.
func TestConsoleLinesRepeat(t *testing.T) {
	at := time.Date(2026, 11, 20, 23, 4, 4, 0, time.UTC)
	busy := ConsoleEntry{Level: "error", Message: "Binding call failed: Bound method returned an error: a block is running"}
	var es []ConsoleEntry
	for i, d := range []int{0, 5, 5, 17} {
		e := busy
		e.Seq, e.Time = i+1, at.Add(time.Duration(d)*time.Second)
		es = append(es, e)
	}
	es = append(es, ConsoleEntry{Seq: 5, Time: at.Add(20 * time.Second), Level: "warn", Message: "near", File: "/home/j/a.md", Line: 3})
	e := busy
	e.Seq, e.Time = 6, at.Add(30*time.Second)
	es = append(es, e)

	got := consoleLines(es, "/home/j")
	want := "23:04:04.000 ERR Binding call failed: Bound method returned an error: a block is running (×4, last 23:04:21.000)\n" +
		"23:04:24.000 WRN ~/a.md:3 near\n" +
		"23:04:34.000 ERR Binding call failed: Bound method returned an error: a block is running\n"
	if got != want {
		t.Errorf("consoleLines:\n%s\nwant:\n%s", got, want)
	}
}

func TestSettingsMapLeavesOutPrivate(t *testing.T) {
	m := settingsMap(settings.Settings{
		Vim: true, Shell: "zsh", Recent: []string{"/home/j/secret.md"}, Pinned: []string{"/p"},
		LastFolder: "/home/j", HistoryRC: "/home/j/.zshrc", InteractiveShell: true,
	})
	for _, k := range privateSettings {
		if _, ok := m[k]; ok {
			t.Errorf("settings kept %s", k)
		}
	}
	if m["vim"] != true || m["shell"] != "zsh" || m["interactiveShell"] != true || m["historyRC"] != "/home/j/.zshrc" {
		t.Errorf("settings = %v", m)
	}
	if _, ok := m["autosave"]; ok {
		t.Errorf("an unset setting is kept: %v", m)
	}
}

func TestBugReportMarkdown(t *testing.T) {
	r := bugReport{
		Build:    Build{Version: "dev", Commit: "101568b2cce5", Modified: true},
		Go:       "go1.27.4",
		OS:       "darwin/arm64",
		Made:     time.Date(2026, 11, 20, 23, 7, 0, 0, time.UTC),
		Settings: map[string]any{"historyRC": "/home/j/.zshrc", "vim": true},
		File:     "/home/j/notes/run.md",
		Tab:      []string{"History pane: minimized"},
		Console:  []ConsoleEntry{{Level: "info", Message: "printed ```"}},
		Events:   []notebook.AuditEvent{{JSON: `{"event":"block.start"}`}},
	}
	got := r.markdown("/home/j")
	for _, want := range []string{
		"# mdsh bug report",
		"- Version: dev (101568b2cce5 modified)",
		"- Go: go1.27.4, darwin/arm64",
		"historyRC: \"~/.zshrc\"\nvim: true\n",
		"- File: ~/notes/run.md\n- History pane: minimized\n",
		"No shells running.",
		// A fence the console's text can't close.
		"````\n",
		"```jsonl\n{\"event\":\"block.start\"}\n```",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/home/j") {
		t.Errorf("report shows the home folder:\n%s", got)
	}
}
