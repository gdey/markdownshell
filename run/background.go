package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/env"
)

// BackgroundWait is how long a run waits, after its last block, for a
// background block with no timeout= to end before killing it.
const BackgroundWait = 5 * time.Second

// bgJob is a background block that is still running.
type bgJob struct {
	r       *Result
	job     *env.Job
	timeout string // the block's timeout=, or ""
	timer   *time.Timer
	revoke  func() // stops the block's $MDSH_TOKEN working
	// timedOut is set when timeout= killed the job, and overdue when
	// the run's end did.
	timedOut, overdue atomic.Bool
}

// PIDVar is the shell variable that holds the process ID of background
// block id, so later blocks can wait for it: MDSH_BG_ and the id, with
// anything but letters, digits and _ turned into _.
func PIDVar(id string) string {
	return "MDSH_BG_" + strings.Map(func(r rune) rune {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, id)
}

// background starts b in the background of its env's shell. The run
// goes on at once; r is Running until Reap sees the block end.
func (s *Session) background(ctx context.Context, b *doc.Block, r *Result) {
	r.Ran = true
	done := func(err error) {
		r.Err = err
		s.emit(Event{Kind: Done, Block: b, Result: r})
	}
	e, err := envFor(ctx, s.stack, s.d, b)
	if err != nil {
		done(err)
		return
	}
	var dur time.Duration
	t, hasTimeout := b.Attrs["timeout"]
	if hasTimeout {
		if dur, err = time.ParseDuration(t); err != nil {
			done(fmt.Errorf("timeout: %w", err))
			return
		}
	}
	pre, revoke := s.issue(b)
	job, err := e.Shell.Background(ctx, pre+b.Body, PIDVar(b.ID))
	if err != nil {
		revoke()
		done(err)
		return
	}
	r.Running = true
	j := &bgJob{r: r, job: job, timeout: t, revoke: revoke}
	if hasTimeout {
		j.timer = time.AfterFunc(dur, func() {
			j.timedOut.Store(true)
			job.Kill()
		})
	}
	s.jobs = append(s.jobs, j)
}

// Reap finishes the results of background blocks that have ended, and
// reports each with a Done event. Run calls it before each block and
// at the end; a front end running blocks one at a time can call it to
// catch up while nothing runs.
func (s *Session) Reap() {
	running := s.jobs[:0]
	for _, j := range s.jobs {
		status, done, err := j.job.Done()
		if !done {
			running = append(running, j)
			continue
		}
		s.finish(j, status, err)
	}
	clear(s.jobs[len(running):])
	s.jobs = running
}

// Wait waits for the background blocks still running, each until its
// timeout= or, without one, for BackgroundWait, and kills those that
// don't end in time. If ctx ends first, it kills them all.
func (s *Session) Wait(ctx context.Context) error {
	end := time.Now()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		s.Reap()
		if len(s.jobs) == 0 {
			return nil
		}
		for _, j := range s.jobs {
			if j.timer == nil && time.Since(end) >= BackgroundWait {
				j.overdue.Store(true)
				j.job.Kill()
			}
		}
		select {
		case <-ctx.Done():
			for _, j := range s.jobs {
				j.job.Kill()
			}
			s.Reap()
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// finish fills in j's result now that it has ended.
func (s *Session) finish(j *bgJob, status int, err error) {
	if j.timer != nil {
		j.timer.Stop()
	}
	j.revoke()
	r, b := j.r, j.r.Block
	r.Running = false
	r.Stdout, r.Stderr = j.job.Output()
	if kind, _ := outKind(b); kind != "image" {
		r.Stdout, r.Stderr = Overstrike(r.Stdout), Overstrike(r.Stderr)
	}
	r.Status = status
	if errors.Is(err, env.ErrKilled) {
		switch {
		case j.timedOut.Load():
			err = fmt.Errorf("timed out after %s", j.timeout)
		case j.overdue.Load():
			err = fmt.Errorf("still running %s after the last block, so it was killed", BackgroundWait)
		default:
			err = errors.New("killed before it ended")
		}
	}
	r.Err = err
	if kind, _ := outKind(b); (kind == "image" || kind == "screenshot") && r.Err == nil && len(r.Stdout) > 0 {
		r.Image, r.Err = saveImage(b, r.Stdout, s.opts)
	}
	s.emit(Event{Kind: Done, Block: b, Result: r})
}
