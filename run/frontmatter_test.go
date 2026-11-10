package run_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gdey/markdownshell/run"
)

// A document whose front matter doesn't parse opens, but runs nothing
// until it is fixed; then the shells start again with its settings.
func TestSessionBadFrontMatter(t *testing.T) {
	d := parse(t, "---\ntitle: Setup: first steps\n---\n\n```sh {#a}\necho hi\n```\n")
	if d.FrontMatterErr == nil {
		t.Fatal("front matter parsed")
	}
	s, err := run.NewSession(d, run.Options{Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	var fe *run.FrontMatterError
	if _, err := s.RunBlock(ctx, 0); !errors.As(err, &fe) || !errors.Is(err, d.FrontMatterErr) {
		t.Errorf("RunBlock: err %v, want a FrontMatterError", err)
	}
	if err := s.RunFrom(ctx, 0); !errors.As(err, &fe) {
		t.Errorf("RunFrom: err %v, want a FrontMatterError", err)
	}
	if _, _, err := s.RunTyped(ctx, 0); !errors.As(err, &fe) {
		t.Errorf("RunTyped: err %v, want a FrontMatterError", err)
	}
	if _, err := s.Ask(ctx, "echo hi"); !errors.As(err, &fe) {
		t.Errorf("Ask: err %v, want a FrontMatterError", err)
	}
	if r := s.Results()[0]; r.Ran {
		t.Error("the block ran")
	}
	if _, err := run.Run(ctx, d, run.Options{Environ: []string{}}); !errors.As(err, &fe) {
		t.Errorf("Run: err %v, want a FrontMatterError", err)
	}

	// An edit elsewhere keeps it broken: no restart.
	if err := d.SetSource([]byte("---\ntitle: Setup: first steps\n---\n\nMore.\n\n```sh {#a}\necho hi\n```\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(); err != nil {
		t.Errorf("Sync, still broken: %v", err)
	}
	fixes := d.Fixes()
	if len(fixes) == 0 {
		t.Fatal("no fix offered")
	}
	if err := d.SetSource(fixes[0].Src); err != nil {
		t.Fatal(err)
	}
	if err := s.Sync(); !errors.Is(err, run.ErrFrontMatterFixed) || !errors.Is(err, run.ErrRestart) {
		t.Errorf("Sync, fixed: err %v, want ErrFrontMatterFixed", err)
	}
	s2, err := run.NewSession(d, run.Options{Environ: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	r, err := s2.RunBlock(ctx, 0)
	if err != nil || string(r.Stdout) != "hi\n" {
		t.Errorf("fixed: %q, %v", r.Stdout, err)
	}
}
