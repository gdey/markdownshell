package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/notebook"
	"github.com/gdey/markdownshell/ui"
)

// This file is the Server as a ui.Host: windows are browser pages, and
// dialogs are drawn by runtime.js in the page they are for.

var _ ui.Host = (*Server)(nil)

// window is a page, or the pages, showing one of the App's windows, or a
// settings page. A doc window lasts until the App closes it; a settings
// page, until its last page goes.
type window struct {
	s   *Server
	id  uint
	doc bool // one of the App's windows, not a settings page

	mu     sync.Mutex
	conns  map[*conn]bool
	title  string
	closed bool
}

// conn is one page's WebSocket. send holds the messages not yet
// written; gone closes if the page falls too far behind.
type conn struct {
	send     chan []byte
	gone     chan struct{}
	goneOnce sync.Once
}

// message is what runtime.js gets over the WebSocket: an event, as
// Wails sends it.
type message struct {
	Name   string `json:"name"`
	Data   any    `json:"data"`
	Sender string `json:"sender,omitempty"`
}

func (s *Server) newWindow(doc bool) *window {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastID++
	w := &window{s: s, id: s.lastID, doc: doc, conns: map[*conn]bool{}, title: "mdsh"}
	s.windows[w.id] = w
	return w
}

func (w *window) name() string { return "w" + strconv.FormatUint(uint64(w.id), 10) }

func (w *window) attach(c *conn) {
	w.mu.Lock()
	w.conns[c] = true
	title, closed := w.title, w.closed
	w.mu.Unlock()
	w.sendTo(c, message{Name: "mdsh:title", Data: title})
	if closed {
		w.sendTo(c, message{Name: "mdsh:closed"})
	}
}

// detach forgets c. Questions waiting on the window's last page are
// cancelled, and a settings page with none left goes.
func (w *window) detach(c *conn) {
	w.mu.Lock()
	delete(w.conns, c)
	empty := len(w.conns) == 0
	w.mu.Unlock()
	if !empty {
		return
	}
	w.s.cancelAsks(w.id)
	if !w.doc {
		w.s.mu.Lock()
		delete(w.s.windows, w.id)
		w.s.mu.Unlock()
	}
}

func (w *window) send(m message) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for c := range w.conns {
		c.push(b)
	}
}

func (w *window) sendTo(c *conn, m message) {
	if b, err := json.Marshal(m); err == nil {
		c.push(b)
	}
}

// connected reports whether a page shows the window.
func (w *window) connected() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.conns) > 0
}

func (c *conn) push(b []byte) {
	select {
	case c.send <- b:
	default:
		c.goneOnce.Do(func() { close(c.gone) })
	}
}

func (w *window) ID() uint { return w.id }

func (w *window) Emit(name string, data any) {
	w.send(message{Name: name, Data: data, Sender: w.name()})
}

func (w *window) SetTitle(title string) {
	w.mu.Lock()
	w.title = title
	w.mu.Unlock()
	w.send(message{Name: "mdsh:title", Data: title})
}

// Close closes the window if the App lets it; it then tells its pages,
// which say so and offer a new one.
func (w *window) Close() {
	if !w.s.ctrl.Closing(w.id) {
		return
	}
	w.s.mu.Lock()
	delete(w.s.windows, w.id)
	w.s.mu.Unlock()
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	w.send(message{Name: "mdsh:closed"})
}

func (w *window) Focus() { w.send(message{Name: "mdsh:focus"}) }

// MoveTo does nothing: a page can't move its browser window.
func (w *window) MoveTo(x, y int) {}

// NewWindow makes a window, and asks the page in front to open it.
func (s *Server) NewWindow() ui.Window {
	w := s.newWindow(true)
	if from := s.front(); from != nil {
		from.send(message{Name: "mdsh:open-window", Data: "/window.html?w=" + strconv.FormatUint(uint64(w.id), 10)})
	}
	return w
}

// front is the window that last made a call and is still shown, or nil.
func (s *Server) front() *window {
	s.mu.Lock()
	w := s.windows[s.active]
	s.mu.Unlock()
	if w == nil || !w.connected() {
		return nil
	}
	return w
}

func (s *Server) WindowOf(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value(windowKey{}).(uint)
	return id, ok
}

func (s *Server) Current() (uint, bool) {
	if w := s.front(); w != nil {
		return w.id, true
	}
	return 0, false
}

func (s *Server) EmitAll(name string, data any) {
	s.mu.Lock()
	wins := make([]*window, 0, len(s.windows))
	for _, w := range s.windows {
		wins = append(wins, w)
	}
	s.mu.Unlock()
	for _, w := range wins {
		w.send(message{Name: name, Data: data})
	}
}

func (s *Server) OnMain(fn func()) { fn() }

// over is the window a dialog for win shows in: win, else the window in
// front.
func (s *Server) over(win ui.Window) *window {
	if w, ok := win.(*window); ok && w != nil {
		return w
	}
	return s.front()
}

func (s *Server) ShowError(win ui.Window, err error) {
	if w := s.over(win); w != nil {
		w.send(message{Name: "mdsh:error", Data: err.Error()})
	}
}

// ask is a question or prompt waiting for its page's answer.
type ask struct {
	window uint
	reply  chan string
}

// question is what runtime.js draws for Ask.
type question struct {
	ID      int      `json:"id"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Buttons []string `json:"buttons"`
	Default int      `json:"default"` // -1 for none
	Cancel  int      `json:"cancel"`  // -1 for none
}

// prompt is what runtime.js draws for a file or folder: a path to type.
type prompt struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Value string `json:"value"`
}

// errNoPage is returned when a dialog has no page to show in.
var errNoPage = errors.New("no page is open to ask in")

// wait shows m in w and waits for its answer, "" if w's last page goes
// first.
func (s *Server) wait(w *window, m func(id int) message) string {
	s.mu.Lock()
	s.lastAsk++
	id := s.lastAsk
	a := &ask{window: w.id, reply: make(chan string, 1)}
	s.asks[id] = a
	s.mu.Unlock()
	w.send(m(id))
	return <-a.reply
}

// cancelAsks answers "" to the questions waiting on window id.
func (s *Server) cancelAsks(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for n, a := range s.asks {
		if a.window == id {
			delete(s.asks, n)
			a.reply <- ""
		}
	}
}

func (s *Server) Ask(win ui.Window, q ui.Question) {
	w := s.over(win)
	if w == nil {
		return
	}
	qq := question{Title: q.Title, Message: q.Message, Default: -1, Cancel: -1}
	for i, b := range q.Buttons {
		qq.Buttons = append(qq.Buttons, b.Label)
		if b.Default {
			qq.Default = i
		}
		if b.Cancel {
			qq.Cancel = i
		}
	}
	go func() {
		reply := s.wait(w, func(id int) message {
			qq.ID = id
			return message{Name: "mdsh:ask", Data: qq}
		})
		i, err := strconv.Atoi(reply)
		if err != nil || i < 0 || i >= len(q.Buttons) {
			return
		}
		if do := q.Buttons[i].Do; do != nil {
			do()
		}
	}()
}

// promptPath asks for a path in w, starting with value.
func (s *Server) promptPath(w *window, title, value string) (string, error) {
	if w == nil {
		return "", errNoPage
	}
	return s.wait(w, func(id int) message {
		return message{Name: "mdsh:prompt", Data: prompt{ID: id, Title: title, Value: value}}
	}), nil
}

func (s *Server) OpenFiles(win ui.Window, dir string) ([]string, error) {
	file, err := s.promptPath(s.over(win), "Open a markdown file", strings.TrimSuffix(dir, "/")+"/")
	if err != nil || file == "" {
		return nil, err
	}
	return []string{file}, nil
}

func (s *Server) SaveFile(win ui.Window, sf ui.SaveFile) (string, error) {
	return s.promptPath(s.over(win), sf.Message, strings.TrimSuffix(sf.Dir, "/")+"/"+sf.Name)
}

func (s *Server) ChooseFolder(ctx context.Context, title, dir string) (string, error) {
	var w *window
	if id, ok := s.WindowOf(ctx); ok {
		s.mu.Lock()
		w = s.windows[id]
		s.mu.Unlock()
	}
	return s.promptPath(w, title, dir)
}

// Reveal can't show a file manager in the browser.
func (s *Server) Reveal(file string) error {
	return errors.New("the browser can't show the file in a file manager; it is at " + file)
}

// OpenURL opens url in a new browser tab, from the window in front.
// runtime.js opens web links itself, as the click happens, so this is
// for what gets past it.
func (s *Server) OpenURL(url string) error {
	w := s.front()
	if w == nil {
		return errNoPage
	}
	w.send(message{Name: "mdsh:open-url", Data: url})
	return nil
}

// OpenFile opens file in a new browser tab, as the server serves files
// beside open documents.
func (s *Server) OpenFile(file string) error {
	u := url.URL{Path: notebook.DocPrefix + filepath.ToSlash(file)}
	return s.OpenURL(u.EscapedPath())
}

// ShowView does nothing: pages have no View menu.
func (s *Server) ShowView(ui.View) {}

// RecentChanged does nothing: pages have no Open Recent menu.
func (s *Server) RecentChanged() {}

// Quit does nothing: the server runs until it is stopped.
func (s *Server) Quit() {}
