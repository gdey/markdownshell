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
	if !strings.Contains(img, ".gif") {
		t.Errorf("no recording in the cell: %q", img)
	}
	// Keys with no tty block running go nowhere.
	n.Type("x")
	if len(n.term.keys) != 0 {
		t.Error("keys kept with nothing running")
	}
}
