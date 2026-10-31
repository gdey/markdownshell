package notebook

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
	if !on {
		return false, nil
	}
	if n.sess.PTY() {
		// The shells have terminals; they only need the rc file.
		if rc := n.opts.Run.HistoryRC; rc != "" {
			n.running = true
			n.mu.Unlock()
			_, err := n.sess.Ask(context.Background(), rcScript(rc))
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
	return ro.Source + "\n" + rcScript(ro.HistoryRC)
}

// rcScript reads the file at path, if there is one, in the shell, as an
// interactive shell reads its rc file. A leading ~/ is the home folder.
// What it prints is dropped, along with any errors, since an rc file
// often does things only an interactive shell can.
func rcScript(path string) string {
	p := quote(path)
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		p = `"$HOME"/` + quote(rest)
	}
	return "if [ -r " + p + " ]; then . " + p + " </dev/null >/dev/null 2>&1; fi\n"
}

// Prompt is the shell's prompts, PS1 and PS2, expanded as the shell
// would show them, with their colors as ANSI escapes. Empty means the
// shell has none.
type Prompt struct {
	PS1 string `json:"ps1"`
	PS2 string `json:"ps2"`
}

// promptMark divides PS1 from PS2 in what promptScript prints.
const promptMark = "\n--mdsh-ps2--\n"

// promptScript prints PS1 and PS2, expanded. zsh runs its precmd hooks
// first, as many prompt themes need, and expands % escapes, and $(…)
// too with promptsubst; bash expands its \ escapes; other shells, as
// dash does, expand the variables in them.
func promptScript(zsh bool) string {
	if zsh {
		return `(for f in precmd $precmd_functions; do (( $+functions[$f] )) && $f >/dev/null 2>&1 </dev/null; done
p1=$PS1 p2=$PS2
if [[ -o promptsubst ]]; then p1=${(e)p1} p2=${(e)p2}; fi 2>/dev/null
print -rn -- "${(%)p1}"$'` + strings.ReplaceAll(promptMark, "\n", `\n`) + `'"${(%)p2}")`
	}
	// ${PS1@P} needs bash 4.4; macOS's sh is bash 3.2, where it is an
	// error that ends the subshell, so older shells expand only the
	// variables, each in a subshell of its own in case that fails too.
	return `(p1=${PS1-} p2=${PS2-}
case ${BASH_VERSION-} in
''|[123].*|4.[0-3]*)
	x=$(eval "printf '%s' \"$p1\"" 2>/dev/null) && p1=$x
	x=$(eval "printf '%s' \"$p2\"" 2>/dev/null) && p2=$x ;;
*) eval 'p1=${PS1@P} p2=${PS2@P}' ;;
esac
printf '%s\n%s\n%s' "$p1" '--mdsh-ps2--' "$p2")`
}

// Prompt asks the shell blocks run in for its prompts, so the History
// pane can show them. It returns ErrBusy while a block runs.
func (n *Notebook) Prompt() (Prompt, error) {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return Prompt{}, ErrBusy
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
	out, err := sess.Ask(context.Background(), promptScript(zsh))
	if err != nil {
		return Prompt{}, err
	}
	ps1, ps2, _ := strings.Cut(out, promptMark)
	// bash's \[ and \] mark where escapes start and end.
	clean := strings.NewReplacer("\x01", "", "\x02", "")
	return Prompt{PS1: clean.Replace(ps1), PS2: clean.Replace(ps2)}, nil
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
	if text, ok := note(input); ok {
		return n.addNote(text)
	}
	n.mu.Lock()
	on := n.history
	n.mu.Unlock()
	if !on {
		return ErrHistoryOff
	}
	body, bg := backgrounded(input)
	return n.start(func(ctx context.Context) error {
		n.mu.Lock()
		info := fmt.Sprintf("%s {#%s}", n.lang(), n.nextHistoryID())
		if bg {
			info = strings.TrimSuffix(info, "}") + " background}"
		}
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

// backgrounded reports whether input ends in a lone &, as a command
// run in the background does, and returns it without the &.
func backgrounded(input string) (string, bool) {
	s := strings.TrimRight(input, " \t\n")
	if !strings.HasSuffix(s, "&") || strings.HasSuffix(s, "&&") || escaped(s, len(s)-1) {
		return input, false
	}
	// >& and <& redirect; |& pipes stderr too.
	if prev := strings.TrimSuffix(s, "&"); strings.HasSuffix(prev, ">") || strings.HasSuffix(prev, "<") || strings.HasSuffix(prev, "|") {
		return input, false
	}
	return strings.TrimRight(strings.TrimSuffix(s, "&"), " \t"), true
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
