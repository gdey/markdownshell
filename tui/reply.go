package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// replyFilter drops a terminal's late reply to a query, which Bubble Tea
// would otherwise hand on as typing. Lip Gloss asks the terminal for its
// background colour (OSC 11) when mdsh starts, and gives up on the reply
// once the cursor report it asks for next comes back. A terminal that
// sends the two out of order, as one in a GUI can, has its colour reply,
// such as ESC ]11;rgb:1e1e/1e1e/1e1e ESC \, read by the TUI, which sees
// it as alt+], the runes 11;rgb:1e1e/1e1e/1e1e and alt+\.
//
// The filter holds back alt+], drops what follows if it starts the way
// an OSC reply does, with a number and a semicolon, up to the alt+\ or
// ctrl+g (BEL) that ends it, and passes anything else on, though not the
// alt+] itself, which no key binding uses. A key that is not runes ends
// a reply early, so a lost terminator cannot eat keys.
type replyFilter struct {
	// held is set after alt+], which may start a reply.
	held bool
	// in is set while in a reply.
	in bool
}

func (f *replyFilter) filter(_ tea.Model, msg tea.Msg) tea.Msg {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return msg
	}
	held := f.held
	f.held = false
	switch {
	case f.in:
		switch {
		case k.Type == tea.KeyCtrlG, k.Alt && k.Type == tea.KeyRunes && string(k.Runes) == `\`:
			f.in = false
			return nil
		case k.Type == tea.KeyRunes && !k.Alt:
			return nil
		}
		f.in = false
		return msg
	case held && k.Type == tea.KeyRunes && !k.Alt && oscStart(k.Runes):
		f.in = true
		return nil
	case k.Alt && k.Type == tea.KeyRunes && string(k.Runes) == "]":
		f.held = true
		return nil
	}
	return msg
}

// oscStart reports whether r starts the way an OSC reply's body does:
// digits, then a semicolon.
func oscStart(r []rune) bool {
	for i, c := range r {
		switch {
		case c == ';':
			return i > 0
		case c < '0' || c > '9':
			return false
		}
	}
	return false
}
