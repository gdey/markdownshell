//go:build unix

package env

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// BackgroundTerminal starts script as a background job, as Background
// does, but on a pseudo-terminal of its own rather than with no
// terminal at all: what is typed on t goes to the script, and what the
// script shows goes to t.Out and to rec, until the job ends. The
// terminal takes t's size, and follows it.
//
// The job can't be a subshell of the shell, since a program such as
// vim or ssh needs its terminal to be its session's controlling
// terminal, and a subshell is in the shell's session. So it runs in a
// new shell of the same kind, in a session of its own, that first takes
// on the shell's state: its exported variables through the environment,
// and its other variables, functions, aliases, options, positional
// parameters and working directory as stateScript lists them. In a
// shell other than bash and zsh, functions don't carry over. The job
// isn't the shell's child, so the shell can't wait for it; pidVar still
// holds its process ID, for kill.
//
// A script with a syntax error doesn't start: the job is already done,
// with status 2 and the error as its stderr.
func (s *Shell) BackgroundTerminal(ctx context.Context, script, pidVar string, t Terminal, rec io.Writer) (*Job, error) {
	if s.ia != nil {
		return nil, errors.New("background blocks need a scripted shell, not an interactive one")
	}
	s.mu.Lock()
	s.bgRun++
	dir := filepath.Join(s.tmp, "bg"+strconv.Itoa(s.bgRun))
	s.mu.Unlock()
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, err
	}
	j := &Job{sh: s, dir: dir}
	block := filepath.Join(dir, "block.sh")
	if err := os.WriteFile(block, []byte(script), 0o600); err != nil {
		return nil, err
	}
	var syn bytes.Buffer
	ok, err := s.check(ctx, block, &syn)
	if err != nil {
		return nil, err
	}
	if !ok {
		os.WriteFile(filepath.Join(dir, "stderr"), syn.Bytes(), 0o600)
		j.done, j.status = true, 2
		return j, nil
	}

	state, opts := filepath.Join(dir, "state.sh"), filepath.Join(dir, "opts.sh")
	var errb bytes.Buffer
	if _, exited, err := s.Run(ctx, stateScript(state, opts), io.Discard, &errb); err != nil || exited {
		return nil, fmt.Errorf("starting background job: copying the shell's state: %v %s", err, strings.TrimSpace(errb.String()))
	}
	vars, err := s.Exported(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting background job: %w", err)
	}
	set, err := os.ReadFile(opts)
	if err != nil {
		return nil, fmt.Errorf("starting background job: copying the shell's options: %w", err)
	}
	runner := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(runner, []byte(runnerScript(state, set, block)), 0o600); err != nil {
		return nil, err
	}

	ptm, pts, err := pty.Open()
	if err != nil {
		return nil, err
	}
	resize := func() {
		cols, rows := t.Dims()
		pty.Setsize(ptm, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}
	resize()
	cmd := exec.Command(s.path, runner)
	cmd.Env = environ(vars)
	cmd.Dir = s.dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pts, pts, pts
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		ptm.Close()
		pts.Close()
		return nil, fmt.Errorf("starting background job: %w", err)
	}
	// Only the job holds the terminal's other end now, so reading ends
	// once it, and anything it left running there, is gone.
	pts.Close()
	j.PID = cmd.Process.Pid
	j.term = &termJob{cmd: cmd, ended: make(chan struct{})}

	out := io.MultiWriter(t.Out, rec)
	read := make(chan struct{})
	go func() {
		defer close(read)
		buf := make([]byte, 32*1024)
		for {
			n, err := ptm.Read(buf)
			if n > 0 {
				out.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	stop := make(chan struct{})
	go func() {
		keys, resized := t.Keys, t.Resized
		for {
			select {
			case <-stop:
				return
			case k, ok := <-keys:
				if !ok {
					keys = nil
					continue
				}
				ptm.Write(k)
			case <-resized:
				resize()
			}
		}
	}()
	go func() {
		err := cmd.Wait()
		status := 0
		var ee *exec.ExitError
		switch {
		case errors.As(err, &ee) && ee.ExitCode() >= 0:
			status = ee.ExitCode()
		case err != nil:
			status = -1
		}
		// What the job wrote last may still be on its way; something it
		// left running on the terminal may keep it open for good.
		select {
		case <-read:
		case <-time.After(termDrain):
		}
		close(stop)
		ptm.Close()
		j.mu.Lock()
		if status >= 0 {
			j.done, j.status = true, status
		} else {
			j.killed = true
		}
		j.mu.Unlock()
		s.forgetTermJob(j)
		close(j.term.ended)
	}()

	s.jobMu.Lock()
	if s.termJobs == nil {
		s.termJobs = map[int]*Job{}
	}
	s.termJobs[j.PID] = j
	s.jobMu.Unlock()

	if pidVar != "" {
		s.Run(ctx, pidVar+"="+strconv.Itoa(j.PID)+"\n", io.Discard, io.Discard)
	}
	return j, nil
}

// termDrain bounds how long a BackgroundTerminal job waits, once its
// process has ended, for what it wrote to be read.
const termDrain = 500 * time.Millisecond

// environ is vars as KEY=value, in order.
func environ(vars map[string]string) []string {
	env := make([]string, 0, len(vars))
	for k, v := range vars {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	return env
}

// killSession ends the job's session, everything on its terminal, as
// well as what it started elsewhere.
func killSession(pid int) {
	syscall.Kill(-pid, syscall.SIGKILL)
	killTree(pid)
}
