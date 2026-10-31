package notebook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// The History pane is a terminal at the bottom of the window, like a
// paper tape: a command typed there runs in the document's shell, on the
// terminal, and is added to the end of the document as a block with its
// output. A note, lines starting with #, is added as prose.

// ErrHistoryOff is returned by Enter while the History pane is off.
var ErrHistoryOff = errors.New("the History pane is off")

// SetHistory turns the History pane on or off. Commands typed there run
// on a terminal, so turning it on starts the shells again, with
// terminals of their own, unless they have them already. It reports
// whether that lost what blocks that ran had set up. The output shown so
// far is kept.
func (n *Notebook) SetHistory(on bool) (restarted bool, err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return false, ErrBusy
	}
	n.history = on
	if !on || n.sess.PTY() {
		return false, nil
	}
	n.waitReap()
	sess, err := n.newSession(n.opts.Run)
	if err != nil {
		n.history = false
		return false, err
	}
	n.closeSession()
	n.sess = sess
	for _, r := range n.results {
		if r.Ran && run.Runnable(r.Block) {
			restarted = true
		}
	}
	return restarted, nil
}

// environ is the environment the shells start with. With the History
// pane on, the shells' terminal is the pane, an xterm, so TERM says so:
// an app started from the desktop has none, and full-screen programs
// need it.
func (n *Notebook) environ() []string {
	env := os.Environ()
	if !n.history {
		return env
	}
	env = slices.DeleteFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "TERM=") })
	return append(env, "TERM=xterm-256color")
}

// History reports whether the History pane is on.
func (n *Notebook) History() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.history
}

// Enter takes what was typed in the History pane. A note, where every
// line starts with #, is added to the end of the document as prose with
// the # taken off, joining the paragraph of the note before it if
// nothing came between. Anything else is a command: it is added as a
// block, #history-N, and run on the terminal pane, waiting for it to
// finish. A command that took over the screen, as vim does, is marked
// tty, so its output is a recording. A command that fails is kept, with
// its exit status.
func (n *Notebook) Enter(input string) error {
	input = strings.TrimRight(input, "\n")
	if strings.TrimSpace(input) == "" {
		return nil
	}
	if text, ok := note(input); ok {
		return n.addNote(text)
	}
	n.mu.Lock()
	on := n.history
	n.mu.Unlock()
	if !on {
		return ErrHistoryOff
	}
	return n.start(func(ctx context.Context) error {
		n.mu.Lock()
		info := fmt.Sprintf("%s {#%s}", n.lang(), n.nextHistoryID())
		b, err := n.d.AppendBlock(info, input)
		if err == nil {
			err = n.sync()
		}
		if err != nil {
			n.mu.Unlock()
			return fmt.Errorf("can't add the command to the document: %w", err)
		}
		n.noteEnd = -1
		i := n.index[b]
		n.term.mu.Lock()
		n.term.history = true
		n.term.mu.Unlock()
		n.mu.Unlock()
		defer func() {
			n.term.mu.Lock()
			n.term.history = false
			n.term.mu.Unlock()
		}()

		_, full, err := n.sess.RunTyped(ctx, i)
		if !full {
			return err
		}
		n.mu.Lock()
		defer n.mu.Unlock()
		text := b.Fence + strings.TrimSuffix(b.Info, "}") + " tty}\n" + b.Body + b.Fence + "\n"
		if serr := n.d.SetBlock(b, text); serr != nil {
			return errors.Join(err, serr)
		}
		return errors.Join(err, n.sync())
	})
}

// note returns the text of input if it is a note: every line starts
// with #, which is taken off along with a space after it.
func note(input string) (string, bool) {
	lines := strings.Split(input, "\n")
	for i, l := range lines {
		l = strings.TrimLeft(l, " \t")
		if !strings.HasPrefix(l, "#") {
			return "", false
		}
		l = strings.TrimPrefix(l[1:], " ")
		lines[i] = l
	}
	return strings.Join(lines, "\n"), true
}

func (n *Notebook) addNote(text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	src := n.d.Src
	var out []byte
	if n.noteEnd == len(src) {
		// The note before this one ends the document: carry on its
		// paragraph.
		out = append(append([]byte(nil), src...), text+"\n"...)
	} else {
		out = append(bytes.TrimRight(src, "\n"), "\n\n"+text+"\n"...)
		if len(bytes.TrimSpace(src)) == 0 {
			out = []byte(text + "\n")
		}
	}
	// The note must be prose: the blocks stay as they are.
	type same struct {
		info, body string
		span       doc.Span
	}
	var before []same
	for _, b := range n.d.Blocks {
		before = append(before, same{b.Info, b.Body, b.Span})
	}
	old := n.d.Src
	if err := n.d.SetSource(out); err != nil {
		return err
	}
	changed := len(n.d.Blocks) != len(before)
	for i, b := range n.d.Blocks {
		changed = changed || same{b.Info, b.Body, b.Span} != before[i]
	}
	if changed {
		if err := n.d.SetSource(old); err != nil {
			return err
		}
		return errors.New("the note would change the document's blocks; is a fence left open?")
	}
	if err := n.sync(); err != nil {
		return err
	}
	n.noteEnd = len(n.d.Src)
	n.dirty = true
	return nil
}

var historyID = regexp.MustCompile(`^history-([0-9]+)$`)

// nextHistoryID is the id for the next command, one past the highest
// history-N in the document, with n.mu held.
func (n *Notebook) nextHistoryID() string {
	high := 0
	for _, b := range n.d.Blocks {
		if m := historyID.FindStringSubmatch(b.ID); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil && v > high {
				high = v
			}
		}
	}
	return "history-" + strconv.Itoa(high+1)
}

// shell is the shell blocks run in, with n.mu held.
func (n *Notebook) shell() string {
	if n.opts.Run.Shell != "" {
		return n.opts.Run.Shell
	}
	if sh := run.Shell(n.d); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// lang is the language commands are added as: zsh if the blocks run in
// zsh, else sh.
func (n *Notebook) lang() string {
	if filepath.Base(n.shell()) == "zsh" {
		return "zsh"
	}
	return "sh"
}

// incomplete matches what sh, bash and zsh say about input that ends
// partway through a command.
var incomplete = regexp.MustCompile("end of file|EOF|[Uu]nterminated|unmatched|near `\\\\n'")

// Complete reports whether input is a whole command, or a note, so
// pressing enter in the History pane runs it rather than starting
// another line. Input ending in \, |, && or ||, or inside a quote, an
// if, a loop or a here-document, isn't.
func (n *Notebook) Complete(input string) bool {
	input = strings.TrimRight(input, "\n")
	if _, ok := note(input); ok || strings.TrimSpace(input) == "" {
		return true
	}
	end := strings.TrimRight(input, " \t")
	trailing := len(end) - len(strings.TrimRight(end, `\`))
	if trailing%2 == 1 {
		return false
	}
	for _, op := range []string{"|", "&&", "||"} {
		if strings.HasSuffix(end, op) && !strings.HasSuffix(end, `\`+op) {
			return false
		}
	}
	if openHeredoc(input) {
		return false
	}
	n.mu.Lock()
	sh := n.shell()
	n.mu.Unlock()
	cmd := exec.Command(sh, "-n")
	cmd.Stdin = strings.NewReader(input + "\n")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true
	}
	var ee *exec.ExitError
	return !errors.As(err, &ee) || !incomplete.Match(out)
}

var heredoc = regexp.MustCompile(`<<-?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?`)

// openHeredoc reports whether input starts a here-document it doesn't
// end.
func openHeredoc(input string) bool {
	lines := strings.Split(input, "\n")
	for i := 0; i < len(lines); i++ {
		for _, m := range heredoc.FindAllStringSubmatch(lines[i], -1) {
			word, found := m[1], false
			for i+1 < len(lines) {
				i++
				if strings.TrimLeft(lines[i], "\t") == word {
					found = true
					break
				}
			}
			if !found {
				return true
			}
		}
	}
	return false
}
