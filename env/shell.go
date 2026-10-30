// Package env runs code in long-lived shell processes and stacks them
// into named environments.
package env

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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

// ErrExited is returned by Run when the block ended the shell, for
// example by calling exit. The shell can't run anything else.
var ErrExited = errors.New("shell exited")

// Shell is one long-lived shell process. Blocks run in it one at a time,
// so variables, functions and the working directory carry over.
type Shell struct {
	path   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mark   string
	tmp    string
	out    *demux
	errs   *demux
	waited chan struct{}
	pty    *ptyConn // nil unless started with Config.TTY
	ttyRun int      // blocks run on the pty, to tell their markers apart
	bgRun  int      // background jobs started, to name their files
	// pipes are the read ends of stdout and stderr.
	pipes []io.Closer
	exit  int
	mu    sync.Mutex // one Run at a time
}

// Terminal is a terminal mdsh draws itself, such as a pane in a window,
// for RunTerminal. It has no file of its own: keys arrive on a channel
// and what the script shows goes to a writer.
type Terminal struct {
	// Keys carries what is typed, as the bytes a terminal would send.
	Keys <-chan []byte
	// Out shows what the script writes to its terminal.
	Out io.Writer
	// Size is the terminal's size in cells. Nil, or a zero size, means
	// 80 by 24.
	Size func() (cols, rows int)
	// Resized, if not nil, says when Size changes.
	Resized <-chan struct{}
}

// Dims is t's size, or 80 by 24 if it has none.
func (t Terminal) Dims() (cols, rows int) {
	if t.Size != nil {
		if c, r := t.Size(); c > 0 && r > 0 {
			return c, r
		}
	}
	return 80, 24
}

// Config starts a shell.
type Config struct {
	Path string   // shell binary; defaults to /bin/sh
	Env  []string // KEY=value, the shell's whole environment
	Dir  string   // working directory
	Args []string // positional parameters, $1 and on
	// TTY gives the shell a pseudo-terminal of its own as its
	// controlling terminal, so RunTTY can run blocks on it.
	TTY bool
	// Init runs in the shell once it starts, before any block, such as
	// aliases and functions to define. Its exit status is ignored, but
	// an Init that ends the shell is an error.
	Init string
}

// Start launches a shell.
func Start(cfg Config) (*Shell, error) {
	if cfg.Path == "" {
		cfg.Path = "/bin/sh"
	}
	var rb [12]byte
	if _, err := rand.Read(rb[:]); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "mdsh-")
	if err != nil {
		return nil, err
	}
	s := &Shell{
		path:   cfg.Path,
		mark:   "__MDSH_" + hex.EncodeToString(rb[:]) + "__",
		tmp:    tmp,
		waited: make(chan struct{}),
	}
	s.cmd = exec.Command(cfg.Path)
	s.cmd.Env = cfg.Env
	s.cmd.Dir = cfg.Dir
	if cfg.TTY {
		if s.pty, err = openPTY(s.cmd); err != nil {
			os.RemoveAll(tmp)
			return nil, err
		}
	} else {
		ownGroup(s.cmd)
	}
	if s.stdin, err = s.cmd.StdinPipe(); err != nil {
		return nil, err
	}
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := s.cmd.Start(); err != nil {
		os.RemoveAll(tmp)
		if s.pty != nil {
			s.pty.close()
		}
		return nil, err
	}
	if s.pty != nil {
		s.pty.started(s.mark)
	}
	s.pipes = []io.Closer{stdout, stderr}
	s.out = newDemux(stdout, s.mark)
	s.errs = newDemux(stderr, s.mark)
	go func() {
		<-s.out.eof
		<-s.errs.eof
		err := s.cmd.Wait()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			s.exit = ee.ExitCode()
		}
		close(s.waited)
	}()

	init := "set -- " + quoteAll(cfg.Args) + "\n" + helpers
	if cfg.TTY {
		// The terminal stays the shell's controlling terminal, at
		// /dev/tty, without blocks inheriting the descriptor.
		init = "exec 3>&-\n" + init
	}
	if _, _, err := s.Run(context.Background(), init, io.Discard, io.Discard); err != nil {
		s.Close()
		return nil, fmt.Errorf("starting %s: %w", cfg.Path, err)
	}
	if cfg.Init != "" {
		var errb bytes.Buffer
		if _, _, err := s.Run(context.Background(), cfg.Init, io.Discard, &errb); err != nil {
			s.Close()
			if msg := strings.TrimSpace(errb.String()); msg != "" {
				err = fmt.Errorf("%w: %s", err, msg)
			}
			return nil, fmt.Errorf("sourcing files in %s: %w", cfg.Path, err)
		}
	}
	return s, nil
}

// Run runs script in the shell and returns its exit status. The script's
// stdin is /dev/null. If the script ends the shell, Run returns ErrExited
// and the shell's exit status. If ctx ends first, the shell is killed.
func (s *Shell) Run(ctx context.Context, script string, stdout, stderr io.Writer) (status int, exited bool, err error) {
	return s.run(ctx, script, stdout, stderr, false)
}

// run runs script, on the shell's terminal if onTTY is set.
func (s *Shell) run(ctx context.Context, script string, stdout, stderr io.Writer, onTTY bool) (status int, exited bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.waited:
		return s.exit, true, ErrExited
	default:
	}

	f := filepath.Join(s.tmp, "block.sh")
	if err := os.WriteFile(f, []byte(script), 0o600); err != nil {
		return 0, false, err
	}
	if ok, err := s.check(ctx, f, stderr); err != nil || !ok {
		return 2, false, err
	}

	s.out.attach(stdout)
	s.errs.attach(stderr)
	defer s.out.attach(nil)
	defer s.errs.attach(nil)

	cmd := fmt.Sprintf(". %s </dev/null\nprintf '%%s%%d\\n' '%s' \"$?\"\nprintf '%%s0\\n' '%s' >&2\n", quote(f), s.mark, s.mark)
	if onTTY {
		// The INT trap keeps ctrl+c from ending the shell along with the
		// command it is waiting for. Commands start with the default
		// action, since a trap isn't inherited.
		// A marker on the terminal too tells when all the block wrote
		// there has been read.
		s.ttyRun++
		cmd = fmt.Sprintf("trap : INT\n. %s </dev/tty >/dev/tty 2>/dev/tty\nprintf '%%s%%d\\n' '%s' \"$?\"\nprintf '%%s%%d\\n' '%s' %d >/dev/tty\ntrap - INT\nprintf '%%s0\\n' '%s' >&2\n", quote(f), s.mark, s.mark, s.ttyRun, s.mark)
	}
	if _, err := io.WriteString(s.stdin, cmd); err != nil {
		<-s.waited
		return s.exit, true, ErrExited
	}

	var outRC string
	gotOut, gotErr := false, false
	for !gotOut || !gotErr {
		select {
		case v, ok := <-s.out.done:
			if !ok {
				<-s.waited
				return s.exit, true, ErrExited
			}
			outRC, gotOut = v, true
		case _, ok := <-s.errs.done:
			if !ok {
				<-s.waited
				return s.exit, true, ErrExited
			}
			gotErr = true
		case <-ctx.Done():
			s.kill()
			<-s.waited
			return -1, true, ctx.Err()
		}
	}
	if onTTY {
		s.pty.wait(s.ttyRun)
	}
	rc, err := strconv.Atoi(outRC)
	if err != nil {
		return 0, false, fmt.Errorf("bad status %q", outRC)
	}
	return rc, false, nil
}

// kill ends the shell and whatever it started, and stops reading its
// output, so a child that outlives the kill can't keep the shell from
// being waited for.
func (s *Shell) kill() {
	kill(s.cmd)
	for _, p := range s.pipes {
		p.Close()
	}
}

// Exited reports whether the shell has ended, by a block calling exit or
// by being killed.
func (s *Shell) Exited() bool {
	select {
	case <-s.waited:
		return true
	default:
		return false
	}
}

// Exported returns the shell's exported variables.
func (s *Shell) Exported(ctx context.Context) (map[string]string, error) {
	// awk's ENVIRON is the exported environment it inherits. Escape
	// backslashes and newlines so every variable is one line.
	const script = `awk 'BEGIN { for (k in ENVIRON) { v = ENVIRON[k]; gsub(/\\/, "\\\\", v); gsub(/\n/, "\\n", v); print k "=" v } }'`
	var out, errb bytes.Buffer
	rc, _, err := s.Run(ctx, script, &out, &errb)
	if err != nil {
		return nil, err
	}
	if rc != 0 {
		return nil, fmt.Errorf("listing exported vars: %s", strings.TrimSpace(errb.String()))
	}
	vars := map[string]string{}
	for _, line := range strings.Split(out.String(), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || k == "" {
			continue
		}
		vars[k] = unescape(v)
	}
	// awk itself may add these.
	delete(vars, "AWKPATH")
	delete(vars, "AWKLIBPATH")
	return vars, nil
}

// Close ends the shell and removes its temp files.
func (s *Shell) Close() error {
	s.stdin.Close()
	select {
	case <-s.waited:
	default:
		s.kill()
		<-s.waited
	}
	if s.pty != nil {
		s.pty.close()
	}
	return os.RemoveAll(s.tmp)
}

func unescape(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			i++
			if v[i] == 'n' {
				b.WriteByte('\n')
			} else {
				b.WriteByte(v[i])
			}
			continue
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// quote single-quotes s for the shell.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func quoteAll(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = quote(a)
	}
	return strings.Join(q, " ")
}
