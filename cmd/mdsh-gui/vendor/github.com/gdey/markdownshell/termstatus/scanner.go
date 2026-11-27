package termstatus

import (
	"bytes"
	"io"
)

// Scanner is a terminal's end of a program's stream: it passes
// everything written to it on to w unchanged, as the terminal shows it,
// and finds the OSC 7501 reports in it on the way, however they are
// split across writes. A nil Scanner passes nothing and finds nothing.
type Scanner struct {
	w io.Writer
	// Report is called with each report that parses, and Query when
	// the program asks whether the terminal speaks the protocol, so the
	// caller can write QueryReply to the program.
	Report func(Report)
	Query  func()

	// osc is the sequence being collected, from its ESC, or nil
	// between sequences; esc says the last byte was a lone ESC.
	osc []byte
	esc bool
}

// NewScanner passes what is written to it on to w and reports the OSC
// 7501 reports it finds to report, and the feature query to query. Both
// may be nil.
func NewScanner(w io.Writer, report func(Report), query func()) *Scanner {
	return &Scanner{w: w, Report: report, Query: query}
}

var oscPrefix = []byte("\x1b]7501;")

// Write passes p on to w and scans it.
func (s *Scanner) Write(p []byte) (int, error) {
	s.scan(p)
	if s.w == nil {
		return len(p), nil
	}
	return s.w.Write(p)
}

func (s *Scanner) scan(p []byte) {
	for len(p) > 0 {
		if s.osc == nil {
			// Between sequences: look for ESC ].
			i := bytes.IndexByte(p, 0x1b)
			if s.esc {
				i, s.esc = 0, false
				if p[0] != ']' {
					continue
				}
				s.osc = []byte{0x1b}
			} else if i < 0 {
				return
			} else if i == len(p)-1 {
				s.esc = true
				return
			} else if p[i+1] != ']' {
				p = p[i+1:]
				continue
			} else {
				s.osc = []byte{0x1b}
				p = p[i+1:]
			}
		}
		// In a sequence: collect to BEL or ESC \, giving up past the
		// longest a report may be.
		for len(p) > 0 && s.osc != nil {
			c := p[0]
			p = p[1:]
			switch {
			case c == 0x07:
				s.end(s.osc)
			case c == 0x1b:
				s.osc = append(s.osc, c)
				s.esc = true
			case s.esc:
				s.esc = false
				if c == '\\' {
					s.end(s.osc[:len(s.osc)-1])
				} else {
					// An escape inside the sequence ends it as noise.
					s.osc = nil
				}
			default:
				s.osc = append(s.osc, c)
				if len(s.osc) > MaxSequence {
					s.osc = nil
				}
			}
		}
	}
}

// end takes a whole sequence, osc without its terminator.
func (s *Scanner) end(osc []byte) {
	s.osc = nil
	if !bytes.HasPrefix(osc, oscPrefix) {
		return
	}
	body := string(osc[len(oscPrefix):])
	if IsQuery(body) {
		if s.Query != nil {
			s.Query()
		}
		return
	}
	if r, ok := Parse(body); ok && s.Report != nil {
		s.Report(r)
	}
}
