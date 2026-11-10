package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/gdey/markdownshell/ctl"
	"github.com/gdey/markdownshell/run"
)

// askAsync asks q as the control socket would, and returns the answer
// once it comes.
func askAsync(m *Model, ctx context.Context, q run.Question) <-chan askReply {
	out := make(chan askReply, 1)
	go func() {
		a, by := m.ask(ctx, q)
		out <- askReply{a, by}
	}()
	return out
}

// TestAskInCard asks a block's mdsh ctl questions in its card: text,
// a secret, choices and confirm.
func TestAskInCard(t *testing.T) {
	m, _ := newModel(t, twoBlocks, Options{})
	two := m.d.Blocks[1]
	for _, tc := range []struct {
		q     run.Question
		keys  []string
		shown string
		want  askReply
	}{
		{run.Question{ID: "q1", Kind: run.AskCtl, Block: two, Text: "Your name?"}, []string{"j", "o", "enter"}, "enter answer", askReply{"jo", run.ByUser}},
		{run.Question{ID: "q2", Kind: run.AskCtl, Block: two, Text: "Password?", Secret: true}, []string{"p", "w", "enter"}, "hidden as you type", askReply{"pw", run.ByUser}},
		{run.Question{ID: "q3", Kind: run.AskCtl, Block: two, Text: "Env?", Choices: []string{"dev", "prod"}, Default: "prod", HasDefault: true}, []string{"enter"}, "▸ 2 prod", askReply{"prod", run.ByUser}},
		{run.Question{ID: "q4", Kind: run.AskCtl, Block: two, Text: "Env?", Choices: []string{"dev", "prod"}}, []string{"right", "right", "enter"}, "▸ 1 dev", askReply{"dev", run.ByUser}},
		{run.Question{ID: "q5", Kind: run.AskCtl, Block: two, Text: "Skip me?"}, []string{"esc"}, "esc no answer", askReply{"", run.ByNone}},
		{run.Question{ID: "q6", Kind: run.AskCtlConfirm, Block: two, Text: "Deploy?"}, []string{"enter", "x", "n"}, "y yes · n no", askReply{ctl.NoValue, run.ByUser}},
		{run.Question{ID: "q7", Kind: run.AskCtlConfirm, Block: two, Text: "Tag?", Default: "yes", HasDefault: true}, []string{"enter"}, "enter yes", askReply{ctl.Yes, run.ByUser}},
	} {
		got := askAsync(m, context.Background(), tc.q)
		pump(t, m)
		if m.asking() == nil {
			t.Fatalf("%s: not asked", tc.q.ID)
		}
		if v := m.View(); !strings.Contains(v, tc.q.Text) || !strings.Contains(v, tc.shown) {
			t.Errorf("%s: want %q in\n%s", tc.q.ID, tc.shown, v)
		}
		last := len(tc.keys) - 1
		press(m, tc.keys[:last]...)
		if v := m.View(); tc.q.Secret && (strings.Contains(v, "pw") || !strings.Contains(v, "> ••")) {
			t.Errorf("%s: secret shown:\n%s", tc.q.ID, v)
		}
		press(m, tc.keys[last])
		if r := <-got; r != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.q.ID, r, tc.want)
		}
		if m.asking() != nil {
			t.Errorf("%s: still asking", tc.q.ID)
		}
	}
}

// TestAskWithdrawn checks that a question goes away when its block
// ends, and that --yes answers confirm without asking.
func TestAskWithdrawn(t *testing.T) {
	m, _ := newModel(t, twoBlocks, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	got := askAsync(m, ctx, run.Question{ID: "q1", Kind: run.AskCtl, Block: m.d.Blocks[0], Text: "Wait?"})
	pump(t, m)
	cancel()
	pump(t, m)
	if r := <-got; r.by != run.ByNone || m.asking() != nil {
		t.Errorf("withdrawn: %+v, asking %v", r, m.asking())
	}

	m, _ = newModel(t, twoBlocks, Options{Yes: true})
	if a, by := m.ask(ctx, run.Question{Kind: run.AskCtlConfirm, Text: "Go?"}); a != ctl.Yes || by != run.ByYes {
		t.Errorf("--yes: %q by %s", a, by)
	}
	if a, by := m.ask(ctx, run.Question{Kind: run.AskCtlConfirm, Text: "Go?", Default: "no", HasDefault: true}); a != ctl.NoValue || by != run.ByDefault {
		t.Errorf("--yes with a default: %q by %s", a, by)
	}
}
