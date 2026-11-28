package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termstatus"
)

// reports decodes the OSC 7501 reports written to out as "state kind
// progress msg" lines, fields left out as -.
func reports(out *bytes.Buffer) []string {
	var got []string
	for _, s := range strings.Split(out.String(), "\x1b\\") {
		body, ok := strings.CutPrefix(s, "\x1b]7501;")
		if !ok {
			continue
		}
		f := map[string]string{}
		for _, pair := range strings.Split(body, ":") {
			k, v, _ := strings.Cut(pair, "=")
			if k == "msg" {
				b, _ := base64.StdEncoding.DecodeString(v)
				v = string(b)
			}
			f[k] = v
		}
		get := func(k string) string {
			if f[k] == "" {
				return "-"
			}
			return f[k]
		}
		got = append(got, strings.Join([]string{get("state"), get("kind"), get("progress"), get("msg")}, " "))
	}
	return got
}

func statusRun(t *testing.T, src string, answer run.Answer) ([]string, func(e run.Event), func([]*run.Result, error), *bytes.Buffer) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte(src), 0o644)
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	opts := run.Options{File: f, Environ: []string{"PATH=" + os.Getenv("PATH")}, Confirm: func(*doc.Block) run.Answer { return answer }}
	finish := trackStatus(d, &opts, termstatus.NewTracker(&out, "mdsh"))
	results, err := run.Run(context.Background(), d, opts)
	finish(results, err)
	return reports(&out), opts.OnEvent, finish, &out
}

func TestStatusReports(t *testing.T) {
	src := "```sh {#build}\necho built\n```\n\n```sh {#deploy confirm}\necho deployed\n```\n"
	// Blocked keeps the progress of the last working report, and the
	// answer brings that report back.
	want := []string{
		"working - 0 build",
		"blocked permission 0 Run deploy?",
		"working - 0 build",
		"working - 50 deploy",
		"done - - 2 blocks ran",
	}
	got, _, _, _ := statusRun(t, src, run.Go)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Declined: the run stops, and nothing failed.
	if got, _, _, _ := statusRun(t, src, run.Stop); got[len(got)-1] != "done - - 1 block ran" {
		t.Errorf("declined: %v", got)
	}
	// Interrupted: idle, the user being there.
	_, _, finish, out := statusRun(t, src, run.Stop)
	finish(nil, context.Canceled)
	if got := reports(out); got[len(got)-1] != "idle - - -" {
		t.Errorf("interrupted: %v", got)
	}
	// A failing block ends in error, naming it.
	got, _, _, _ = statusRun(t, "```sh {#a}\necho a\n```\n\n```sh {#b}\nexit 7\n```\n", run.Go)
	if got[len(got)-1] != "error - - b failed, exit 7" {
		t.Errorf("failed: %v", got)
	}
	// A ctl status report shows as working, with the block's own
	// progress when it gives one.
	d, _ := doc.Parse([]byte("```sh {#a}\necho a\n```\n"))
	_, onEvent, _, out := statusRun(t, "```sh {#a}\necho a\n```\n", run.Go)
	out.Reset()
	onEvent(run.Event{Kind: run.Status, Block: d.Blocks[0], Progress: &run.Progress{Text: "unpacking", Step: 1, Of: 4}})
	onEvent(run.Event{Kind: run.Status, Block: d.Blocks[0], Progress: &run.Progress{Text: "linking"}})
	if got := reports(out); strings.Join(got, "\n") != "working - 25 a: unpacking\nworking - 100 a: linking" {
		t.Errorf("ctl status: %v", got)
	}
}

func TestStatusNoTerminal(t *testing.T) {
	d, _ := doc.Parse([]byte("```sh {#a}\necho a\n```\n"))
	opts := run.Options{Confirm: func(*doc.Block) run.Answer { return run.Go }}
	confirm := opts.Confirm
	finish := trackStatus(d, &opts, nil)
	finish(nil, nil)
	if opts.Confirm == nil || opts.OnEvent != nil {
		t.Error("hooks changed with no tracker")
	}
	_ = confirm
}
