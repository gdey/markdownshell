package render

import (
	"strings"
	"testing"
	"time"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

const (
	histBlock   = "```sh {#check history=3}\necho ok\n```\n"
	histCurrent = "```output {for=check exit=0 at=2026-11-13T19:08:00Z}\nok\n```\n"
	histEarlier = "```output {for=check exit=7 history=1 at=2026-11-12T19:02:11Z}\nno\n```\n\n" +
		"```output {for=check history=1 stream=stderr}\ncurl: (7)\n```\n"
	histSrc = histBlock + "\n" + histCurrent + "\n" + histEarlier + "\nEnd.\n"
)

func parseHist(t *testing.T, src string) *doc.Document {
	t.Helper()
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// Printing shows a block's current output, without its earlier runs;
// write-back keeps them.
func TestHistoryPrinting(t *testing.T) {
	d := parseHist(t, histSrc)
	if got, want := string(Markdown(d, nil, Options{})), histBlock+"\n"+histCurrent+"\nEnd.\n"; got != want {
		t.Errorf("printed:\ngot  %q\nwant %q", got, want)
	}
	if got := string(Markdown(d, nil, Options{WriteBack: true})); got != histSrc {
		t.Errorf("write-back without a run:\ngot  %q\nwant %q", got, histSrc)
	}
	if got, want := string(Markdown(d, nil, Options{HideCode: true})), histCurrent+"\nEnd.\n"; got != want {
		t.Errorf("printed without code:\ngot  %q\nwant %q", got, want)
	}
}

// A run pushes the output the file holds down as earlier run 1, and the
// others down one, up to the cap. A run that shows nothing is kept too,
// as an empty fence with its time.
func TestHistoryRotate(t *testing.T) {
	d := parseHist(t, histSrc)
	at := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	r := []*run.Result{{Block: d.Blocks[0], Ran: true, Start: at, Stdout: []byte("fine\n")}}
	pushed := "```output {for=check exit=0 at=2026-11-13T19:08:00Z history=1}\nok\n```\n\n" +
		"```output {for=check exit=7 history=2 at=2026-11-12T19:02:11Z}\nno\n```\n\n" +
		"```output {for=check history=2 stream=stderr}\ncurl: (7)\n```\n"
	want := histBlock + "\n```output {for=check exit=0 at=2026-11-14T08:00:00Z}\nfine\n```\n\n" + pushed + "\nEnd.\n"
	got := string(Markdown(d, r, Options{WriteBack: true}))
	if got != want {
		t.Errorf("after a run:\ngot  %q\nwant %q", got, want)
	}
	if got := string(Markdown(d, r, Options{})); strings.Contains(got, "history=1") {
		t.Errorf("printed after a run shows earlier runs:\n%s", got)
	}

	// Saving again in the same session, as mdsh-gui does after each
	// run, doesn't push the same run twice.
	d2 := parseHist(t, got)
	r[0].Block = d2.Blocks[0]
	inSource := func(*run.Result) bool { return true }
	if again := string(Markdown(d2, r, Options{WriteBack: true, InSource: inSource})); again != got {
		t.Errorf("second save of the same run:\ngot  %q\nwant %q", again, got)
	}

	// The next run pushes again, and the oldest falls off past 3.
	r = []*run.Result{{Block: d2.Blocks[0], Ran: true, Start: at.Add(time.Hour), Status: 1}}
	got = string(Markdown(d2, r, Options{WriteBack: true}))
	want = histBlock + "\n```output {for=check exit=1 at=2026-11-14T09:00:00Z}\n```\n\n" +
		"```output {for=check exit=0 at=2026-11-14T08:00:00Z history=1}\nfine\n```\n\n" +
		"```output {for=check exit=0 at=2026-11-13T19:08:00Z history=2}\nok\n```\n\n" +
		"```output {for=check exit=7 history=3 at=2026-11-12T19:02:11Z}\nno\n```\n\n" +
		"```output {for=check history=3 stream=stderr}\ncurl: (7)\n```\n\nEnd.\n"
	if got != want {
		t.Errorf("third run:\ngot  %q\nwant %q", got, want)
	}
	d3 := parseHist(t, got)
	r = []*run.Result{{Block: d3.Blocks[0], Ran: true, Start: at.Add(2 * time.Hour), Stdout: []byte("x\n")}}
	got = string(Markdown(d3, r, Options{WriteBack: true}))
	if strings.Contains(got, "curl: (7)") || strings.Count(got, "history=") != 4 || !strings.Contains(got, "history=3}\nok\n") {
		t.Errorf("fourth run, past the cap:\n%s", got)
	}
}

// The first run of a block with a history, and a run after output an
// older mdsh wrote without at=, push what the file holds.
func TestHistoryRotateFirst(t *testing.T) {
	d := parseHist(t, "```sh {#a history}\necho a\n```\n\n```output {for=a exit=0}\nold\n```\n")
	r := []*run.Result{{Block: d.Blocks[0], Ran: true, Stdout: []byte("new\n")}}
	want := "```sh {#a history}\necho a\n```\n\n```output {for=a exit=0}\nnew\n```\n\n```output {for=a exit=0 history=1}\nold\n```\n"
	if got := string(Markdown(d, r, Options{WriteBack: true})); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	d = parseHist(t, "```sh {#a history}\necho a\n```\n")
	r = []*run.Result{{Block: d.Blocks[0], Ran: true, Stdout: []byte("new\n")}}
	if got, want := string(Markdown(d, r, Options{WriteBack: true})), "```sh {#a history}\necho a\n```\n\n```output {for=a exit=0}\nnew\n```\n"; got != want {
		t.Errorf("no output before:\ngot  %q\nwant %q", got, want)
	}
}

// Saving holds the earlier runs to the cap, and numbers them again, even
// for a block that didn't run; a block without history= drops them.
func TestHistoryCapOnSave(t *testing.T) {
	src := "```sh {#a history=1}\necho a\n```\n\n```output {for=a exit=0}\nnow\n```\n\n" +
		"```output {for=a exit=0 history=2}\none\n```\n\n```output {for=a exit=0 history=5}\ntwo\n```\n\nEnd.\n"
	want := "```sh {#a history=1}\necho a\n```\n\n```output {for=a exit=0}\nnow\n```\n\n```output {for=a exit=0 history=1}\none\n```\n\nEnd.\n"
	if got := string(Markdown(parseHist(t, src), nil, Options{WriteBack: true})); got != want {
		t.Errorf("history=1:\ngot  %q\nwant %q", got, want)
	}
	src = strings.Replace(src, " history=1}", "}", 1)
	want = "```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\nnow\n```\n\nEnd.\n"
	if got := string(Markdown(parseHist(t, src), nil, Options{WriteBack: true})); got != want {
		t.Errorf("no history=:\ngot  %q\nwant %q", got, want)
	}
}

// An image output pushed down links to its file's earlier-run name.
func TestHistoryRotateImage(t *testing.T) {
	src := "```sh {#s out=screenshot history}\necho hi\n```\n\n<!-- mdsh:output for=\"s\" at=\"2026-11-13T19:08:00Z\" -->\n![s](d.md.assets/s.svg)\n<!-- /mdsh:output -->\n"
	d := parseHist(t, src)
	r := []*run.Result{{Block: d.Blocks[0], Ran: true, Start: time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC), Image: "d.md.assets/s.svg"}}
	want := "```sh {#s out=screenshot history}\necho hi\n```\n\n" +
		"<!-- mdsh:output for=\"s\" at=\"2026-11-14T08:00:00Z\" -->\n![s](d.md.assets/s.svg)\n<!-- /mdsh:output -->\n\n" +
		"<!-- mdsh:output for=\"s\" at=\"2026-11-13T19:08:00Z\" history=\"1\" -->\n![s](d.md.assets/s.2026-11-13T190800Z.svg)\n<!-- /mdsh:output -->\n"
	if got := string(Markdown(d, r, Options{WriteBack: true})); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// In a list item, the earlier runs keep the item's indent.
func TestHistoryWriteBackNested(t *testing.T) {
	src := "- step:\n\n  ```sh {#a history=2}\n  echo a\n  ```\n\n  ```output {for=a exit=0}\n  a\n  ```\n\n  ```output {for=a exit=0 history=1}\n  old\n  ```\n\n- next\n"
	d := parseHist(t, src)
	r := []*run.Result{{Block: d.Blocks[0], Ran: true, Stdout: []byte("new\n")}}
	want := "- step:\n\n  ```sh {#a history=2}\n  echo a\n  ```\n\n  ```output {for=a exit=0}\n  new\n  ```\n\n" +
		"  ```output {for=a exit=0 history=1}\n  a\n  ```\n\n  ```output {for=a exit=0 history=2}\n  old\n  ```\n\n- next\n"
	if got := string(Markdown(d, r, Options{WriteBack: true})); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if _, err := doc.Parse([]byte(want)); err != nil {
		t.Error(err)
	}
}

// Clear output removes the current output and every earlier run.
func TestClearHistory(t *testing.T) {
	d := parseHist(t, histSrc)
	if got, want := string(Clear(d)), histBlock+"\nEnd.\n"; got != want {
		t.Errorf("cleared:\ngot  %q\nwant %q", got, want)
	}
}

func TestDropEarlierRuns(t *testing.T) {
	src := "```sh {#n history=3}\necho 4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:03Z history=1}\n3\n```\n\n" +
		"```output {for=n exit=1 at=2026-11-16T22:20:02Z history=2}\n2\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:01Z history=3}\n1\n```\n\nAfter.\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	b := d.Blocks[0]
	got := string(DropEarlierRuns(d.Src, b, func(e doc.Entry) bool { return e.K == 2 }))
	want := "```sh {#n history=3}\necho 4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:03Z history=1}\n3\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:01Z history=2}\n1\n```\n\nAfter.\n"
	if got != want {
		t.Errorf("dropping run 2:\n%s\nwant\n%s", got, want)
	}
	if d2, err := doc.Parse([]byte(got)); err != nil || len(d2.Blocks[0].History) != 2 {
		t.Errorf("after dropping run 2: %v, %d earlier runs", err, len(d2.Blocks[0].History))
	}
	got = string(DropEarlierRuns(d.Src, b, func(doc.Entry) bool { return true }))
	want = "```sh {#n history=3}\necho 4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\nAfter.\n"
	if got != want {
		t.Errorf("dropping all:\n%s\nwant\n%s", got, want)
	}
}

// Printing with History shows the earlier runs under the output, each
// under a line saying when it ran; without it, only the current output.
func TestPrintHistory(t *testing.T) {
	src := "```sh {#n history=3}\necho 4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:03Z history=1}\n3\n```\n\n" +
		"```output {for=n exit=7 at=2026-11-12T19:02:00Z history=2}\n```\n\n```output {for=n stream=stderr history=2}\nrefused\n```\n\nAfter.\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Markdown(d, nil, Options{})); strings.Contains(got, "history=1") || strings.Contains(got, "Run 1 back") {
		t.Errorf("printed without History:\n%s", got)
	}
	got := string(Markdown(d, nil, Options{History: true}))
	want := "```sh {#n history=3}\necho 4\n```\n\n" +
		"```output {for=n exit=0 at=2026-11-16T22:20:04Z}\n4\n```\n\n" +
		"*Run 1 back, 2026-11-16 22:20 UTC, exit 0*\n\n```output {for=n exit=0 at=2026-11-16T22:20:03Z history=1}\n3\n```\n\n" +
		"*Run 2 back, 2026-11-12 19:02 UTC, exit 7*\n\n```output {for=n exit=7 at=2026-11-12T19:02:00Z history=2}\n```\n\n```output {for=n stream=stderr history=2}\nrefused\n```\n\nAfter.\n"
	if got != want {
		t.Errorf("printed with History:\n%s\nwant\n%s", got, want)
	}
	// After a run, what the file holds is the run before it.
	r := &run.Result{Block: d.Blocks[0], Ran: true, Stdout: []byte("5\n"), Start: time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)}
	got = string(Markdown(d, []*run.Result{r}, Options{History: true}))
	for _, want := range []string{"5\n```", "*Run 1 back, 2026-11-16 22:20 UTC, exit 0*\n\n```output {for=n exit=0 at=2026-11-16T22:20:04Z history=1}\n4", "*Run 3 back, 2026-11-12 19:02 UTC, exit 7*"} {
		if !strings.Contains(got, want) {
			t.Errorf("printed after a run has no %q:\n%s", want, got)
		}
	}
}
