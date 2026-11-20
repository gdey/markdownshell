package notebook

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTerminal(t *testing.T) {
	file := write(t, "```sh {#ask tty}\necho name?\nread -r name\necho \"hi $name\"\n```\n")
	var (
		mu    sync.Mutex
		kinds []string
		shown strings.Builder
		n     *Notebook
	)
	n, err := Open(file, Options{OnEvent: func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind != "tty" {
			kinds = append(kinds, e.Kind)
		}
		shown.Write(e.Bytes)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	n.SetTerminalSize(100, 20)
	go func() {
		for end := time.Now().Add(20 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			asked := strings.Contains(shown.String(), "name?")
			mu.Unlock()
			if asked {
				n.Type("bob\r")
				return
			}
		}
	}()
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(shown.String(), "hi bob") {
		t.Errorf("terminal showed %q", shown.String())
	}
	if got := strings.Join(kinds, " "); got != "started terminal terminal-end done" {
		t.Errorf("events = %s", got)
	}
	c := n.Cells()
	var img string
	for _, cell := range c {
		if cell.Block == 0 {
			img = cell.HTML + cell.Output
		}
	}
	if !strings.Contains(img, ".cast") {
		t.Errorf("no recording in the cell: %q", img)
	}
	// Keys with no tty block running go nowhere.
	n.Type("x")
	if len(n.term.keys) != 0 {
		t.Error("keys kept with nothing running")
	}
}

// A background tty block runs on a terminal of its own, in its card,
// while another block runs; keys typed in its card reach it, and its
// events say they are the background terminal's.
func TestBackgroundTerminal(t *testing.T) {
	file := write(t, "```sh {#edit background tty}\necho name?\nread -r name\necho \"hi $name\"\n```\n\n```sh {#b}\necho b\n```\n")
	var (
		mu    sync.Mutex
		kinds []string
		shown strings.Builder
	)
	n, err := Open(file, Options{OnEvent: func(e Event) {
		mu.Lock()
		defer mu.Unlock()
		if e.Kind == "tty" && e.Block == 0 && e.Data == "background" {
			shown.Write(e.Bytes)
			return
		}
		if e.Kind != "stdout" {
			kinds = append(kinds, e.Kind+":"+e.Data)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	n.SetBackgroundTerminalSize(90, 12)
	if err := n.RunBlock(0); err != nil {
		t.Fatal(err)
	}
	if err := n.RunBlock(1); err != nil {
		t.Fatal(err)
	}
	waitShown := func(want string) {
		t.Helper()
		for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			ok := strings.Contains(shown.String(), want)
			mu.Unlock()
			if ok {
				return
			}
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("terminal showed %q, want %q", shown.String(), want)
	}
	waitShown("name?")
	var running bool
	for _, c := range n.Cells() {
		if c.Block == 0 {
			running = c.Running && c.Terminal
		}
	}
	if !running {
		t.Error("edit's cell doesn't say it runs on a terminal")
	}
	n.TypeBackground(0, "bob\r")
	waitShown("hi bob")
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		n.reap()
		if c := n.Cells(); !c[cellOf(c, 0)].Running {
			break
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if got := strings.Join(kinds, " "); got != "background: terminal:background started: done: terminal-end:background done:" {
		t.Errorf("events = %s", got)
	}
}

func cellOf(cells []Cell, block int) int {
	for i, c := range cells {
		if c.Block == block {
			return i
		}
	}
	return -1
}
