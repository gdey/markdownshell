package env

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// State is what an interactive shell reports about itself at each
// prompt, through the state fifo: the last command's status, where it
// is, its jobs and its variables.
type State struct {
	Status int
	PWD    string
	OldPWD string
	PID    int
	// Jobs is the shell's job table, as jobs -l lists it. A shell that
	// can't list its jobs at the prompt, such as dash, whose prompt
	// hook runs in a subshell, leaves it nil and Shell.Jobs asks ps.
	Jobs []ShellJob
	// JobsKnown says the shell listed its jobs, even if none.
	JobsKnown bool
	// Exported is the shell's exported environment.
	Exported map[string]string
	// Vars are the variables named in the want file, exported or not.
	// A variable that isn't set is left out.
	Vars map[string]string
}

// ShellJob is one entry of an interactive shell's job table.
type ShellJob struct {
	// N is the job number, as in %1. Zero for a job found through ps,
	// which has no number.
	N int
	// PGID is the job's process group: the PID jobs -l shows for it.
	PGID int
	// State is "running", "stopped" or "done", or what the shell said
	// if it is none of those.
	State string
	// Command is the job's command line.
	Command string
}

// Stopped reports whether the job is stopped, as by ctrl+z.
func (j ShellJob) Stopped() bool { return j.State == "stopped" }

// record is one record of the state fifo: a state dump, or the command
// line a shell is about to run, which zsh's preexec sends.
type record struct {
	state *State
	cmd   *string
}

// readRecords reads records from r until it ends, sending each to out.
// A record is a kind line ("state" or "cmd"), "key value" lines, and
// "end". Values are escaped as unescape undoes.
func readRecords(r io.Reader, out func(record)) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var st *State
	var cmd *string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "state":
			st = &State{Status: -1, Exported: map[string]string{}, Vars: map[string]string{}}
			cmd = nil
			continue
		case line == "cmd":
			st = nil
			cmd = new(string)
			continue
		case line == "end":
			if st != nil {
				out(record{state: st})
			} else if cmd != nil {
				out(record{cmd: cmd})
			}
			st, cmd = nil, nil
			continue
		}
		k, v, _ := strings.Cut(line, " ")
		if cmd != nil {
			if k == "text" {
				*cmd = unescape(v)
			}
			continue
		}
		if st == nil {
			continue
		}
		switch k {
		case "status":
			if n, err := strconv.Atoi(v); err == nil {
				st.Status = n
			}
		case "pwd":
			st.PWD = unescape(v)
		case "oldpwd":
			st.OldPWD = unescape(v)
		case "pid":
			st.PID, _ = strconv.Atoi(v)
		case "jobs":
			st.JobsKnown = true
		case "job":
			if j, ok := parseJob(v); ok {
				st.Jobs = append(st.Jobs, j)
			}
		case "env":
			if name, val, ok := strings.Cut(v, "="); ok && name != "" {
				st.Exported[name] = unescape(val)
			}
		case "var":
			if name, val, ok := strings.Cut(v, "="); ok && name != "" {
				st.Vars[name] = unescape(val)
			}
		}
	}
}

// jobLine matches a line of jobs -l: bash "[1]+  807 Stopped    sleep
// 100", dash "[1] + 756 Stopped (SIGTSTP)   sleep 100", zsh "[1]  + 807
// suspended  sleep 100". Lines that continue a pipeline, with no job
// number, don't match.
var jobLine = regexp.MustCompile(`^\[(\d+)\]\s*[+-]?\s+(\d+)\s+(.*)$`)

// jobStates are the words shells start a job's state with, longest
// first where one is a prefix of another, and what they mean.
var jobStates = []struct{ word, state string }{
	{"Stopped", "stopped"},
	{"suspended", "stopped"},
	{"Suspended", "stopped"}, // bash on macOS: "Suspended: 18"
	{"Running", "running"},
	{"running", "running"},
	{"Done", "done"},
	{"done", "done"},
	{"Exit", "done"},
	{"exit", "done"},
	{"Killed", "done"},
	{"killed", "done"},
	{"Terminated", "done"},
	{"terminated", "done"},
	{"Hangup", "done"},
	{"hangup", "done"},
	{"Interrupt", "done"},
	{"interrupt", "done"},
}

func parseJob(line string) (ShellJob, bool) {
	m := jobLine.FindStringSubmatch(line)
	if m == nil {
		return ShellJob{}, false
	}
	n, _ := strconv.Atoi(m[1])
	pgid, _ := strconv.Atoi(m[2])
	rest := m[3]
	j := ShellJob{N: n, PGID: pgid}
	for _, s := range jobStates {
		if strings.HasPrefix(rest, s.word) {
			j.State = s.state
			rest = rest[len(s.word):]
			// The state may go on: "Stopped (SIGTSTP)", "Exit 1",
			// "suspended (tty output)". The command follows a run of
			// spaces.
			if i := strings.Index(rest, "  "); i >= 0 {
				rest = rest[i:]
			} else if strings.HasPrefix(rest, " (") {
				if k := strings.Index(rest, ")"); k >= 0 {
					rest = rest[k+1:]
				}
			}
			j.Command = strings.TrimSpace(rest)
			return j, true
		}
	}
	// Unknown state: the first word is the state.
	f := strings.SplitN(strings.TrimSpace(rest), " ", 2)
	j.State = strings.ToLower(strings.TrimRight(f[0], ":"))
	if len(f) == 2 {
		j.Command = strings.TrimSpace(f[1])
	}
	return j, true
}
