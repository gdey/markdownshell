package ctl

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// MaxRequest is the longest request line the server reads.
const MaxRequest = 1 << 20

// readTimeout is how long a client has to send its request.
const readTimeout = 10 * time.Second

// Handler answers a call from caller, the value Issue was given for the
// call's token.
type Handler func(caller any, req Request) Response

// Server listens on the control socket.
type Server struct {
	ln      net.Listener
	dir     string
	path    string
	handler Handler

	mu     sync.Mutex
	tokens map[string]any

	done chan struct{}
	wg   sync.WaitGroup
}

// Listen creates the socket in a new temp directory that only the
// current user can open, and starts serving calls with h.
func Listen(h Handler) (*Server, error) {
	ln, dir, path, err := listen()
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, dir: dir, path: path, handler: h, tokens: map[string]any{}, done: make(chan struct{})}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// Path is the socket's path, for $MDSH_SOCK.
func (s *Server) Path() string { return s.path }

// Issue makes a new token for caller, for $MDSH_TOKEN. Calls made with
// it reach the handler with caller until Revoke.
func (s *Server) Issue(caller any) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand doesn't fail
	}
	t := hex.EncodeToString(b[:])
	s.mu.Lock()
	s.tokens[t] = caller
	s.mu.Unlock()
	return t
}

// Revoke stops token working.
func (s *Server) Revoke(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// Close stops serving and removes the socket and its directory.
func (s *Server) Close() error {
	close(s.done)
	err := s.ln.Close()
	s.wg.Wait()
	os.RemoveAll(s.dir)
	return err
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		// One call at a time, in the order they arrive.
		s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	resp := s.answer(conn)
	resp.V = Version
	data, _ := json.Marshal(resp)
	conn.SetWriteDeadline(time.Now().Add(readTimeout))
	conn.Write(append(data, '\n'))
}

func (s *Server) answer(conn net.Conn) Response {
	if !sameUser(conn) {
		return Refuse("the caller is not the user running mdsh")
	}
	conn.SetReadDeadline(time.Now().Add(readTimeout))
	r := bufio.NewReader(&limitReader{conn, MaxRequest})
	line, err := r.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return Refuse("no request: %v", err)
	}
	conn.SetReadDeadline(time.Time{})
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Refuse("bad request: %v", err)
	}
	if req.V != Version {
		return Refuse("protocol version %d is not supported (mdsh speaks %d)", req.V, Version)
	}
	s.mu.Lock()
	caller, ok := s.tokens[req.Token]
	s.mu.Unlock()
	if !ok || req.Token == "" {
		return Refuse("the token is not one this run gave out, or its block has ended")
	}
	return s.handler(caller, req)
}

// limitReader reads at most n bytes, then fails.
type limitReader struct {
	r net.Conn
	n int
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errors.New("request too long")
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= n
	return n, err
}
