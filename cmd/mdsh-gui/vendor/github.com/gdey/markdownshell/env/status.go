package env

import (
	"io"
	"sync"

	"github.com/gdey/markdownshell/termstatus"
)

// Programs running on a terminal mdsh draws (a tty block's pane, a
// background tty block's, the History pane's shell) can report what they
// are doing with the Program Status Protocol, OSC 7501 (package
// termstatus). mdsh is their terminal, so it keeps their records, as a
// terminal would, and answers the feature query; a Terminal's Status,
// or the interactive shell's "status" Event, says when they change.

// statusTee keeps the records of one terminal: it scans what a program
// writes, on its way to the terminal's writer, and tells changed of each
// change, with the records as they stand. end drops what doesn't outlive
// the program and tells changed(nil): the terminal is gone.
type statusTee struct {
	store   termstatus.Store
	changed func([]termstatus.Report)
	mu      sync.Mutex
	ended   bool
}

// newStatusTee scans what is written to the writer it returns, passing
// it on to w. Reports go to the store, and the feature query is answered
// on ptm, the program's input; OSC 52 copies go to copied. With changed
// and copied both nil, nothing is scanned and w is returned as it is;
// with changed nil, only copies are looked for.
func newStatusTee(w io.Writer, ptm io.Writer, changed func([]termstatus.Report), copied func(string)) (io.Writer, *statusTee) {
	if changed == nil && copied == nil {
		return w, nil
	}
	sc := termstatus.NewScanner(w, nil, nil)
	sc.Clipboard = copied
	if changed == nil {
		return sc, nil
	}
	t := &statusTee{changed: changed}
	sc.Report = t.report
	sc.Query = func() {
		if ptm != nil {
			io.WriteString(ptm, termstatus.QueryReply)
		}
	}
	return sc, t
}

func (t *statusTee) report(r termstatus.Report) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return
	}
	if t.store.Apply(r) {
		t.changed(t.store.Records())
	}
}

// end says the program ended: its terminal goes away.
func (t *statusTee) end() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.ended {
		return
	}
	t.ended = true
	t.changed(nil)
}
