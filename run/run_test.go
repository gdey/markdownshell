package run_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/render"
	"github.com/gdey/markdownshell/run"
)

func runDoc(t *testing.T, src string, opts run.Options) (*doc.Document, []*run.Result) {
	t.Helper()
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	if opts.Environ == nil {
		opts.Environ = []string{"PATH=" + os.Getenv("PATH"), "OUTER=outer"}
	}
	results, err := run.Run(context.Background(), d, opts)
	if err != nil {
		t.Fatal(err)
	}
	return d, results
}

func byID(results []*run.Result) map[string]*run.Result {
	m := map[string]*run.Result{}
	for _, r := range results {
		m[r.Block.ID] = r
	}
	return m
}

func TestSharedTopLevel(t *testing.T) {
	_, results := runDoc(t, "```sh {#a}\nX=1\nf() { echo f$X; }\ncd /\n```\n```sh {#b}\nf; pwd; echo $OUTER $1\n```\n",
		run.Options{Args: []string{"arg"}})
	r := byID(results)
	if got := string(r["b"].Stdout); got != "f1\n/\nouter arg\n" {
		t.Errorf("b stdout = %q", got)
	}
}

func TestSubEnvSharing(t *testing.T) {
	src := "```sh {#a}\nexport A=1 B=2\nC=3\n```\n" +
		"```sh {#b env=child share=A}\necho \"$A-${B-x}-${C-x}\"\nexport D=4\n```\n" +
		"```sh {#c env=child/grand}\necho \"$A-$D\"\n```\n" +
		"```sh {#d env=child}\necho \"$D\"\n```\n" +
		"```sh {#e}\necho \"${D-x}\"\n```\n" +
		"```sh {#f env=clean share=}\necho \"${A-x}\"\n```\n"
	_, results := runDoc(t, src, run.Options{})
	want := map[string]string{"b": "1-x-x\n", "c": "1-4\n", "d": "4\n", "e": "x\n", "f": "x\n"}
	r := byID(results)
	for id, w := range want {
		if got := string(r[id].Stdout); got != w {
			t.Errorf("%s stdout = %q, want %q (stderr %q, err %v)", id, got, w, r[id].Stderr, r[id].Err)
		}
	}
}

func TestShareExclusions(t *testing.T) {
	src := "---\nmdsh:\n  env:\n    share: [..., -SECRET]\n---\n" +
		"```sh {#a}\necho \"$OUTER-${SECRET-x}\"\nexport A=1 B=2\n```\n" +
		"```sh {#b env=sub share=...,-B}\necho \"$A-${B-x}\"\n```\n"
	_, results := runDoc(t, src, run.Options{Environ: []string{"PATH=" + os.Getenv("PATH"), "OUTER=outer", "SECRET=s"}})
	want := map[string]string{"a": "outer-x\n", "b": "1-x\n"}
	r := byID(results)
	for id, w := range want {
		if got := string(r[id].Stdout); got != w {
			t.Errorf("%s stdout = %q, want %q (stderr %q, err %v)", id, got, w, r[id].Stderr, r[id].Err)
		}
	}
}

func TestFailureContinuesByDefault(t *testing.T) {
	_, results := runDoc(t, "```sh {#a}\nfalse\n```\n```sh {#b}\necho b\n```\n", run.Options{})
	r := byID(results)
	if r["a"].Status != 1 || !r["a"].Failed() {
		t.Errorf("a = %+v", r["a"])
	}
	if !r["b"].Ran {
		t.Error("b should run after a failure by default")
	}
}

func TestStopOnError(t *testing.T) {
	for name, src := range map[string]string{
		"front matter": "---\nmdsh:\n  on_error: stop\n---\n```sh {#a}\nfalse\n```\n```sh {#b}\necho b\n```\n",
		"block":        "```sh {#a on_error=stop}\nfalse\n```\n```sh {#b}\necho b\n```\n",
	} {
		_, results := runDoc(t, src, run.Options{})
		if byID(results)["b"].Ran {
			t.Errorf("%s: b ran after a stopping failure", name)
		}
	}
}

func TestExitStops(t *testing.T) {
	_, results := runDoc(t, "```sh {#a}\necho bye; exit 3\n```\n```sh {#b}\necho b\n```\n", run.Options{})
	r := byID(results)
	if !r["a"].Exited || r["a"].Status != 3 || string(r["a"].Stdout) != "bye\n" {
		t.Errorf("a = %+v", r["a"])
	}
	if r["b"].Ran {
		t.Error("b ran after exit")
	}
}

func TestSyntaxErrorKeepsShell(t *testing.T) {
	_, results := runDoc(t, "```sh {#a}\nX=1\n```\n```sh {#bad}\nif then\n```\n```sh {#c}\necho $X\n```\n", run.Options{})
	r := byID(results)
	if r["bad"].Status == 0 || len(r["bad"].Stderr) == 0 {
		t.Errorf("bad = %+v", r["bad"])
	}
	if string(r["c"].Stdout) != "1\n" {
		t.Errorf("c stdout = %q, stderr %q", r["c"].Stdout, r["c"].Stderr)
	}
}

func TestSkipAndUnknownLanguage(t *testing.T) {
	_, results := runDoc(t, "```sh {#a skip}\necho a\n```\n```go {#g}\nx\n```\n", run.Options{})
	for _, r := range results {
		if r.Ran || !r.Skipped {
			t.Errorf("%s should be skipped", r.Block.ID)
		}
	}
}

func TestOnly(t *testing.T) {
	src := "```sh {#a}\nX=1\n```\n```sh {#other env=other}\necho o\n```\n```sh {#b env=e}\necho b\n```\n```sh {#c env=e}\necho c\n```\n```sh {#after}\necho z\n```\n"
	_, results := runDoc(t, src, run.Options{Only: "c"})
	r := byID(results)
	for id, ran := range map[string]bool{"a": true, "other": false, "b": true, "c": true, "after": false} {
		if r[id].Ran != ran {
			t.Errorf("%s ran = %v, want %v", id, r[id].Ran, ran)
		}
	}
}

func TestMarkdownOutput(t *testing.T) {
	d, results := runDoc(t, "```sh {#t out=markdown}\nprintf 'a\\tb\\n1\\t2\\n' | md_table\n```\n", run.Options{})
	got := string(render.Markdown(d, results, render.Options{}))
	if !strings.Contains(got, "| a | b |\n| --- | --- |\n| 1 | 2 |\n") {
		t.Errorf("rendered:\n%s", got)
	}
}

func TestMarkdownTableOptions(t *testing.T) {
	src := "```sh {#t out=markdown}\nprintf 'total 8\\n-rw-r--r--  1  j  2 a|b\\n' | md_table -s 1 -F ' '\nprintf 'x,y\\n1,2\\n' | md_table -F ,\nmd_table -s x </dev/null\n```\n"
	d, results := runDoc(t, src, run.Options{})
	got := string(render.Markdown(d, results, render.Options{}))
	for _, want := range []string{
		"| -rw-r--r-- | 1 | j | 2 | a\\|b |\n| --- | --- | --- | --- | --- |\n",
		"| x | y |\n| --- | --- |\n| 1 | 2 |\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in:\n%s", want, got)
		}
	}
	if r := results[0]; r.ExitStatus() != 2 || !strings.Contains(string(r.Stderr), "-s takes a number") {
		t.Errorf("md_table -s x: exit %d, stderr %q", r.ExitStatus(), r.Stderr)
	}
}

func TestImageOutput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "doc.md")
	src := "```sh {#pic out=image}\nprintf '<svg xmlns=\"http://www.w3.org/2000/svg\"/>'\n```\n"
	d, results := runDoc(t, src, run.Options{Dir: dir, File: file})
	r := results[0]
	if r.Err != nil || r.Image != "doc.md.assets/pic.svg" {
		t.Fatalf("result = %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc.md.assets", "pic.svg")); err != nil {
		t.Error(err)
	}
	if got := string(render.Markdown(d, results, render.Options{})); !strings.Contains(got, "![pic](doc.md.assets/pic.svg)") {
		t.Errorf("rendered:\n%s", got)
	}
}

func TestWriteBackIsStable(t *testing.T) {
	src := "#!/usr/bin/env mdsh\n# T\n\n```sh {#a}\necho '```'\necho err >&2\n```\n\n```sh {#m out=markdown}\necho '*hi*'\n```\n\ntext\n"
	d, results := runDoc(t, src, run.Options{})
	once := render.Markdown(d, results, render.Options{WriteBack: true})

	d2, results2 := runDoc(t, string(once), run.Options{})
	twice := render.Markdown(d2, results2, render.Options{WriteBack: true})
	if string(once) != string(twice) {
		t.Errorf("not stable:\n--- once\n%s\n--- twice\n%s", once, twice)
	}
	if !strings.HasPrefix(string(once), "#!") {
		t.Error("shebang dropped on write-back")
	}
	if len(d2.Blocks) != 2 {
		t.Errorf("output parsed as blocks: %d blocks", len(d2.Blocks))
	}
}

func TestPlaceholders(t *testing.T) {
	src := "```sh {#setup}\nexport G=hello\nL='local \"v\"'\n```\n\n" +
		"```sh {#sub env=s share=G}\nG=\"$G there\"\n```\n\n" +
		"```sh {#later}\nexport G=changed\n```\n\n" +
		"G is {{#setup/env(G):\"stale\"}}, L is {{#setup/env(L)}}, sub says {{#sub/env(G)}}, missing is [{{#setup/env(NOPE)}}].\n"
	d, results := runDoc(t, src, run.Options{})

	printed := string(render.Markdown(d, results, render.Options{}))
	want := `G is hello, L is local "v", sub says hello there, missing is [].`
	if !strings.Contains(printed, want) {
		t.Errorf("printed:\n%s\nwant line %q", printed, want)
	}

	saved := string(render.Markdown(d, results, render.Options{WriteBack: true}))
	want = `G is {{#setup/env(G):"hello"}}, L is {{#setup/env(L):"local \"v\""}}, sub says {{#sub/env(G):"hello there"}}, missing is [{{#setup/env(NOPE):""}}].`
	if !strings.Contains(saved, want) {
		t.Errorf("saved:\n%s\nwant line %q", saved, want)
	}

	// Saved values survive a run that skips the block.
	d2, results2 := runDoc(t, saved, run.Options{Only: "later"})
	if got := string(render.Markdown(d2, results2, render.Options{})); !strings.Contains(got, "sub says hello there,") {
		t.Errorf("stored value lost:\n%s", got)
	}
}

func TestPlaceholdersOff(t *testing.T) {
	src := "---\nmdsh:\n  interpolate: false\n---\n```sh {#a}\nX=1\n```\n\n{{#a/env(X)}}\n"
	d, results := runDoc(t, src, run.Options{})
	if got := string(render.Markdown(d, results, render.Options{})); !strings.Contains(got, "\n{{#a/env(X)}}\n") {
		t.Errorf("placeholder changed with interpolate off:\n%s", got)
	}
}

func TestTextAsImage(t *testing.T) {
	dir := t.TempDir()
	src := "```sh {#svg out=screenshot}\necho 'hi <there>'\n```\n```sh {#png out=screenshot:shots/p.png}\necho hi\n```\n"
	_, results := runDoc(t, src, run.Options{Dir: dir, File: filepath.Join(dir, "d.md")})
	r := byID(results)
	if r["svg"].Err != nil || r["svg"].Image != "d.md.assets/svg.svg" {
		t.Fatalf("svg = %+v", r["svg"])
	}
	b, _ := os.ReadFile(filepath.Join(dir, "d.md.assets", "svg.svg"))
	if !strings.Contains(string(b), "hi &lt;there&gt;") {
		t.Errorf("svg = %s", b)
	}
	if r["png"].Err != nil || r["png"].Image != "shots/p.png" {
		t.Fatalf("png = %+v", r["png"])
	}
	b, _ = os.ReadFile(filepath.Join(dir, "shots", "p.png"))
	if !bytes.HasPrefix(b, []byte("\x89PNG")) {
		t.Errorf("not a PNG")
	}
}

func TestImageRejectsText(t *testing.T) {
	_, results := runDoc(t, "```sh {#a out=image}\necho hi\n```\n", run.Options{})
	if results[0].Err == nil || results[0].Image != "" {
		t.Errorf("result = %+v, want an error", results[0])
	}
}

func TestConfirm(t *testing.T) {
	src := "```sh {#a}\necho a\n```\n```sh {#deploy confirm=\"Ship it?\"}\necho deploy\n```\n```sh {#after}\necho after\n```\n"
	d, err := doc.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	run1 := func(confirm func(*doc.Block) run.Answer) ([]*run.Result, error) {
		return run.Run(context.Background(), d, run.Options{Dir: t.TempDir(), Confirm: confirm})
	}

	results, err := run1(nil)
	if !errors.Is(err, run.ErrNoConfirm) {
		t.Fatalf("no confirm func: err = %v", err)
	}
	r := byID(results)
	if !r["a"].Ran || r["deploy"].Ran || !r["deploy"].Declined || r["after"].Ran {
		t.Errorf("no confirm func: a=%v deploy=%v/%v after=%v", r["a"].Ran, r["deploy"].Ran, r["deploy"].Declined, r["after"].Ran)
	}

	var asked string
	results, err = run1(func(b *doc.Block) run.Answer { asked = b.Attrs["confirm"]; return run.Stop })
	r = byID(results)
	if err != nil || asked != "Ship it?" || r["deploy"].Ran || !r["deploy"].Declined || r["after"].Ran {
		t.Errorf("declined: err=%v asked=%q deploy=%+v after=%v", err, asked, r["deploy"], r["after"].Ran)
	}

	results, err = run1(func(*doc.Block) run.Answer { return run.Go })
	r = byID(results)
	if err != nil || !r["deploy"].Ran || !r["after"].Ran {
		t.Errorf("accepted: err=%v deploy=%v after=%v", err, r["deploy"].Ran, r["after"].Ran)
	}

	results, err = run1(func(*doc.Block) run.Answer { return run.Skip })
	r = byID(results)
	if err != nil || r["deploy"].Ran || !r["deploy"].Skipped || r["deploy"].Declined || !r["after"].Ran {
		t.Errorf("skipped: err=%v deploy=%+v after=%v", err, r["deploy"], r["after"].Ran)
	}
}

func TestStep(t *testing.T) {
	body := "```sh {#a}\necho a\n```\n```sh {#b}\necho b\n```\n```md\nnot run\n```\n```sh {#c skip}\necho c\n```\n"
	for name, tc := range map[string]struct {
		src  string
		step bool
	}{
		"option":       {body, true},
		"front matter": {"---\nmdsh:\n  step: true\n---\n" + body, false},
	} {
		d, err := doc.Parse([]byte(tc.src))
		if err != nil {
			t.Fatal(err)
		}
		var asked []string
		results, err := run.Run(context.Background(), d, run.Options{Dir: t.TempDir(), Step: tc.step, Confirm: func(b *doc.Block) run.Answer {
			asked = append(asked, b.ID)
			if b.ID == "a" {
				return run.Skip
			}
			return run.Go
		}})
		r := byID(results)
		if err != nil || strings.Join(asked, ",") != "a,b" || r["a"].Ran || !r["b"].Ran {
			t.Errorf("%s: err=%v asked=%v a=%v b=%v", name, err, asked, r["a"].Ran, r["b"].Ran)
		}
	}
}

func TestShell(t *testing.T) {
	for src, want := range map[string]string{
		"```sh\necho\n```\n":                                       "",
		"```sh\necho\n```\n```zsh\necho\n```\n":                    "zsh",
		"---\nmdsh:\n  shell: /bin/bash\n---\n```zsh\necho\n```\n": "/bin/bash",
		"```zsh {<include file=x}\n```\n":                          "",
	} {
		d, err := doc.Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if got := run.Shell(d); got != want {
			t.Errorf("Shell(%q) = %q, want %q", src, got, want)
		}
	}
}

func TestZsh(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh not installed")
	}
	src := "```zsh {#a}\narr=(one two)\nexport X=\"a b\"\nf() { print -r -- \"f $1\" }\n```\n" +
		"```sh {#b}\nfor w in $X; do echo \"w=$w\"; done\nf ${arr[1]}\n```\n" +
		"```zsh {#bad}\necho $(( 1 +\n```\n" +
		"```zsh {#c env=sub}\necho \"sub $X\" ${+ZSH_VERSION}\n```\n"
	_, results := runDoc(t, src, run.Options{})
	r := byID(results)
	if got := string(r["b"].Stdout); got != "w=a b\nf one\n" {
		t.Errorf("b stdout = %q, stderr %q", got, r["b"].Stderr)
	}
	if r["bad"].Status == 0 || r["bad"].Exited {
		t.Errorf("bad: status %d exited %v", r["bad"].Status, r["bad"].Exited)
	}
	if got := string(r["c"].Stdout); got != "sub a b 1\n" {
		t.Errorf("c stdout = %q, stderr %q", got, r["c"].Stderr)
	}
}

func TestTTYWithoutTerminal(t *testing.T) {
	_, results := runDoc(t, "```sh {#a tty}\necho hi\n```\n```sh {#b}\necho next\n```\n", run.Options{})
	r := byID(results)
	if !errors.Is(r["a"].Err, run.ErrNoTTY) || !r["a"].Failed() {
		t.Errorf("a: err = %v, want ErrNoTTY", r["a"].Err)
	}
	if len(r["a"].Stdout) != 0 {
		t.Errorf("a ran: stdout = %q", r["a"].Stdout)
	}
	if got := string(r["b"].Stdout); got != "next\n" {
		t.Errorf("b stdout = %q", got)
	}
}

func TestTTYOutKind(t *testing.T) {
	d, err := doc.Parse([]byte("```sh {#a tty}\nx\n```\n\n```sh {#b tty out=recording:s/b.gif}\nx\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range d.Blocks {
		if k := run.OutKind(b); k != "recording" {
			t.Errorf("%s: OutKind = %q", b.ID, k)
		}
	}
}

func TestTapedBlock(t *testing.T) {
	src := "```sh {#ask tty}\nread -r name\necho \"hi $name\"\n```\n\n```tape {for=ask}\nSet TypingSpeed 1ms\nType \"bob\"\nEnter\n```\n\n```sh {#next}\necho \"$name\"\n```\n"
	dir := t.TempDir()
	_, results := runDoc(t, src, run.Options{Dir: dir, File: filepath.Join(dir, "d.md")})
	r := byID(results)
	ask := r["ask"]
	if ask.Err != nil || ask.Status != 0 {
		t.Fatalf("ask: status %d, err %v", ask.Status, ask.Err)
	}
	if ask.Image != "d.md.assets/ask.gif" {
		t.Errorf("ask image = %q", ask.Image)
	}
	cast, err := os.ReadFile(filepath.Join(dir, "d.md.assets", "ask.cast"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cast), "hi bob") || !strings.Contains(string(cast), `"width":80,"height":24`) {
		t.Errorf("cast = %s", cast)
	}
	if _, err := os.Stat(filepath.Join(dir, "d.md.assets", "ask.gif")); err != nil {
		t.Error(err)
	}
	if got := string(r["next"].Stdout); got != "bob\n" {
		t.Errorf("next stdout = %q", got)
	}
}

func TestTapedBlockFromFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in.tape"), []byte("Set TypingSpeed 1ms\nType \"x\"\nEnter\nWait@100ms /never/\n"), 0o644)
	src := "```sh {#ask tty}\nread -r v\ncat\n```\n\n```tape {for=ask <include file=in.tape}\n```\n"
	_, results := runDoc(t, src, run.Options{Dir: dir, File: filepath.Join(dir, "d.md")})
	ask := byID(results)["ask"]
	if ask.Err == nil || !strings.Contains(ask.Err.Error(), "tape line 4: Wait /never/") {
		t.Errorf("err = %v, want the Wait to time out", ask.Err)
	}
	// What happened is still saved, to show where it got stuck.
	if ask.Image == "" {
		t.Error("no recording saved")
	}
}

func TestTapePlaceholders(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "in.tape"), []byte("Set TypingSpeed 1ms\nType \"{{#secret/env(PW)}}\"\nEnter\n"), 0o644)
	src := "```sh {#secret}\nPW='s3\"cr t'\n```\n\n" +
		"```sh {#a tty}\nread -r u; read -r p; read -r lit\n```\n\n```tape {for=a}\nSet TypingSpeed 1ms\nType \"{{$WHO}}\"\nEnter\nType \"{{#secret/env(PW)}}\"\nEnter\nType '{{$WHO}}'\nEnter\n```\n\n" +
		"```sh {#b tty}\nread -r p2\n```\n\n```tape {for=b <include file=in.tape}\n```\n\n" +
		"```sh {#check}\nprintf '%s|%s|%s|%s' \"$u\" \"$p\" \"$lit\" \"$p2\"\n```\n"
	_, results := runDoc(t, src, run.Options{Dir: dir, File: filepath.Join(dir, "d.md"), Environ: []string{"PATH=" + os.Getenv("PATH"), "WHO=ada"}})
	r := byID(results)
	for _, id := range []string{"a", "b"} {
		if r[id].Err != nil {
			t.Fatalf("%s: %v", id, r[id].Err)
		}
	}
	if got := string(r["check"].Stdout); got != `ada|s3"cr t|{{$WHO}}|s3"cr t` {
		t.Errorf("typed %q", got)
	}
}

func TestTakeTTY(t *testing.T) {
	src := "```sh {#ask tty}\nread -r name\necho \"hi $name\"\n```\n\n```tape {for=ask}\nSet TypingSpeed 1ms\nType \"bob\"\nEnter\n```\n\n```sh {#next}\necho next\n```\n"
	dir := t.TempDir()
	tty, err := os.Create(filepath.Join(dir, "tty"))
	if err != nil {
		t.Fatal(err)
	}
	defer tty.Close()
	var calls []string
	_, results := runDoc(t, src, run.Options{Dir: dir, File: filepath.Join(dir, "d.md"), TTY: tty,
		TakeTTY: func(b *doc.Block) func() {
			calls = append(calls, "take "+b.ID)
			return func() { calls = append(calls, "give back "+b.ID) }
		}})
	if r := byID(results)["ask"]; r.Err != nil {
		t.Fatal(r.Err)
	}
	if got := strings.Join(calls, ", "); got != "take ask, give back ask" {
		t.Errorf("calls: %s", got)
	}
	if shown, _ := os.ReadFile(tty.Name()); !strings.Contains(string(shown), "hi bob") {
		t.Errorf("the tape's run wasn't shown on the terminal: %q", shown)
	}
}
