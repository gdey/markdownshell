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
		{"all", map[string]run.Answer{"build": run.Go, "fetch": run.Go, "login": run.Go}, []string{"build<deploy", "fetch<build", "login<deploy"}, []string{"fetch", "build", "login", "deploy"}},
		{"skip", map[string]run.Answer{"build": run.Skip, "login": run.Go}, []string{"build<deploy", "login<deploy"}, []string{"login", "deploy"}},
		{"stop", map[string]run.Answer{"build": run.Go, "fetch": run.Go, "login": run.Stop}, []string{"build<deploy", "fetch<build", "login<deploy"}, []string{"fetch", "build"}},
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
