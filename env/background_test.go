//go:build unix

package env

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func waitJob(t *testing.T, j *Job) (int, error) {
	t.Helper()
	for range 250 {
		if status, done, err := j.Done(); done {
			return status, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job didn't end")
	return 0, nil
}

func TestBackground(t *testing.T) {
	sh, err := Start(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	ctx := context.Background()
	sh.Run(ctx, "X=1", io.Discard, io.Discard)
	j, err := sh.Background(ctx, "echo out $X; echo err >&2; X=2; exit 4", "PID")
	if err != nil {
		t.Fatal(err)
	}
	if status, err := waitJob(t, j); status != 4 || err != nil {
		t.Errorf("status %d, err %v; want 4", status, err)
	}
	if out, errs := j.Output(); string(out) != "out 1\n" || string(errs) != "err\n" {
		t.Errorf("output %q, %q", out, errs)
	}
	var buf bytes.Buffer
	sh.Run(ctx, `wait "$PID"; echo "$? $X"`, &buf, io.Discard)
	if buf.String() != "4 1\n" {
		t.Errorf("wait in the shell: %q, want 4 1", buf.String())
	}
}

func TestBackgroundKill(t *testing.T) {
	sh, err := Start(Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	j, err := sh.Background(context.Background(), "echo before; sleep 30", "")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	j.Kill()
	if status, err := waitJob(t, j); status != -1 || !errors.Is(err, ErrKilled) {
		t.Errorf("status %d, err %v; want killed", status, err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("kill took %s", d)
	}
	if out, _ := j.Output(); string(out) != "before\n" {
		t.Errorf("output %q", out)
	}
}
