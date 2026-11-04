package report

import (
	"path/filepath"
	"strings"

	"github.com/gdey/markdownshell/run"
)

// Command describes a report as an mdsh command line, so the GUI's
// Report dialog and the TUI's report form can show how to make the same
// report from a shell.
type Command struct {
	// File is the document, and Dest the report to write.
	File, Dest string
	Format     Format
	HideCode   bool
	HideOutput bool
	// Run says the report comes from running the blocks. Otherwise it is
	// of the file as saved, like mdsh -n.
	Run bool
	// Args, Only, Step and StopOnError are the run's options, shown when
	// Run is set.
	Args        []string
	Only        string
	Step        bool
	StopOnError bool
	// Home is the user's home folder, for ~/ paths. Empty names no path
	// that way.
	Home string
}

// String is the command, run in the document's folder: files there are
// named on their own, and other paths under Home as ~/….
func (c Command) String() string {
	dir := filepath.Dir(c.File)
	path := func(p string) string {
		if filepath.Dir(p) == dir {
			return run.JoinArgs([]string{filepath.Base(p)})
		}
		if c.Home != "" {
			if rest, ok := strings.CutPrefix(p, c.Home+string(filepath.Separator)); ok && run.JoinArgs([]string{rest}) == rest {
				return "~/" + filepath.ToSlash(rest)
			}
		}
		return run.JoinArgs([]string{p})
	}
	cmd := []string{"mdsh"}
	if !c.Run {
		cmd = append(cmd, "-n")
	} else {
		if c.Only != "" {
			cmd = append(cmd, "--only", run.JoinArgs([]string{c.Only}))
		}
		if c.Step {
			cmd = append(cmd, "--step")
		}
		if c.StopOnError {
			cmd = append(cmd, "--stop-on-error")
		}
	}
	if c.HideCode {
		cmd = append(cmd, "--hide-code")
	}
	if c.HideOutput {
		cmd = append(cmd, "--hide-output")
	}
	dest := path(c.Dest)
	if c.Dest == "" {
		dest = "FILE"
	}
	redirect := false
	switch c.Format {
	case PDF:
		cmd = append(cmd, "--pdf", dest)
	case HTML:
		cmd = append(cmd, "--html", dest)
	default:
		// -o saves the printed view only with -n; a run's printout goes
		// to stdout.
		if c.Run {
			redirect = true
		} else {
			cmd = append(cmd, "-o", dest)
		}
	}
	cmd = append(cmd, path(c.File))
	if c.Run && len(c.Args) > 0 {
		cmd = append(cmd, run.JoinArgs(c.Args))
	}
	if redirect {
		cmd = append(cmd, ">", dest)
	}
	return strings.Join(cmd, " ")
}
