package render

import (
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

func TestFence(t *testing.T) {
	if got := Fence("output", "for=a", "x"); got != "```output {for=a}\nx\n```\n" {
		t.Errorf("got %q", got)
	}
	if got := Fence("output", "", "a ```` b\n"); got != "`````output\na ```` b\n`````\n" {
		t.Errorf("got %q", got)
	}
}

func TestHideFlags(t *testing.T) {
	src := "#!/usr/bin/env mdsh\n# T\n\n```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\nold\n```\n\n```go\nx\n```\n\nend\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	results := []*run.Result{
		{Block: d.Blocks[0], Ran: true, Stdout: []byte("new\n")},
		{Block: d.Blocks[1], Skipped: true},
	}
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{"all", Options{}, "# T\n\n```sh {#a}\necho a\n```\n\n```output {for=a exit=0}\nnew\n```\n\n```go\nx\n```\n\nend\n"},
		{"hide code", Options{HideCode: true}, "# T\n\n```output {for=a exit=0}\nnew\n```\n\n```go\nx\n```\n\nend\n"},
		{"hide output", Options{HideOutput: true}, "# T\n\n```sh {#a}\necho a\n```\n\n```go\nx\n```\n\nend\n"},
		{"prose", Options{HideCode: true, HideOutput: true}, "# T\n\n```go\nx\n```\n\nend\n"},
	}
	for _, tt := range tests {
		if got := string(Markdown(d, results, tt.opts)); got != tt.want {
			t.Errorf("%s:\ngot  %q\nwant %q", tt.name, got, tt.want)
		}
	}
	// Without results, existing output is kept.
	if got := string(Markdown(d, nil, Options{WriteBack: true})); got != src {
		t.Errorf("no results:\ngot  %q\nwant %q", got, src)
	}
}
