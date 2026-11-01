package serve

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/gdey/markdownshell/ui"
)

// This file is mdsh --serve=stdio: the same calls, events and questions
// as the browser gets, as lines of JSON on stdin and stdout, for a
// program that starts mdsh itself, such as an editor plugin. Only that
// program holds the pipes, so there is no token.
//
// Each line the client writes is one of:
//
//	{"id":1,"name":"ui.App.Cells","args":[],"window":1,"tab":2}
//	{"answer":7,"value":"0"}
//
// A call names a ui.App method as the bindings do (the name only has to
// end in ui.App.<Method>), with its arguments, and the window and tab it
// is for when the method acts on one. id is the client's, any JSON
// value, and comes back on the call's reply:
//
//	{"id":1,"result":[...]}
//	{"id":1,"error":"no tab 2"}
//
// An answer answers the mdsh:ask or mdsh:prompt event with that id: for
// a question, the index of the button picked; for a prompt, the path, or
// "" to cancel.
//
// Every other line mdsh writes is an event for a window, as its page
// would get it over the WebSocket, with the window added:
//
//	{"window":1,"name":"block","data":{"mdshTab":2,"data":{...}},"sender":"w1"}
//
// An event sent to every window comes once for each. Calls run at once,
// each on its own, so one waiting for an answer doesn't hold up the
// others, and replies and events may come in any order. Logs go to
// stderr. mdsh stops, closing every shell, when stdin ends.

// Stdio is the --serve address that serves on stdin and stdout.
const Stdio = "stdio"

// stdio is a client on stdin and stdout.
type stdio struct {
	wmu sync.Mutex // held while writing a line
	out *bufio.Writer

	mu    sync.Mutex
	conns map[uint]*conn // by window
	done  chan struct{}  // closes when the session ends
	fail  chan error     // a reason to end the session early
}

// stdioFrame is a line the client writes.
type stdioFrame struct {
	ID     json.RawMessage   `json:"id,omitempty"`
	Name   string            `json:"name,omitempty"`
	Args   []json.RawMessage `json:"args,omitempty"`
	Window *uint             `json:"window,omitempty"`
	Tab    *int              `json:"tab,omitempty"`
	Answer *int              `json:"answer,omitempty"`
	Value  string            `json:"value,omitempty"`
}

// stdioReply is the line mdsh writes for a call.
type stdioReply struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func (c *stdio) write(line []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if _, err := c.out.Write(line); err != nil {
		return err
	}
	if err := c.out.WriteByte('\n'); err != nil {
		return err
	}
	return c.out.Flush()
}

func (c *stdio) failed(err error) {
	select {
	case c.fail <- err:
	default:
	}
}

// attach starts sending w's events to the client.
func (c *stdio) attach(w *window) {
	select {
	case <-c.done:
		return
	default:
	}
	c.mu.Lock()
	if c.conns[w.id] != nil {
		c.mu.Unlock()
		return
	}
	cn := &conn{send: make(chan []byte, 4096), gone: make(chan struct{})}
	c.conns[w.id] = cn
	c.mu.Unlock()
	prefix := []byte(`{"window":` + strconv.FormatUint(uint64(w.id), 10) + `,`)
	go func() {
		for {
			select {
			case <-c.done:
				return
			case <-cn.gone:
				c.failed(errors.New("the client fell too far behind reading events"))
				return
			case msg := <-cn.send:
				// msg is a JSON object: put the window first in it.
				line := append(append([]byte{}, prefix...), bytes.TrimPrefix(msg, []byte("{"))...)
				if err := c.write(line); err != nil {
					c.failed(err)
					return
				}
			}
		}
	}()
	w.attach(cn)
}

// ServeStdio serves one client on in and out, as mdsh --serve=stdio
// does, until in ends or ctx does, then closes every window's shells.
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error {
	c := &stdio{out: bufio.NewWriter(out), conns: map[uint]*conn{}, done: make(chan struct{}), fail: make(chan error, 1)}
	s.mu.Lock()
	if s.stdio != nil {
		s.mu.Unlock()
		return errors.New("already serving on stdio")
	}
	s.stdio = c
	var wins []*window
	for _, w := range s.windows {
		if w.doc {
			wins = append(wins, w)
		}
	}
	s.mu.Unlock()
	for _, w := range wins {
		c.attach(w)
	}

	lines := make(chan []byte)
	readErr := make(chan error, 1)
	go func() {
		r := bufio.NewReaderSize(in, 64<<10)
		for {
			line, err := r.ReadBytes('\n')
			if len(bytes.TrimSpace(line)) > 0 {
				select {
				case lines <- line:
				case <-c.done:
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = nil
				}
				readErr <- err
				return
			}
		}
	}()

	var calls sync.WaitGroup
	var err error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case err = <-readErr:
			break loop
		case err = <-c.fail:
			break loop
		case line := <-lines:
			var f stdioFrame
			if jerr := json.Unmarshal(line, &f); jerr != nil {
				s.logf("mdsh: stdio: bad line: %v", jerr)
				continue
			}
			if f.Answer != nil {
				s.answerAsk(*f.Answer, f.Value)
				continue
			}
			calls.Add(1)
			go func() {
				defer calls.Done()
				s.stdioCall(ctx, c, f)
			}()
		}
	}
	close(c.done)
	s.mu.Lock()
	s.stdio = nil
	s.mu.Unlock()
	// Nobody is left to answer: questions get "", as when a page goes.
	c.mu.Lock()
	conns := c.conns
	c.conns = nil
	c.mu.Unlock()
	for id, cn := range conns {
		s.mu.Lock()
		w := s.windows[id]
		s.mu.Unlock()
		if w != nil {
			w.detach(cn)
		}
	}
	calls.Wait()
	s.shutdown()
	return err
}

// stdioCall runs a call and writes its reply.
func (s *Server) stdioCall(ctx context.Context, c *stdio, f stdioFrame) {
	if f.Window != nil {
		ctx = s.fromWindow(ctx, *f.Window)
	}
	if f.Tab != nil {
		ctx = ui.WithTab(ctx, *f.Tab)
	}
	rep := stdioReply{ID: f.ID}
	if rep.ID == nil {
		rep.ID = json.RawMessage("null")
	}
	out, err := s.invoke(ctx, callRequest{Name: f.Name, Args: f.Args})
	if err != nil {
		rep.Error = err.Error()
	} else {
		rep.Result = out
	}
	b, err := json.Marshal(rep)
	if err != nil {
		return
	}
	if err := c.write(b); err != nil {
		c.failed(err)
	}
}

// answerAsk answers question or prompt id, if it is still waiting.
func (s *Server) answerAsk(id int, value string) {
	s.mu.Lock()
	a := s.asks[id]
	delete(s.asks, id)
	s.mu.Unlock()
	if a != nil {
		a.reply <- value
	}
}

func (s *Server) logf(format string, args ...any) {
	b := []byte(fmt.Sprintf(format, args...) + "\n")
	_, _ = s.log.Write(b)
}
