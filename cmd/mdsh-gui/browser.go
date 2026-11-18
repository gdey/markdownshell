package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/gdey/markdownshell/serve"
	"github.com/gdey/markdownshell/settings"
)

// errServeNoFile is why Serve in Browser does nothing: there's no saved
// file in front to serve.
var errServeNoFile = errors.New("Serve in Browser serves the file in front: open or save one first")

// browsers serves files to the web browser for File › Serve in Browser,
// as mdsh --serve does: one server for each file, on a free port of
// 127.0.0.1, with its own token, shells and windows, until the app
// quits. Serving a file again opens the same address.
type browsers struct {
	// opts are the options every server starts from; File and Addr are
	// filled in for each.
	opts serve.Options

	mu   sync.Mutex
	ctx  context.Context
	stop context.CancelFunc
	urls map[string]string // by file, while its server runs
	wg   sync.WaitGroup
}

func newBrowsers(opts serve.Options) *browsers {
	ctx, stop := context.WithCancel(context.Background())
	return &browsers{opts: opts, ctx: ctx, stop: stop, urls: map[string]string{}}
}

// serve starts serving file, unless it is served already, and returns
// the address to open, token included.
func (b *browsers) serve(file string) (string, error) {
	file, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.ctx.Err() != nil {
		return "", errors.New("mdsh-gui is quitting")
	}
	if u, ok := b.urls[file]; ok {
		return u, nil
	}
	opts := b.opts
	opts.File, opts.Addr, opts.Log = file, "127.0.0.1:0", io.Discard
	s, err := serve.New(opts)
	if err != nil {
		return "", err
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	u := s.URL(l)
	b.urls[file] = u
	slog.Info("serving in the browser", "file", file, "addr", l.Addr().String())
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		if err := s.Serve(b.ctx, l); err != nil {
			slog.Warn("serving in the browser", "file", file, "err", err)
		}
		b.mu.Lock()
		delete(b.urls, file)
		b.mu.Unlock()
	}()
	return u, nil
}

// close stops every server, closing its shells, and waits up to wait
// for them to end.
func (b *browsers) close(wait time.Duration) {
	b.mu.Lock()
	b.stop()
	b.mu.Unlock()
	done := make(chan struct{})
	go func() { b.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(wait):
	}
}

// serveInBrowser serves the file in front to the web browser and opens
// it there, when Settings turns serving on.
func (g *gui) serveInBrowser() {
	if !g.ctrl.Settings().Get().Serve {
		g.ctrl.ShowError(errServeOff)
		return
	}
	file := ""
	if id, ok := g.Current(); ok {
		file = g.ctrl.File(id)
	}
	if file == "" {
		g.ctrl.ShowError(errServeNoFile)
		return
	}
	u, err := g.browsers.serve(file)
	if err == nil {
		err = g.OpenURL(u)
	}
	if err != nil {
		g.ctrl.ShowError(err)
	}
}

// addServeItem adds File › Serve in Browser to file, shown while
// Settings turns serving on.
func (g *gui) addServeItem(file *application.Menu) {
	g.serveItem = file.Add("Serve in Browser").OnClick(func(*application.Context) { go g.serveInBrowser() })
	st := g.ctrl.Settings()
	g.serveItem.SetHidden(!st.Get().Serve)
	st.OnChange(func(s settings.Settings) {
		g.OnMain(func() {
			g.serveItem.SetHidden(!s.Serve)
			if g.menu != nil {
				g.menu.Update()
			}
		})
	})
}
