package notebook

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Completion is what Tab offers in the History pane for the word before
// the cursor: each of Items replaces Word, the end of what was typed.
type Completion struct {
	Word  string   `json:"word"`
	Items []string `json:"items"`
}

// maxItems is the most a completion offers.
const maxItems = 500

// Completions completes the word at the end of before, what is typed
// in the History pane up to the cursor, asking the document's shell, so
// it sees the directory and variables blocks left there: a $ word is a
// variable, the first word of a command is a command, and any other
// word, or one with a /, a file. A directory ends in /. Items are
// quoted for the shell, as they would be typed.
func (n *Notebook) Completions(before string) (Completion, error) {
	word := lastWord(before)
	c := Completion{Word: word}
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return c, ErrBusy
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

	ask := func(script string) []string {
		out, err := sess.Ask(context.Background(), script)
		if err != nil {
			return nil
		}
		return strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	}
	text := unescape(word)
	var items []string
	switch {
	case strings.HasPrefix(text, "${"):
		items = vars(ask(varsScript), text[2:], "${", "}")
	case strings.HasPrefix(text, "$"):
		items = vars(ask(varsScript), text[1:], "$", "")
	case commandAt(before[:len(before)-len(word)]) && !strings.Contains(text, "/"):
		items = commands(ask(commandScript(text, zsh)), text, zsh)
	default:
		items = files(ask(fileScript(text)), text)
	}
	if len(items) > maxItems {
		items = items[:maxItems]
	}
	c.Items = items
	return c, nil
}

// wordEnds are the characters a word stops at, unless escaped.
const wordEnds = " \t\n;|&<>()`\"'="

// lastWord is the word at the end of s, with any backslash escapes it
// has.
func lastWord(s string) string {
	i := len(s)
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		if strings.ContainsRune(wordEnds, r) && !escaped(s, i-size) {
			break
		}
		i -= size
	}
	return s[i:]
}

// escaped reports whether the byte at i in s follows an odd number of
// backslashes.
func escaped(s string, i int) bool {
	n := 0
	for i > 0 && s[i-1] == '\\' {
		n++
		i--
	}
	return n%2 == 1
}

// unescape takes the backslash escapes out of word.
func unescape(word string) string {
	var b strings.Builder
	for i := 0; i < len(word); i++ {
		if word[i] == '\\' && i+1 < len(word) {
			i++
		}
		b.WriteByte(word[i])
	}
	return b.String()
}

// quoteChars are escaped in items, as the shell would need them typed.
const quoteChars = " \t\n'\"\\$&;|<>()*?[]#!{}`"

// escape quotes s for the shell with backslashes.
func escape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(quoteChars, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// commandKeywords are the words after which a command starts.
var commandKeywords = []string{"if", "then", "else", "elif", "while", "until", "do", "!", "time", "sudo", "exec", "command", "nohup", "xargs", "builtin"}

// commandAt reports whether a word after s is a command's name.
func commandAt(s string) bool {
	s = strings.TrimRight(s, " \t")
	if s == "" {
		return true
	}
	if strings.ContainsRune(";|&(\n`{", rune(s[len(s)-1])) {
		return true
	}
	return slices.Contains(commandKeywords, lastWord(s))
}

// quote single-quotes s for the shell.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// varsScript lists the shell's variables by name. Only the names are
// printed: a value can hold anything, such as bash's $_, which after a
// block is mdsh's own end-of-block marker.
const varsScript = `(set) 2>/dev/null | sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1=/p'`

var varName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// vars are the variables set lists that start with prefix, between open
// and close.
func vars(lines []string, prefix, open, close string) []string {
	var names []string
	for _, l := range lines {
		if m := varName.FindString(l); m != "" && strings.HasPrefix(m, prefix) {
			names = append(names, open+strings.TrimSuffix(m, "=")+close)
		}
	}
	return tidy(names)
}

// shBuiltins are the builtins of sh and bash, which have nothing to list
// them with that both know.
var shBuiltins = []string{"alias", "bg", "break", "cd", "command", "continue", "echo", "eval", "exec", "exit", "export", "false", "fg", "getopts", "hash", "jobs", "kill", "local", "printf", "pwd", "read", "readonly", "return", "set", "shift", "test", "times", "trap", "true", "type", "ulimit", "umask", "unalias", "unset", "wait"}

// aliasMark comes before the aliases in what commandScript prints.
const aliasMark = "--mdsh-aliases--"

// commandScript lists the commands, aliases and functions that could
// start with prefix: zsh knows them all, and in sh the commands are
// looked for on $PATH.
func commandScript(prefix string, zsh bool) string {
	if zsh {
		return "(print -rl -- ${(k)commands} ${(k)aliases} ${(k)functions} ${(k)builtins} ${(k)reswords})"
	}
	return `(IFS=:; for d in $PATH; do for f in "$d"/` + quote(prefix) + `*; do [ -f "$f" ] && [ -x "$f" ] && printf '%s\n' "${f##*/}"; done; done; printf '%s\n' ` + aliasMark + `; alias) 2>/dev/null`
}

// commands are the names in lines that start with prefix.
func commands(lines []string, prefix string, zsh bool) []string {
	var names []string
	if !zsh {
		lines = append(lines, shBuiltins...)
	}
	aliases := false
	for _, l := range lines {
		if l == aliasMark {
			aliases = true
			continue
		}
		if aliases {
			// bash says alias ll='ls -l', and dash ll='ls -l'.
			name, _, ok := strings.Cut(strings.TrimPrefix(l, "alias "), "=")
			if !ok {
				continue
			}
			l = name
		}
		if l != "" && strings.HasPrefix(l, prefix) {
			names = append(names, escape(l))
		}
	}
	return tidy(names)
}

// fileScript lists the files whose paths start with prefix, a directory
// with a / after it, after a line with $HOME.
func fileScript(prefix string) string {
	glob := quote(prefix) + "*"
	if rest, ok := strings.CutPrefix(prefix, "~/"); ok || prefix == "~" {
		glob = "~/" + quote(rest) + "*"
	}
	return `(setopt nullglob 2>/dev/null; printf '%s\n' "$HOME"; for f in ` + glob + `; do if [ -d "$f" ]; then printf '%s/\n' "$f"; elif [ -e "$f" ] || [ -L "$f" ]; then printf '%s\n' "$f"; fi; done)`
}

// files are the paths fileScript listed, written as prefix was: with ~
// for $HOME if it started with ~.
func files(lines []string, prefix string) []string {
	if len(lines) == 0 {
		return nil
	}
	home, lines := lines[0], lines[1:]
	tilde := prefix == "~" || strings.HasPrefix(prefix, "~/")
	var paths []string
	for _, l := range lines {
		if l == "" {
			continue
		}
		if tilde {
			rest, ok := strings.CutPrefix(l, home)
			if !ok {
				continue
			}
			paths = append(paths, "~"+escape(rest))
			continue
		}
		paths = append(paths, escape(l))
	}
	return tidy(paths)
}

// tidy sorts names and drops repeats.
func tidy(names []string) []string {
	slices.Sort(names)
	return slices.Compact(names)
}
