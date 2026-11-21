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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// phase is where an interactive shell is between its marks.
type phase int

const (
	phaseStarting phase = iota // no prompt yet
	phasePrompt                // A: drawing the prompt
	phaseInput                 // B: at the prompt, reading a line
	phaseRunning               // C: running a command
	phaseDone                  // D: command ended, prompt to come
)

// interactive is the state of a shell started with Config.Interactive:
// the mdsh end of its terminal and what the marks and the state fifo
// have said.
type interactive struct {
	kind    shellKind
	ptm     *os.File
	ttyName string
	state   *os.File // the state fifo, held open for reading and writing

	mu    sync.Mutex
	cond  *sync.Cond // signalled on every mark and state record
	phase phase
	// dirty is set when keys were typed at the prompt since the last
	// B, so a line may be half typed.
	dirty bool
	// view gets the whole stream, as a terminal would show it.
	view io.Writer
	// out gets a command's output, C to D, for the RunLine waiting on
	// it; typed says RunLine typed a line and waits for its end.
	out     io.Writer
	typed   bool
	ended   bool
	status  int
	echo    bytes.Buffer // what was echoed after B, the command line
	marksD  int          // mdsh D marks seen
	states  int          // state records read
	last    State
	lastCmd string // the command line zsh's preexec sent
	readEOF bool
	watch   func(Event)
	scan    markScanner
}

// Event is something an interactive shell did, for Shell.Watch.
type Event struct {
	// Kind is "start" when a command starts, with Command; "end" when
	// it ends, with Status; "state" when the shell sent its state, at
	// each prompt.
	Kind    string
	Command string
	Status  int
	State   *State
}

// startTimeout bounds how long Start waits for an interactive shell's
// first prompt, as slow rc files can take a while.
var startTimeout = 30 * time.Second

// stateWait bounds how long RunLine waits, after a command ended, for
// the state the shell sends with its next prompt.
const stateWait = 2 * time.Second

func startInteractive(ctx context.Context, s *Shell, cfg Config, init string, launched func(*Shell)) error {
	kind, err := kindOf(cfg.Path)
	if err != nil {
		return err
	}
	if cfg.Env == nil {
		cfg.Env = os.Environ()
	}
	args, extra, err := writeIntegration(kind, s.tmp, cfg, init)
	if err != nil {
		return err
	}
	fifo := filepath.Join(s.tmp, "state")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		return err
	}
	// Opened for reading and writing, so neither the open nor a
	// hook's write blocks, and the reader sees no end between writes.
	state, err := os.OpenFile(fifo, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	ptm, pts, err := pty.Open()
	if err != nil {
		state.Close()
		return err
	}
	pty.Setsize(ptm, &pty.Winsize{Cols: 80, Rows: 24})

	environ := cfg.Env
	if _, ok := lookup(environ, "TERM"); !ok {
		environ = setEnv(environ, "TERM=xterm-256color")
	}
	for _, kv := range extra {
		environ = setEnv(environ, kv)
	}
	if kind == kindZsh {
		if _, ok := lookup(extra, "MDSH_USER_ZDOTDIR"); !ok {
			environ = unsetEnv(environ, "MDSH_USER_ZDOTDIR")
		}
	}
	s.cmd = exec.Command(cfg.Path, args...)
	s.cmd.Env = environ
	s.cmd.Dir = cfg.Dir
	s.cmd.Stdin, s.cmd.Stdout, s.cmd.Stderr = pts, pts, pts
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := s.cmd.Start(); err != nil {
		ptm.Close()
		pts.Close()
		state.Close()
		return err
	}
	ia := &interactive{kind: kind, ptm: ptm, ttyName: pts.Name(), state: state, status: -1}
	ia.cond = sync.NewCond(&ia.mu)
	ia.scan = markScanner{text: ia.text, mark: ia.onMark}
	s.ia = ia
	// The shell has its own copies; mdsh keeps none, so reading ends
	// once the shell and its jobs have all let go of the terminal.
	pts.Close()

	go ia.read()
	go readRecords(state, ia.onRecord)
	go func() {
		err := s.cmd.Wait()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			s.exit = ee.ExitCode()
		}
		close(s.waited)
		ia.mu.Lock()
		ia.cond.Broadcast()
		ia.mu.Unlock()
	}()

	if launched != nil {
		launched(s)
	}

	// Wait for the first prompt and the state sent with it; ending ctx
	// first kills the shell.
	var startOut bytes.Buffer
	ia.mu.Lock()
	ia.view = &startOut
	ia.mu.Unlock()
	deadline := time.Now().Add(startTimeout)
	stop := time.AfterFunc(startTimeout, func() {
		ia.mu.Lock()
		ia.cond.Broadcast()
		ia.mu.Unlock()
	})
	defer stop.Stop()
	unwatch := context.AfterFunc(ctx, func() {
		ia.mu.Lock()
		ia.cond.Broadcast()
		ia.mu.Unlock()
	})
	defer unwatch()
	ia.mu.Lock()
	for !(ia.phase == phaseInput && ia.states > 0) && !s.Exited() && time.Now().Before(deadline) && ctx.Err() == nil {
		ia.cond.Wait()
	}
	ok := ia.phase == phaseInput && ia.states > 0
	ia.view = nil
	ia.mu.Unlock()
	if !ok {
		msg := strings.TrimSpace(stripEsc(startOut.String()))
		if ctx.Err() != nil {
			s.kill()
			return ctx.Err()
		}
		if s.Exited() {
			return fmt.Errorf("%s -i exited as it started: %s", filepath.Base(cfg.Path), msg)
		}
		s.kill()
		return fmt.Errorf("%s -i showed no prompt within %s: %s", filepath.Base(cfg.Path), startTimeout, msg)
	}
	return nil
}

// read copies the terminal's stream through the mark scanner until the
// terminal closes.
func (ia *interactive) read() {
	buf := make([]byte, 32*1024)
	for {
		n, err := ia.ptm.Read(buf)
		if n > 0 {
			ia.mu.Lock()
			ia.scan.Write(buf[:n])
			ia.mu.Unlock()
		}
		if err != nil {
			ia.mu.Lock()
			ia.readEOF = true
			ia.cond.Broadcast()
			ia.mu.Unlock()
			return
		}
	}
}

// text takes the stream between marks; ia.mu is held.
func (ia *interactive) text(p []byte) {
	if ia.view != nil {
		ia.view.Write(p)
	}
	switch ia.phase {
	case phaseInput:
		if ia.kind == kindSh {
			// sh has no C mark: the command starts where its echoed
			// line ends.
			if i := bytes.IndexByte(p, '\n'); i >= 0 {
				ia.echo.Write(p[:i+1])
				ia.start()
				if rest := p[i+1:]; len(rest) > 0 && ia.out != nil {
					ia.out.Write(rest)
				}
				return
			}
		}
		ia.echo.Write(p)
	case phaseRunning:
		if ia.out != nil {
			ia.out.Write(p)
		}
	}
}

// writeView passes a mark on to the view, which is a terminal that
// may want it too.
func (ia *interactive) writeView(mk mark) {
	if ia.view == nil {
		return
	}
	if mk.kind == markDone {
		fmt.Fprintf(ia.view, "\x1b]133;D;%d\x07", mk.status)
	} else {
		fmt.Fprintf(ia.view, "\x1b]133;%c\x07", mk.kind)
	}
}

// onMark handles one of mdsh's marks; ia.mu is held.
func (ia *interactive) onMark(mk mark) {
	ia.writeView(mk)
	defer ia.cond.Broadcast()
	switch mk.kind {
	case markPrompt:
		if ia.phase == phaseRunning && ia.typed {
			// A command mdsh typed ends at its D, which the hooks
			// always write; an A in its output is the prompt printed,
			// as by a script that expands PS1.
			return
		}
		if ia.phase == phaseRunning {
			// The D was lost, as when a ctrl+c threw away the end of
			// a line: the prompt alone ends the command.
			ia.end(-1)
		}
		ia.phase = phasePrompt
	case markInput:
		if ia.phase == phaseRunning && ia.typed {
			return
		}
		ia.phase = phaseInput
		ia.dirty = false
		ia.echo.Reset()
	case markCommand:
		if ia.phase == phaseInput {
			ia.start()
		}
	case markDone:
		ia.marksD++
		if ia.phase == phaseRunning || (ia.phase == phaseInput && ia.typed) {
			ia.end(mk.status)
		}
		ia.phase = phaseDone
	}
}

func (ia *interactive) start() {
	ia.phase = phaseRunning
	cmd := ia.lastCmd
	ia.lastCmd = ""
	if cmd == "" {
		cmd = strings.TrimSpace(stripEsc(ia.echo.String()))
	}
	if ia.watch != nil {
		ia.watch(Event{Kind: "start", Command: cmd})
	}
}

func (ia *interactive) end(status int) {
	ia.status = status
	ia.ended = true
	ia.phase = phaseDone
	if ia.watch != nil {
		ia.watch(Event{Kind: "end", Status: status})
	}
}

func (ia *interactive) onRecord(r record) {
	ia.mu.Lock()
	defer ia.mu.Unlock()
	if r.cmd != nil {
		ia.lastCmd = *r.cmd
		return
	}
	ia.states++
	ia.last = *r.state
	if ia.watch != nil {
		st := *r.state
		ia.watch(Event{Kind: "state", State: &st, Status: st.Status})
	}
	ia.cond.Broadcast()
}

// ErrBusy is returned by RunLine when the shell is running a command,
// so it isn't at a prompt to type at.
var ErrBusy = errors.New("the shell is running a command")

// ErrLineTyped is returned by RunLine when keys were typed at the
// prompt since it was drawn, so a line may be half typed.
var ErrLineTyped = errors.New("a line is being typed at the prompt")

// ErrNotInteractive is returned by the methods that need a shell
// started with Config.Interactive.
var ErrNotInteractive = errors.New("shell isn't interactive")

// promptWait bounds how long RunLine waits for the prompt after a
// command has ended and before the shell has drawn its next one.
const promptWait = 5 * time.Second

// runLine types line at the prompt and waits for the command to end,
// copying its output, from C to D, to out. ctx ending interrupts it as
// ctrl+c would.
func (s *Shell) runLine(ctx context.Context, line string, out io.Writer) (status int, exited bool, err error) {
	ia := s.ia
	select {
	case <-s.waited:
		return s.exit, true, ErrExited
	default:
	}
	ia.mu.Lock()
	// Between a command's end and the next prompt, wait for the prompt.
	if ia.phase == phaseDone || ia.phase == phasePrompt || ia.phase == phaseStarting {
		t := time.AfterFunc(promptWait, func() { ia.mu.Lock(); ia.cond.Broadcast(); ia.mu.Unlock() })
		deadline := time.Now().Add(promptWait)
		for ia.phase != phaseInput && ia.phase != phaseRunning && !s.Exited() && time.Now().Before(deadline) {
			ia.cond.Wait()
		}
		t.Stop()
	}
	switch {
	case s.Exited():
		ia.mu.Unlock()
		return s.exit, true, ErrExited
	case ia.phase != phaseInput:
		ia.mu.Unlock()
		return 0, false, ErrBusy
	case ia.dirty:
		ia.mu.Unlock()
		return 0, false, ErrLineTyped
	}
	ia.out, ia.typed, ia.ended, ia.status = out, true, false, -1
	ia.mu.Unlock()
	defer func() {
		ia.mu.Lock()
		ia.out, ia.typed = nil, false
		ia.mu.Unlock()
	}()

	if _, err := io.WriteString(ia.ptm, line+"\r"); err != nil {
		<-s.waited
		return s.exit, true, ErrExited
	}
	s.busy.Store(true)
	defer s.busy.Store(false)

	// Wake the wait below when ctx ends or the shell is killed.
	stopWake := make(chan struct{})
	defer close(stopWake)
	var stopped error
	go func() {
		select {
		case <-ctx.Done():
		case <-s.forced:
		case <-stopWake:
			return
		}
		ia.mu.Lock()
		ia.cond.Broadcast()
		ia.mu.Unlock()
	}()
	var interrupting chan struct{}
	defer func() {
		if interrupting != nil {
			close(interrupting)
		}
	}()

	ia.mu.Lock()
	for !ia.ended && !s.Exited() {
		if ctx.Err() != nil && interrupting == nil {
			stopped = ctx.Err()
			interrupting = make(chan struct{})
			go s.interruptTTY(interrupting)
		}
		select {
		case <-s.forced:
			ia.mu.Unlock()
			<-s.waited
			ia.drain()
			if stopped == nil {
				stopped = ErrExited
			}
			return -1, true, stopped
		default:
		}
		ia.cond.Wait()
	}
	if !ia.ended {
		ia.mu.Unlock()
		ia.drain()
		if stopped != nil {
			return s.exit, true, stopped
		}
		return s.exit, true, ErrExited
	}
	status = ia.status
	// The state record the hook wrote before the D mark.
	want := ia.marksD
	t := time.AfterFunc(stateWait, func() { ia.mu.Lock(); ia.cond.Broadcast(); ia.mu.Unlock() })
	deadline := time.Now().Add(stateWait)
	for ia.states < want && !s.Exited() && time.Now().Before(deadline) {
		ia.cond.Wait()
	}
	t.Stop()
	if status == -1 && ia.states >= want {
		status = ia.last.Status
	}
	ia.mu.Unlock()
	if stopped != nil {
		return 130, false, stopped
	}
	return status, false, nil
}

// drain waits a moment for the rest of what the shell wrote before it
// ended to be read, until reading ends or nothing more comes.
func (ia *interactive) drain() {
	deadline := time.Now().Add(drainWait)
	for time.Now().Before(deadline) {
		ia.mu.Lock()
		done := ia.readEOF
		ia.mu.Unlock()
		if done {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// interruptTTY stops the command in the foreground until stop closes:
// ctrl+c, as typed, so the terminal sends SIGINT to the foreground
// process group, and SIGINT to the shell, once a second; after interruptFor it kills that group,
// which the terminal names; after Config.KillAfter, the shell. Stopped
// and background jobs aren't touched.
func (s *Shell) interruptTTY(stop <-chan struct{}) {
	start := time.Now()
	t := time.NewTicker(interruptEvery)
	defer t.Stop()
	killed := false
	var sent time.Time
	for {
		if time.Since(sent) >= time.Second {
			s.ia.ptm.Write([]byte{0x03})
			// The shell isn't in the foreground group, so the ctrl+c
			// doesn't reach it, and bash 3.2 runs on with the rest of
			// a sourced file after the command it ends. SIGINT to the
			// shell itself makes an interactive shell drop what it is
			// running and go back to its prompt.
			syscall.Kill(s.cmd.Process.Pid, syscall.SIGINT)
			sent = time.Now()
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
		if !killed && time.Since(start) >= interruptFor {
			killed = true
			if pg, err := tcgetpgrp(s.ia.ptm); err == nil && pg > 0 && pg != s.cmd.Process.Pid {
				syscall.Kill(-pg, syscall.SIGKILL)
			}
		}
		if s.killAfter > 0 && time.Since(start) >= s.killAfter {
			s.Kill()
			return
		}
	}
}

// RunLine types line at an interactive shell's prompt, as if typed and
// followed by Enter, and waits for the command to end. Its output, as
// the terminal showed it, escapes and all, goes to out, and its exit
// status is returned; -1 if the shell didn't say. It fails with ErrBusy
// if a command is running, and ErrLineTyped if someone has typed at the
// prompt, rather than add to their line. If ctx ends first, the command
// is interrupted as ctrl+c would and RunLine returns 130 and ctx's
// error.
func (s *Shell) RunLine(ctx context.Context, line string, out io.Writer) (status int, exited bool, err error) {
	if s.ia == nil {
		return 0, false, ErrNotInteractive
	}
	if strings.ContainsAny(line, "\r\n") {
		return 0, false, errors.New("RunLine takes one line")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runLine(ctx, line, out)
}

// runInteractive is Run for an interactive shell: the script is written
// to a file, checked, and sourced by a line typed at the prompt. Its
// output and errors share the terminal, so all of it goes to stdout.
func (s *Shell) runInteractive(ctx context.Context, script string, stdout, stderr io.Writer) (int, bool, error) {
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
	w := newRenamer(stdout, f, "block")
	defer w.Flush()
	return s.runLine(ctx, sourceLine(f), w)
}

// runTerminalInteractive is RunTerminal for an interactive shell: the
// script is sourced by a typed line, as Run does, with t's keys going
// to the terminal while it runs, and its output, from C to D, to t.Out
// and rec. Job control is the shell's own, so t.JobControl is moot.
func (s *Shell) runTerminalInteractive(ctx context.Context, script string, t Terminal, rec io.Writer) (int, bool, error) {
	cols, rows := t.Dims()
	s.ia.setSize(cols, rows)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
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
				s.ia.ptm.Write(k)
			case <-resized:
				s.ia.setSize(t.Dims())
			}
		}
	}()
	defer func() {
		close(stop)
		wg.Wait()
	}()
	out := io.MultiWriter(t.Out, rec)
	return s.runInteractive(ctx, script, out, out)
}

// sourceLine is the line typed to run the script in file f. The
// integration keeps lines of this form out of the shell's history.
func sourceLine(f string) string {
	if strings.ContainsAny(f, "\"$`\\") {
		return ". " + quote(f)
	}
	return `. "` + f + `"`
}

// Type sends keys to an interactive shell's terminal, as typed.
func (s *Shell) Type(keys []byte) error {
	if s.ia == nil {
		return ErrNotInteractive
	}
	s.ia.mu.Lock()
	if s.ia.phase == phaseInput && len(keys) > 0 {
		s.ia.dirty = true
	}
	s.ia.mu.Unlock()
	_, err := s.ia.ptm.Write(keys)
	return err
}

// View sets where an interactive shell's terminal stream goes, as a
// terminal would show it, from now on; nil drops it.
func (s *Shell) View(w io.Writer) {
	if s.ia == nil {
		return
	}
	s.ia.mu.Lock()
	s.ia.view = w
	s.ia.mu.Unlock()
}

// Watch sets fn to be called with what an interactive shell does:
// commands starting and ending and its state at each prompt. fn is
// called with the shell's lock held, so it must not call back in.
func (s *Shell) Watch(fn func(Event)) {
	if s.ia == nil {
		return
	}
	s.ia.mu.Lock()
	s.ia.watch = fn
	s.ia.mu.Unlock()
}

// State is what an interactive shell reported at its last prompt.
func (s *Shell) State() (State, bool) {
	if s.ia == nil {
		return State{}, false
	}
	s.ia.mu.Lock()
	defer s.ia.mu.Unlock()
	return s.ia.last, s.ia.states > 0
}

// Want names the variables an interactive shell should report in its
// state, exported or not, from its next prompt on.
func (s *Shell) Want(names []string) error {
	if s.ia == nil {
		return ErrNotInteractive
	}
	var b strings.Builder
	for _, n := range names {
		if validName(n) {
			b.WriteString(n + "\n")
		}
	}
	return os.WriteFile(filepath.Join(s.tmp, "want"), []byte(b.String()), 0o600)
}

func validName(n string) bool {
	if n == "" || (n[0] >= '0' && n[0] <= '9') {
		return false
	}
	for _, c := range n {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// Jobs lists an interactive shell's jobs that haven't ended: from its
// job table where the shell reports it, or else from the process
// groups on its terminal other than its own, which have no numbers.
func (s *Shell) Jobs() []ShellJob {
	if s.ia == nil {
		return nil
	}
	s.ia.mu.Lock()
	st, known := s.ia.last, s.ia.states > 0 && s.ia.last.JobsKnown
	s.ia.mu.Unlock()
	if known {
		var jobs []ShellJob
		for _, j := range st.Jobs {
			if j.State != "done" {
				jobs = append(jobs, j)
			}
		}
		return jobs
	}
	return ttyJobs(strings.TrimPrefix(s.ia.ttyName, "/dev/"), s.cmd.Process.Pid)
}

// ttyJobs lists the process groups with processes on terminal tty,
// other than the shell's own, as ps shows them.
func ttyJobs(tty string, shell int) []ShellJob {
	out, _ := exec.Command("ps", "-t", tty, "-o", "pid=", "-o", "pgid=", "-o", "stat=", "-o", "args=").Output()
	byGroup := map[int]*ShellJob{}
	var order []int
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		pg, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil || pg == shell || strings.HasPrefix(f[2], "Z") {
			continue
		}
		j := byGroup[pg]
		if j == nil {
			j = &ShellJob{PGID: pg, State: "running"}
			byGroup[pg] = j
			order = append(order, pg)
		}
		if strings.HasPrefix(f[2], "T") {
			j.State = "stopped"
		}
		if pid == pg {
			j.Command = strings.Join(f[3:], " ")
		}
	}
	jobs := make([]ShellJob, 0, len(order))
	for _, pg := range order {
		jobs = append(jobs, *byGroup[pg])
	}
	return jobs
}

// setSizeInteractive sizes an interactive shell's terminal; the
// kernel tells the foreground group with SIGWINCH.
func (ia *interactive) setSize(cols, rows int) {
	pty.Setsize(ia.ptm, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// close ends an interactive shell as closing its terminal window would:
// SIGHUP, which bash and zsh pass on to their jobs, then, for whatever
// is left on the terminal, SIGKILL.
func (s *Shell) closeInteractive() {
	ia := s.ia
	tty := strings.TrimPrefix(ia.ttyName, "/dev/")
	if !s.Exited() {
		syscall.Kill(s.cmd.Process.Pid, syscall.SIGHUP)
		select {
		case <-s.waited:
		case <-time.After(time.Second):
		}
	}
	for _, j := range ttyJobs(tty, -1) {
		syscall.Kill(-j.PGID, syscall.SIGCONT)
		syscall.Kill(-j.PGID, syscall.SIGKILL)
	}
	if !s.Exited() {
		kill(s.cmd)
		<-s.waited
	}
	ia.ptm.Close()
	ia.state.Close()
}

// unsetEnv is environ without key.
func unsetEnv(environ []string, key string) []string {
	out := environ[:0:0]
	for _, e := range environ {
		if k, _, _ := strings.Cut(e, "="); k != key {
			out = append(out, e)
		}
	}
	return out
}

// tcgetpgrp is the foreground process group of the terminal whose
// master is ptm.
func tcgetpgrp(ptm *os.File) (int, error) {
	return unix.IoctlGetInt(int(ptm.Fd()), unix.TIOCGPGRP)
}

// CanInteractive reports whether the shell at path can run with
// Config.Interactive: a POSIX shell, on Unix.
func CanInteractive(path string) bool {
	if path == "" {
		path = "/bin/sh"
	}
	_, err := kindOf(path)
	return err == nil
}
