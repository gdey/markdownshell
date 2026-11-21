//go:build !unix

package env

import (
	"context"
	"errors"
	"io"
)

// interactive is only on Unix.
type interactive struct{}

// Event is something an interactive shell did, for Shell.Watch.
type Event struct {
	Kind    string
	Command string
	Status  int
	State   *State
}

var (
	ErrBusy           = errors.New("the shell is running a command")
	ErrLineTyped      = errors.New("a line is being typed at the prompt")
	ErrNotInteractive = errors.New("shell isn't interactive")
)

func startInteractive(context.Context, *Shell, Config, string, func(*Shell)) error {
	return errors.New("interactive shells need Unix")
}

func (s *Shell) runInteractive(context.Context, string, io.Writer, io.Writer) (int, bool, error) {
	return 0, false, ErrNotInteractive
}

func (s *Shell) closeInteractive() {}

func (s *Shell) RunLine(context.Context, string, io.Writer) (int, bool, error) {
	return 0, false, ErrNotInteractive
}
func (s *Shell) Type([]byte) error    { return ErrNotInteractive }
func (s *Shell) View(io.Writer)       {}
func (s *Shell) Watch(func(Event))    {}
func (s *Shell) State() (State, bool) { return State{}, false }
func (s *Shell) Want([]string) error  { return ErrNotInteractive }
func (s *Shell) Jobs() []ShellJob     { return nil }

// CanInteractive is false: interactive shells need Unix.
func CanInteractive(string) bool { return false }
