package main

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

	"github.com/gdey/markdownshell/cmd/mdsh-gui/settings"
)

// client drives a bridge over pipes, as the Swift app does.
type client struct {
	t      *testing.T
	in     io.WriteCloser
	lines  chan map[string]json.RawMessage
	nextID int64
	events []map[string]json.RawMessage
}

func newClient(t *testing.T) *client {
	t.Helper()
	st, err := settings.Open(filepath.Join(t.TempDir(), "swift.json"))
	if err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := newConn(outW)
	b := newBridge(c, st)
	done := make(chan struct{})
	go func() {
		serve(context.Background(), inR, c, b.methods(), b.closeAll)
		outW.Close()
		close(done)
	}()
	cl := &client{t: t, in: inW, lines: make(chan map[string]json.RawMessage, 1024)}
	go func() {
		sc := bufio.NewScanner(outR)
		sc.Buffer(nil, 16*1024*1024)
		for sc.Scan() {
			var m map[string]json.RawMessage
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				t.Errorf("bad line %q: %v", sc.Text(), err)
				continue
			}
			cl.lines <- m
		}
		close(cl.lines)
	}()
	t.Cleanup(func() {
		inW.Close()
		<-done
	})
	return cl
}

// call sends a request and waits for its answer, keeping the events
// that come before it. It fails the test on an error unless wantErr.
func (c *client) call(method string, params any, result any) string {
	c.t.Helper()
	c.nextID++
	id := c.nextID
	raw, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if _, err := c.in.Write(append(raw, '\n')); err != nil {
		c.t.Fatal(err)
	}
	timeout := time.After(20 * time.Second)
	for {
		select {
		case m, ok := <-c.lines:
			if !ok {
				c.t.Fatalf("%s: bridge closed", method)
			}
			if _, isEvent := m["event"]; isEvent {
				c.events = append(c.events, m)
				continue
			}
			var got int64
			json.Unmarshal(m["id"], &got)
			if got != id {
				c.t.Fatalf("%s: answer for %d, want %d", method, got, id)
			}
			if e, ok := m["error"]; ok {
				var s string
				json.Unmarshal(e, &s)
				return s
			}
			if result != nil {
				if err := json.Unmarshal(m["result"], result); err != nil {
					c.t.Fatalf("%s: result: %v", method, err)
				}
			}
			return ""
		case <-timeout:
			c.t.Fatalf("%s: no answer", method)
		}
	}
}

func (c *client) mustCall(method string, params any, result any) {
	c.t.Helper()
	if err := c.call(method, params, result); err != "" {
		c.t.Fatalf("%s: %s", method, err)
	}
}

func TestRunAndSave(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.md")
	os.WriteFile(file, []byte("# A\n\n```sh {#hi}\necho hello\n```\n"), 0o644)
	c := newClient(t)

	var info docInfo
	c.mustCall("open", map[string]any{"file": file}, &info)
	if info.ID == 0 || info.File != file {
		t.Fatalf("open: %+v", info)
	}
	var again docInfo
	c.mustCall("open", map[string]any{"file": file}, &again)
	if again.ID != info.ID {
		t.Errorf("opening the file again gave doc %d, want %d", again.ID, info.ID)
	}

	var cells []struct {
		Block int    `json:"block"`
		ID    string `json:"id"`
		Body  string `json:"body"`
	}
	c.mustCall("cells", map[string]any{"doc": info.ID}, &cells)
	var block = -1
	for _, cl := range cells {
		if cl.ID == "hi" {
			block = cl.Block
		}
	}
	if block < 0 {
		t.Fatalf("no #hi block in %+v", cells)
	}

	c.mustCall("runBlock", map[string]any{"doc": info.ID, "block": block}, nil)
	var out strings.Builder
	var ran bool
	for _, e := range c.events {
		var name string
		json.Unmarshal(e["event"], &name)
		switch name {
		case "block":
			var be struct{ Kind, Data string }
			json.Unmarshal(e["data"], &be)
			if be.Kind == "stdout" {
				out.WriteString(be.Data)
			}
		case "ran":
			ran = true
		}
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("stdout events = %q, want hello", out.String())
	}
	if !ran {
		t.Error("no ran event")
	}

	c.mustCall("info", map[string]any{"doc": info.ID}, &info)
	if !info.Dirty {
		t.Error("not dirty after a run")
	}
	c.mustCall("save", map[string]any{"doc": info.ID}, &info)
	src, _ := os.ReadFile(file)
	if !strings.Contains(string(src), "hello") {
		t.Errorf("saved file has no output:\n%s", src)
	}

	var start start
	c.mustCall("startScreen", map[string]any{"dir": dir}, &start)
	if len(start.Recent) != 1 || start.Recent[0] != file {
		t.Errorf("recent = %v, want [%s]", start.Recent, file)
	}
	if len(start.Files) != 1 {
		t.Errorf("files = %+v, want a.md", start.Files)
	}

	c.mustCall("close", map[string]any{"doc": info.ID}, nil)
	if err := c.call("info", map[string]any{"doc": info.ID}, nil); !strings.Contains(err, "no such document") {
		t.Errorf("info after close: %q", err)
	}
}

func TestConfirm(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c.md")
	os.WriteFile(file, []byte("```sh {#ok confirm}\necho yes\n```\n"), 0o644)
	c := newClient(t)
	var info docInfo
	c.mustCall("open", map[string]any{"file": file}, &info)

	// runAll waits on the confirm, so send it without waiting, answer
	// the confirm event, then read runAll's answer.
	c.nextID++
	runID := c.nextID
	raw, _ := json.Marshal(map[string]any{"id": runID, "method": "runAll", "params": map[string]any{"doc": info.ID}})
	c.in.Write(append(raw, '\n'))
	for m := range c.lines {
		var name string
		json.Unmarshal(m["event"], &name)
		if name == "confirm" {
			break
		}
	}
	c.mustCall("answer", map[string]any{"doc": info.ID, "reply": "run"}, nil)
	for m := range c.lines {
		var id int64
		json.Unmarshal(m["id"], &id)
		if id == runID {
			if e, ok := m["error"]; ok {
				t.Fatalf("runAll: %s", e)
			}
			break
		}
	}
	var cells []struct {
		ID     string `json:"id"`
		Output string `json:"output"`
	}
	c.mustCall("cells", map[string]any{"doc": info.ID}, &cells)
	for _, cl := range cells {
		if cl.ID == "ok" && !strings.Contains(cl.Output, "yes") {
			t.Errorf("output after confirm = %q", cl.Output)
		}
	}
}

func TestUnknownMethod(t *testing.T) {
	c := newClient(t)
	if err := c.call("nope", nil, nil); !strings.Contains(err, "unknown method") {
		t.Errorf("err = %q", err)
	}
}

func TestSetSettingsDropsBadValues(t *testing.T) {
	c := newClient(t)
	var got settings.Settings
	c.mustCall("setSettings", settings.Settings{Theme: "purple", Look: "tty", TextSize: 400, Accent: "#ABCDEF"}, &got)
	if got.Theme != "" || got.Look != "tty" || got.TextSize != 150 || got.Accent != "#abcdef" {
		t.Errorf("settings = %+v", got)
	}
}

// TestReloadStopsQuestion checks that reloading while a block waits on
// its confirm question ends that run, so a later answer has nothing to
// resume.
func TestReloadStopsQuestion(t *testing.T) {
	file := filepath.Join(t.TempDir(), "r.md")
	os.WriteFile(file, []byte("```sh {#ok confirm}\necho yes\n```\n"), 0o644)
	c := newClient(t)
	var info docInfo
	c.mustCall("open", map[string]any{"file": file}, &info)

	c.nextID++
	runID := c.nextID
	raw, _ := json.Marshal(map[string]any{"id": runID, "method": "runAll", "params": map[string]any{"doc": info.ID}})
	c.in.Write(append(raw, '\n'))
	for m := range c.lines {
		var name string
		json.Unmarshal(m["event"], &name)
		if name == "confirm" {
			break
		}
	}
	// reload's answer and runAll's can come in either order.
	c.nextID++
	reloadID := c.nextID
	raw, _ = json.Marshal(map[string]any{"id": reloadID, "method": "reload", "params": map[string]any{"doc": info.ID}})
	c.in.Write(append(raw, '\n'))
	got := map[int64]bool{}
	timeout := time.After(20 * time.Second)
	for !got[runID] || !got[reloadID] {
		select {
		case m := <-c.lines:
			var id int64
			json.Unmarshal(m["id"], &id)
			got[id] = true
		case <-timeout:
			t.Fatalf("answers so far: %v", got)
		}
	}
	if err := c.call("answer", map[string]any{"doc": info.ID, "reply": "run"}, nil); !strings.Contains(err, "nothing is waiting") {
		t.Errorf("answer after reload: %q", err)
	}
}

// TestNosave checks that typing in a nosave textbox and running a
// nosave block leave the document clean, and that saving keeps the
// file's text for both.
func TestNosave(t *testing.T) {
	file := filepath.Join(t.TempDir(), "n.md")
	src := "```textbox {#token var=TOKEN nosave}\nfrom the file\n```\n\n```sh {#now nosave}\necho \"token is $MDSH_VAR_TOKEN\"\n```\n"
	os.WriteFile(file, []byte(src), 0o644)
	c := newClient(t)
	var info docInfo
	c.mustCall("open", map[string]any{"file": file}, &info)
	var cells []struct {
		Block   int `json:"block"`
		Textbox *struct {
			Block  int  `json:"block"`
			NoSave bool `json:"nosave"`
		} `json:"textbox"`
		ID     string `json:"id"`
		NoSave bool   `json:"nosave"`
		Output string `json:"output"`
	}
	c.mustCall("cells", map[string]any{"doc": info.ID}, &cells)
	box, run := -1, -1
	for _, cl := range cells {
		if cl.Textbox != nil && cl.Textbox.NoSave {
			box = cl.Textbox.Block
		}
		if cl.ID == "now" && cl.NoSave {
			run = cl.Block
		}
	}
	if box < 0 || run < 0 {
		t.Fatalf("no nosave textbox and block in %+v", cells)
	}
	c.mustCall("setTextbox", map[string]any{"doc": info.ID, "block": box, "text": "typed"}, nil)
	c.mustCall("runBlock", map[string]any{"doc": info.ID, "block": run}, nil)
	c.mustCall("cells", map[string]any{"doc": info.ID}, &cells)
	for _, cl := range cells {
		if cl.ID == "now" && !strings.Contains(cl.Output, "token is typed") {
			t.Errorf("output = %q, want token is typed", cl.Output)
		}
	}
	c.mustCall("info", map[string]any{"doc": info.ID}, &info)
	if info.Dirty {
		t.Error("dirty after typing in a nosave textbox and running a nosave block")
	}
	c.mustCall("save", map[string]any{"doc": info.ID}, &info)
	if got, _ := os.ReadFile(file); string(got) != src {
		t.Errorf("saved file changed:\n%s", got)
	}
}
