package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/settings"
)

// stdioClient drives ServeStdio as a plugin would.
type stdioClient struct {
	t     *testing.T
	in    *io.PipeWriter
	lines chan map[string]any
	done  chan error
	id    int
	seen  []map[string]any // lines read but not yet wanted
}

func startStdio(t *testing.T, file string) *stdioClient {
	t.Helper()
	store, err := settings.Open(filepath.Join(t.TempDir(), "gui.json"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Settings: store, Cwd: t.TempDir(), File: file})
	if err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &stdioClient{t: t, in: inW, lines: make(chan map[string]any, 1024), done: make(chan error, 1)}
	go func() {
		c.done <- s.ServeStdio(context.Background(), inR, outW)
		outW.Close()
	}()
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(nil, 64<<20)
		for sc.Scan() {
			var m map[string]any
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("bad line %q: %v", sc.Text(), err)
				continue
			}
			c.lines <- m
		}
		close(c.lines)
	}()
	t.Cleanup(func() { c.close() })
	return c
}

func (c *stdioClient) send(v any) {
	c.t.Helper()
	b, _ := json.Marshal(v)
	if _, err := c.in.Write(append(b, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

// next returns the first line that ok accepts and no earlier next took.
// Replies and events can come in any order.
func (c *stdioClient) next(what string, ok func(map[string]any) bool) map[string]any {
	c.t.Helper()
	for i, m := range c.seen {
		if ok(m) {
			c.seen = append(c.seen[:i], c.seen[i+1:]...)
			return m
		}
	}
	timeout := time.After(10 * time.Second)
	for {
		select {
		case m, open := <-c.lines:
			if !open {
				c.t.Fatalf("waiting for %s: output ended", what)
			}
			if ok(m) {
				return m
			}
			c.seen = append(c.seen, m)
		case <-timeout:
			c.t.Fatalf("waiting for %s: timed out", what)
		}
	}
}

// call calls name from window 1 and tab, if not 0, and returns the reply.
func (c *stdioClient) call(tab int, name string, args ...any) map[string]any {
	c.t.Helper()
	if args == nil {
		args = []any{}
	}
	c.id++
	f := map[string]any{"id": c.id, "name": "ui.App." + name, "args": args, "window": 1}
	if tab != 0 {
		f["tab"] = tab
	}
	c.send(f)
	id := float64(c.id)
	return c.next("reply to "+name, func(m map[string]any) bool { return m["id"] == id })
}

func (c *stdioClient) event(name string) map[string]any {
	c.t.Helper()
	return c.next(name, func(m map[string]any) bool { return m["name"] == name })
}

func (c *stdioClient) close() error {
	c.in.Close()
	select {
	case err := <-c.done:
		c.done <- err
		return err
	case <-time.After(10 * time.Second):
		c.t.Fatal("ServeStdio didn't end when stdin did")
		return nil
	}
}

func TestStdio(t *testing.T) {
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte("# Doc\n\n```sh\necho hi\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := startStdio(t, file)

	if r := c.call(1, "Doc"); r["result"].(map[string]any)["file"] != file {
		t.Errorf("Doc: %v", r)
	}
	if r := c.call(0, "Tabs"); !strings.Contains(toJSON(r["result"]), `"title":"doc.md"`) {
		t.Errorf("Tabs: %v", r)
	}
	if r := c.call(1, "Nope"); !strings.Contains(r["error"].(string), "no call") {
		t.Errorf("an unknown call: %v", r)
	}
	if r := c.call(9, "Doc"); r["error"] != "no tab 9" {
		t.Errorf("a call's error: %v", r)
	}
	// A bad line is skipped, and the next call still works.
	if _, err := c.in.Write([]byte("not json\n")); err != nil {
		t.Fatal(err)
	}

	// A run sends its events, marked with their window.
	if r := c.call(1, "RunBlock", 0); r["error"] != nil {
		t.Fatalf("RunBlock: %v", r)
	}
	m := c.event("block")
	if m["window"] != float64(1) || m["sender"] != "w1" {
		t.Errorf("block event: %v", m)
	}
	c.event("ran")

	// Closing the tab asks about the unsaved output; the answer closes
	// the window.
	if r := c.call(0, "CloseTab", 1); r["error"] != nil {
		t.Fatalf("CloseTab: %v", r)
	}
	q := c.event("mdsh:ask")["data"].(map[string]any)
	if q["message"] != "Save the output in doc.md before closing?" {
		t.Errorf("question: %v", q)
	}
	c.send(map[string]any{"answer": q["id"], "value": "1"}) // Don't Save
	c.event("mdsh:closed")

	if err := c.close(); err != nil {
		t.Errorf("ServeStdio: %v", err)
	}
}

// When stdin ends, a question still waiting is cancelled and the
// session ends.
func TestStdioEOF(t *testing.T) {
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte("```sh\necho hi\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := startStdio(t, file)
	c.call(1, "RunBlock", 0)
	c.event("ran")
	c.call(0, "CloseTab", 1)
	c.event("mdsh:ask")
	if err := c.close(); err != nil {
		t.Errorf("ServeStdio: %v", err)
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
