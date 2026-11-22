package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/gdey/markdownshell/env"
	"github.com/gdey/markdownshell/termsafe"
)

// With the interactive shell, @ moves the live History pane to another
// env's shell, as mdsh-gui's picker on the pane's bar does: the env
// starts interactively, on a terminal of its own, and what is typed
// there goes into the file with its env=. The shell the pane leaves
// keeps running, with its screen, for when it comes back.

// envPicker is the env list @ opens in the footer.
type envPicker struct {
	names []string
	at    int
	// armed is an env whose shell runs scripted, which enter again
	// starts over, interactively.
	armed string
}

// openEnvPicker opens the env list, on the env the pane is on.
func (m *Model) openEnvPicker() {
	if !m.isLive() {
		m.status = "@ moves History to another env's shell, with --interactive-shell and History open (h)"
		return
	}
	names := m.sess.Envs()
	if len(names) < 2 {
		m.status = "no block names an env=; History is on the file's shell"
		return
	}
	ep := &envPicker{names: names}
	for i, n := range names {
		if n == m.lterm.env {
			ep.at = i
		}
	}
	m.envPick = ep
}

// envKey takes a key while the env list is open.
func (m *Model) envKey(k tea.KeyMsg) tea.Cmd {
	ep := m.envPick
	switch k.String() {
	case "left", "h", "up", "k", "shift+tab":
		ep.at = (ep.at + len(ep.names) - 1) % len(ep.names)
	case "right", "l", "down", "j", "tab":
		ep.at = (ep.at + 1) % len(ep.names)
	case "enter":
		if m.moveHistory(ep.names[ep.at]) {
			m.envPick = nil
		}
	case "esc", "q", "ctrl+c", "@":
		m.envPick = nil
	}
	m.refresh()
	return nil
}

// envName names an env for the list and the pane's rule.
func envName(name string) string {
	if name == env.TopLevel {
		return "top level"
	}
	return "env=" + name
}

// envHelp is the footer while the env list is open.
func (m *Model) envHelp() string {
	ep := m.envPick
	var parts []string
	for i, n := range ep.names {
		label := termsafe.Visible(envName(n))
		if m.sess.Scripted(n) {
			label += " (scripted)"
		}
		if i == ep.at {
			label = "[" + label + "]"
		}
		parts = append(parts, label)
	}
	head := "History on:"
	if ep.armed != "" {
		head = fmt.Sprintf("%s's shell runs scripted: enter again starts it over, losing what blocks set there.", termsafe.Visible(envName(ep.armed)))
	}
	return promptStyle.Render(head) + " " + strings.Join(parts, " ") + helpStyle.Render("  ←/→ pick · enter move · esc cancel")
}

// moveHistory moves the live pane to env name's shell, starting it
// interactively. A scripted shell running for name is ended only once
// enter is pressed again on it. It reports whether the pane moved, or
// was there already.
func (m *Model) moveHistory(name string) bool {
	lt := m.lterm
	if name == lt.env && lt.sh != nil {
		return true
	}
	if m.running || lt.busy {
		m.status = "a command is running: stop it, with ctrl+z or ctrl+c, or let it end, before moving History"
		return false
	}
	if name != env.TopLevel {
		if m.sess.Scripted(name) && m.envPick != nil && m.envPick.armed != name {
			m.envPick.armed = name
			return false
		}
		m.sess.SetInteractive(name)
	}
	if lt.sh != nil {
		lt.parked[lt.env] = parkedShell{lt.sh, lt.p}
	}
	lt.env, lt.sh, lt.jobs = name, nil, nil
	if pk, ok := lt.parked[name]; ok && !pk.sh.Exited() {
		lt.sh, lt.p = pk.sh, pk.p
		delete(lt.parked, name)
	} else {
		delete(lt.parked, name)
		lt.p = lt.newPane()
	}
	if err := m.attachLive(); err != nil {
		m.status = "can't start the shell: " + err.Error()
		return true
	}
	m.fit()
	if lt.sh != nil {
		m.liveJobs(lt.sh)
	}
	m.hist.focus = true
	m.status = "History is on " + termsafe.Visible(envName(lt.env)) + "'s shell"
	return true
}
