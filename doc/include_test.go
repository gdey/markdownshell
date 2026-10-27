package doc

import (
	"strings"
	"testing"
)

func TestInclude(t *testing.T) {
	tests := []struct {
		info string
		want Include
		err  string
	}{
		{"go {<include file=a.go}", Include{File: "a.go"}, ""},
		{"go {<include file=a.go from=2 to=4 number_on}", Include{File: "a.go", From: 2, To: 4, Numbered: true}, ""},
		{"{<include file=Makefile number_on=false}", Include{File: "Makefile"}, ""},
		{"go {<include from=1}", Include{}, "needs file="},
		{"go {<include file=a.go from=x}", Include{}, "from= must be a line number"},
		{"go {<include file=a.go from=-1}", Include{}, "from= must be a line number"},
		{"go {<include file=a.go from=5 to=4}", Include{}, "before from=5"},
		{"sh {<include file=a.sh env=x}", Include{}, "can't have env="},
		{"sh {#a <include file=a.sh}", Include{}, "has no name"},
	}
	for _, tt := range tests {
		d, err := Parse([]byte("```" + tt.info + "\n```\n"))
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%s: err = %v, want %q", tt.info, err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tt.info, err)
			continue
		}
		got, _ := d.Blocks[0].Include()
		if !d.Blocks[0].IsInclude() || got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.info, got, tt.want)
		}
	}
}

func TestIncludeIDs(t *testing.T) {
	src := "```sh\n```\n```go {<include file=a.go}\nold\n```\n```sh\n```\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, b := range d.Blocks {
		ids = append(ids, b.ID)
	}
	// Includes don't shift block-N numbering.
	if got := strings.Join(ids, " "); got != "block-1 <include-1 block-2" {
		t.Errorf("ids = %s", got)
	}
	inc := d.Blocks[1]
	if body := src[inc.BodySpan.Start:inc.BodySpan.End]; body != "old\n" || inc.Fence != "```" || inc.Info != "go {<include file=a.go}" {
		t.Errorf("body %q fence %q info %q", body, inc.Fence, inc.Info)
	}
	if _, err := Parse([]byte(src + "{{#<include-1/?}}\n")); err == nil {
		t.Error("placeholder for an include block: no error")
	}
}
