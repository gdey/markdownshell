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

// A run replaces the current output and, until rotation is built, keeps
// the earlier runs as the file has them, under it. A run with nothing to
// show keeps them too.
func TestHistoryWriteBackKeepsEntries(t *testing.T) {
	d := parseHist(t, histSrc)
	at := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	r := []*run.Result{{Block: d.Blocks[0], Ran: true, Start: at, Stdout: []byte("fine\n")}}
	want := histBlock + "\n```output {for=check exit=0 at=2026-11-14T08:00:00Z}\nfine\n```\n\n" + histEarlier + "\nEnd.\n"
	if got := string(Markdown(d, r, Options{WriteBack: true})); got != want {
		t.Errorf("after a run:\ngot  %q\nwant %q", got, want)
	}
	if got := string(Markdown(d, r, Options{})); strings.Contains(got, "history=1") {
		t.Errorf("printed after a run shows earlier runs:\n%s", got)
	}

	r[0].Stdout = nil
	want = histBlock + "\n" + histEarlier + "\nEnd.\n"
	got := string(Markdown(d, r, Options{WriteBack: true}))
	if got != want {
		t.Errorf("after a run with no output:\ngot  %q\nwant %q", got, want)
	}
	if again := parseHist(t, got); len(again.Blocks[0].History) != 1 {
		t.Errorf("earlier runs lost on reparse: %+v", again.Blocks[0].History)
	}
}

// In a list item, the earlier runs keep the item's indent.
func TestHistoryWriteBackNested(t *testing.T) {
	src := "- step:\n\n  ```sh {#a history=2}\n  echo a\n  ```\n\n  ```output {for=a exit=0}\n  a\n  ```\n\n  ```output {for=a exit=0 history=1}\n  old\n  ```\n\n- next\n"
	d := parseHist(t, src)
	r := []*run.Result{{Block: d.Blocks[0], Ran: true, Stdout: []byte("new\n")}}
	want := strings.Replace(src, "  a\n  ```\n\n  ```output {for=a exit=0 history=1}", "  new\n  ```\n\n  ```output {for=a exit=0 history=1}", 1)
	if got := string(Markdown(d, r, Options{WriteBack: true})); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// Clear output removes the current output and every earlier run.
func TestClearHistory(t *testing.T) {
	d := parseHist(t, histSrc)
	if got, want := string(Clear(d)), histBlock+"\nEnd.\n"; got != want {
		t.Errorf("cleared:\ngot  %q\nwant %q", got, want)
	}
}
