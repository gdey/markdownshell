// Package audit writes a run's audit log: one JSON object per line, in
// order, saying what the run did, so a run can be reviewed afterwards.
package audit

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"sync"
	"time"
)

// Version is the log's format version, every event's "v".
const Version = 1

// Fields are an event's fields besides v, seq, t and event.
type Fields map[string]any

// Log writes events. A nil *Log writes nothing, so callers need not
// check whether a run keeps one. It is safe to use from more than one
// goroutine.
type Log struct {
	mu  sync.Mutex
	w   io.Writer
	seq int
	key []byte
	err error
	now func() time.Time
}

// New returns a log that writes to w, with a new random key for
// Secret.
func New(w io.Writer) *Log {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err) // crypto/rand doesn't fail
	}
	return &Log{w: w, key: key, now: time.Now}
}

// Event writes one event. seq counts from 1; t is RFC 3339 with
// milliseconds. The other fields follow in name order.
func (l *Log) Event(event string, f Fields) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return
	}
	l.seq++
	var b bytes.Buffer
	head, _ := json.Marshal(struct {
		V     int    `json:"v"`
		Seq   int    `json:"seq"`
		T     string `json:"t"`
		Event string `json:"event"`
	}{Version, l.seq, l.now().UTC().Format("2006-01-02T15:04:05.000Z07:00"), event})
	b.Write(head[:len(head)-1])
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, err := json.Marshal(f[k])
		if err != nil {
			v, _ = json.Marshal(err.Error())
		}
		kj, _ := json.Marshal(k)
		b.WriteByte(',')
		b.Write(kj)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteString("}\n")
	_, l.err = l.w.Write(b.Bytes())
}

// Secret is what the log stores where a secret value would go: an
// HMAC-SHA-256 of it, keyed with the log's key, so two secrets in one
// run can be compared but a short one can't be guessed.
func (l *Log) Secret(value string) any {
	if l == nil {
		return nil
	}
	m := hmac.New(sha256.New, l.key)
	m.Write([]byte(value))
	return map[string]any{"secret": true, "hmac": hex.EncodeToString(m.Sum(nil))}
}

// Err is the first error writing the log. Nothing is written after it.
func (l *Log) Err() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}
