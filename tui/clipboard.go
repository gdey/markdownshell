package tui

import (
	"encoding/base64"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// copiedMsg says how putting a block's code on the clipboard went.
type copiedMsg struct {
	label string
	lines int
	err   error
}

// copyBlock puts the code of the block under the cursor on the
// clipboard, as written in the file, without its fences.
func (m *Model) copyBlock() tea.Cmd {
	i := m.cur()
	if i < 0 {
		return nil
	}
	b := m.d.Blocks[i]
	text := strings.TrimSuffix(b.Body, "\n")
	label := m.d.Label(b)
	return func() tea.Msg {
		lines := 0
		if text != "" {
			lines = strings.Count(text, "\n") + 1
		}
		return copiedMsg{label: label, lines: lines, err: setClipboard(text)}
	}
}

// copied says in the status line how copyBlock went.
func (m *Model) copied(msg copiedMsg) {
	if msg.err != nil {
		m.status = "copy: " + msg.err.Error()
		return
	}
	unit := "lines"
	if msg.lines == 1 {
		unit = "line"
	}
	m.status = "copied " + msg.label + ", " + strconv.Itoa(msg.lines) + " " + unit
}

// setClipboard puts text on the clipboard. It sends the terminal an OSC
// 52 sequence, which sets the clipboard of the machine the terminal runs
// on, over ssh too, in terminals that allow it. Run locally, outside ssh,
// it also sets the system clipboard (pbcopy, wl-copy, xclip or xsel), for
// terminals that don't, such as macOS's Terminal or tmux by default.
// Tests replace it.
var setClipboard = func(text string) error {
	_, err := io.WriteString(os.Stdout, osc52(text))
	if os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_TTY") == "" {
		if cerr := clipboard.WriteAll(text); cerr == nil {
			return nil
		}
	}
	return err
}

// osc52 is the escape sequence that asks the terminal to put text on the
// clipboard.
func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}
