package tui

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
)

// copiedMsg says how putting a block's code, or lines of History, on
// the clipboard went.
type copiedMsg struct {
	// label names the block; it is "" for History's lines.
	label string
	lines int
	err   error
	// program says a program in the block copied, with OSC 52.
	program bool
}

// programCopyMsg is what a program on a terminal mdsh draws, a tty
// block's pane, a background one's or History's, copied with OSC 52.
// label names where it runs; block, if label is "", is the block that
// was on the tty pane.
type programCopyMsg struct {
	text  string
	label string
	block int
}

// programCopy puts what a program copied on the clipboard, as the
// terminal it would run in otherwise would, and says so in the status
// line, so no program changes the clipboard unseen.
func (m *Model) programCopy(msg programCopyMsg) tea.Cmd {
	label := msg.label
	if label == "" {
		if i := msg.block; i >= 0 && i < len(m.d.Blocks) {
			label = m.d.Label(m.d.Blocks[msg.block])
		} else {
			label = "a program"
		}
	}
	text := msg.text
	return func() tea.Msg {
		return copiedMsg{label: label, lines: strings.Count(text, "\n") + 1, err: setClipboard(text), program: true}
	}
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
	if msg.program {
		m.status = msg.label + " copied " + strconv.Itoa(msg.lines) + " " + unit
		if m.running {
			m.runNote = m.status
		}
		return
	}
	if msg.label == "" {
		m.status = "copied " + strconv.Itoa(msg.lines) + " " + unit + " from History"
		return
	}
	m.status = "copied " + msg.label + ", " + strconv.Itoa(msg.lines) + " " + unit
}

// setClipboard puts text on the clipboard. It sends the terminal an OSC
// 52 sequence, which sets the clipboard of the machine the terminal runs
// on, over ssh too, in terminals that allow it. Inside tmux it also
// hands tmux the text (see tmuxCopy), and wraps the sequence so tmux
// passes it on to the terminal. Run locally, outside ssh, it also sets
// the system clipboard (pbcopy, wl-copy, xclip or xsel), for terminals
// that don't, such as macOS's Terminal. Tests replace it.
var setClipboard = func(text string) error {
	inTmux := os.Getenv("TMUX") != ""
	if inTmux {
		_ = tmuxCopy(text)
	}
	_, err := io.WriteString(os.Stdout, clipboardSeq(text, inTmux))
	if os.Getenv("SSH_CONNECTION") == "" && os.Getenv("SSH_TTY") == "" {
		if cerr := clipboard.WriteAll(text); cerr == nil {
			return nil
		}
	}
	return err
}

// tmuxCopy puts text in a tmux paste buffer with load-buffer -w, which
// has tmux (3.2 and later) set the terminal's clipboard with an OSC 52
// of its own. That works under tmux's default set-clipboard external,
// which drops the OSC 52 a program sends; an older tmux fails it, and
// the passthrough clipboardSeq sends does the job. Tests replace it.
var tmuxCopy = func(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "tmux", "load-buffer", "-w", "-")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// clipboardSeq is what setClipboard writes to the terminal: the OSC 52
// for text, or, inside tmux, the same in tmux's passthrough, which tmux
// 3.3 and later pass on only with allow-passthrough on, and earlier ones
// always.
func clipboardSeq(text string, inTmux bool) string {
	if inTmux {
		return tmuxPassthrough(osc52(text))
	}
	return osc52(text)
}

// osc52 is the escape sequence that asks the terminal to put text on the
// clipboard.
func osc52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}

// tmuxPassthrough wraps seq in a DCS that tmux hands to the terminal
// outside it as it is, each ESC in seq doubled.
func tmuxPassthrough(seq string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
}
