// Package serve shows the notebook in a web browser, for mdsh --serve.
// It serves the pages in web, the same ones mdsh-gui shows, and stands
// in for the Wails runtime they talk to: calls to ui.App are POSTs to
// /wails/call, and events come over a WebSocket.
//
// Each browser tab showing window.html is one of the App's windows, named
// in its URL. A window lasts until its last tab closes, not when the
// browser tab does, so its shells keep running and opening the server's
// address again picks it up.
//
// Anyone who can reach the server can run commands as the user who
// started it, so every request needs the token printed at start: the
// first visit gives it in the URL and gets a cookie for it.
package serve

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/gdey/markdownshell/settings"
	"github.com/gdey/markdownshell/ui"
	"github.com/gdey/markdownshell/web"
)

// DefaultAddr is where --serve listens when given no address: this
// machine only.
const DefaultAddr = "127.0.0.1:8080"

// Options say what to serve and where.
type Options struct {
	Addr     string // host:port to listen on; a missing host or port is filled in from DefaultAddr
	Settings *settings.Store
	Cwd      string   // the folder mdsh started in
	Dir      string   // the folder named on the command line, if any
	File     string   // the file to open in the first window, if any
	Args     []string // its arguments
	Token    string   // the access token; "" makes a random one
	Log      io.Writer
}

// Server is a running mdsh --serve.
type Server struct {
	app   *ui.App
	ctrl  ui.Control
	token string
	addr  string // the address listened on
	local bool   // listening on a loopback address only
	log   io.Writer
	calls map[string]method

	mu      sync.Mutex
	windows map[uint]*window
	lastID  uint
	asks    map[int]*ask
	lastAsk int
	active  uint // the window that last made a call
}

// New makes a Server for opts. Serve or ListenAndServe start it.
func New(opts Options) (*Server, error) {
	addr, err := fillAddr(opts.Addr)
	if err != nil {
		return nil, err
	}
	s := &Server{
		token:   opts.Token,
		addr:    addr,
		log:     opts.Log,
		windows: map[uint]*window{},
		asks:    map[int]*ask{},
	}
	if s.log == nil {
		s.log = io.Discard
	}
	if s.token == "" {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		s.token = hex.EncodeToString(b)
	}
	host, _, _ := net.SplitHostPort(addr)
	s.local = isLoopback(host)
	s.app = ui.New(s, opts.Settings, opts.Cwd, opts.Dir)
	s.ctrl = ui.ControlOf(s.app)
	s.calls = methods(s.app)
	if err := s.ctrl.Start(opts.File, opts.Args); err != nil {
		return nil, err
	}
	return s, nil
}

// fillAddr fills in a missing host or port in addr from DefaultAddr.
func fillAddr(addr string) (string, error) {
	defHost, defPort, _ := net.SplitHostPort(DefaultAddr)
	if addr == "" {
		return DefaultAddr, nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// A host alone, such as localhost or 0.0.0.0.
		host, port, err = addr, defPort, nil
		if strings.Contains(addr, ":") && !strings.HasPrefix(addr, "[") {
			host = "[" + addr + "]"
		}
		if _, _, err = net.SplitHostPort(host + ":" + port); err != nil {
			return "", fmt.Errorf("--serve: bad address %q, want host:port", addr)
		}
		return host + ":" + port, nil
	}
	if host == "" && strings.HasPrefix(addr, ":") {
		// :port listens on every address, as Go's listeners do.
		return addr, nil
	}
	if port == "" {
		port = defPort
	}
	if host == "" {
		host = defHost
	}
	return net.JoinHostPort(host, port), nil
}

// isLoopback reports whether host names only this machine.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Token is the access token.
func (s *Server) Token() string { return s.token }

// Local reports whether the server only listens on this machine.
func (s *Server) Local() bool { return s.local }

// URL is the address to open, token included, once the server listens
// on l.
func (s *Server) URL(l net.Listener) string {
	host, port, _ := net.SplitHostPort(l.Addr().String())
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port) + "/?token=" + s.token
}

// ListenAndServe listens on the address in Options and serves until ctx
// ends.
func (s *Server) ListenAndServe(ctx context.Context) error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, l)
}

// Serve serves on l until ctx ends, then closes every window's shells.
func (s *Server) Serve(ctx context.Context, l net.Listener) error {
	if !s.local {
		fmt.Fprintln(s.log, "mdsh: warning: serving to other machines. Anyone who can reach this address and has the token can run commands as you, and the traffic is not encrypted.")
	}
	fmt.Fprintln(s.log, "mdsh: serving the notebook at", s.URL(l))
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	select {
	case err := <-done:
		s.shutdown()
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
	s.shutdown()
	return nil
}

// shutdown closes every window, ending their shells.
func (s *Server) shutdown() {
	s.mu.Lock()
	ids := make([]uint, 0, len(s.windows))
	for id, w := range s.windows {
		if w.doc {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	s.ctrl.Quitting()
	for _, id := range ids {
		s.ctrl.Closing(id)
	}
}

//go:embed runtime.js
var runtimeJS []byte

// Handler is the server's HTTP handler.
func (s *Server) Handler() http.Handler {
	files := s.ctrl.Assets(http.FileServerFS(web.FS))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.root)
	mux.HandleFunc("GET /settings", s.settingsPage)
	mux.HandleFunc("GET /wails/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(runtimeJS)
	})
	mux.Handle("POST /wails/call", ui.TabCalls(http.HandlerFunc(s.call)))
	mux.HandleFunc("POST /wails/answer", s.answer)
	mux.HandleFunc("GET /wails/events", s.events)
	mux.HandleFunc("GET /index.html", s.tabPage)
	mux.Handle("GET /", files)
	return s.guard(mux)
}

// tabPage serves a tab's page, which http.FileServer would send to
// "/" instead, as it does any index.html.
func (s *Server) tabPage(w http.ResponseWriter, r *http.Request) {
	b, err := web.FS.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

// cookie is the name of the cookie that holds the token. Cookies don't
// keep to a port, so it names the port, and servers on other ports keep
// their own.
func (s *Server) cookie(r *http.Request) string {
	_, port, _ := net.SplitHostPort(r.Host)
	return "mdsh-token-" + port
}

// guard lets a request through only with the token, from a page of this
// server.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		// On this machine only, a page must have been loaded from a
		// loopback name, which keeps out other sites' pages that point
		// their own name at 127.0.0.1.
		if s.local && !isLoopback(strings.Trim(host, "[]")) {
			http.Error(w, "mdsh: unknown host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			if u, err := url.Parse(o); err != nil || u.Host != r.Host {
				http.Error(w, "mdsh: request from another site", http.StatusForbidden)
				return
			}
		}
		if t := r.URL.Query().Get("token"); t != "" && r.Method == http.MethodGet {
			if !s.tokenOK(t) {
				http.Error(w, "mdsh: wrong token", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     s.cookie(r),
				Value:    s.token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			q := r.URL.Query()
			q.Del("token")
			u := *r.URL
			u.RawQuery = q.Encode()
			http.Redirect(w, r, u.RequestURI(), http.StatusSeeOther)
			return
		}
		c, err := r.Cookie(s.cookie(r))
		if err != nil || !s.tokenOK(c.Value) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "mdsh: open the address mdsh --serve printed, with its ?token=…")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) tokenOK(t string) bool {
	return subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) == 1
}

// root sends the browser to a window: the first one no page shows, such
// as the one mdsh started with or one whose page was closed, or else a
// new one.
func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	var free *window
	for _, win := range s.windows {
		if win.doc && len(win.conns) == 0 && (free == nil || win.id < free.id) {
			free = win
		}
	}
	s.mu.Unlock()
	var id uint
	if free != nil {
		id = free.id
	} else {
		id = s.ctrl.NewWindow().ID()
	}
	http.Redirect(w, r, "/window.html?w="+strconv.FormatUint(uint64(id), 10), http.StatusSeeOther)
}

// settingsPage sends the browser to a settings page of its own.
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	win := s.newWindow(false)
	http.Redirect(w, r, "/settings.html?w="+strconv.FormatUint(uint64(win.id), 10), http.StatusSeeOther)
}

// windowHeader names the window a call comes from; runtime.js sends it.
const windowHeader = "x-mdsh-window"

type windowKey struct{}

// call runs a call to the App, as Call.ByName in runtime.js asks: its
// name and arguments in a JSON object. It answers with the result as
// JSON, or the error as text.
func (s *Server) call(w http.ResponseWriter, r *http.Request) {
	var req callRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if id, err := strconv.ParseUint(r.Header.Get(windowHeader), 10, 0); err == nil {
		ctx = context.WithValue(ctx, windowKey{}, uint(id))
		s.mu.Lock()
		if win := s.windows[uint(id)]; win != nil && win.doc {
			s.active = win.id
		}
		s.mu.Unlock()
	}
	out, err := s.invoke(ctx, req)
	if err != nil {
		var bad badCall
		code := http.StatusUnprocessableEntity
		if errors.As(err, &bad) {
			code = http.StatusBadRequest
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

// events sends the events for a window to its page over a WebSocket.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.URL.Query().Get("w"), 10, 0)
	if err != nil {
		http.Error(w, "mdsh: no window", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	win := s.windows[uint(id)]
	s.mu.Unlock()
	if win == nil {
		http.Error(w, "mdsh: no such window", http.StatusNotFound)
		return
	}
	ws, err := websocket.Accept(w, r, nil) // checks the Origin too
	if err != nil {
		return
	}
	c := &conn{send: make(chan []byte, 4096), gone: make(chan struct{})}
	win.attach(c)
	defer win.detach(c)
	ctx := ws.CloseRead(r.Context())
	for {
		select {
		case <-ctx.Done():
			ws.CloseNow()
			return
		case <-c.gone:
			// Too far behind: the page loads again and catches up.
			ws.Close(websocket.StatusTryAgainLater, "too many events")
			return
		case msg := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := ws.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				ws.CloseNow()
				return
			}
		}
	}
}

// answer takes a page's answer to a question or a prompt.
func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    int    `json:"id"`
		Value string `json:"value"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	a := s.asks[req.ID]
	delete(s.asks, req.ID)
	s.mu.Unlock()
	if a != nil {
		a.reply <- req.Value
	}
	w.WriteHeader(http.StatusNoContent)
}
