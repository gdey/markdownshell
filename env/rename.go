package env

import (
	"bytes"
	"io"
	"sync"
)

// renamer copies to w with every old replaced by new. It holds back
// only an end of what it was given that could be the start of old, so
// a prompt with no newline still shows; Flush writes what it holds.
type renamer struct {
	w        io.Writer
	old, new []byte
	mu       sync.Mutex
	held     []byte
}

func newRenamer(w io.Writer, old, new string) *renamer {
	return &renamer{w: w, old: []byte(old), new: []byte(new)}
}

func (r *renamer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	buf := append(r.held, p...)
	buf = bytes.ReplaceAll(buf, r.old, r.new)
	keep := partial(buf, r.old)
	_, err := r.w.Write(buf[:len(buf)-keep])
	r.held = append([]byte(nil), buf[len(buf)-keep:]...)
	return len(p), err
}

// Flush writes what Write held back.
func (r *renamer) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.held) > 0 {
		r.w.Write(r.held)
		r.held = nil
	}
}
