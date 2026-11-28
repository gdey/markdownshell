package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
	"github.com/gdey/markdownshell/termsafe"
)

// A block with history=N keeps its earlier runs' outputs under its
// current output. Under the output a dim line says how many there are;
// b on the block shows them, each under a dim line with its time and
// exit status, and b again hides them.

// toggleEarlier shows or hides the current block's earlier runs.
func (m *Model) toggleEarlier() {
	i := m.cur()
	if i < 0 || len(m.d.Blocks[i].History) == 0 {
		m.status = "b shows a block's earlier runs, kept by history=N"
		return
	}
	if m.earlierShown == nil {
		m.earlierShown = map[*doc.Block]bool{}
	}
	b := m.d.Blocks[i]
	m.earlierShown[b] = !m.earlierShown[b]
}

// earlier is what shows under block b's output for its earlier runs:
// a line saying how many, and, while b shows them, each one.
func (m *Model) earlier(b *doc.Block, w int) string {
	n := len(b.History)
	if n == 0 {
		return ""
	}
	runs := "earlier runs"
	if n == 1 {
		runs = "earlier run"
	}
	shown := m.earlierShown[b]
	key := "b shows them"
	if n == 1 {
		key = "b shows it"
	}
	if shown {
		key = "b hides"
	}
	out := savedStyle.Render(fmt.Sprintf("%d %s (%s)", n, runs, key))
	if !shown {
		return out
	}
	wrap := lipgloss.NewStyle().Width(max(w-2, 10))
	now := time.Now()
	for _, e := range b.History {
		out += "\n" + entryHead(e, now) + "\n" + wrap.Render(m.entryText(b, e))
	}
	return out
}

// entryHead is the dim line over an earlier run: how far back it is,
// its exit status, red when it failed, and when it ran.
func entryHead(e doc.Entry, now time.Time) string {
	back := fmt.Sprintf("run %d back", e.K)
	exit := ""
	if !e.Region || e.Exit != 0 || e.Exited {
		exit = fmt.Sprintf("exit %d", e.Exit)
	}
	when := "time not saved"
	if t, err := time.Parse(time.RFC3339, e.At); err == nil {
		when = runAt(t, now)
	}
	parts := []string{savedStyle.Render("── " + back)}
	if exit != "" {
		if e.Exit != 0 || e.Exited {
			parts = append(parts, stderrStyle.Render("✗ "+exit))
		} else {
			parts = append(parts, savedStyle.Render(exit))
		}
	}
	parts = append(parts, savedStyle.Render(when))
	return strings.Join(parts, savedStyle.Render(", "))
}

// entryText is earlier run e's output, as saved output shows: its
// stderr red, and an image or recording by its link.
func (m *Model) entryText(b *doc.Block, e doc.Entry) string {
	var parts []string
	for _, p := range e.Parts {
		src := m.d.Src[p.Start:p.End]
		first, _, _ := bytes.Cut(src, []byte("\n"))
		text := saved(src)
		switch {
		case text == "":
			continue
		case bytes.Contains(first, []byte("stream=stderr")):
			parts = append(parts, stderrStyle.Render(clean(termsafe.Styled(text))))
		case imageLink(text) != "":
			parts = append(parts, savedStyle.Render("image: "+termsafe.Visible(imageLink(text))))
		case run.OutKind(b) == "markdown":
			parts = append(parts, m.markdown(text))
		default:
			// Dim, as saved output is, so the current output stands out.
			parts = append(parts, savedStyle.Render(links(b, clean(termsafe.Styled(markCopyable(text))))))
		}
	}
	if len(parts) == 0 {
		return savedStyle.Render("(no output)")
	}
	return strings.Join(parts, "\n")
}

// runAt says when t was, in local time, as a person would: "today
// 19:02", "yesterday 19:02", "Mon 19:02" within the week, then "Nov 3
// 19:02", with the year once it isn't now's.
func runAt(t, now time.Time) string {
	t = t.In(now.Location())
	clock := t.Format("15:04")
	day := func(x time.Time) time.Time { return time.Date(x.Year(), x.Month(), x.Day(), 0, 0, 0, 0, x.Location()) }
	days := int(day(now).Sub(day(t)).Hours()/24 + 0.5)
	switch {
	case days == 0:
		return "today " + clock
	case days == 1:
		return "yesterday " + clock
	case days > 1 && days < 7:
		return t.Format("Mon") + " " + clock
	case t.Year() != now.Year():
		return t.Format("Jan 2, 2006") + " " + clock
	}
	return t.Format("Jan 2") + " " + clock
}

// foldRuns writes the results into the document, as saving does, once
// a run that ran a block with a history ends. The next run of that
// block then pushes this one down as its run before, though the file
// isn't saved in between, and the earlier runs show at once. Results
// written in are marked, so neither this nor saving pushes them twice.
func (m *Model) foldRuns() {
	any := false
	for _, r := range m.results {
		if r != nil && r.Ran && !m.inSource[r] && r.Block.HistoryCap(m.d.Options) > 0 {
			any = true
			break
		}
	}
	if !any {
		return
	}
	trimmed := trims(m.d)
	out := render.Markdown(m.d, m.results, m.writeOptions())
	m.markInSource()
	if bytes.Equal(out, m.d.Src) {
		return
	}
	// Earlier runs that fell past their block's cap take their files
	// along.
	run.TidyHistoryAssets(m.d.Src, out, m.opts.Run)
	if err := m.d.SetSource(out); err != nil {
		m.status = "keeping earlier runs: " + err.Error()
		return
	}
	if err := m.syncSession(); err != nil {
		m.status = err.Error()
	}
	m.results = append([]*run.Result(nil), m.sess.Results()...)
	m.index = map[*doc.Block]int{}
	m.layout()
	m.md = map[string]string{}
	if trimmed != "" {
		m.status = trimmed
	}
}

// writeOptions are the options for writing the document back with its
// results.
func (m *Model) writeOptions() render.Options {
	return render.Options{WriteBack: true, Env: m.opts.Run.Environ, InSource: func(r *run.Result) bool { return m.inSource[r] }}
}

// markInSource notes that every result that ran is in the document's
// source now.
func (m *Model) markInSource() {
	if m.inSource == nil {
		m.inSource = map[*run.Result]bool{}
	}
	for _, r := range m.results {
		if r != nil && r.Ran {
			m.inSource[r] = true
		}
	}
}

// trims says which blocks' earlier runs writing the document back
// drops because their history= keeps fewer than they have, for the
// footer, or "".
func trims(d *doc.Document) string {
	var says []string
	for _, b := range d.Blocks {
		keep := b.HistoryCap(d.Options)
		if len(b.History) <= keep {
			continue
		}
		drop := len(b.History) - keep
		runs := "earlier runs"
		if drop == 1 {
			runs = "earlier run"
		}
		says = append(says, fmt.Sprintf("dropped %d %s of %s", drop, runs, termsafe.Visible(d.Label(b))))
	}
	if len(says) == 0 {
		return ""
	}
	return strings.Join(says, ", ") + ", past history="
}
