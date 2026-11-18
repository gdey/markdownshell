package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	label   string // the block's name, as Processes shows it
	timeout string // the block's timeout=, or ""
	timer   *time.Timer
	revoke  func() // stops the block's $MDSH_TOKEN working
	start   time.Time
	// timedOut is set when timeout= killed the job, overdue when the
	// run's end did, and stopped when StopBackground did.
	timedOut, overdue, stopped atomic.Bool
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
		s.logEnd(b, r, time.Now())
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
	j := &bgJob{r: r, job: job, label: s.d.Label(b), timeout: t, revoke: revoke, start: time.Now()}
	if hasTimeout {
		j.timer = time.AfterFunc(dur, func() {
			j.timedOut.Store(true)
			job.Kill()
		})
	}
	s.jobsMu.Lock()
	s.jobs = append(s.jobs, j)
	s.jobsMu.Unlock()
}

// running returns the background blocks still running.
func (s *Session) running() []*bgJob {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	return slices.Clone(s.jobs)
}

// Processes lists the session's shells, each with the processes under
// it, as env.Stack.Processes does. It may be called while a run goes.
// Each background block's process is named by its block.
func (s *Session) Processes() []env.ShellProcs {
	all := s.stack.Processes()
	names := map[int]string{}
	for _, j := range s.running() {
		names[j.job.PID] = j.label
	}
	var name func(ps []*env.Proc)
	name = func(ps []*env.Proc) {
		for _, p := range ps {
			p.Block = names[p.PID]
			name(p.Children)
		}
	}
	for _, sh := range all {
		name(sh.Children)
	}
	return all
}

// KillProcess kills process pid and what it started, if it runs under
// one of the session's shells, and reports whether it did. A background
// block it ends is reported killed by the next Reap.
func (s *Session) KillProcess(pid int) bool {
	// A background block's own process is stopped as the block, so its
	// job knows it was killed rather than waiting for a status that
	// won't come.
	for _, j := range s.running() {
		if j.job.PID == pid {
			j.stopped.Store(true)
			j.job.Kill()
			return true
		}
	}
	return s.stack.KillProcess(pid)
}

// StopBackground kills background block b, if it is still running, and
// reports whether it was. Its result, stopped, comes with the next Reap.
func (s *Session) StopBackground(b *doc.Block) bool {
	for _, j := range s.running() {
		if j.r.Block == b {
			j.stopped.Store(true)
			j.job.Kill()
			return true
		}
	}
	return false
}

// Reap finishes the results of background blocks that have ended, and
// reports each with a Done event. Run calls it before each block and
// at the end; a front end running blocks one at a time can call it to
// catch up while nothing runs.
func (s *Session) Reap() {
	type end struct {
		j      *bgJob
		status int
		err    error
	}
	var ended []end
	s.jobsMu.Lock()
	running := s.jobs[:0]
	for _, j := range s.jobs {
		status, done, err := j.job.Done()
		if !done {
			running = append(running, j)
			continue
		}
		ended = append(ended, end{j, status, err})
	}
	clear(s.jobs[len(running):])
	s.jobs = running
	s.jobsMu.Unlock()
	// Done events go out without the lock, as what follows them may
	// stop another block.
	for _, e := range ended {
		s.finish(e.j, e.status, e.err)
	}
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
		jobs := s.running()
		if len(jobs) == 0 {
			return nil
		}
		for _, j := range jobs {
			if j.timer == nil && time.Since(end) >= BackgroundWait {
				j.overdue.Store(true)
				j.job.Kill()
			}
		}
		select {
		case <-ctx.Done():
			for _, j := range jobs {
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
			r.TimedOut = true
			err = fmt.Errorf("timed out after %s", j.timeout)
		case j.overdue.Load():
			err = fmt.Errorf("still running %s after the last block, so it was killed", BackgroundWait)
		case j.stopped.Load():
			err = errors.New("stopped")
		default:
			err = errors.New("killed before it ended")
		}
	}
	r.Err = err
	if kind, _ := outKind(b); (kind == "image" || kind == "screenshot") && r.Err == nil && len(r.Stdout) > 0 {
		r.Image, r.Err = saveImage(b, r.Stdout, s.opts)
	}
	s.logEnd(b, r, j.start)
	s.emit(Event{Kind: Done, Block: b, Result: r})
}
