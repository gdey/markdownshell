//go:build !windows

package run_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/ctl"
	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// TestCtlBackground checks that a background block's token works while
// it runs and stops when it ends, that a set reaches blocks that start
// later and the conditions before them, and that Close removes the
// socket.
func TestCtlBackground(t *testing.T) {
	dir := t.TempDir()
	d := parse(t, "```sh {#bg background}\nprintf '%s\\n%s\\n' \"$MDSH_SOCK\" \"$MDSH_TOKEN\" > call.tmp && mv call.tmp call\nwhile [ ! -e go ]; do sleep 0.02; done\n```\n\n"+
		"```sh {#later run={{$MDSH_VAR_who}}==bg}\necho \"later: $MDSH_VAR_who\"\n```\n")
	var statuses []string
	s, err := run.NewSession(d, run.Options{Dir: dir, Environ: []string{}, OnEvent: func(e run.Event) {
		if e.Kind == run.Status {
			statuses = append(statuses, e.Block.ID+":"+e.Progress.Text)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.RunBlock(ctx, 0); err != nil {
		t.Fatal(err)
	}
	var sock, token string
	for deadline := time.Now().Add(5 * time.Second); ; {
		data, err := os.ReadFile(filepath.Join(dir, "call"))
		if err == nil {
			sock, token, _ = strings.Cut(strings.TrimSpace(string(data)), "\n")
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background block never wrote its token")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if resp, err := ctl.Call(sock, token, "set", ctl.SetArgs{Name: "who", Value: "bg"}); err != nil || !resp.OK {
		t.Fatalf("set: %+v, %v", resp, err)
	}
	if resp, err := ctl.Call(sock, token, "status", ctl.StatusArgs{Text: "half"}); err != nil || !resp.OK {
		t.Fatalf("status: %+v, %v", resp, err)
	}
	os.WriteFile(filepath.Join(dir, "go"), nil, 0o644)
	if err := s.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if resp, err := ctl.Call(sock, token, "get", ctl.NameArgs{Name: "who"}); err != nil || resp.OK || resp.Code != ctl.Refused {
		t.Errorf("token after its block ended: %+v, %v", resp, err)
	}
	if err := s.RunFrom(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if r := s.Results()[1]; !r.Ran || string(r.Stdout) != "later: bg\n" {
		t.Errorf("later: ran %v, stdout %q", r.Ran, r.Stdout)
	}
	if len(statuses) != 1 || statuses[0] != "bg:half" {
		t.Errorf("statuses = %q", statuses)
	}
	s.Close()
	if _, err := os.Stat(filepath.Dir(sock)); !os.IsNotExist(err) {
		t.Errorf("socket dir left after Close: %v", err)
	}
}

// TestCtlSetNoSaveInput checks that set fills a nosave input's value for
// the session, where it reaches later blocks and conditions, and is
// still refused on an input the file keeps.
func TestCtlSetNoSaveInput(t *testing.T) {
	dir := t.TempDir()
	d := parse(t, "```textbox {#host var=host nosave}\ndefault\n```\n\n```textbox {#user var=user}\nme\n```\n\n"+
		"```sh {#bg background}\nprintf '%s\\n%s\\n' \"$MDSH_SOCK\" \"$MDSH_TOKEN\" > call.tmp && mv call.tmp call\nwhile [ ! -e go ]; do sleep 0.02; done\n```\n\n"+
		"```sh {#later run={{$MDSH_VAR_host}}==db1}\necho \"later: $MDSH_VAR_host\"\n```\n")
	s, err := run.NewSession(d, run.Options{Dir: dir, Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.RunBlock(ctx, 2); err != nil {
		t.Fatal(err)
	}
	var sock, token string
	for deadline := time.Now().Add(5 * time.Second); ; {
		data, err := os.ReadFile(filepath.Join(dir, "call"))
		if err == nil {
			sock, token, _ = strings.Cut(strings.TrimSpace(string(data)), "\n")
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background block never wrote its token")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if resp, err := ctl.Call(sock, token, "set", ctl.SetArgs{Name: "host", Value: "db1"}); err != nil || !resp.OK {
		t.Fatalf("set host: %+v, %v", resp, err)
	}
	if resp, err := ctl.Call(sock, token, "set", ctl.SetArgs{Name: "user", Value: "root"}); err != nil || resp.OK || !strings.Contains(resp.Error, "belongs to an input") {
		t.Fatalf("set user: %+v, %v", resp, err)
	}
	os.WriteFile(filepath.Join(dir, "go"), nil, 0o644)
	if err := s.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if v, ok := d.Blocks[0].SessionValue(); !ok || v != "db1" {
		t.Errorf("host = %q, %v", v, ok)
	}
	if string(d.Src) != "```textbox {#host var=host nosave}\ndefault\n```\n\n```textbox {#user var=user}\nme\n```\n\n"+
		"```sh {#bg background}\nprintf '%s\\n%s\\n' \"$MDSH_SOCK\" \"$MDSH_TOKEN\" > call.tmp && mv call.tmp call\nwhile [ ! -e go ]; do sleep 0.02; done\n```\n\n"+
		"```sh {#later run={{$MDSH_VAR_host}}==db1}\necho \"later: $MDSH_VAR_host\"\n```\n" {
		t.Errorf("source changed: %q", d.Src)
	}
	r, err := s.RunBlock(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Ran || string(r.Stdout) != "later: db1\n" {
		t.Errorf("later: ran %v, printed %q", r.Ran, r.Stdout)
	}
}

// TestSecretVars checks that SecretVars names the variables set with
// --secret, sorted, and not the others.
func TestSecretVars(t *testing.T) {
	dir := t.TempDir()
	d := parse(t, "```sh {#bg background}\nprintf '%s\\n%s\\n' \"$MDSH_SOCK\" \"$MDSH_TOKEN\" > call.tmp && mv call.tmp call\nwhile [ ! -e go ]; do sleep 0.02; done\n```\n")
	s, err := run.NewSession(d, run.Options{Dir: dir, Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.RunBlock(ctx, 0); err != nil {
		t.Fatal(err)
	}
	var sock, token string
	for deadline := time.Now().Add(5 * time.Second); ; {
		data, err := os.ReadFile(filepath.Join(dir, "call"))
		if err == nil {
			sock, token, _ = strings.Cut(strings.TrimSpace(string(data)), "\n")
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the background block never wrote its token")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, a := range []ctl.SetArgs{{Name: "zeta", Value: "z", Secret: true}, {Name: "plain", Value: "p"}, {Name: "alpha", Value: "a", Secret: true}} {
		if resp, err := ctl.Call(sock, token, "set", a); err != nil || !resp.OK {
			t.Fatalf("set %s: %+v, %v", a.Name, resp, err)
		}
	}
	os.WriteFile(filepath.Join(dir, "go"), nil, 0o644)
	if err := s.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(s.SecretVars(), " "); got != "MDSH_VAR_alpha MDSH_VAR_zeta" {
		t.Errorf("SecretVars = %q", got)
	}
}

func TestNamesAny(t *testing.T) {
	names := []string{"MDSH_VAR_token"}
	for code, want := range map[string]bool{
		"curl -H \"$MDSH_VAR_token\"":            true,
		"echo ${MDSH_VAR_token}":                 true,
		"echo {{$MDSH_VAR_token}}":               true,
		"echo $MDSH_VAR_token_hint":              false,
		"echo $XMDSH_VAR_token":                  false,
		"echo $MDSH_VAR_tok":                     false,
		"echo $MDSH_VAR_token_x $MDSH_VAR_token": true,
	} {
		if got := run.NamesAny(code, names); got != want {
			t.Errorf("NamesAny(%q) = %v, want %v", code, got, want)
		}
	}
}

// TestCtlQuestions checks that ask and confirm from a block, and a
// confirm block, are pending questions with ids while they are asked,
// that the answers reach the block, and that a question is withdrawn
// when its block times out.
func TestCtlQuestions(t *testing.T) {
	d := parse(t, "```sh {#a}\n"+
		"echo \"$(mdsh_ctl ask --choice x,y 'Pick?')\"\n"+
		"mdsh_ctl confirm 'Go?'; echo \"confirm=$?\"\n"+
		"```\n\n"+
		"```sh {#b confirm=\"Sure?\"}\necho b\n```\n\n"+
		"```sh {#c timeout=300ms}\nmdsh_ctl ask 'Slow?'; echo \"slow=$?\"\n```\n")
	var s *run.Session
	var seen []string
	opts := run.Options{Environ: []string{}, Ask: func(ctx context.Context, q run.Question) (string, run.Answered) {
		p := s.Pending()
		if len(p) != 1 || p[0].ID != q.ID {
			t.Errorf("pending while asking %s: %+v", q.ID, p)
		}
		seen = append(seen, q.ID+":"+string(q.Kind)+":"+q.Text)
		switch q.Text {
		case "Pick?":
			return "y", run.ByUser
		case "Go?":
			return ctl.NoValue, run.ByUser
		}
		<-ctx.Done()
		return "", run.ByNone
	}, Confirm: func(b *doc.Block) run.Answer {
		p := s.Pending()
		if len(p) != 1 || p[0].Kind != run.AskConfirm || p[0].Block != b || p[0].Text != "Sure?" {
			t.Errorf("pending while confirming: %+v", p)
		}
		seen = append(seen, p[0].ID+":"+string(p[0].Kind))
		return run.Go
	}}
	opts.Source = "mdsh_ctl() { " + ctlClient(t) + " \"$@\"; }"
	var err error
	if s, err = run.NewSession(d, opts); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.RunFrom(context.Background(), 0)
	r := s.Results()
	if got := string(r[0].Stdout); got != "y\nconfirm=1\n" {
		t.Errorf("a: %q %q", got, r[0].Stderr)
	}
	if !r[2].TimedOut {
		t.Errorf("c didn't time out: %+v", r[2])
	}
	want := "q1:ask:Pick? q2:ctl.confirm:Go? q3:confirm q4:ask:Slow?"
	if got := strings.Join(seen, " "); got != want {
		t.Errorf("asked %s, want %s", got, want)
	}
	// The withdrawn question comes off on the socket's goroutine.
	for deadline := time.Now().Add(5 * time.Second); len(s.Pending()) != 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("left pending: %+v", s.Pending())
		}
	}
}
