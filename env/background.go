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
	"strconv"
	"strings"
	"sync"
)

// Job is a script running in the background of its shell, started by
// Background. Its output goes to files, so it never mixes with the
// output of what the shell runs next.
type Job struct {
	// PID is the background job's process ID, as $! gave it.
	PID int
	sh  *Shell
	dir string

	mu     sync.Mutex
	killed bool
	// done is set once the job's status is known.
	done   bool
	status int
}

// Background starts script as a background job of the shell, the way
// `script &` would: in a subshell, so it sees the shell's variables,
// functions and working directory but what it sets stays its own. Its
// stdin is /dev/null and its stdout and stderr are kept apart from the
// shell's. pidVar, if not empty, is a shell variable set to the job's
// process ID, so later scripts can wait for it.
//
// A script with a syntax error doesn't start: the job is already done,
// with status 2 and the error as its stderr.
func (s *Shell) Background(ctx context.Context, script, pidVar string) (*Job, error) {
	s.mu.Lock()
	s.bgRun++
	dir := filepath.Join(s.tmp, "bg"+strconv.Itoa(s.bgRun))
	s.mu.Unlock()
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, err
	}
	j := &Job{sh: s, dir: dir}
	f := filepath.Join(dir, "block.sh")
	if err := os.WriteFile(f, []byte(script), 0o600); err != nil {
		return nil, err
	}
	var syn bytes.Buffer
	ok, err := s.check(ctx, f, &syn)
	if err != nil {
		return nil, err
	}
	if !ok {
		os.WriteFile(filepath.Join(dir, "stderr"), syn.Bytes(), 0o600)
		j.done, j.status = true, 2
		return j, nil
	}

	// The status is written last, once the script has ended, so its
	// file says the job is done, and the job exits with it for wait. A
	// script that calls exit ends only the inner subshell.
	launch := fmt.Sprintf("{ ( . %s ); _mdsh_rc=$?; echo $_mdsh_rc >%s; exit $_mdsh_rc; } >%s 2>%s </dev/null &\n",
		quote(f), quote(filepath.Join(dir, "status")), quote(filepath.Join(dir, "stdout")), quote(filepath.Join(dir, "stderr")))
	if pidVar != "" {
		launch += pidVar + "=$!\n"
	}
	launch += "printf '%s' \"$!\"\n"
	var out, errb bytes.Buffer
	rc, exited, err := s.Run(ctx, launch, &out, &errb)
	switch {
	case err != nil:
		return nil, err
	case exited || rc != 0:
		return nil, fmt.Errorf("starting background job: %s", strings.TrimSpace(errb.String()))
	}
	if j.PID, err = strconv.Atoi(out.String()); err != nil {
		return nil, fmt.Errorf("starting background job: bad pid %q", out.String())
	}
	return j, nil
}

// ErrKilled is a job's error when Kill ended it, or its shell ended
// first.
var ErrKilled = errors.New("killed")

// Done reports whether the job has ended, and its exit status. A job
// that was killed, or whose shell ended, has status -1 and ErrKilled.
func (j *Job) Done() (status int, done bool, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.done {
		b, rerr := os.ReadFile(filepath.Join(j.dir, "status"))
		// The status is one line; an empty or partial file is still
		// being written.
		if rerr == nil && bytes.HasSuffix(b, []byte("\n")) {
			if n, aerr := strconv.Atoi(strings.TrimSpace(string(b))); aerr == nil {
				j.done, j.status = true, n
			}
		}
	}
	switch {
	case j.done:
		return j.status, true, nil
	case j.killed:
		return -1, true, ErrKilled
	case j.sh.Exited():
		// A block that called exit leaves the job running on its own.
		j.killed = true
		killTree(j.PID)
		return -1, true, ErrKilled
	}
	return 0, false, nil
}

// Output returns what the job has written so far.
func (j *Job) Output() (stdout, stderr []byte) {
	stdout, _ = os.ReadFile(filepath.Join(j.dir, "stdout"))
	stderr, _ = os.ReadFile(filepath.Join(j.dir, "stderr"))
	return stdout, stderr
}

// Kill ends the job and every process it started, unless it is done.
func (j *Job) Kill() {
	if _, done, _ := j.Done(); done {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.killed = true
	killTree(j.PID)
}

// closeBraceHint follows zsh's error for a lone } anywhere on a line,
// such as echo }: unlike sh, zsh takes it as the end of a { } group even
// as an argument. setopt ignore_close_braces can't help, since check
// parses the block without running it.
const closeBraceHint = "mdsh: zsh reads a lone } as the end of a { } group, even as an argument; quote it, as in echo '}'\n"

// check checks the syntax of the script in file f in a separate
// process, since a syntax error in a dot script ends a non-interactive
// POSIX shell. It writes the error, if any, to w, naming the file
// "block".
func (s *Shell) check(ctx context.Context, f string, w io.Writer) (ok bool, err error) {
	var syn bytes.Buffer
	chk := exec.CommandContext(ctx, s.path, "-n", f)
	chk.Stdout, chk.Stderr = &syn, &syn
	if err := chk.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := bytes.ReplaceAll(syn.Bytes(), []byte(f), []byte("block"))
			w.Write(msg)
			if filepath.Base(s.path) == "zsh" && bytes.Contains(msg, []byte("parse error near `}'")) {
				io.WriteString(w, closeBraceHint)
			}
			return false, nil
		}
		return false, err
	}
	return true, nil
}
