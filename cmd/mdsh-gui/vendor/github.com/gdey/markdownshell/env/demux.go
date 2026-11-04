package env

import (
	"bytes"
	"io"
	"sync"
)

// demux copies a shell's output stream to whichever writer is attached,
// and reports each marker line it finds instead of copying it.
type demux struct {
	mark []byte
	mu   sync.Mutex
	w    io.Writer
	// done receives the text between a marker and the end of its line.
	// It is closed when the stream ends.
	done chan string
	eof  chan struct{}
}

func newDemux(r io.Reader, mark string) *demux {
	d := &demux{
		mark: []byte(mark),
		done: make(chan string, 1),
		eof:  make(chan struct{}),
	}
	go d.loop(r)
	return d
}

func (d *demux) attach(w io.Writer) {
	d.mu.Lock()
	d.w = w
	d.mu.Unlock()
}

func (d *demux) write(p []byte) {
	if len(p) == 0 {
		return
	}
	d.mu.Lock()
	if d.w != nil {
		d.w.Write(p)
	}
	d.mu.Unlock()
}

func (d *demux) loop(r io.Reader) {
	defer close(d.eof)
	defer close(d.done)
	var buf []byte
	chunk := make([]byte, 32*1024)
	for {
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		for {
			i := bytes.Index(buf, d.mark)
			if i < 0 {
				// Hold back a possible partial marker.
				safe := len(buf) - partial(buf, d.mark)
				if safe > 0 {
					d.write(buf[:safe])
					buf = append(buf[:0], buf[safe:]...)
				}
				break
			}
			rest := buf[i+len(d.mark):]
			nl := bytes.IndexByte(rest, '\n')
			if nl < 0 {
				// Write what came before the marker; wait for the rest.
				d.write(buf[:i])
				buf = append(buf[:0], buf[i:]...)
				break
			}
			d.write(buf[:i])
			d.done <- string(rest[:nl])
			buf = append(buf[:0], rest[nl+1:]...)
		}
		if err != nil {
			d.write(buf)
			return
		}
	}
}

// partial returns the length of the longest end of buf that is the start
// of mark, so only what could be a marker is held back and a prompt
// with no newline still shows.
func partial(buf, mark []byte) int {
	n := min(len(buf), len(mark)-1)
	for ; n > 0; n-- {
		if bytes.HasPrefix(mark, buf[len(buf)-n:]) {
			return n
		}
	}
	return 0
}
