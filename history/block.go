package history

import (
	"errors"
	"regexp"
	"strings"

	"github.com/gdey/markdownshell/doc"
)

// A block typed in a History terminal starts with a line such as
// "# ```bash {#setup}": a # and a fence with its info string. The # makes
// the line a shell comment, so it does nothing if it reaches a shell.
// The lines after it are the block's body, kept rather than run, until
// a line of just a fence as long as the opening one, or longer, ends it.
// The block is then added to the end of the document, as it was typed,
// without running.

// openFence matches a line that opens a block: a #, a fence of three or
// more backticks or tildes, and the info string. A backtick fence's
// info string has no backticks, as in Markdown.
var openFence = regexp.MustCompile("^[ \t]*#[ \t]*(`{3,}|~{3,})[ \t]*(.*?)[ \t]*$")

// OpenBlock returns the fence and info string of line if it opens a
// block typed in the terminal.
func OpenBlock(line string) (fence, info string, ok bool) {
	m := openFence.FindStringSubmatch(line)
	if m == nil || (m[1][0] == '`' && strings.Contains(m[2], "`")) {
		return "", "", false
	}
	return m[1], m[2], true
}

// closes reports whether line closes a block opened with fence.
func closes(line, fence string) bool {
	l := strings.TrimRight(line, " \t")
	if len(l)-len(strings.TrimLeft(l, " ")) > 3 {
		return false
	}
	l = strings.TrimLeft(l, " ")
	return len(l) >= len(fence) && strings.Trim(l, fence[:1]) == ""
}

// InBlock reports whether input opens a block and doesn't close it yet,
// so enter starts another line of it. It returns the info string the
// block opened with.
func InBlock(input string) (info string, ok bool) {
	first, rest, more := strings.Cut(input, "\n")
	fence, info, ok := OpenBlock(first)
	if !ok {
		return "", false
	}
	if more {
		lines := strings.Split(rest, "\n")
		if closes(lines[len(lines)-1], fence) {
			return "", false
		}
	}
	return info, true
}

// Block returns the info string and body of input if it is a whole block
// typed in the terminal: an opening line, its body, and a closing fence.
func Block(input string) (info, body string, ok bool) {
	input = strings.TrimRight(input, "\n")
	first, rest, more := strings.Cut(input, "\n")
	fence, info, ok := OpenBlock(first)
	if !ok || !more {
		return "", "", false
	}
	lines := strings.Split(rest, "\n")
	if !closes(lines[len(lines)-1], fence) {
		return "", "", false
	}
	body = strings.Join(lines[:len(lines)-1], "\n")
	if body != "" {
		body += "\n"
	}
	return info, body, true
}

// BlockPrompt is the prompt on the lines of a block being typed, in
// place of PS2, so it is plain the lines are kept, not run: the opening
// fence and the block's language, as "```bash> ".
func BlockPrompt(info string) string {
	lang, _, _ := strings.Cut(strings.TrimLeft(info, "{"), " ")
	lang, _, _ = strings.Cut(lang, "}")
	if strings.HasPrefix(lang, "#") || strings.HasPrefix(lang, ".") || strings.Contains(lang, "=") {
		lang = ""
	}
	return "```" + lang + "> "
}

// AddBlock adds a block typed in the terminal, with info and body, to
// the end of d, as it was typed. It fails, leaving d as it was, for a
// block with no body or one d can't take.
func AddBlock(d *doc.Document, info, body string) (*doc.Block, error) {
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("the block is empty, so it wasn't added")
	}
	b, err := d.AppendBlock(info, body)
	if err != nil {
		return nil, errors.New("can't add the block to the file: " + err.Error())
	}
	return b, nil
}
