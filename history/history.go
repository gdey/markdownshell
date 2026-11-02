// Package history is what a History terminal needs, as mdsh-gui's
// History pane and mdsh -i have: a terminal where a command typed runs in
// the document's shell and is added to the end of the document as a
// block, #history-N, with its output, and a note, lines starting with #,
// is added as prose.
//
// It asks the shell through an Asker, such as a *run.Session, so what it
// sees is what blocks left there: the directory, variables and prompts.
package history

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// Asker runs script in the shell blocks run in and returns what it
// printed. *run.Session is one.
type Asker interface {
	Ask(ctx context.Context, script string) (string, error)
}

// Shell is the shell blocks of d run in: override if set, else what d
// asks for, else /bin/sh.
func Shell(d *doc.Document, override string) string {
	if override != "" {
		return override
	}
	if sh := run.Shell(d); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// Lang is the language commands are added as: zsh if the blocks run in
// shell zsh, else sh.
func Lang(shell string) string {
	if filepath.Base(shell) == "zsh" {
		return "zsh"
	}
	return "sh"
}

// RCScript reads the file at path, if there is one, in the shell, as an
// interactive shell reads its rc file. A leading ~/ is the home folder.
// What it prints is dropped, along with any errors, since an rc file
// often does things only an interactive shell can.
func RCScript(path string) string {
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

// AskPrompt asks the shell for its prompts. zsh says the shell is zsh.
func AskPrompt(ctx context.Context, a Asker, zsh bool) (Prompt, error) {
	out, err := a.Ask(ctx, promptScript(zsh))
	if err != nil {
		return Prompt{}, err
	}
	ps1, ps2, _ := strings.Cut(out, promptMark)
	// bash's \[ and \] mark where escapes start and end.
	clean := strings.NewReplacer("\x01", "", "\x02", "")
	return Prompt{PS1: clean.Replace(ps1), PS2: clean.Replace(ps2)}, nil
}

// Background reports whether input ends in a lone &, as a command run in
// the background does, and returns it without the &.
func Background(input string) (string, bool) {
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

// Note returns the text of input if it is a note: every line starts
// with #, which is taken off along with a space after it.
func Note(input string) (string, bool) {
	lines := strings.Split(input, "\n")
	for i, l := range lines {
		l = strings.TrimLeft(l, " \t")
		if !strings.HasPrefix(l, "#") {
			return "", false
		}
		lines[i] = strings.TrimPrefix(l[1:], " ")
	}
	return strings.Join(lines, "\n"), true
}

// AddNote adds text to the end of d as prose. With carry, it carries on
// the paragraph that ends d, as a note added just before does; else it
// starts a paragraph. It fails, leaving d as it was, if text would change
// d's blocks, as it would inside a fence left open.
func AddNote(d *doc.Document, text string, carry bool) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	src := d.Src
	var out []byte
	switch {
	case carry:
		out = append(append([]byte(nil), src...), text+"\n"...)
	case len(bytes.TrimSpace(src)) == 0:
		out = []byte(text + "\n")
	default:
		out = append(bytes.TrimRight(src, "\n"), "\n\n"+text+"\n"...)
	}
	type same struct {
		info, body string
		span       doc.Span
	}
	var before []same
	for _, b := range d.Blocks {
		before = append(before, same{b.Info, b.Body, b.Span})
	}
	old := d.Src
	if err := d.SetSource(out); err != nil {
		return err
	}
	changed := len(d.Blocks) != len(before)
	for i, b := range d.Blocks {
		changed = changed || same{b.Info, b.Body, b.Span} != before[i]
	}
	if changed {
		if err := d.SetSource(old); err != nil {
			return err
		}
		return errors.New("the note would change the document's blocks; is a fence left open?")
	}
	return nil
}

var historyID = regexp.MustCompile(`^history-([0-9]+)$`)

// NextID is the id for the next command in d: one past the highest
// history-N there.
func NextID(d *doc.Document) string {
	high := 0
	for _, b := range d.Blocks {
		if m := historyID.FindStringSubmatch(b.ID); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil && v > high {
				high = v
			}
		}
	}
	return "history-" + strconv.Itoa(high+1)
}

// Info is the info string a command is added with: lang and id, and
// background if it was typed with a trailing &.
func Info(lang, id string, background bool) string {
	if background {
		return lang + " {#" + id + " background}"
	}
	return lang + " {#" + id + "}"
}

// incomplete matches what sh, bash and zsh say about input that ends
// partway through a command.
var incomplete = regexp.MustCompile("end of file|EOF|[Uu]nterminated|unmatched|near `\\\\n'")

// Whole reports whether input is a whole command, or a note, so pressing
// enter runs it rather than starting another line. Input ending in \, |,
// && or ||, or inside a quote, an if, a loop or a here-document, isn't.
// shell checks what the simple rules don't.
func Whole(shell, input string) bool {
	input = strings.TrimRight(input, "\n")
	if _, ok := Note(input); ok || strings.TrimSpace(input) == "" {
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
	cmd := exec.Command(shell, "-n")
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
