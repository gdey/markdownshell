package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/notebook"
)

// A bug report is one Markdown document, for pasting into an issue or a
// chat with Claude: what the app is, how it is set up, what the tab it
// was made from is doing, and the console, events and processes as the
// Console pane's Copy copies them. Help › Copy Bug Report and the
// Console pane's Bug Report button make one.

// bugReportLines is how many of the newest console entries and events a
// bug report keeps.
const bugReportLines = 200

// bugReport is what a bug report says.
type bugReport struct {
	Build    Build
	Go, OS   string // the Go version, and GOOS/GOARCH
	Made     time.Time
	Settings map[string]any
	// File is the file open in the tab, "" for none.
	File string
	Tab  []string // what the tab is doing, as "name: value" lines
	// Procs are the tab's shells and what runs under them.
	Procs   []env.ShellProcs
	Console []ConsoleEntry
	Events  []notebook.AuditEvent
}

// privateSettings are the settings a bug report leaves out: lists of
// the files and folders used, which say nothing about how the app
// behaves.
var privateSettings = []string{"recent", "pinned", "places", "fileViews", "lastFolder"}

// settingsMap is s as its JSON, less privateSettings and empty values.
func settingsMap(s any) map[string]any {
	b, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	for _, k := range privateSettings {
		delete(m, k)
	}
	for k, v := range m {
		switch v := v.(type) {
		case nil:
			delete(m, k)
		case bool:
			if !v {
				delete(m, k)
			}
		case string:
			if v == "" {
				delete(m, k)
			}
		case float64:
			if v == 0 {
				delete(m, k)
			}
		}
	}
	return m
}

// tilde shows path under home as ~/….
func tilde(path, home string) string {
	if home == "" || path == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return path
}

// consoleLine is e as the Console pane's Copy copies it.
func consoleLine(e ConsoleEntry, home string) string {
	var b strings.Builder
	b.WriteString(e.Time.Format("15:04:05.000"))
	b.WriteString(" ")
	b.WriteString(map[string]string{"debug": "DBG", "info": "INF", "warn": "WRN", "error": "ERR"}[e.Level])
	if e.File != "" {
		b.WriteString(" ")
		b.WriteString(tilde(e.File, home))
		if e.Line != 0 {
			fmt.Fprintf(&b, ":%d", e.Line)
		}
	}
	b.WriteString(" ")
	b.WriteString(e.Message)
	if e.Stale {
		b.WriteString(" (fixed)")
	}
	return b.String()
}

// sameLine reports whether a and b say the same thing, so that the
// console shows b as a repeat of a.
func sameLine(a, b ConsoleEntry) bool {
	return a.Level == b.Level && a.Message == b.Message && a.File == b.File && a.Line == b.Line && a.Fix == b.Fix && a.Stale == b.Stale
}

// consoleLines are entries as lines, as the Console pane's Copy copies
// them: a line repeated back to back is one line, with how many times
// and when the last came.
func consoleLines(entries []ConsoleEntry, home string) string {
	var b strings.Builder
	for i := 0; i < len(entries); {
		j := i + 1
		for j < len(entries) && sameLine(entries[i], entries[j]) {
			j++
		}
		b.WriteString(consoleLine(entries[i], home))
		if n := j - i; n > 1 {
			fmt.Fprintf(&b, " (×%d, last %s)", n, entries[j-1].Time.Format("15:04:05.000"))
		}
		b.WriteString("\n")
		i = j
	}
	return b.String()
}

// running says how long a process has run: 42s, 3m 07s, 2h 05m, 4d 3h,
// as the Processes tab does.
func running(secs int) string {
	switch {
	case secs < 60:
		return fmt.Sprintf("%ds", secs)
	case secs < 3600:
		return fmt.Sprintf("%dm %02ds", secs/60, secs%60)
	case secs < 86400:
		return fmt.Sprintf("%dh %02dm", secs/3600, secs/60%60)
	}
	return fmt.Sprintf("%dd %dh", secs/86400, secs/3600%24)
}

// procsText is shells as the Processes tab shows them, as text: one
// line for each, indented under what started it, with its pid, what it
// is and how long it has run.
func procsText(shells []env.ShellProcs) string {
	var b strings.Builder
	var procs func(ps []*env.Proc, depth int)
	procs = func(ps []*env.Proc, depth int) {
		for _, p := range ps {
			if p == nil {
				continue
			}
			b.WriteString(strings.Repeat("  ", depth))
			fmt.Fprintf(&b, "%d ", p.PID)
			if p.Block != "" {
				fmt.Fprintf(&b, "[background block %s] ", p.Block)
			}
			fmt.Fprintf(&b, "%s  (%s)\n", p.Command, running(p.Secs))
			procs(p.Children, depth+1)
		}
	}
	for _, s := range shells {
		name := "shell"
		if s.Env != "" {
			name = "env " + s.Env
		}
		fmt.Fprintf(&b, "%d %s  (%s)", s.PID, name, running(s.Secs))
		if len(s.Children) == 0 {
			b.WriteString(" · nothing running under it")
		}
		b.WriteString("\n")
		procs(s.Children, 1)
	}
	return b.String()
}

// fence puts text in a code fence of info that text can't close.
func fence(b *strings.Builder, info, text string) {
	f := "```"
	for strings.Contains(text, f) {
		f += "`"
	}
	b.WriteString(f + info + "\n")
	b.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(f + "\n")
}

// last is the newest n of s.
func last[T any](s []T, n int) []T {
	return s[max(0, len(s)-n):]
}

// markdown is r as the document to paste.
func (r bugReport) markdown(home string) string {
	var b strings.Builder
	b.WriteString("# mdsh bug report\n\n")
	b.WriteString("## What happened\n\n")
	b.WriteString("<!-- What you did, what you expected, and what happened instead. -->\n\n")
	b.WriteString("<!-- This report holds file paths, commands and what blocks printed. Read it before you share it. -->\n\n")

	b.WriteString("## App\n\n")
	fmt.Fprintf(&b, "- Version: %s\n", r.Build)
	fmt.Fprintf(&b, "- Go: %s, %s\n", r.Go, r.OS)
	fmt.Fprintf(&b, "- Made: %s\n\n", r.Made.Format(time.RFC3339))

	b.WriteString("## Settings\n\n")
	if len(r.Settings) == 0 {
		b.WriteString("All at their defaults.\n\n")
	} else {
		var s strings.Builder
		for _, k := range slices.Sorted(maps.Keys(r.Settings)) {
			v := r.Settings[k]
			if p, ok := v.(string); ok {
				v = tilde(p, home)
			}
			j, _ := json.Marshal(v)
			fmt.Fprintf(&s, "%s: %s\n", k, j)
		}
		fence(&b, "", s.String())
		b.WriteString("\n")
	}

	b.WriteString("## This tab\n\n")
	if r.File == "" {
		b.WriteString("- File: none\n")
	} else {
		fmt.Fprintf(&b, "- File: %s\n", tilde(r.File, home))
	}
	for _, l := range r.Tab {
		fmt.Fprintf(&b, "- %s\n", l)
	}
	b.WriteString("\n")

	b.WriteString("## Processes\n\n")
	if len(r.Procs) == 0 {
		b.WriteString("No shells running.\n\n")
	} else {
		fence(&b, "", procsText(r.Procs))
		b.WriteString("\n")
	}

	b.WriteString("## Console\n\n")
	if len(r.Console) > bugReportLines {
		fmt.Fprintf(&b, "The newest %d of %d lines.\n\n", bugReportLines, len(r.Console))
	}
	if len(r.Console) == 0 {
		b.WriteString("Empty.\n\n")
	} else {
		fence(&b, "", consoleLines(last(r.Console, bugReportLines), home))
		b.WriteString("\n")
	}

	b.WriteString("## Events\n\n")
	if len(r.Events) > bugReportLines {
		fmt.Fprintf(&b, "The newest %d of %d events, as mdsh --audit writes them.\n\n", bugReportLines, len(r.Events))
	} else if len(r.Events) > 0 {
		b.WriteString("As mdsh --audit writes them.\n\n")
	}
	if len(r.Events) == 0 {
		b.WriteString("None.\n")
	} else {
		var s strings.Builder
		for _, e := range last(r.Events, bugReportLines) {
			s.WriteString(e.JSON)
			s.WriteString("\n")
		}
		fence(&b, "jsonl", s.String())
	}
	return b.String()
}

// BugReport is a Markdown bug report from the calling tab, to copy:
// the app's version and settings, the tab's file and what it is doing,
// its processes, the console and the file's events. page is what the
// page says it is doing, such as whether the History pane is minimized,
// as "name: value" lines.
func (a *App) BugReport(ctx context.Context, page []string) (string, error) {
	t, err := a.tab(ctx)
	if err != nil {
		return "", err
	}
	r := bugReport{
		Build:    a.build,
		Go:       runtime.Version(),
		OS:       runtime.GOOS + "/" + runtime.GOARCH,
		Made:     time.Now(),
		Settings: settingsMap(a.settings.Get()),
		Console:  a.console.list(),
	}
	t.mu.Lock()
	v, busy := t.view, t.running
	t.mu.Unlock()
	r.Tab = append(r.Tab,
		fmt.Sprintf("Run going (tab): %t", busy),
		fmt.Sprintf("View: source %t, rendered %t, outline %t, history %t, console %t, code %t, output %t", v.Split, v.Rendered, v.Outline, v.History, v.Console, v.Code, v.Output),
	)
	if n, err := t.notebook(); err == nil {
		r.File = n.File()
		ran, total := n.Ran()
		ro := n.RunOptions()
		r.Tab = append(r.Tab,
			fmt.Sprintf("Run going (notebook): %t", n.Busy()),
			fmt.Sprintf("Unsaved output: %t", n.Dirty()),
			fmt.Sprintf("Changed on disk since read or saved: %t", n.Changed()),
			fmt.Sprintf("History on: %t", n.History()),
			fmt.Sprintf("Blocks ran: %d of %d", ran, total),
			fmt.Sprintf("Run options: args %q, only %q, step %t, stop on error %t, shell %q", ro.Args, ro.Only, ro.Step, ro.StopOnError, ro.Shell),
		)
		r.Procs = n.Processes()
	}
	if l := t.eventLog(); l != nil {
		r.Events = l.List()
	}
	r.Tab = append(r.Tab, page...)
	home, _ := os.UserHomeDir()
	return r.markdown(home), nil
}

// ProcessesText is the calling tab's processes as the Processes tab
// shows them, as text, for its Copy.
func (a *App) ProcessesText(ctx context.Context) string {
	return procsText(a.Processes(ctx))
}
