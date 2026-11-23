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
		{`text {<include file=a.log match="^FAIL" before=3 lines=20}`, Include{File: "a.log", Match: "^FAIL", Before: 3, Lines: 20}, ""},
		{`text {<include file=a.log match=start until="^end \"x\""}`, Include{File: "a.log", Match: "start", Until: `^end "x"`}, ""},
		{"go {<include file=a.go symbol=T.M comment=false number_on}", Include{File: "a.go", Symbol: "T.M", NoComment: true, Numbered: true}, ""},
		{"go {<include file=a.go symbol=F comment}", Include{File: "a.go", Symbol: "F"}, ""},
		{"text {<include file=a.log from=4 lines=2}", Include{File: "a.log", From: 4, Lines: 2}, ""},
		{"text {<include file=a.log match=x from=2}", Include{}, "both from= and match="},
		{"go {<include file=a.go symbol=F to=2}", Include{}, "both to= and symbol="},
		{"go {<include file=a.go symbol=F match=x}", Include{}, "both match= and symbol="},
		{"text {<include file=a.log match=x lines=2 until=y}", Include{}, "both lines= and until="},
		{"text {<include file=a.log match=(}", Include{}, "match=:"},
		{"text {<include file=a.log match=}", Include{}, "match= is empty"},
		{"text {<include file=a.log lines=x}", Include{}, "lines= must be a number of lines"},
		{"text {<include file=a.log match=x comment=false}", Include{}, "comment= is for symbol="},
		{"text {<include file=a.log match=x nth=2}", Include{}, "nth= isn't supported yet"},
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
