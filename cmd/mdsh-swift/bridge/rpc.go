package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// The bridge speaks JSON lines on stdin and stdout. The app sends one
// request per line:
//
//	{"id": 1, "method": "open", "params": {...}}
//
// and the bridge answers each one, in whatever order they finish, with
//
//	{"id": 1, "result": ...}  or  {"id": 1, "error": "..."}
//
// Requests are served at the same time, so a long run doesn't hold up
// Stop or an answer to its confirm. Between answers the bridge pushes
// events, which have no id:
//
//	{"event": "block", "doc": 3, "data": {...}}

// request is one line from the app.
type request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// response answers a request.
type response struct {
	ID     int64  `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// event is pushed to the app. Doc is 0 for an event about the app, such
// as settings changing.
type event struct {
	Event string `json:"event"`
	Doc   int    `json:"doc,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// handler serves one method. params is the request's params, which it
// decodes itself.
type handler func(ctx context.Context, params json.RawMessage) (any, error)

// conn writes answers and events to the app, one line at a time.
type conn struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func newConn(w io.Writer) *conn { return &conn{enc: json.NewEncoder(w)} }

func (c *conn) send(v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.enc.Encode(v); err != nil {
		slog.Error("bridge: write", "err", err)
	}
}

// serve reads requests from r until it ends, serving each with the
// method's handler. When r ends it calls done, which should end what is
// still going, such as a run waiting on a question, and waits for the
// calls to return.
func serve(ctx context.Context, r io.Reader, c *conn, methods map[string]handler, done func()) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	var wg sync.WaitGroup
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			c.send(response{Error: fmt.Sprintf("bad request: %v", err)})
			continue
		}
		h, ok := methods[req.Method]
		if !ok {
			c.send(response{ID: req.ID, Error: fmt.Sprintf("unknown method %q", req.Method)})
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := call(ctx, h, req.Params)
			resp := response{ID: req.ID, Result: res}
			if err != nil {
				resp = response{ID: req.ID, Error: err.Error()}
			}
			c.send(resp)
		}()
	}
	if done != nil {
		done()
	}
	wg.Wait()
	return sc.Err()
}

// call runs h, turning a panic into an error so one bad call doesn't
// take the app's documents down with it.
func call(ctx context.Context, h handler, params json.RawMessage) (res any, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("bridge: panic: %v", p)
			slog.Error("bridge: panic", "panic", p)
		}
	}()
	return h(ctx, params)
}

// method makes a handler of f, decoding params into P.
func method[P any](f func(ctx context.Context, p P) (any, error)) handler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p P
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, fmt.Errorf("params: %w", err)
			}
		}
		return f(ctx, p)
	}
}
