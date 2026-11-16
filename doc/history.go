package doc

import (
	"fmt"
	"strconv"
)

// DefaultHistory is how many earlier runs a bare history attribute
// keeps when the front matter doesn't say.
const DefaultHistory = 10

// Entry is one earlier run's output, kept under a block's current
// output by history=N: an output fence or region marked history=K, and
// its stderr fence, if any, marked the same.
type Entry struct {
	// K is the entry's place: 1 for the run before the current one, 2
	// for the one before that, and so on.
	K int
	// Span covers the entry's parts, prefixes included, from the first
	// part's opening line to the end of its last part.
	Span Span
	// At is the run's start time as its at= has it, or "" when the
	// entry has none.
	At string
	// Exit is its exit= status, and Exited is set when it has exited.
	Exit   int
	Exited bool
	// Region is set when its stdout part is an output region, as for
	// markdown, image and recording output.
	Region bool

	main, err bool // the parts it has: stdout and stderr
}

// HistoryCap is how many earlier runs block b keeps: N for history=N,
// the front matter's history, else DefaultHistory, for a bare history,
// and 0 without it. Parse checks the value.
func (b *Block) HistoryCap(o Options) int {
	v, ok := b.Attrs["history"]
	if !ok {
		return 0
	}
	if v == "" {
		if o.History > 0 {
			return o.History
		}
		return DefaultHistory
	}
	n, _ := strconv.Atoi(v)
	return n
}

// CurrentOutput covers block b's current output: Output without the
// earlier runs under it.
func (b *Block) CurrentOutput() Span {
	if len(b.History) == 0 {
		return b.Output
	}
	return Span{b.Output.Start, b.outCur}
}

// checkHistoryAttr checks a runnable block's history=: bare, or a whole
// number from 0.
func checkHistoryAttr(b *Block) error {
	v, ok := b.Attrs["history"]
	if !ok || v == "" {
		return nil
	}
	if n, err := strconv.Atoi(v); err != nil || n < 0 {
		return fmt.Errorf("history=%s: want how many earlier runs to keep, a whole number from 0", v)
	}
	return nil
}

// historyK reads the K of an output part's history=.
func historyK(v string) (int, error) {
	k, err := strconv.Atoi(v)
	if err != nil || k < 1 {
		return 0, fmt.Errorf("history=%s on an output: want the run's place, a whole number from 1", v)
	}
	return k, nil
}

// addEntryPart adds an output part marked history=k to b's history:
// the stdout part, or the stderr fence when stderr is set, spanning
// [start, end). attrs are the part's attributes. Entries come newest
// first, and each has at most one part of each kind.
func (b *Block) addEntryPart(k int, start, end int, attrs map[string]string, region, stderr bool) error {
	if len(b.History) == 0 {
		b.outCur = b.Output.End
	}
	var e *Entry
	if n := len(b.History); n > 0 && b.History[n-1].K == k {
		e = &b.History[n-1]
	} else if n > 0 && b.History[n-1].K > k {
		return fmt.Errorf("history=%d comes after history=%d: earlier runs of %s go newest first", k, b.History[n-1].K, b.ID)
	} else {
		b.History = append(b.History, Entry{K: k, Span: Span{start, end}})
		e = &b.History[len(b.History)-1]
	}
	if stderr && e.err || !stderr && e.main {
		part := "stdout output"
		if stderr {
			part = "stderr output"
		}
		return fmt.Errorf("%s has two %s parts for history=%d", b.ID, part, k)
	}
	if stderr {
		e.err = true
	} else {
		e.main = true
		e.Region = region
		e.Exit, _ = strconv.Atoi(attrs["exit"])
		_, e.Exited = attrs["exited"]
	}
	if e.At == "" || !stderr {
		if at := attrs["at"]; at != "" {
			e.At = at
		}
	}
	e.Span.End = end
	b.Output.End = end
	return nil
}
