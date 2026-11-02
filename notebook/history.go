package notebook

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gdey/markdownshell/history"
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
	n.waitAsking()
	if n.running {
		return false, ErrBusy
	}
	n.history = on
	if !on {
		return false, nil
	}
	if n.sess.PTY() {
		// The shells have terminals; they only need the rc file.
		if rc := n.opts.Run.HistoryRC; rc != "" {
			n.running = true
			n.mu.Unlock()
			_, err := n.sess.Ask(context.Background(), history.RCScript(rc))
			n.mu.Lock()
			n.running = false
			return false, err
		}
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

// shellEnv is the environment the shells start with. With the History
// pane on, the shells' terminal is the pane, an xterm, so TERM says so:
// an app started from the desktop has none, and full-screen programs
// need it.
func (n *Notebook) shellEnv() []string {
	env := environ()
	if !n.history {
		return env
	}
	env = slices.DeleteFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "TERM=") })
	return append(env, "TERM=xterm-256color")
}

// SetHistoryRC sets the file the shells read while the History pane is
// on, from the next time it is turned on.
func (n *Notebook) SetHistoryRC(path string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.opts.Run.HistoryRC = path
}

// source is what the shells read as they start: ro.Source, and the rc
// file while the History pane is on.
func (n *Notebook) source(ro RunOptions) string {
	if !n.history || ro.HistoryRC == "" {
		return ro.Source
	}
	return ro.Source + "\n" + history.RCScript(ro.HistoryRC)
}

// Prompt asks the shell blocks run in for its prompts, so the History
// pane can show them. It returns ErrBusy while a block runs.
func (n *Notebook) Prompt() (history.Prompt, error) {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return history.Prompt{}, ErrBusy
	}
	n.waitReap()
	n.running, n.asking = true, true
	sess, zsh := n.sess, n.lang() == "zsh"
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		n.running, n.asking = false, false
		n.idle.Broadcast()
		n.mu.Unlock()
	}()
	return history.AskPrompt(context.Background(), sess, zsh)
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
// finish; one ending in a lone & is added as a background block instead,
// without the &, and left running. A command that took over the screen, as vim does, is marked
// tty, so its output is a recording. A command that fails is kept, with
// its exit status.
func (n *Notebook) Enter(input string) error {
	input = strings.TrimRight(input, "\n")
	if strings.TrimSpace(input) == "" {
		return nil
	}
	if text, ok := history.Note(input); ok {
		return n.addNote(text)
	}
	n.mu.Lock()
	on := n.history
	n.mu.Unlock()
	if !on {
		return ErrHistoryOff
	}
	body, bg := history.Background(input)
	return n.start(func(ctx context.Context) error {
		n.mu.Lock()
		info := history.Info(n.lang(), history.NextID(n.d), bg)
		b, err := n.d.AppendBlock(info, body)
		if err == nil {
			err = n.sync()
		}
		if err != nil {
			n.mu.Unlock()
			return fmt.Errorf("can't add the command to the document: %w", err)
		}
		n.noteEnd = -1
		i := n.index[b]
		if bg {
			// It runs as any background block does, apart from the
			// terminal, and its output goes to the block when it ends.
			n.mu.Unlock()
			_, err := n.sess.RunBlock(ctx, i)
			return err
		}
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

func (n *Notebook) addNote(text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.running {
		return ErrBusy
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	// The note before this one ending the document carries on its
	// paragraph.
	if err := history.AddNote(n.d, text, n.noteEnd == len(n.d.Src)); err != nil {
		return err
	}
	if err := n.sync(); err != nil {
		return err
	}
	n.noteEnd = len(n.d.Src)
	n.dirty = true
	return nil
}

// shell is the shell blocks run in, with n.mu held.
func (n *Notebook) shell() string { return history.Shell(n.d, n.opts.Run.Shell) }

// lang is the language commands are added as, with n.mu held.
func (n *Notebook) lang() string { return history.Lang(n.shell()) }

// Complete reports whether input is a whole command, or a note, so
// pressing enter in the History pane runs it rather than starting
// another line. See history.Whole.
func (n *Notebook) Complete(input string) bool {
	n.mu.Lock()
	sh := n.shell()
	n.mu.Unlock()
	return history.Whole(sh, input)
}

// Completions completes the word at the end of before, what is typed
// in the History pane up to the cursor, asking the document's shell, so
// it sees the directory and variables blocks left there. See
// history.Complete. It returns ErrBusy while a block runs.
func (n *Notebook) Completions(before string) (history.Completion, error) {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return history.Completion{}, ErrBusy
	}
	n.waitReap()
	n.running = true
	sess, zsh := n.sess, n.lang() == "zsh"
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		n.running = false
		n.mu.Unlock()
	}()
	return history.Complete(context.Background(), sess, before, zsh), nil
}
