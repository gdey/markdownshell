package run_test

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
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
		_, results := runDoc(t, src, run.Options{Environ: environ("DEBUG="+debug, "MDSH_EXPOSE=DEBUG")})
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
	d, results := runDoc(t, src, run.Options{Now: fixedNow})

	got := string(render.Markdown(d, results, render.Options{Env: environ("QUIET=1", "MDSH_EXPOSE=QUIET")}))
	want := "intro\n\n```output {for=a exit=0 at=2026-11-13T19:08:00Z}\na\n```\n\n```sh {#b show-output=!{{$QUIET}}}\necho b\n```\n\nend\n"
	if got != want {
		t.Errorf("printed:\ngot  %q\nwant %q", got, want)
	}

	saved := string(render.Markdown(d, results, render.Options{WriteBack: true, Env: environ("QUIET=1", "MDSH_EXPOSE=QUIET")}))
	for _, w := range []string{"echo a\n```", "```output {for=b exit=0 at=2026-11-13T19:08:00Z}\nb\n```"} {
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
	on := string(render.Markdown(d, results, render.Options{Env: environ("DEBUG=1", "MDSH_EXPOSE=DEBUG")}))
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
	got := noAt(string(render.Markdown(d, results, render.Options{})))
	for w, want := range map[string]bool{"warn": false, "oops": true, "loud": true, "hush": false} {
		if strings.Contains(got, "stream=stderr}\n"+w) != want {
			t.Errorf("%q shown = %v, want %v:\n%s", w, !want, want, got)
		}
	}

	d, results = runDoc(t, "---\nmdsh:\n  show_stderr: true\n---\n"+src, run.Options{})
	got = noAt(string(render.Markdown(d, results, render.Options{})))
	if !strings.Contains(got, "stream=stderr}\nwarn") || strings.Contains(got, "stream=stderr}\nhush") {
		t.Errorf("front matter show_stderr:\n%s", got)
	}
}

func TestCallerPlaceholders(t *testing.T) {
	d, results := runDoc(t, "```sh\ntrue\n```\n\nby {{$WHO:\"old\"}} in {{$MDSH_MODE}}, {{$SECRET}}\n", run.Options{})
	env := environ("WHO=j", "SECRET=s3cr3t", "MDSH_MODE=report", "MDSH_EXPOSE=WHO")
	// Only exposed variables and mdsh's own are read; others print as
	// written.
	if got := string(render.Markdown(d, results, render.Options{Env: env})); !strings.Contains(got, "by j in report, {{$SECRET}}\n") {
		t.Errorf("printed: %q", got)
	}
	// Write-back never stores the caller's value, and drops an old one.
	if got := string(render.Markdown(d, results, render.Options{WriteBack: true, Env: env})); !strings.Contains(got, "by {{$WHO}} in {{$MDSH_MODE}}, {{$SECRET}}\n") {
		t.Errorf("saved: %q", got)
	}
	if got := string(render.Markdown(d, results, render.Options{})); !strings.Contains(got, "by old in") {
		t.Errorf("no env: %q", got)
	}
}

// TestCallerVars checks what {{$VAR}} may read from the caller's
// environment (v1.2.1 review, finding 2): names the person exposed, as
// env.share passes them, and mdsh's own; never what the document asks
// for itself.
func TestCallerVars(t *testing.T) {
	env := environ("SECRET_TOKEN=s3cr3t", "WHO=j", "MDSH_MODE=running")
	for _, tc := range []struct {
		front  string
		expose []string
		want   map[string]string
	}{
		{"", nil, map[string]string{"MDSH_MODE": "running"}},
		{"---\nmdsh:\n  env:\n    share: [\"...\"]\n---\n", nil, map[string]string{"MDSH_MODE": "running"}},
		{"", []string{"WHO"}, map[string]string{"MDSH_MODE": "running", "MDSH_EXPOSE": "WHO", "WHO": "j"}},
		{"---\nmdsh:\n  env:\n    share: [PATH]\n---\n", []string{"WHO", "PATH"}, map[string]string{"MDSH_MODE": "running", "MDSH_EXPOSE": "WHO,PATH", "PATH": os.Getenv("PATH")}},
	} {
		d, err := doc.Parse([]byte(tc.front + "x\n"))
		if err != nil {
			t.Fatal(err)
		}
		got := run.CallerVars(d, run.WithExpose(env, tc.expose))
		if !maps.Equal(got, tc.want) {
			t.Errorf("%q, expose %v: got %v, want %v", tc.front, tc.expose, got, tc.want)
		}
	}
	if got := run.WithExpose([]string{"MDSH_EXPOSE=A", "X=1"}, []string{"B"}); !slices.Equal(got, []string{"X=1", "MDSH_EXPOSE=A,B"}) {
		t.Errorf("WithExpose = %v", got)
	}
}

// TestCallerPlaceholdersShare checks that conditions and the lookup
// tapes use don't read an unexposed variable.
func TestCallerPlaceholdersShare(t *testing.T) {
	src := "Token: [{{$SECRET_TOKEN}}]\n\n<!-- mdsh:if {{$SECRET_TOKEN}} -->\nleaked\n<!-- mdsh:end -->\n"
	env := environ("SECRET_TOKEN=s3cr3t")
	d, results := runDoc(t, src, run.Options{Environ: env})
	got := string(render.Markdown(d, results, render.Options{Env: env}))
	if strings.Contains(got, "s3cr3t") || strings.Contains(got, "leaked") || !strings.Contains(got, "Token: [{{$SECRET_TOKEN}}]") {
		t.Errorf("printed:\n%s", got)
	}
	if v, _ := run.NewLookup(d, results, env)(doc.Ref{Var: "SECRET_TOKEN"}); v != "" {
		t.Errorf("lookup = %q", v)
	}
	if v, _ := run.NewLookup(d, results, run.WithExpose(env, []string{"SECRET_TOKEN"}))(doc.Ref{Var: "SECRET_TOKEN"}); v != "s3cr3t" {
		t.Errorf("exposed lookup = %q", v)
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
