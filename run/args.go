package run

import (
	"errors"
	"strings"
)

// SplitArgs splits s into arguments the way a shell splits words:
// at spaces, with '…' and "…" quoting and \ escaping the next character
// (outside single quotes). It expands nothing.
func SplitArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inWord := false
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case quote == '"':
			if r == '"' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inWord {
				args = append(args, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unclosed quote")
	}
	if escaped {
		return nil, errors.New("trailing backslash")
	}
	if inWord {
		args = append(args, cur.String())
	}
	return args, nil
}

// JoinArgs is the reverse of SplitArgs: it quotes each argument that
// needs it.
func JoinArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`*?[]{}()<>|&;#~") {
			quoted[i] = a
			continue
		}
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
