//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/run"
)

// TestMain lets the test binary stand in for mdsh, so blocks that call
// mdsh ctl reach the run under test. The tests themselves run as with no
// terminal, wherever they are started: one would be asked questions
// nobody answers, as a dependency marked skip is asked about even with
// --yes.
func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) == "mdsh" {
		os.Exit(mdsh(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
	openTTY = func() (*os.File, error) { return nil, os.ErrNotExist }
	os.Exit(m.Run())
}

// mdshOnPath puts this test binary on $PATH as mdsh.
func mdshOnPath(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(exe, filepath.Join(dir, "mdsh")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCtl(t *testing.T) {
	mdshOnPath(t)
	f := filepath.Join(t.TempDir(), "d.md")
	os.WriteFile(f, []byte("```sh {#a}\n"+
		"mdsh ctl set env prod\n"+
		"mdsh ctl status --step 1 --of 2 building\n"+
		"echo \"a: ${MDSH_VAR_env-unset} $(mdsh ctl get env)\"\n"+
		"mdsh ctl set name 2>/dev/null; echo \"args=$?\"\n"+
		"mdsh ctl set 'no good' x 2>/dev/null; echo \"name=$?\"\n"+
		"mdsh ctl set picked x 2>/dev/null; echo \"input=$?\"\n"+
		"printf 'two\\nlines\\n' | mdsh ctl set multi -\n"+
		"```\n\n"+
		"```textbox {var=picked}\nhi\n```\n\n"+
		"```sh {#b run={{$MDSH_VAR_env}}==prod}\n"+
		"echo \"b: $MDSH_VAR_env\"\n"+
		"echo \"$MDSH_VAR_multi\"\n"+
		"mdsh ctl unset env\n"+
		"```\n\n"+
		"```sh {#c run={{$MDSH_VAR_env}}==prod}\necho c ran\n```\n\n"+
		"```sh {#d}\n"+
		"echo \"d: ${MDSH_VAR_env-unset}\"\n"+
		"echo \"$MDSH_TOKEN\" > tok\n"+
		"```\n\n"+
		"```sh {#e}\n"+
		"MDSH_TOKEN=$(cat tok) mdsh ctl get multi 2>/dev/null; echo \"old token=$?\"\n"+
		"```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--no-color", f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d\n%s\n%s", rc, out.String(), errb.String())
	}
	for _, want := range []string{
		"a: unset prod\nargs=2\nname=2\ninput=2\n",
		"b: prod\ntwo\nlines\n",
		"d: unset\n",
		"old token=2\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "{for=c ") {
		t.Errorf("c ran after unset:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "a: [1/2] building\n") {
		t.Errorf("stderr = %q", errb.String())
	}
}

func TestCtlOutsideMdsh(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := ctlCall([]string{"get", "x"}, "", "", nil, &out, &errb); rc != 3 {
		t.Errorf("get: rc = %d, %s", rc, errb.String())
	}
	if rc := ctlCall([]string{"status", "hi"}, "", "", nil, &out, &errb); rc != 0 {
		t.Errorf("status: rc = %d", rc)
	}
	if rc := ctlCall([]string{"frob"}, "", "", nil, &out, &errb); rc != 2 {
		t.Errorf("unknown: rc = %d", rc)
	}
	if rc := ctlCall([]string{"status", "--step", "1", "hi"}, "", "", nil, &out, &errb); rc != 2 {
		t.Errorf("--step without --of: rc = %d", rc)
	}
}

func TestAudit(t *testing.T) {
	mdshOnPath(t)
	dir := t.TempDir()
	f, logFile := filepath.Join(dir, "d.md"), filepath.Join(dir, "audit.jsonl")
	os.WriteFile(f, []byte("---\nmdsh:\n  env:\n    share: [PATH, HOME]\n---\n\n"+
		"```sh {#a}\nmdsh ctl set --secret pw hunter2\nmdsh ctl set env prod\nmdsh ctl set 'x y' 1 2>/dev/null\ntrue\n```\n\n"+
		"```sh {#b env=sub run={{$MDSH_VAR_env}}==dev}\necho no\n```\n\n"+
		"```sh {#c skip}\necho no\n```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"-w", "--audit", logFile, f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d\n%s", rc, errb.String())
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(logFile); info.Mode().Perm() != 0o600 {
		t.Errorf("log mode %o", info.Mode().Perm())
	}
	if strings.Contains(string(data), "hunter2") {
		t.Errorf("secret in the log:\n%s", data)
	}
	var events []string
	for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var e map[string]any
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d: %v: %s", i+1, err, line)
		}
		if e["v"] != 1.0 || e["seq"] != float64(i+1) || e["t"] == "" {
			t.Errorf("line %d header: %s", i+1, line)
		}
		ev := e["event"].(string)
		switch ev {
		case "env.start":
			// The base set (SHELL, LANG, ...) passes too, as much of it
			// as the test's environment has; list only the rest.
			var shared []string
			for _, name := range e["shared"].([]any) {
				if n := name.(string); n == "HOME" || n == "PATH" || !env.IsBase(n) {
					shared = append(shared, n)
				}
			}
			ev += ":" + fmt.Sprint(shared)
		case "ctl":
			ev += ":" + fmt.Sprint(e["code"])
		case "var":
			if _, secret := e["new"].(map[string]any); secret {
				ev += ":secret"
			}
		case "block.skip":
			ev += ":" + e["block"].(string) + ":" + e["reason"].(string)
			if e["reason"] == "run=" && e["saw"] != "prod" {
				t.Errorf("run= saw %v", e["saw"])
			}
		case "block.end":
			ev += ":" + fmt.Sprint(e["exit"])
		case "run.end":
			ev += ":" + fmt.Sprint(e["exit"], e["stopped"])
		}
		events = append(events, ev)
	}
	want := []string{"run.start", "block.start", "env.start:[HOME PATH]", "var:secret", "ctl:0", "var", "ctl:0", "ctl:2", "block.end:0",
		"block.skip:b:run=", "block.skip:c:skip", "write", "run.end:0 false"}
	if strings.Join(events, "\n") != strings.Join(want, "\n") {
		t.Errorf("events:\n%s\nwant:\n%s\nlog:\n%s", strings.Join(events, "\n"), strings.Join(want, "\n"), data)
	}

	if rc := mdsh([]string{"-i", "--audit", logFile, f}, nil, &out, &errb); rc != 2 {
		t.Errorf("-i --audit: rc = %d", rc)
	}
}

// TestCtlAsk runs ask and confirm with nobody to answer: the default
// answers, or the call exits 4; --yes answers a confirm with no
// default. The log records who answered, and never a secret answer.
func TestCtlAsk(t *testing.T) {
	mdshOnPath(t)
	dir := t.TempDir()
	f, logFile := filepath.Join(dir, "d.md"), filepath.Join(dir, "audit.jsonl")
	os.WriteFile(f, []byte("```sh {#a}\n"+
		"echo \"env=$(mdsh ctl ask --choice dev,prod --default dev 'Which env?')\"\n"+
		"echo \"pw=$(mdsh ctl ask --secret --default hunter2 'Password?')\"\n"+
		"mdsh ctl ask 'Name?'; echo \"none=$?\"\n"+
		"mdsh ctl confirm --default no 'Deploy?'; echo \"no=$?\"\n"+
		"mdsh ctl confirm --default yes 'Tag?'; echo \"yes=$?\"\n"+
		"mdsh ctl confirm 'Really?'; echo \"bare=$?\"\n"+
		"mdsh ctl ask --choice a,b --default c 'Pick?' 2>/dev/null; echo \"bad default=$?\"\n"+
		"mdsh ctl confirm --default maybe 'Hm?' 2>/dev/null; echo \"bad confirm=$?\"\n"+
		"```\n"), 0o644)
	var out, errb bytes.Buffer
	if rc := mdsh([]string{"--no-color", "--audit", logFile, f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("rc = %d\n%s\n%s", rc, out.String(), errb.String())
	}
	want := "env=dev\npw=hunter2\nnone=4\nno=1\nyes=0\nbare=4\nbad default=2\nbad confirm=2\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("missing %q in\n%s", want, out.String())
	}
	if !strings.Contains(errb.String(), "mdsh: block a: no answer: Name?\n") {
		t.Errorf("no answer not shown:\n%s", errb.String())
	}
	data, _ := os.ReadFile(logFile)
	if strings.Contains(string(data), "hunter2") {
		t.Errorf("secret in the log:\n%s", data)
	}
	var asks []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var e map[string]any
		json.Unmarshal([]byte(line), &e)
		if e["event"] != "ask" {
			continue
		}
		answer := fmt.Sprint(e["answer"])
		if _, secret := e["answer"].(map[string]any); secret {
			answer = "secret"
		}
		asks = append(asks, fmt.Sprint(e["kind"], " ", e["question"], " ", e["by"], " ", answer))
	}
	wantAsks := []string{"ask Which env? default dev", "ask Password? default secret", "ask Name? none <nil>",
		"confirm Deploy? default no", "confirm Tag? default yes", "confirm Really? none <nil>"}
	if strings.Join(asks, "\n") != strings.Join(wantAsks, "\n") {
		t.Errorf("ask events:\n%s\nwant:\n%s", strings.Join(asks, "\n"), strings.Join(wantAsks, "\n"))
	}

	os.WriteFile(f, []byte("```sh {#a}\nmdsh ctl confirm 'Really?'; echo \"yes=$?\"\nmdsh ctl confirm --default no 'Deploy?'; echo \"no=$?\"\nmdsh ctl ask 'Name?'; echo \"ask=$?\"\n```\n"), 0o644)
	out.Reset()
	if rc := mdsh([]string{"--no-color", "--yes", f}, nil, &out, &errb); rc != 0 {
		t.Fatalf("--yes: rc = %d\n%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "yes=0\nno=1\n") || !strings.Contains(out.String(), "ask=4") {
		t.Errorf("--yes:\n%s", out.String())
	}
}

// TestPrompterAsk answers ask and confirm questions on a terminal.
func TestPrompterAsk(t *testing.T) {
	b := &doc.Block{ID: "a"}
	def := func(q run.Question, v string) run.Question {
		q.Default, q.HasDefault = v, true
		return q
	}
	confirm := run.Question{Kind: run.AskCtlConfirm, Block: b, Text: "Deploy?"}
	ask := run.Question{Kind: run.AskCtl, Block: b, Text: "Env?"}
	choose := run.Question{Kind: run.AskCtl, Block: b, Text: "Env?", Choices: []string{"dev", "prod"}}
	for _, tc := range []struct {
		in   string
		q    run.Question
		want string
		by   run.Answered
	}{
		{"y\n", confirm, "yes", run.ByUser},
		{"maybe\nNo\n", confirm, "no", run.ByUser},
		{"\n", def(confirm, "no"), "no", run.ByUser},
		{"", confirm, "", run.ByNone},
		{"  two words \n", ask, "  two words ", run.ByUser},
		{"\n", ask, "", run.ByUser},
		{"\n", def(ask, "x"), "x", run.ByUser},
		{"staging\n2\n", choose, "prod", run.ByUser},
		{"dev\n", choose, "dev", run.ByUser},
		{"staging\n", choose, "", run.ByNone},
	} {
		var out bytes.Buffer
		got, by := newPrompter(strings.NewReader(tc.in), &out).ask(context.Background(), tc.q)
		if got != tc.want || by != tc.by {
			t.Errorf("%q to %+v: %q by %s, want %q by %s\n%s", tc.in, tc.q, got, by, tc.want, tc.by, out.String())
		}
	}
}

// TestPrompterWithdrawn checks that a question whose block ended stops
// waiting, and that the line typed for it goes to the next question.
func TestPrompterWithdrawn(t *testing.T) {
	r, w := io.Pipe()
	var out bytes.Buffer
	p := newPrompter(r, &out)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan run.Answered)
	go func() {
		_, by := p.ask(ctx, run.Question{Kind: run.AskCtl, Block: &doc.Block{ID: "a"}, Text: "Name?"})
		done <- by
	}()
	cancel()
	if by := <-done; by != run.ByNone {
		t.Errorf("withdrawn: by %s", by)
	}
	if !strings.Contains(out.String(), "its question is withdrawn") {
		t.Errorf("out = %q", out.String())
	}
	go w.Write([]byte("y\n"))
	got, by := p.ask(context.Background(), run.Question{Kind: run.AskCtlConfirm, Block: &doc.Block{ID: "b"}, Text: "Go?"})
	if got != "yes" || by != run.ByUser {
		t.Errorf("next: %q by %s", got, by)
	}
}
