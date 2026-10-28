//go:build unix

package env

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// ErrNotForeground is returned by RunTTY when mdsh isn't in the
// terminal's foreground, for example when it runs in the background.
var ErrNotForeground = errors.New("mdsh is not in the terminal's foreground")

// ErrNoPTY is returned by RunTTY on a shell started without Config.TTY.
var ErrNoPTY = errors.New("shell has no terminal of its own")

// RunTTY runs script like Run, but on the shell's own pseudo-terminal,
// with mdsh in the middle: what is typed on tty goes to the script, and
// what the script writes to its terminal goes to tty and to rec. Since
// the pseudo-terminal is the controlling terminal of the shell's session,
// programs that need a terminal, such as docker run -it, work, ctrl+c
// reaches the running command and window size changes are passed on. An
// interrupted command ends; the rest of the script goes on. tty must be
// mdsh's controlling terminal, with mdsh in its foreground group.
func (s *Shell) RunTTY(ctx context.Context, script string, tty *os.File, rec io.Writer) (status int, exited bool, err error) {
	if s.pty == nil {
		return 0, false, ErrNoPTY
	}
	fd := int(tty.Fd())
	pgrp, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return 0, false, err
	}
	if pgrp != unix.Getpgrp() {
		return 0, false, ErrNotForeground
	}
	ptm := s.pty.ptm
	pty.InheritSize(tty, ptm)
	// Put the pseudo-terminal back as it was, in case a program left it
	// raw, and the same for mdsh's terminal.
	if st, err := term.GetState(int(ptm.Fd())); err == nil {
		defer term.Restore(int(ptm.Fd()), st)
	}
	st, err := term.MakeRaw(fd)
	if err != nil {
		return 0, false, err
	}
	defer term.Restore(fd, st)

	s.pty.attach(io.MultiWriter(tty, rec))
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		forward(fd, tty, ptm, stop)
	}()

	status, exited, err = s.run(ctx, script, io.Discard, io.Discard, true)

	s.pty.attach(nil)
	close(stop)
	wg.Wait()
	return status, exited, err
}

// ErrTapeEnded is returned by RunScripted when the block is still
// running a while after drive has typed everything, most likely waiting
// for input that will never come.
var ErrTapeEnded = errors.New("still running 10s after its input ended")

// tapeGrace is how long RunScripted waits for a script once drive is done.
var tapeGrace = 10 * time.Second

// RunScripted runs script on the shell's own pseudo-terminal, sized cols
// by rows, with no person at it: drive types the input, writing to keys,
// while the script runs, and everything the script shows goes to out.
// If drive fails, or the script is still running 10s after drive is
// done, the shell is killed and the error returned.
func (s *Shell) RunScripted(ctx context.Context, script string, cols, rows int, out io.Writer, drive func(ctx context.Context, keys io.Writer) error) (status int, exited bool, err error) {
	if s.pty == nil {
		return 0, false, ErrNoPTY
	}
	ptm := s.pty.ptm
	pty.Setsize(ptm, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if st, err := term.GetState(int(ptm.Fd())); err == nil {
		defer term.Restore(int(ptm.Fd()), st)
	}
	s.pty.attach(out)
	defer s.pty.attach(nil)

	runCtx, stopRun := context.WithCancelCause(ctx)
	defer stopRun(nil)
	type result struct {
		status int
		exited bool
		err    error
	}
	ran := make(chan result, 1)
	go func() {
		st, ex, err := s.run(runCtx, script, io.Discard, io.Discard, true)
		ran <- result{st, ex, err}
	}()
	driveCtx, stopDrive := context.WithCancel(ctx)
	defer stopDrive()
	driven := make(chan error, 1)
	go func() { driven <- drive(driveCtx, ptm) }()

	var res result
	select {
	case res = <-ran:
		stopDrive()
		<-driven
	case derr := <-driven:
		if derr == nil {
			t := time.NewTimer(tapeGrace)
			defer t.Stop()
			select {
			case res = <-ran:
			case <-t.C:
				derr = ErrTapeEnded
			}
		}
		if derr != nil {
			stopRun(derr)
			res = <-ran
			res.err = derr
		}
	}
	return res.status, res.exited, res.err
}

// forward copies what is typed on the terminal fd to ptm, and the
// terminal's size when it changes, until stop is closed. It polls, so
// it never holds a read that would take keys meant for whatever reads
// the terminal next.
func forward(fd int, tty, ptm *os.File, stop <-chan struct{}) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	buf := make([]byte, 1024)
	for {
		select {
		case <-stop:
			return
		case <-winch:
			pty.InheritSize(tty, ptm)
		default:
		}
		// select, not poll, since macOS's poll doesn't work on terminals.
		var fds unix.FdSet
		fds.Set(fd)
		tv := unix.Timeval{Usec: 50000}
		n, err := unix.Select(fd+1, &fds, nil, nil, &tv)
		if err != nil && err != unix.EINTR {
			return
		}
		if n <= 0 || !fds.IsSet(fd) {
			continue
		}
		n, err = unix.Read(fd, buf)
		if n > 0 {
			ptm.Write(buf[:n])
		}
		if err != nil && err != unix.EINTR && err != unix.EAGAIN {
			return
		}
	}
}

// ptyConn is the mdsh end of a shell's pseudo-terminal. It always reads
// what is written to the terminal, so a program writing to /dev/tty
// outside RunTTY can't fill it up and hang, and copies it to whichever
// writer is attached.
type ptyConn struct {
	ptm, pts *os.File
	d        *demux
}

// openPTY makes cmd start in a session of its own, with a new
// pseudo-terminal as its controlling terminal, on descriptor 3.
func openPTY(cmd *exec.Cmd) (*ptyConn, error) {
	ptm, pts, err := pty.Open()
	if err != nil {
		return nil, err
	}
	cmd.ExtraFiles = []*os.File{pts}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 3}
	return &ptyConn{ptm: ptm, pts: pts}, nil
}

// started starts reading, taking out marker lines. mdsh keeps the
// shell's end open too, since reading the pseudo-terminal fails once
// nothing has that end open, as between blocks.
func (p *ptyConn) started(mark string) {
	p.d = newDemux(p.ptm, mark)
}

func (p *ptyConn) attach(w io.Writer) {
	p.d.attach(w)
}

// wait waits for the marker of block n, which the shell writes to the
// terminal after the block, so everything the block wrote before it has
// gone to the attached writer. A marker left by a block cut short is
// skipped.
func (p *ptyConn) wait(n int) {
	want := strconv.Itoa(n)
	for v := range p.d.done {
		if strings.TrimRight(v, "\r") == want {
			return
		}
	}
}

func (p *ptyConn) close() {
	p.pts.Close()
	p.ptm.Close()
}
