package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gdey/markdownshell/ctl"
	"github.com/gdey/markdownshell/run"
)

const ctlUsage = `usage: mdsh ctl COMMAND [flags] ARGS
commands, for blocks that mdsh runs:
  set [--secret] NAME VALUE   set $MDSH_VAR_NAME for later blocks (- reads VALUE from stdin)
  unset NAME                  remove a variable that set made
  get NAME                    print $MDSH_VAR_NAME as it is now
  status [--step N --of M] TEXT
                              show progress for this block
exit codes: 0 done, 2 refused, 3 not running under mdsh`

// ctlCmd is mdsh ctl: a block talking back to the run that started it,
// over the socket in $MDSH_SOCK. It returns the exit code.
func ctlCmd(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return ctlCall(args, os.Getenv(ctl.SockVar), os.Getenv(ctl.TokenVar), stdin, stdout, stderr)
}

func ctlCall(args []string, sock, token string, stdin io.Reader, stdout, stderr io.Writer) int {
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(stderr, "mdsh ctl: "+format+"\n", a...)
		return code
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stderr, ctlUsage)
		if len(args) == 0 {
			return ctl.Refused
		}
		return ctl.OK
	}
	cmd := args[0]
	fs := flag.NewFlagSet("mdsh ctl "+cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var call any
	var nargs int
	switch cmd {
	case "set":
		fs.Bool("secret", false, "")
		call, nargs = &ctl.SetArgs{}, 2
	case "unset", "get":
		call, nargs = &ctl.NameArgs{}, 1
	case "status":
		a := &ctl.StatusArgs{}
		fs.IntVar(&a.Step, "step", 0, "")
		fs.IntVar(&a.Of, "of", 0, "")
		call, nargs = a, -1
	default:
		return fail(ctl.Refused, "unknown command %q\n%s", cmd, ctlUsage)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return fail(ctl.Refused, "%s: %v", cmd, err)
	}
	rest := fs.Args()
	if nargs > 0 && len(rest) != nargs || nargs < 0 && len(rest) == 0 {
		return fail(ctl.Refused, "%s: wrong number of arguments\n%s", cmd, ctlUsage)
	}
	switch a := call.(type) {
	case *ctl.SetArgs:
		a.Name, a.Value = rest[0], rest[1]
		if a.Value == "-" {
			data, err := io.ReadAll(stdin)
			if err != nil {
				return fail(ctl.Refused, "set: reading stdin: %v", err)
			}
			a.Value = strings.TrimSuffix(string(data), "\n")
		}
		a.Secret = fs.Lookup("secret").Value.String() == "true"
	case *ctl.NameArgs:
		a.Name = rest[0]
	case *ctl.StatusArgs:
		a.Text = strings.Join(rest, " ")
		if (a.Step == 0) != (a.Of == 0) {
			return fail(ctl.Refused, "status: --step and --of go together")
		}
	}
	resp, err := ctl.Call(sock, token, cmd, call)
	switch {
	case errors.Is(err, ctl.ErrNotRunning) && cmd == "status":
		// A script that reports progress still runs on its own.
		return ctl.OK
	case errors.Is(err, ctl.ErrNotRunning):
		return fail(ctl.NotRunning, "not running under mdsh ($%s is unset or nothing is listening on it)", ctl.SockVar)
	case err != nil:
		return fail(ctl.Refused, "%s: %v", cmd, err)
	case !resp.OK:
		return fail(resp.Code, "%s", resp.Error)
	}
	if cmd == "get" {
		fmt.Fprintln(stdout, resp.Value)
	}
	return ctl.OK
}

// progressLine is how the CLI shows a block's status on stderr.
func progressLine(label string, p *run.Progress) string {
	if p.Of > 0 {
		return fmt.Sprintf("%s: [%d/%d] %s", label, p.Step, p.Of, p.Text)
	}
	return fmt.Sprintf("%s: %s", label, p.Text)
}
