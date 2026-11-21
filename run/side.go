package run

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoState is RunSide's error when the top-level shell isn't
// interactive, or hasn't said what state it is in yet.
var ErrNoState = errors.New("the shell hasn't reported its state, which a side shell starts from")

// RunSide runs block i in a side shell: a scripted shell of its own,
// started from the state the interactive top-level shell last reported,
// its exported variables and directory, so it can run while a command
// holds that shell. What the block changes stays in the side shell,
// which ends with it. Its dependencies aren't run, as they would run in
// the top-level shell. A background block, or a request, runs as
// RunBlock runs it.
func (s *Session) RunSide(ctx context.Context, i int) (*Result, error) {
	if i < 0 || i >= len(s.d.Blocks) {
		return nil, fmt.Errorf("no block %d", i)
	}
	if err := s.checkFrontMatter(); err != nil {
		return s.results[i], err
	}
	b := s.d.Blocks[i]
	if b.IsInclude() || b.IsHTTP() || b.Attrs.Has("background") || !Runnable(b) {
		return s.RunBlock(ctx, i)
	}
	top, err := s.stack.Get(ctx, "", s.d.Options.Env.Share)
	if err != nil {
		return s.results[i], err
	}
	st, ok := top.Shell.State()
	if !top.Shell.Interactive() || !ok {
		return s.results[i], ErrNoState
	}
	s.Reap()
	if b.Attrs.Has("confirm") {
		if s.opts.Confirm == nil {
			_, err := s.confirm(b)
			return s.results[i], err
		}
		if s.askConfirm(b) != Go {
			return s.results[i], nil
		}
	}
	side := s.stack.Side(st)
	defer side.Close()
	s.side = side
	defer func() { s.side = nil }()
	r := &Result{Block: b}
	s.results[i] = r
	s.run(ctx, b, r)
	return r, ctx.Err()
}
