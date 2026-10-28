package run_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

func parse(t *testing.T, src string) *doc.Document {
	t.Helper()
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestSessionRunBlock(t *testing.T) {
	d := parse(t, "```sh {#a}\nN=$((${N:-0}+1)); echo $N\n```\n\n```sh {#b skip}\necho skipped\n```\n")
	s, err := run.NewSession(d, run.Options{Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	for _, want := range []string{"1\n", "2\n"} {
		r, err := s.RunBlock(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if string(r.Stdout) != want {
			t.Errorf("a: got %q, want %q (the env lives between runs)", r.Stdout, want)
		}
		if s.Results()[0] != r {
			t.Error("Results doesn't hold the new result")
		}
	}

	// Asked for by name, a skip block runs.
	if r, _ := s.RunBlock(ctx, 1); !r.Ran || string(r.Stdout) != "skipped\n" {
		t.Errorf("b: ran=%v stdout=%q", r.Ran, r.Stdout)
	}
	// RunFrom honors skip.
	if err := s.RunFrom(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if r := s.Results()[1]; r.Ran || !r.Skipped {
		t.Errorf("RunFrom ran a skip block")
	}
	if got := string(s.Results()[0].Stdout); got != "3\n" {
		t.Errorf("RunFrom: a got %q, want 3", got)
	}
}

func TestSessionRunBlockConfirm(t *testing.T) {
	d := parse(t, "```sh {#a confirm}\necho hi\n```\n")
	s, err := run.NewSession(d, run.Options{Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RunBlock(context.Background(), 0); !errors.Is(err, run.ErrNoConfirm) {
		t.Errorf("err = %v, want ErrNoConfirm", err)
	}

	s, _ = run.NewSession(d, run.Options{Environ: []string{}, Confirm: func(*doc.Block) run.Answer { return run.Skip }})
	defer s.Close()
	if r, err := s.RunBlock(context.Background(), 0); err != nil || r.Ran {
		t.Errorf("skipped at the prompt: ran=%v err=%v", r.Ran, err)
	}
}

func TestSessionStopAndGoOn(t *testing.T) {
	d := parse(t, "```sh {#slow}\necho start; sleep 30; echo end\n```\n\n```sh {#next}\necho next\n```\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := run.NewSession(d, run.Options{Environ: []string{"PATH=/usr/bin:/bin"}, OnEvent: func(e run.Event) {
		// Output streams in as it is written, so this sees "start"
		// while the block sleeps.
		if e.Kind == run.Stdout && string(e.Data) == "start\n" {
			cancel()
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	begin := time.Now()
	r, err := s.RunBlock(ctx, 0)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want canceled", err)
	}
	if took := time.Since(begin); took > 10*time.Second {
		t.Errorf("stopping took %s; the sleep kept running", took)
	}
	if string(r.Stdout) != "start\n" {
		t.Errorf("stdout %q", r.Stdout)
	}
	// The stopped block's shell is gone; the next block gets a new one.
	r, err = s.RunBlock(context.Background(), 1)
	if err != nil || string(r.Stdout) != "next\n" {
		t.Errorf("next: stdout %q, err %v, result err %v", r.Stdout, err, r.Err)
	}
}

func TestSessionGrow(t *testing.T) {
	d := parse(t, "```sh {#a}\nX=1\n```\n")
	s, err := run.NewSession(d, run.Options{Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RunBlock(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AppendBlock("sh", "echo x=$X"); err != nil {
		t.Fatal(err)
	}
	s.Grow()
	r, err := s.RunBlock(context.Background(), 1)
	if err != nil || string(r.Stdout) != "x=1\n" {
		t.Errorf("appended block: stdout %q, err %v", r.Stdout, err)
	}
}
