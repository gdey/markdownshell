package run

import (
	"bytes"
	"io"
	"unicode/utf8"
)

// Overstrike applies the backspaces in p: each one erases the rune
// before it on its line. Programs that format for a printer, such as
// man, mark bold as "c\bc" and underline as "_\bc"; this leaves the
// plain "c". Backspace spinners keep only what they showed last.
func Overstrike(p []byte) []byte {
	if bytes.IndexByte(p, '\b') < 0 {
		return p
	}
	out := make([]byte, 0, len(p))
	for _, c := range p {
		if c != '\b' {
			out = append(out, c)
			continue
		}
		if len(out) == 0 || out[len(out)-1] == '\n' {
			continue
		}
		_, n := utf8.DecodeLastRune(out)
		out = out[:len(out)-n]
	}
	return out
}

// overstrikeWriter applies Overstrike to a stream. A backspace may come
// in the write after the rune it erases, so it holds back the end of
// each write, up to a line end, until the next one or Flush.
type overstrikeWriter struct {
	w    io.Writer
	held []byte
}

func (o *overstrikeWriter) Write(p []byte) (int, error) {
	out := Overstrike(append(o.held, p...))
	keep := 0
	if len(out) > 0 && out[len(out)-1] != '\n' {
		keep = min(len(out), utf8.UTFMax)
		if nl := bytes.LastIndexByte(out, '\n'); nl >= 0 {
			keep = min(keep, len(out)-nl-1)
		}
	}
	send := out[:len(out)-keep]
	o.held = append([]byte(nil), out[len(out)-keep:]...)
	if len(send) == 0 {
		return len(p), nil
	}
	if _, err := o.w.Write(send); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush writes what Write held back.
func (o *overstrikeWriter) Flush() error {
	if len(o.held) == 0 {
		return nil
	}
	_, err := o.w.Write(o.held)
	o.held = nil
	return err
}
