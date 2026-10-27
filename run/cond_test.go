package run_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func environ(kv ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH")}, kv...)
}

func TestRunCondition(t *testing.T) {
	src := "```sh {#detect}\nOS=Linux\n```\n" +
		"```sh {#dbg run={{$DEBUG}}}\necho dbg\n```\n" +
		"```sh {#lin run={{#detect/env(OS)}}==Linux}\necho lin\n```\n" +
		"```sh {#mac run={{#detect/env(OS)}}==Darwin}\necho mac\n```\n"
	for debug, want := range map[string]map[string]bool{
		"":  {"dbg": false, "lin": true, "mac": false},
		"1": {"dbg": true, "lin": true, "mac": false},
	} {
		_, results := runDoc(t, src, run.Options{Environ: environ("DEBUG=" + debug)})
		r := byID(results)
		for id, ran := range want {
			if r[id].Ran != ran {
				t.Errorf("DEBUG=%q: %s ran = %v, want %v", debug, id, r[id].Ran, ran)
			}
		}
	}
}

func TestShowConditions(t *testing.T) {
	src := "intro\n\n```sh {#a show={{$DEBUG}}}\necho a\n```\n\n```sh {#b show-output=!{{$QUIET}}}\necho b\n```\n\nend\n"
	d, results := runDoc(t, src, run.Options{})

	got := string(render.Markdown(d, results, render.Options{Env: environ("QUIET=1")}))
	want := "intro\n\n```output {for=a exit=0}\na\n```\n\n```sh {#b show-output=!{{$QUIET}}}\necho b\n```\n\nend\n"
	if got != want {
		t.Errorf("printed:\ngot  %q\nwant %q", got, want)
	}

	saved := string(render.Markdown(d, results, render.Options{WriteBack: true, Env: environ("QUIET=1")}))
	for _, w := range []string{"echo a\n```", "```output {for=b exit=0}\nb\n```"} {
		if !strings.Contains(saved, w) {
			t.Errorf("write-back dropped %q:\n%s", w, saved)
		}
	}
}

func TestSections(t *testing.T) {
	src := "top\n\n<!-- mdsh:if {{$DEBUG}} -->\nDebug notes.\n\n```sh {#dbg}\necho dbg\n```\n<!-- mdsh:if {{$VERBOSE}} -->\nvery\n<!-- mdsh:end -->\n<!-- mdsh:end -->\n\nend\n"
	d, results := runDoc(t, src, run.Options{})

	off := string(render.Markdown(d, results, render.Options{Env: environ()}))
	if off != "top\n\n\nend\n" {
		t.Errorf("DEBUG unset: %q", off)
	}
	on := string(render.Markdown(d, results, render.Options{Env: environ("DEBUG=1")}))
	if !strings.Contains(on, "Debug notes.") || !strings.Contains(on, "echo dbg") || strings.Contains(on, "very") || strings.Contains(on, "mdsh:if") {
		t.Errorf("DEBUG=1: %q", on)
	}
	saved := string(render.Markdown(d, results, render.Options{WriteBack: true, Env: environ()}))
	if !strings.Contains(saved, "<!-- mdsh:if {{$DEBUG}} -->\nDebug notes.") || !strings.Contains(saved, "very") {
		t.Errorf("write-back: %q", saved)
	}
}

func TestStderrVisibility(t *testing.T) {
	src := "```sh {#ok}\necho warn >&2\n```\n```sh {#bad}\necho oops >&2; false\n```\n```sh {#loud show-stderr}\necho loud >&2\n```\n```sh {#hush show-stderr=false}\necho hush >&2; false\n```\n"
	d, results := runDoc(t, src, run.Options{})
	got := string(render.Markdown(d, results, render.Options{}))
	for w, want := range map[string]bool{"warn": false, "oops": true, "loud": true, "hush": false} {
		if strings.Contains(got, "stream=stderr}\n"+w) != want {
			t.Errorf("%q shown = %v, want %v:\n%s", w, !want, want, got)
		}
	}

	d, results = runDoc(t, "---\nmdsh:\n  show_stderr: true\n---\n"+src, run.Options{})
	got = string(render.Markdown(d, results, render.Options{}))
	if !strings.Contains(got, "stream=stderr}\nwarn") || strings.Contains(got, "stream=stderr}\nhush") {
		t.Errorf("front matter show_stderr:\n%s", got)
	}
}

func TestCallerPlaceholders(t *testing.T) {
	d, results := runDoc(t, "```sh\ntrue\n```\n\nby {{$WHO:\"old\"}}\n", run.Options{})
	if got := string(render.Markdown(d, results, render.Options{Env: environ("WHO=j")})); !strings.Contains(got, "by j\n") {
		t.Errorf("printed: %q", got)
	}
	if got := string(render.Markdown(d, results, render.Options{WriteBack: true, Env: environ("WHO=j")})); !strings.Contains(got, `by {{$WHO:"j"}}`) {
		t.Errorf("saved: %q", got)
	}
	if got := string(render.Markdown(d, results, render.Options{})); !strings.Contains(got, "by old\n") {
		t.Errorf("no env: %q", got)
	}
}

func TestStatusRefs(t *testing.T) {
	src := "```sh {#bad}\nfalse\n```\n" +
		"```sh {#next run={{$?}}==0}\necho next\n```\n" +
		"```sh {#retry run={{#bad/?}}!=0 show-output={{$?}}!=0}\necho retry\n```\n\n" +
		"bad={{#bad/?}} last={{$?}}\n"
	d, results := runDoc(t, src, run.Options{})
	r := byID(results)
	if r["next"].Ran || !r["retry"].Ran {
		t.Fatalf("next ran = %v, retry ran = %v", r["next"].Ran, r["retry"].Ran)
	}

	got := string(render.Markdown(d, results, render.Options{Env: environ()}))
	if strings.Contains(got, "```output {for=retry") {
		t.Errorf("retry succeeded, so its output should be hidden:\n%s", got)
	}
	if !strings.HasSuffix(got, "bad=1 last=0\n") {
		t.Errorf("placeholders:\n%s", got)
	}

	saved := string(render.Markdown(d, results, render.Options{WriteBack: true, Env: environ()}))
	if !strings.HasSuffix(saved, `bad={{#bad/?:"1"}} last={{$?:"0"}}`+"\n") {
		t.Errorf("write-back:\n%s", saved)
	}
}

func TestRanRefs(t *testing.T) {
	src := "```sh {#opt run={{$OPT}}}\necho opt\n```\n" +
		"```sh {#after run=!{{$ran?}}}\necho fallback\n```\n" +
		"```sh {#bad}\nfalse\n```\n" +
		"```sh {#fix run={{$err?}}}\necho fix\n```\n\n" +
		"opt={{#opt/ran?}} after={{#after/ran?}} err={{#bad/err?}} last={{$ran?}}\n"
	d, results := runDoc(t, src, run.Options{Environ: environ()})
	r := byID(results)
	if r["opt"].Ran || !r["after"].Ran || !r["fix"].Ran {
		t.Fatalf("opt ran = %v, after ran = %v, fix ran = %v", r["opt"].Ran, r["after"].Ran, r["fix"].Ran)
	}
	got := string(render.Markdown(d, results, render.Options{Env: environ()}))
	if !strings.HasSuffix(got, "opt=false after=true err=1 last=true\n") {
		t.Errorf("placeholders:\n%s", got)
	}
	saved := string(render.Markdown(d, results, render.Options{WriteBack: true, Env: environ()}))
	if !strings.HasSuffix(saved, `opt={{#opt/ran?:"false"}} after={{#after/ran?:"true"}} err={{#bad/err?:"1"}} last={{$ran?:"true"}}`+"\n") {
		t.Errorf("write-back:\n%s", saved)
	}
}
