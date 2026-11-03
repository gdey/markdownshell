package run_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// dependsDoc has deploy depend on build and login, and build on fetch.
const dependsDoc = "```sh {#fetch}\necho fetch\n```\n\n```sh {#build depends=fetch}\necho build\n```\n\n```sh {#login skip}\necho login\n```\n\n```sh {#deploy depends=build,login}\necho deploy\n```\n"

// asker answers Depend from answers by the dependency's id, and notes
// each question as dep<of.
type asker struct {
	answers map[string]run.Answer
	asked   []string
}

func (a *asker) depend(dep, b *doc.Block) run.Answer {
	a.asked = append(a.asked, dep.ID+"<"+b.ID)
	return a.answers[dep.ID]
}

func ran(s *run.Session) []string {
	var ids []string
	for _, r := range s.Results() {
		if r.Ran {
			ids = append(ids, r.Block.ID)
		}
	}
	return ids
}

func TestRunBlockDepends(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		answers map[string]run.Answer
		asked   []string
		ran     []string
	}{
		{"all", map[string]run.Answer{"build": run.Go, "fetch": run.Go, "login": run.Go}, []string{"fetch<build", "build<deploy", "login<deploy"}, []string{"fetch", "build", "login", "deploy"}},
		{"skip", map[string]run.Answer{"fetch": run.Go, "build": run.Skip, "login": run.Go}, []string{"fetch<build", "build<deploy", "login<deploy"}, []string{"fetch", "login", "deploy"}},
		{"skip top", map[string]run.Answer{"fetch": run.Skip, "build": run.Go, "login": run.Go}, []string{"fetch<build", "build<deploy", "login<deploy"}, []string{"build", "login", "deploy"}},
		{"stop", map[string]run.Answer{"build": run.Go, "fetch": run.Go, "login": run.Stop}, []string{"fetch<build", "build<deploy", "login<deploy"}, []string{"fetch", "build"}},
		{"stop top", map[string]run.Answer{"fetch": run.Stop}, []string{"fetch<build"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &asker{answers: tc.answers}
			s, err := run.NewSession(parse(t, dependsDoc), run.Options{Environ: []string{}, Depend: a.depend})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if _, err := s.RunBlock(ctx, 3); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(a.asked, tc.asked) {
				t.Errorf("asked %q, want %q", a.asked, tc.asked)
			}
			if got := ran(s); !slices.Equal(got, tc.ran) {
				t.Errorf("ran %q, want %q", got, tc.ran)
			}
		})
	}
}

// The chain is worked out to the top first, then each block is asked
// about right before it runs, from the top down: a block answered Go
// runs before the next question, and the block asked for is confirmed
// last, once all it depends on has run. A block two others need is
// asked about once.
func TestRunBlockDependsOrder(t *testing.T) {
	d := parse(t, "```sh {#a}\necho a\n```\n\n```sh {#b depends=a}\necho b\n```\n\n```sh {#c depends=a}\necho c\n```\n\n```sh {#d depends=b,c confirm}\necho d\n```\n")
	var log []string
	s, err := run.NewSession(d, run.Options{
		Environ: []string{},
		Depend: func(dep, b *doc.Block) run.Answer {
			log = append(log, "ask "+dep.ID+"<"+b.ID)
			return run.Go
		},
		Confirm: func(b *doc.Block) run.Answer {
			log = append(log, "confirm "+b.ID)
			return run.Go
		},
		OnEvent: func(e run.Event) {
			if e.Kind == run.Started {
				log = append(log, "run "+e.Block.ID)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RunBlock(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	want := []string{"ask a<b", "run a", "ask b<d", "run b", "ask c<d", "run c", "confirm d", "run d"}
	if !slices.Equal(log, want) {
		t.Errorf("got %q\nwant %q", log, want)
	}

	// In a whole run too, a confirm block's dependencies run before
	// it is confirmed.
	log = nil
	d = parse(t, "```sh {#b depends=a confirm}\necho b\n```\n\n```sh {#a}\necho a\n```\n")
	if _, err := run.Run(context.Background(), d, run.Options{
		Environ: []string{},
		Depend: func(dep, b *doc.Block) run.Answer {
			log = append(log, "ask "+dep.ID+"<"+b.ID)
			return run.Go
		},
		Confirm: func(b *doc.Block) run.Answer {
			log = append(log, "confirm "+b.ID)
			return run.Go
		},
		OnEvent: func(e run.Event) {
			if e.Kind == run.Started {
				log = append(log, "run "+e.Block.ID)
			}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"ask a<b", "run a", "confirm b", "run b"}; !slices.Equal(log, want) {
		t.Errorf("got %q\nwant %q", log, want)
	}
}

func TestRunBlockDependsRanAlready(t *testing.T) {
	ctx := context.Background()
	a := &asker{answers: map[string]run.Answer{}}
	s, err := run.NewSession(parse(t, dependsDoc), run.Options{Environ: []string{}, Depend: a.depend})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, i := range []int{0, 1, 2} {
		if _, err := s.RunBlock(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	if r, err := s.RunBlock(ctx, 3); err != nil || !r.Ran {
		t.Fatalf("deploy: ran=%v err=%v", r.Ran, err)
	}
	if len(a.asked) != 0 {
		t.Errorf("asked %q about blocks that ran", a.asked)
	}
}

func TestRunBlockDependsFailed(t *testing.T) {
	ctx := context.Background()
	d := parse(t, "```sh {#a}\nfalse\n```\n\n```sh {#b depends=a}\necho b\n```\n")
	a := &asker{answers: map[string]run.Answer{"a": run.Go}}
	s, err := run.NewSession(d, run.Options{Environ: []string{}, Depend: a.depend})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// a fails when run by hand, so b asks to run it again.
	s.RunBlock(ctx, 0)
	r, err := s.RunBlock(ctx, 1)
	if !errors.Is(err, run.ErrDependency) || r.Ran {
		t.Errorf("b: ran=%v err=%v, want ErrDependency", r.Ran, err)
	}
	if !slices.Equal(a.asked, []string{"a<b"}) {
		t.Errorf("asked %q", a.asked)
	}
}

func TestRunBlockDependsNoAsker(t *testing.T) {
	s, err := run.NewSession(parse(t, dependsDoc), run.Options{Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RunBlock(context.Background(), 1); !errors.Is(err, run.ErrNoConfirm) {
		t.Errorf("got %v, want ErrNoConfirm", err)
	}
	if got := ran(s); len(got) != 0 {
		t.Errorf("ran %q", got)
	}
}

func TestRunDepends(t *testing.T) {
	// deploy depends on a later block, and on a skip block: both are
	// asked about, and late runs only once.
	d := parse(t, "```sh {#early skip}\necho early\n```\n\n```sh {#deploy depends=early,late}\necho deploy\n```\n\n```sh {#late}\necho late\n```\n\n```sh {#after}\necho \"$L\"\n```\n")
	a := &asker{answers: map[string]run.Answer{"early": run.Go, "late": run.Go}}
	var started []string
	results, err := run.Run(context.Background(), d, run.Options{
		Environ: []string{},
		Depend:  a.depend,
		OnEvent: func(e run.Event) {
			if e.Kind == run.Started {
				started = append(started, e.Block.ID)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"early", "late", "deploy", "after"}; !slices.Equal(started, want) {
		t.Errorf("started %q, want %q", started, want)
	}
	if !results[2].Ran || string(results[2].Stdout) != "late\n" {
		t.Errorf("late: %+v", results[2])
	}

	// A failed dependency stops the run if its on_error says so, as it
	// would in place, and the dependent block doesn't run.
	d = parse(t, "```sh {#b depends=a}\necho b\n```\n\n```sh {#a on_error=stop}\nfalse\n```\n\n```sh {#c}\necho c\n```\n")
	results, err = run.Run(context.Background(), d, run.Options{Environ: []string{}, Depend: func(_, _ *doc.Block) run.Answer { return run.Go }})
	if err != nil {
		t.Fatal(err)
	}
	if !results[1].Failed() || results[0].Ran || results[2].Ran {
		t.Errorf("a failed=%v, b ran=%v, c ran=%v; want a failed and nothing else run", results[1].Failed(), results[0].Ran, results[2].Ran)
	}
}

// A depends= naming no block is ignored: the block runs as if it
// weren't there.
func TestRunDependsUnknown(t *testing.T) {
	d := parse(t, "```sh {#a}\necho a\n```\n\n```sh {#b depends=\"nope, a\"}\necho b\n```\n")
	asked := 0
	s, err := run.NewSession(d, run.Options{Environ: []string{}, Depend: func(_, _ *doc.Block) run.Answer { asked++; return run.Go }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.RunBlock(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := ran(s); asked != 1 || string(r.Stdout) != "b\n" || !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("asked %d times, b's stdout %q, ran %q; want a asked about and run, then b", asked, r.Stdout, got)
	}
}

// An empty dependency, with no commands, only blank lines and #
// comments, runs without asking, even with no asker; its own
// dependencies are still asked about. An empty confirm block is still
// asked about.
func TestRunBlockDependsEmpty(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		doc   string
		ask   bool // whether there is an asker
		asked []string
		ran   []string
	}{
		{"blank", "```sh {#a}\n```\n\n```sh {#b depends=a}\necho b\n```\n", true, nil, []string{"a", "b"}},
		{"whitespace", "```sh {#a}\n  \n\t\n```\n\n```sh {#b depends=a}\necho b\n```\n", true, nil, []string{"a", "b"}},
		{"comments", "```sh {#a}\n#!/bin/sh\n# a group of steps\n  # indented\n```\n\n```sh {#b depends=a}\necho b\n```\n", true, nil, []string{"a", "b"}},
		{"no asker", "```sh {#a}\n# nothing\n```\n\n```sh {#b depends=a}\necho b\n```\n", false, nil, []string{"a", "b"}},
		{"group", "```sh {#x}\necho x\n```\n\n```sh {#a depends=x}\n# setup\n```\n\n```sh {#b depends=a}\necho b\n```\n", true, []string{"x<a"}, []string{"x", "a", "b"}},
		{"command", "```sh {#a}\n# say it\necho a # here\n```\n\n```sh {#b depends=a}\necho b\n```\n", true, []string{"a<b"}, []string{"a", "b"}},
		{"confirm", "```sh {#a confirm}\n```\n\n```sh {#b depends=a}\necho b\n```\n", true, []string{"a<b"}, []string{"a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := parse(t, tc.doc)
			a := &asker{answers: map[string]run.Answer{"a": run.Go, "x": run.Go}}
			opts := run.Options{Environ: []string{}}
			if tc.ask {
				opts.Depend = a.depend
			}
			s, err := run.NewSession(d, opts)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if _, err := s.RunBlock(ctx, len(d.Blocks)-1); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(a.asked, tc.asked) {
				t.Errorf("asked %q, want %q", a.asked, tc.asked)
			}
			if got := ran(s); !slices.Equal(got, tc.ran) {
				t.Errorf("ran %q, want %q", got, tc.ran)
			}
		})
	}
}

// In a whole run, an empty dependency later in the file runs first
// without asking, and only once.
func TestRunDependsEmpty(t *testing.T) {
	d := parse(t, "```sh {#b depends=a}\necho b\n```\n\n```sh {#a}\n# marker\n```\n")
	a := &asker{answers: map[string]run.Answer{}}
	var started []string
	_, err := run.Run(context.Background(), d, run.Options{
		Environ: []string{},
		Depend:  a.depend,
		OnEvent: func(e run.Event) {
			if e.Kind == run.Started {
				started = append(started, e.Block.ID)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.asked) != 0 {
		t.Errorf("asked %q", a.asked)
	}
	if want := []string{"a", "b"}; !slices.Equal(started, want) {
		t.Errorf("started %q, want %q", started, want)
	}
}

// TestRunBlockDependsAlways checks that an always dependency is asked
// about and run again each time, though once for each block run, even
// when two of that block's dependencies share it.
func TestRunBlockDependsAlways(t *testing.T) {
	ctx := context.Background()
	d := parse(t, "```sh {#login always}\necho login\n```\n\n```sh {#build depends=login}\necho build\n```\n\n```sh {#test depends=login}\necho test\n```\n\n```sh {#deploy depends=build,test}\necho deploy\n```\n")
	a := &asker{answers: map[string]run.Answer{"login": run.Go, "build": run.Go, "test": run.Go}}
	s, err := run.NewSession(d, run.Options{Environ: []string{}, Depend: a.depend})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.RunBlock(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if r, err := s.RunBlock(ctx, 1); err != nil || !r.Ran {
		t.Fatalf("build: ran=%v err=%v", r.Ran, err)
	}
	if want := []string{"login<build"}; !slices.Equal(a.asked, want) {
		t.Errorf("build asked %q, want %q", a.asked, want)
	}
	// From a new session: deploy asks about login once, for build.
	a.asked = nil
	s2, err := run.NewSession(d, run.Options{Environ: []string{}, Depend: a.depend})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if _, err := s2.RunBlock(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if want := []string{"login<build", "build<deploy", "test<deploy"}; !slices.Equal(a.asked, want) {
		t.Errorf("deploy asked %q, want %q", a.asked, want)
	}
}

// staleDoc is c on a, d on a, b and c, and e on a and d.
const staleDoc = "```sh {#a}\necho a\n```\n\n```sh {#b}\necho b\n```\n\n```sh {#c depends=a}\necho c\n```\n\n```sh {#d depends=a,b,c}\necho d\n```\n\n```sh {#e depends=a,d}\necho e\n```\n"

// TestRunBlockDependsStale checks that when a has run again since the
// rest ran, running e asks about d and c, which are stale, but not b.
func TestRunBlockDependsStale(t *testing.T) {
	ctx := context.Background()
	a := &asker{answers: map[string]run.Answer{"a": run.Go, "b": run.Go, "c": run.Go, "d": run.Go}}
	d := parse(t, staleDoc)
	s, err := run.NewSession(d, run.Options{Environ: []string{}, Depend: a.depend})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := range 5 {
		if _, err := s.RunBlock(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	if got := run.Stale(d, s.Results()); slices.Contains(got, true) {
		t.Fatalf("stale after running in order: %v", got)
	}
	if _, err := s.RunBlock(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if got, want := run.Stale(d, s.Results()), []bool{false, false, true, true, true}; !slices.Equal(got, want) {
		t.Errorf("stale after a ran again: %v, want %v", got, want)
	}
	a.asked = nil
	if r, err := s.RunBlock(ctx, 4); err != nil || !r.Ran {
		t.Fatalf("e: ran=%v err=%v", r.Ran, err)
	}
	if want := []string{"c<d", "d<e"}; !slices.Equal(a.asked, want) {
		t.Errorf("asked %q, want %q", a.asked, want)
	}
	if got := run.Stale(d, s.Results()); slices.Contains(got, true) {
		t.Errorf("stale after e: %v", got)
	}
}

// TestRunBlockDependsStaleOnTheWay checks that a dependency that was
// current when the chain was worked out, but is made stale by a block
// run on the way, is asked about after that block runs.
func TestRunBlockDependsStaleOnTheWay(t *testing.T) {
	ctx := context.Background()
	d := parse(t, "```sh {#z always}\necho z\n```\n\n```sh {#x depends=z}\necho x\n```\n\n```sh {#y depends=z}\necho y\n```\n\n```sh {#b depends=x,y}\necho b\n```\n")
	a := &asker{answers: map[string]run.Answer{"z": run.Go, "x": run.Go, "y": run.Go}}
	s, err := run.NewSession(d, run.Options{Environ: []string{}, Depend: a.depend})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, i := range []int{0, 2} {
		if _, err := s.RunBlock(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	a.asked = nil
	if r, err := s.RunBlock(ctx, 3); err != nil || !r.Ran {
		t.Fatalf("b: ran=%v err=%v", r.Ran, err)
	}
	if want := []string{"z<x", "x<b", "y<b"}; !slices.Equal(a.asked, want) {
		t.Errorf("asked %q, want %q", a.asked, want)
	}
	if got := run.Stale(d, s.Results()); slices.Contains(got, true) {
		t.Errorf("stale after b: %v", got)
	}
}
