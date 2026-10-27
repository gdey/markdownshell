//go:build unix

package env

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"sync/atomic"
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

	s.pty.drain()
	s.pty.attach(nil)
	close(stop)
	wg.Wait()
	return status, exited, err
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
	mu       sync.Mutex
	w        io.Writer
	last     atomic.Int64 // when output last came, in UnixNano
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

// started starts reading. mdsh keeps the shell's end open too, since
// reading the pseudo-terminal fails once nothing has that end open, as
// between blocks.
func (p *ptyConn) started() {
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := p.ptm.Read(buf)
			if n > 0 {
				p.last.Store(time.Now().UnixNano())
				p.mu.Lock()
				if p.w != nil {
					p.w.Write(buf[:n])
				}
				p.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
}

func (p *ptyConn) attach(w io.Writer) {
	p.mu.Lock()
	p.w = w
	p.mu.Unlock()
}

// drain waits for output still on its way, until none has come for a
// moment, so the end of a block isn't cut off.
func (p *ptyConn) drain() {
	const quiet, most = 30 * time.Millisecond, 500 * time.Millisecond
	end := time.Now().Add(most)
	for time.Now().Before(end) {
		if time.Since(time.Unix(0, p.last.Load())) >= quiet {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *ptyConn) close() {
	p.pts.Close()
	p.ptm.Close()
}
