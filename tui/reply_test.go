package tui

import (
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// keys records the keys a program gets.
type keys struct{ got []string }

func (k *keys) Init() tea.Cmd { return nil }
func (k *keys) View() string  { return "" }
func (k *keys) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(tea.KeyMsg); ok {
		k.got = append(k.got, m.String())
	}
	return k, nil
}

func TestReplyFilter(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"st", "a\x1b]11;rgb:1e1e/1e1e/1e1e\x1b\\b", []string{"a", "b"}},
		{"bel", "\x1b]11;rgb:1e1e/1e1e/1e1e\x07b", []string{"b"}},
		{"keys", "abc", []string{"abc"}},
		{"alt", "\x1b]x", []string{"x"}},
		{"cut short", "\x1b]11;rgb:1e1e\r", []string{"enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, w := io.Pipe()
			k := &keys{}
			var f replyFilter
			p := tea.NewProgram(k, tea.WithInput(r), tea.WithOutput(io.Discard), tea.WithFilter(f.filter))
			go func() {
				w.Write([]byte(tt.in))
				time.Sleep(100 * time.Millisecond)
				p.Quit()
			}()
			if _, err := p.Run(); err != nil {
				t.Fatal(err)
			}
			if len(k.got) != len(tt.want) {
				t.Fatalf("got %q, want %q", k.got, tt.want)
			}
			for i := range k.got {
				if k.got[i] != tt.want[i] {
					t.Fatalf("got %q, want %q", k.got, tt.want)
				}
			}
		})
	}
}
