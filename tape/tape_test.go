package tape

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	src := `# a comment
Output demo.gif
Set FontSize 20
Set TypingSpeed 1ms
Type "echo hi"
Type@2ms 'x'
Enter
Backspace 3
Ctrl+C
Alt+b
Shift+Tab
Sleep 500ms
Sleep 2
Wait
Wait+Screen@3s /done/
Hide
`
	cmds, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []Command{
		{Line: 4, Set: "TypingSpeed", Value: "1ms"},
		{Line: 5, Text: "echo hi"},
		{Line: 6, Text: "x", Literal: true, Speed: 2 * time.Millisecond},
		{Line: 7, Keys: "\r", Count: 1},
		{Line: 8, Keys: "\x7f", Count: 3},
		{Line: 9, Keys: "\x03", Count: 1},
		{Line: 10, Keys: "\x1bb", Count: 1},
		{Line: 11, Keys: "\x1b[Z", Count: 1},
		{Line: 12, Sleep: 500 * time.Millisecond},
		{Line: 13, Sleep: 2 * time.Second},
		{Line: 14, Wait: true},
		{Line: 15, Wait: true, WaitScreen: true, Timeout: 3 * time.Second},
	}
	if len(cmds) != len(want) {
		t.Fatalf("got %d commands, want %d: %+v", len(cmds), len(want), cmds)
	}
	for i, c := range cmds {
		if i == 11 {
			if c.Pattern == nil || c.Pattern.String() != "done" {
				t.Errorf("Wait pattern = %v", c.Pattern)
			}
			c.Pattern = nil
		}
		if c != want[i] {
			t.Errorf("command %d = %+v, want %+v", i, c, want[i])
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{"Type hi", "Bogus", "Enter x", "Sleep soon", "Wait done", "Ctrl+Space", "Set WaitPattern ("} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) = nil error", src)
		}
	}
}

// screen is a fake terminal: what was typed, echoed.
type screen struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *screen) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(bytes.ReplaceAll(p, []byte("\r"), []byte("\n")))
}

func (s *screen) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestPlay(t *testing.T) {
	cmds, err := Parse("Set TypingSpeed 0\nType \"ls\"\nEnter\nWait+Line /ls$/\nCtrl+D\n")
	if err != nil {
		t.Fatal(err)
	}
	var s screen
	if err := Play(context.Background(), cmds, &s, s.String); err != nil {
		t.Fatal(err)
	}
	if got := s.String(); got != "ls\n\x04" {
		t.Errorf("typed %q", got)
	}
}

func TestPlayWaitTimesOut(t *testing.T) {
	cmds, _ := Parse("Wait@100ms /never/")
	var s screen
	err := Play(context.Background(), cmds, &s, s.String)
	if err == nil || !strings.Contains(err.Error(), "tape line 1: Wait /never/") {
		t.Errorf("err = %v", err)
	}
}
