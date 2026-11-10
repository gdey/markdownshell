package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/ctl"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termsafe"
)

// askMsg is a question a running block put with mdsh ctl ask or
// confirm. It is asked in the block's card, as a confirm block's
// question is.
type askMsg struct {
	q     run.Question
	reply chan askReply // buffered, so a late answer never blocks
}

type askReply struct {
	answer string
	by     run.Answered
}

// askGoneMsg withdraws question id: its block ended while it waited.
type askGoneMsg struct{ id string }

// ask asks q from the control socket's goroutine and waits for the
// answer, or for ctx to end when the block does.
func (m *Model) ask(ctx context.Context, q run.Question) (string, run.Answered) {
	if m.opts.Yes && q.Kind == run.AskCtlConfirm {
		if q.HasDefault {
			return q.Default, run.ByDefault
		}
		return ctl.Yes, run.ByYes
	}
	reply := make(chan askReply, 1)
	if !m.send(askMsg{q: q, reply: reply}) {
		return "", run.ByNone
	}
	select {
	case r := <-reply:
		return r.answer, r.by
	case <-ctx.Done():
		m.send(askGoneMsg{q.ID})
		return "", run.ByNone
	case <-m.done:
		return "", run.ByNone
	}
}

// asking is the question being asked, the first of m.asks, or nil.
func (m *Model) asking() *askMsg {
	if len(m.asks) == 0 {
		return nil
	}
	return m.asks[0]
}

// askAdd queues msg, and asks it if nothing else is being asked.
func (m *Model) askAdd(msg askMsg) tea.Cmd {
	m.asks = append(m.asks, &msg)
	if len(m.asks) == 1 {
		return m.askShow()
	}
	return nil
}

// askShow readies the field for the question now asked, and moves to
// its block.
func (m *Model) askShow() tea.Cmd {
	a := m.asking()
	if a == nil {
		return nil
	}
	m.askIn = textinput.New()
	m.askIn.Prompt = "> "
	m.askPick = 0
	if a.q.Secret {
		m.askIn.EchoMode = textinput.EchoPassword
		m.askIn.EchoCharacter = '•'
	}
	if a.q.HasDefault {
		m.askIn.Placeholder = a.q.Default
		for i, c := range a.q.Choices {
			if c == a.q.Default {
				m.askPick = i
			}
		}
	}
	m.helping = false
	if i, ok := m.index[a.q.Block]; ok {
		m.moveTo(i)
	}
	return m.askIn.Focus()
}

// askDone answers the question being asked, and asks the next.
func (m *Model) askDone(answer string, by run.Answered) tea.Cmd {
	a := m.asking()
	a.reply <- askReply{answer, by}
	m.asks = m.asks[1:]
	return m.askShow()
}

// askGone drops question id, answered or not.
func (m *Model) askGone(id string) tea.Cmd {
	for n, a := range m.asks {
		if a.q.ID != id {
			continue
		}
		m.asks = append(m.asks[:n], m.asks[n+1:]...)
		if n == 0 {
			return m.askShow()
		}
		break
	}
	return nil
}

// askKey handles a key while a block's question is up. esc gives no
// answer, so the block's --default answers, or the call exits 4; for
// confirm, it is no. ctrl+c gives no answer and stops the run.
func (m *Model) askKey(k tea.KeyMsg) tea.Cmd {
	q := m.asking().q
	key := k.String()
	switch {
	case key == "ctrl+c":
		// Stop the run, as ctrl+c does when nothing asks.
		cmd := m.askDone("", run.ByNone)
		if m.running {
			m.stop()
		}
		return cmd
	case q.Kind == run.AskCtlConfirm:
		switch key {
		case "y":
			return m.askDone(ctl.Yes, run.ByUser)
		case "n", "esc":
			return m.askDone(ctl.NoValue, run.ByUser)
		case "enter":
			if q.HasDefault {
				return m.askDone(q.Default, run.ByUser)
			}
		}
		return nil
	case key == "esc":
		return m.askDone("", run.ByNone)
	case len(q.Choices) > 0:
		switch key {
		case "enter":
			return m.askDone(q.Choices[m.askPick], run.ByUser)
		case "left", "up", "h", "k", "shift+tab":
			m.askPick = (m.askPick + len(q.Choices) - 1) % len(q.Choices)
		case "right", "down", "l", "j", "tab", " ":
			m.askPick = (m.askPick + 1) % len(q.Choices)
		default:
			if n := int(key[0] - '0'); len(key) == 1 && n >= 1 && n <= len(q.Choices) && n <= 9 {
				return m.askDone(q.Choices[n-1], run.ByUser)
			}
		}
		return nil
	case key == "enter":
		v := m.askIn.Value()
		if v == "" && q.HasDefault {
			v = q.Default
		}
		return m.askDone(v, run.ByUser)
	}
	var cmd tea.Cmd
	m.askIn, cmd = m.askIn.Update(k)
	return cmd
}

// askView draws the question being asked of block b in its card, w
// wide, or "" if b isn't asking.
func (m *Model) askView(i, w int) string {
	a := m.asking()
	if a == nil || m.index[a.q.Block] != i {
		return ""
	}
	q := a.q
	text := promptStyle.Render(termsafe.Visible(q.Text))
	var line, keys string
	switch {
	case q.Kind == run.AskCtlConfirm:
		keys = "y yes · n no"
		if q.HasDefault {
			keys += " · enter " + q.Default
		}
	case len(q.Choices) > 0:
		var cs []string
		for n, c := range q.Choices {
			c = fmt.Sprintf("%d %s", n+1, termsafe.Visible(c))
			if n == m.askPick {
				c = pickedStyle.Render("▸ " + c)
			} else {
				c = "  " + c
			}
			cs = append(cs, c)
		}
		line = strings.Join(cs, "  ")
		keys = "←/→ pick · enter answer · esc no answer"
	default:
		line = m.askIn.View()
		keys = "enter answer · esc no answer"
		if q.Secret {
			keys += " · hidden as you type"
		}
	}
	body := text
	if line != "" {
		body += "\n" + line
	}
	return lipgloss.NewStyle().MarginLeft(4).Width(w - 2).Render(body + "\n" + helpStyle.Render(keys))
}

// askHelp is the help line while a block's question is up.
func (m *Model) askHelp() string {
	q := m.asking().q
	return promptStyle.Render(m.name(q.Block)+" asks:") + helpStyle.Render(" "+termsafe.Visible(q.Text))
}
