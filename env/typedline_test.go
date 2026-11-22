package env

import "testing"

func TestTypedLine(t *testing.T) {
	for _, c := range []struct {
		name, prompt, echo string
		cols               int
		want               string
	}{
		{"plain", "$ ", "echo hi\r\n", 80, "echo hi"},
		{"readline wrap", "bash-5.2 $ ", "\x1b[Kecho wraps\rs-past-the-edge-of-a-\r-narrow\r\n\x1b[?2004l\r", 20, "echo wraps-past-the-edge-of-a-narrow"},
		{"blank at the edge", "0123456789", "echo 1234  \r 6789\r\n", 20, "echo 1234  6789"},
		{"backspace", "$ ", "echo abX\b \bc\r\n", 80, "echo abc"},
		{"cursor keys", "$ ", "echo ac\x1b[D\x1b[1@b\r\n", 80, "echo abc"},
		{"delete", "$ ", "echo abXc\x1b[2D\x1b[P\r\n", 80, "echo abc"},
		{"colored prompt", "\x1b[32m~/src\x1b[0m $ ", "ls\r\n", 80, "ls"},
		{"prompt past the edge", "a-long-prompt-that-wraps> ", "ls\r\n", 10, "ls"},
		{"wide", "$ ", "echo 日本\r\n", 80, "echo 日本"},
		{"continued", "$ ", "echo 'a\r\n> b'\r\n", 80, "echo 'a\n> b'"},
	} {
		if got := typedLine(c.prompt, c.echo, c.cols); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
