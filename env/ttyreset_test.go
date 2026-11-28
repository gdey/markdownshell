package env

import "testing"

func TestColourWatch(t *testing.T) {
	for _, c := range []struct {
		name, in, want string
	}{
		{"nothing", "plain \x1b[31mred\x1b[0m text", ""},
		{"title only", "\x1b]0;hi\x07\x1b]2;there\x1b\\", ""},
		{"queries", "\x1b]11;?\x07\x1b]4;1;?;2;?\x1b\\\x1b]10;?;?\x07", ""},
		{"background", "\x1b]11;#000000\x07", "\x1b]111\x1b\\"},
		{"palette", "\x1b]4;1;?;2;rgb:ff/00/00\x07", "\x1b]104\x1b\\"},
		{"consecutive", "\x1b]10;?;blue;red\x1b\\", "\x1b]111\x1b\\\x1b]112\x1b\\"},
		{"once each", "\x1b]12;red\x07\x1b]12;blue\x07\x1b]5;0;red\x07", "\x1b]112\x1b\\\x1b]105\x1b\\"},
		{"cancelled", "\x1b]11;#000\x18\x1b]10;red\x1b[m", ""},
		{"restarted", "\x1b]11;?\x1b\x1b]10;red\x07", "\x1b]110\x1b\\"},
		{"other OSC", "\x1b]8;;http://x\x1b\\\x1b]52;c;aGk=\x07\x1b]104\x07", ""},
	} {
		w := &colourWatch{}
		// One byte at a time too, as the pty may split anywhere.
		for _, split := range []bool{false, true} {
			*w = colourWatch{}
			if split {
				for i := range len(c.in) {
					w.Write([]byte{c.in[i]})
				}
			} else {
				w.Write([]byte(c.in))
			}
			if got := w.reset(); got != c.want {
				t.Errorf("%s (split %v): reset is %q, want %q", c.name, split, got, c.want)
			}
		}
	}
}
