// Package tape reads the input commands of VHS tape files
// (https://github.com/charmbracelet/vhs) and plays them as keystrokes,
// so a block that needs a terminal can run without a person typing.
package tape

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Command is one line of a tape.
type Command struct {
	Line int
	// Keys are written Count times, or Text is typed a character at a
	// time, Speed apart.
	Keys  string
	Count int
	Text  string
	// Literal is set when Text was in single quotes or backticks, which
	// mdsh types as they are, without filling in placeholders.
	Literal bool
	Speed   time.Duration // 0 means the tape's typing speed
	// Sleep pauses for this long.
	Sleep time.Duration
	// Wait pauses until the screen matches Pattern (nil means the tape's
	// wait pattern), or Timeout passes (0 means the tape's wait timeout).
	Wait       bool
	Pattern    *regexp.Regexp
	WaitScreen bool // match the whole screen, not the last line
	Timeout    time.Duration
	// Set changes a setting: TypingSpeed, WaitTimeout or WaitPattern.
	Set, Value string
}

// Defaults match VHS.
const (
	DefaultTypingSpeed = 50 * time.Millisecond
	DefaultWaitTimeout = 15 * time.Second
	DefaultWaitPattern = `>$`
)

var keys = map[string]string{
	"Enter":     "\r",
	"Space":     " ",
	"Tab":       "\t",
	"Backspace": "\x7f",
	"Delete":    "\x1b[3~",
	"Insert":    "\x1b[2~",
	"Escape":    "\x1b",
	"Up":        "\x1b[A",
	"Down":      "\x1b[B",
	"Right":     "\x1b[C",
	"Left":      "\x1b[D",
	"Home":      "\x1b[H",
	"End":       "\x1b[F",
	"PageUp":    "\x1b[5~",
	"PageDown":  "\x1b[6~",
}

// ignored are VHS commands about the recording itself, which mdsh does
// its own way.
var ignored = map[string]bool{
	"Output": true, "Require": true, "Hide": true, "Show": true,
	"Screenshot": true, "Copy": true, "Paste": true, "Source": true, "Env": true,
}

// Parse reads a tape. Comments (#), blank lines and the VHS commands that
// only shape VHS's own recording (Output, Hide, Set Theme and so on) are
// skipped.
func Parse(src string) ([]Command, error) {
	var cmds []Command
	for i, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		c, skip, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("tape line %d: %w", i+1, err)
		}
		if !skip {
			c.Line = i + 1
			cmds = append(cmds, c)
		}
	}
	return cmds, nil
}

func parseLine(line string) (c Command, skip bool, err error) {
	word, rest, _ := strings.Cut(line, " ")
	rest = strings.TrimSpace(rest)
	name, opt, _ := strings.Cut(word, "@")
	if opt != "" && name != "Wait" && !strings.HasPrefix(name, "Wait+") {
		if c.Speed, err = duration(opt); err != nil {
			return c, false, err
		}
	}
	switch {
	case ignored[name]:
		return c, true, nil
	case name == "Type":
		c.Text, c.Literal, err = unquote(rest)
		return c, false, err
	case name == "Sleep":
		c.Sleep, err = duration(rest)
		return c, false, err
	case name == "Set":
		c.Set, c.Value, _ = strings.Cut(rest, " ")
		c.Value = strings.TrimSpace(c.Value)
		switch c.Set {
		case "TypingSpeed", "WaitTimeout":
			_, err = duration(c.Value)
		case "WaitPattern":
			_, err = regexp.Compile(c.Value)
		default:
			return c, true, nil // fonts, themes, sizes: VHS's concern
		}
		return c, false, err
	case name == "Wait" || name == "Wait+Line" || name == "Wait+Screen":
		c.Wait, c.WaitScreen = true, name == "Wait+Screen"
		if opt != "" {
			if c.Timeout, err = duration(opt); err != nil {
				return c, false, err
			}
		}
		if rest != "" {
			if len(rest) < 2 || rest[0] != '/' || rest[len(rest)-1] != '/' {
				return c, false, fmt.Errorf("Wait takes a /regex/, got %q", rest)
			}
			c.Pattern, err = regexp.Compile(rest[1 : len(rest)-1])
		}
		return c, false, err
	case strings.HasPrefix(name, "Ctrl+"), strings.HasPrefix(name, "Alt+"), name == "Shift+Tab":
		c.Keys, err = combo(name)
		c.Count = 1
		return c, false, err
	}
	k, ok := keys[name]
	if !ok {
		return c, false, fmt.Errorf("unknown command %q", name)
	}
	c.Keys, c.Count = k, 1
	if rest != "" {
		if c.Count, err = strconv.Atoi(rest); err != nil || c.Count < 1 {
			return c, false, fmt.Errorf("%s takes a count, got %q", name, rest)
		}
	}
	return c, false, nil
}

// combo turns Ctrl+C, Alt+x, Ctrl+Alt+x or Shift+Tab into bytes.
func combo(name string) (string, error) {
	if name == "Shift+Tab" {
		return "\x1b[Z", nil
	}
	parts := strings.Split(name, "+")
	key := parts[len(parts)-1]
	ctrl, alt := false, false
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "Ctrl":
			ctrl = true
		case "Alt":
			alt = true
		default:
			return "", fmt.Errorf("unknown modifier %q in %s", m, name)
		}
	}
	var s string
	switch {
	case len(key) == 1:
		s = key
		if ctrl {
			c := key[0]
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			if c < '@' || c > '_' {
				return "", fmt.Errorf("no control code for %s", name)
			}
			s = string(rune(c - '@'))
		}
	case keys[key] != "" && !ctrl:
		s = keys[key]
	default:
		return "", fmt.Errorf("unknown key in %s", name)
	}
	if alt {
		s = "\x1b" + s
	}
	return s, nil
}

// duration reads 500ms, 2s or a bare number of seconds.
func duration(s string) (time.Duration, error) {
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(f * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	return d, nil
}

// unquote reads a string in double quotes (with Go escapes), or in
// single quotes or backticks, which are literal.
func unquote(s string) (text string, literal bool, err error) {
	if len(s) >= 2 {
		switch q := s[0]; {
		case q == '"' && s[len(s)-1] == '"':
			text, err = strconv.Unquote(s)
			return text, false, err
		case (q == '\'' || q == '`') && s[len(s)-1] == q:
			return s[1 : len(s)-1], true, nil
		}
	}
	return "", false, fmt.Errorf("Type takes a quoted string, got %q", s)
}

// Play types cmds into keys. screen returns the terminal's text, one
// line per row, for Wait. It stops early when ctx ends.
func Play(ctx context.Context, cmds []Command, keys io.Writer, screen func() string) error {
	speed, timeout := DefaultTypingSpeed, DefaultWaitTimeout
	pattern := regexp.MustCompile(DefaultWaitPattern)
	sleep := func(d time.Duration) error {
		if d <= 0 {
			return ctx.Err()
		}
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
	for _, c := range cmds {
		per := speed
		if c.Speed > 0 {
			per = c.Speed
		}
		var err error
		switch {
		case c.Set != "":
			switch c.Set {
			case "TypingSpeed":
				speed, _ = duration(c.Value)
			case "WaitTimeout":
				timeout, _ = duration(c.Value)
			case "WaitPattern":
				pattern = regexp.MustCompile(c.Value)
			}
		case c.Sleep > 0:
			err = sleep(c.Sleep)
		case c.Wait:
			re, limit := c.Pattern, timeout
			if re == nil {
				re = pattern
			}
			if c.Timeout > 0 {
				limit = c.Timeout
			}
			err = wait(ctx, re, c.WaitScreen, limit, screen, sleep)
		case c.Text != "":
			for _, r := range c.Text {
				if _, err = io.WriteString(keys, string(r)); err != nil {
					break
				}
				if err = sleep(per); err != nil {
					break
				}
			}
		default:
			for range c.Count {
				if _, err = io.WriteString(keys, c.Keys); err != nil {
					break
				}
				if err = sleep(per); err != nil {
					break
				}
			}
		}
		if err != nil {
			return fmt.Errorf("tape line %d: %w", c.Line, err)
		}
	}
	return nil
}

func wait(ctx context.Context, re *regexp.Regexp, whole bool, limit time.Duration, screen func() string, sleep func(time.Duration) error) error {
	end := time.Now().Add(limit)
	for {
		text := strings.TrimRight(screen(), " \n")
		if !whole {
			if i := strings.LastIndexByte(text, '\n'); i >= 0 {
				text = text[i+1:]
			}
			text = strings.TrimRight(text, " ")
		}
		if re.MatchString(text) {
			return nil
		}
		if time.Now().After(end) {
			return fmt.Errorf("Wait /%s/: not on screen after %s", re, limit)
		}
		if err := sleep(50 * time.Millisecond); err != nil {
			return err
		}
	}
}
