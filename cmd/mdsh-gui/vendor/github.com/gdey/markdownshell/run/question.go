package run

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/gdey/markdownshell/audit"
	"github.com/gdey/markdownshell/ctl"
	"github.com/gdey/markdownshell/doc"
)

// QuestionKind says what a pending question is about.
type QuestionKind string

const (
	// AskConfirm is a block's confirm attribute, or step mode: run,
	// skip or stop. Options.Confirm answers it.
	AskConfirm QuestionKind = "confirm"
	// AskDepend is whether to run a dependency first. Options.Depend
	// answers it.
	AskDepend QuestionKind = "depends"
	// AskDependOff is whether to run a dependency that is switched off
	// where it stands. Options.DependOff answers it.
	AskDependOff QuestionKind = "depends-off"
	// AskCtl is mdsh ctl ask: text, or one of Choices. Options.Ask
	// answers it.
	AskCtl QuestionKind = "ask"
	// AskCtlConfirm is mdsh ctl confirm: yes or no. Options.Ask answers
	// it with ctl.Yes or ctl.NoValue.
	AskCtlConfirm QuestionKind = "ctl.confirm"
)

// Question is a question the session is waiting on someone to answer.
// Every question gets one, whether a confirm block, a depends= chain or
// a block's mdsh ctl call asked it, so there is one list of what is
// waiting, each with an id.
type Question struct {
	// ID names the question for as long as it is pending, and in the
	// audit log: q1, q2 and so on, in the order they were asked.
	ID   string
	Kind QuestionKind
	// Block is the block the question is about: the one to confirm,
	// the dependency, or the block that called mdsh ctl.
	Block *doc.Block
	// Of, for AskDepend and AskDependOff, is the block that depends on
	// Block.
	Of *doc.Block
	// Why, for AskDependOff, is why Block is switched off: see
	// Options.DependOff.
	Why string
	// Text is the question from mdsh ctl, as the block wrote it. It
	// comes from the block, so show it as text a block wrote.
	Text string
	// Choices, if not empty, are the only answers AskCtl takes.
	Choices []string
	// Default is the answer used when nobody can answer, if HasDefault.
	Default    string
	HasDefault bool
	// Secret hides what is typed, and the answer in the log.
	Secret bool
}

// Answered says who answered a question from mdsh ctl, as the audit
// log's ask event records it.
type Answered string

const (
	// ByUser is someone answering.
	ByUser Answered = "user"
	// ByDefault is the call's --default, used because nobody could
	// answer.
	ByDefault Answered = "default"
	// ByYes is --yes answering yes to a confirm with no default.
	ByYes Answered = "yes-flag"
	// ByNone is nobody: there was nobody to ask, or the block ended
	// while it waited. With no default the call exits ctl.NoAnswer.
	ByNone Answered = "none"
)

// questions are the session's pending questions.
type questions struct {
	mu      sync.Mutex
	last    int
	pending []*Question
}

// pose adds q to the pending questions with a new id, and returns a
// function that takes it off once it is answered.
func (s *Session) pose(q *Question) (answered func()) {
	s.qs.mu.Lock()
	s.qs.last++
	q.ID = "q" + strconv.Itoa(s.qs.last)
	s.qs.pending = append(s.qs.pending, q)
	s.qs.mu.Unlock()
	return func() {
		s.qs.mu.Lock()
		s.qs.pending = slices.DeleteFunc(s.qs.pending, func(p *Question) bool { return p == q })
		s.qs.mu.Unlock()
	}
}

// Pending is the questions the session is waiting on, oldest first.
// There can be more than one: a background block can ask while the run
// asks about the next block.
func (s *Session) Pending() []Question {
	if s == nil {
		return nil
	}
	s.qs.mu.Lock()
	defer s.qs.mu.Unlock()
	out := make([]Question, len(s.qs.pending))
	for i, q := range s.qs.pending {
		out[i] = *q
	}
	return out
}

// askConfirm asks Options.Confirm about b, as a pending question.
func (s *Session) askConfirm(b *doc.Block) Answer {
	defer s.pose(&Question{Kind: AskConfirm, Block: b, Text: b.Attrs["confirm"]})()
	return s.opts.Confirm(b)
}

// askDepend asks Options.Depend about dep, which b depends on, as a
// pending question.
func (s *Session) askDepend(dep, b *doc.Block) Answer {
	defer s.pose(&Question{Kind: AskDepend, Block: dep, Of: b})()
	return s.opts.Depend(dep, b)
}

// askDependOff asks Options.DependOff about dep, which b depends on, as
// a pending question.
func (s *Session) askDependOff(dep, b *doc.Block, why string) Answer {
	defer s.pose(&Question{Kind: AskDependOff, Block: dep, Of: b, Why: why})()
	return s.opts.DependOff(dep, b, why)
}

// ctlAsk answers mdsh ctl ask and confirm from block b, whose run ctx
// ends when the block does.
func (s *Session) ctlAsk(ctx context.Context, b *doc.Block, cmd string, raw json.RawMessage) ctl.Response {
	var a ctl.AskArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return ctl.Refuse("%s: %v", cmd, err)
	}
	if strings.TrimSpace(a.Question) == "" {
		return ctl.Refuse("%s: the question is empty", cmd)
	}
	q := &Question{Kind: AskCtl, Block: b, Text: a.Question, Choices: a.Choices, Secret: a.Secret}
	if a.Default != nil {
		q.Default, q.HasDefault = *a.Default, true
	}
	if cmd == "confirm" {
		q.Kind = AskCtlConfirm
		if len(a.Choices) > 0 || a.Secret {
			return ctl.Refuse("confirm: --choice and --secret are for ask")
		}
		if q.HasDefault && q.Default != ctl.Yes && q.Default != ctl.NoValue {
			return ctl.Refuse("confirm: --default is yes or no, not %q", q.Default)
		}
	}
	for _, c := range a.Choices {
		if c == "" {
			return ctl.Refuse("ask: --choice has an empty answer")
		}
	}
	if q.HasDefault && len(q.Choices) > 0 && !slices.Contains(q.Choices, q.Default) {
		return ctl.Refuse("ask: --default %q is not one of the choices", q.Default)
	}

	answer, by := "", ByNone
	if s.opts.Ask != nil && s.opts.Mode != ModeReport {
		answered := s.pose(q)
		answer, by = s.opts.Ask(ctx, *q)
		answered()
		if by == ByUser && !q.takes(answer) {
			by = ByNone
		}
	}
	if by == ByNone && q.HasDefault {
		answer, by = q.Default, ByDefault
	}
	s.logAsk(q, answer, by)
	switch {
	case by == ByNone:
		s.emit(Event{Kind: Unanswered, Block: b, Question: q})
		return ctl.Response{Error: ctl.NoAnswerError(q.Text), Code: ctl.NoAnswer}
	case q.Kind == AskCtlConfirm && answer == ctl.NoValue:
		return ctl.Response{Value: ctl.NoValue, Code: ctl.No}
	}
	return ctl.Done(answer)
}

// takes reports whether answer is one q accepts.
func (q *Question) takes(answer string) bool {
	switch {
	case q.Kind == AskCtlConfirm:
		return answer == ctl.Yes || answer == ctl.NoValue
	case len(q.Choices) > 0:
		return slices.Contains(q.Choices, answer)
	}
	return true
}

// logAsk logs the ask event for q, answered with answer by by.
func (s *Session) logAsk(q *Question, answer string, by Answered) {
	l := s.opts.Audit
	if l == nil {
		return
	}
	f := audit.Fields{"id": q.ID, "question": q.Text, "by": string(by), "answer": nil}
	if q.Kind == AskCtlConfirm {
		f["kind"] = "confirm"
	} else {
		f["kind"] = "ask"
	}
	if q.Block != nil {
		f["block"], f["env"] = q.Block.ID, blockEnv(q.Block)
	}
	if by != ByNone {
		if q.Secret {
			f["answer"] = l.Secret(answer)
		} else {
			f["answer"] = answer
		}
	}
	l.Event("ask", f)
}
