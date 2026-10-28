package termrec

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"time"
	"unicode/utf8"
)

// castHeader is the first line of an asciicast v2 file.
type castHeader struct {
	Version   int   `json:"version"`
	Width     int   `json:"width"`
	Height    int   `json:"height"`
	Timestamp int64 `json:"timestamp,omitempty"`
}

// WriteCast writes rec in asciinema's asciicast v2 format: a JSON header
// line, then one [seconds, "o", text] line per write. A UTF-8 character
// split across writes is kept whole in the later one, since each line
// must be valid text.
func WriteCast(w io.Writer, rec Recording) error {
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	h := castHeader{Version: 2, Width: rec.Cols, Height: rec.Rows}
	if !rec.Start.IsZero() {
		h.Timestamp = rec.Start.Unix()
	}
	if err := enc.Encode(h); err != nil {
		return err
	}
	var carry []byte
	for _, e := range rec.Events {
		data := append(carry, e.Data...)
		n := len(data) - incomplete(data)
		data, carry = data[:n], append([]byte(nil), data[n:]...)
		if len(data) == 0 {
			continue
		}
		if err := enc.Encode([]any{e.At.Seconds(), "o", string(data)}); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// incomplete returns how many bytes at the end of p are the start of a
// UTF-8 character that isn't finished.
func incomplete(p []byte) int {
	for i := 1; i <= min(len(p), utf8.UTFMax-1); i++ {
		c := p[len(p)-i]
		if utf8.RuneStart(c) {
			if !utf8.FullRune(p[len(p)-i:]) {
				return i
			}
			return 0
		}
	}
	return 0
}

// ReadCast reads an asciicast v2 file. Events other than output are
// skipped.
func ReadCast(r io.Reader) (Recording, error) {
	dec := json.NewDecoder(r)
	var h castHeader
	if err := dec.Decode(&h); err != nil {
		return Recording{}, fmt.Errorf("cast header: %w", err)
	}
	if h.Version != 2 {
		return Recording{}, fmt.Errorf("cast version %d, want 2", h.Version)
	}
	rec := Recording{Cols: h.Width, Rows: h.Height}
	if h.Timestamp != 0 {
		rec.Start = time.Unix(h.Timestamp, 0)
	}
	for {
		var ev [3]any
		if err := dec.Decode(&ev); err == io.EOF {
			return rec, nil
		} else if err != nil {
			return rec, fmt.Errorf("cast event: %w", err)
		}
		at, ok1 := ev[0].(float64)
		kind, ok2 := ev[1].(string)
		data, ok3 := ev[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return rec, fmt.Errorf("cast event %v: want [seconds, type, text]", ev)
		}
		if kind == "o" {
			rec.Events = append(rec.Events, Event{At: time.Duration(at * float64(time.Second)), Data: []byte(data)})
		}
	}
}
