package env

import (
	"os/exec"
	"slices"
	"strings"
)

// termJob is what a job started by BackgroundTerminal has besides a
// Job's: the process mdsh started for it, and ended, which closes once
// that process has ended and all it wrote to its terminal has been read.
type termJob struct {
	cmd   *exec.Cmd
	ended chan struct{}
}

// stateScript is the script that, run in a shell, writes to file a
// script that puts another shell of the same kind in the same state:
// the same variables, functions, aliases, options, positional
// parameters and working directory. Its set options, such as set -e,
// go to file opts, to take on last. It runs in a subshell, so it can't
// change the shell or end it, even under set -e.
//
// bash and zsh can list their variables and functions in a form they
// read back; their own special and read-only variables are left out,
// as are COLUMNS and LINES, which belong to the terminal. Other shells
// get what set lists, which holds the variables but no functions.
// Exported variables reach the new shell through its environment.
func stateScript(file, opts string) string {
	return `(
_mdsh_opts=$-
set +eu
{
if [ -n "${BASH_VERSION-}" ]; then
	compgen -v | while IFS= read -r _mdsh_v; do
		case $_mdsh_v in BASH*|COLUMNS|LINES|COMP_*|DIRSTACK|EUID|FUNCNAME|GROUPS|HISTCMD|LINENO|PIPESTATUS|PPID|RANDOM|SECONDS|SRANDOM|EPOCHREALTIME|EPOCHSECONDS|SHELLOPTS|SHLVL|UID|OPTIND|OPTARG|_|_mdsh_*) continue;; esac
		_mdsh_d=$(declare -p "$_mdsh_v" 2>/dev/null) || continue
		_mdsh_a=${_mdsh_d#declare -}
		case ${_mdsh_a%% *} in *r*) continue;; esac
		printf '%s\n' "$_mdsh_d"
	done
	declare -f
	alias -p
	shopt -p 2>/dev/null
elif [ -n "${ZSH_VERSION-}" ]; then
	eval '
	for _mdsh_v in ${(k)parameters}; do
		case ${parameters[$_mdsh_v]} in *special*|*readonly*|*local*) continue;; esac
		case $_mdsh_v in COLUMNS|LINES|_mdsh_*|_) continue;; esac
		typeset -p -- $_mdsh_v
	done
	typeset -f
	alias -L
	for _mdsh_v in $(setopt); do
		case $_mdsh_v in interactive|monitor|shinstdin|zle|login|privileged|restricted|singlecommand) continue;; esac
		printf "setopt %s\n" $_mdsh_v
	done'
else
	set | grep -v -e '^_mdsh_' -e '^PPID=' -e '^COLUMNS=' -e '^LINES=' -e '^OPTIND='
	alias | sed 's/^/alias /'
fi
printf 'cd -- %s\n' "'$(pwd | sed "s/'/'\\\\''/g")'"
printf 'set --'
for _mdsh_v in "$@"; do printf " '%s'" "$(printf '%s' "$_mdsh_v" | sed "s/'/'\\\\''/g")"; done
echo
} >` + quote(file) + ` 2>/dev/null
{
case $_mdsh_opts in *a*) echo 'set -a';; esac
case $_mdsh_opts in *f*) echo 'set -f';; esac
case $_mdsh_opts in *C*) echo 'set -C';; esac
case $_mdsh_opts in *x*) echo 'set -x';; esac
case $_mdsh_opts in *u*) echo 'set -u';; esac
case $_mdsh_opts in *e*) echo 'set -e';; esac
} >` + quote(opts) + `
)
`
}

// runnerScript is the script the shell of a BackgroundTerminal job
// runs: it takes on the state in file state, which it then removes, as
// it may hold secrets, and the set options opts, as stateScript wrote
// them, and runs the block in file block. TERM says the terminal is an
// xterm, which is what mdsh draws, whatever terminal, if any, mdsh
// itself runs on.
func runnerScript(state string, opts []byte, block string) string {
	var set strings.Builder
	for _, line := range strings.Split(string(opts), "\n") {
		// Only what stateScript writes.
		if len(line) == 6 && strings.HasPrefix(line, "set -") && strings.ContainsRune("afCxue", rune(line[5])) {
			set.WriteString(line + "\n")
		}
	}
	return `. ` + quote(state) + ` 2>/dev/null
rm -f -- ` + quote(state) + `
TERM=xterm-256color; export TERM
` + set.String() + `. ` + quote(block) + `
`
}

func (s *Shell) forgetTermJob(j *Job) {
	s.jobMu.Lock()
	delete(s.termJobs, j.PID)
	s.jobMu.Unlock()
}

// killTermJobs kills the shell's BackgroundTerminal jobs, which, in
// sessions of their own, don't end with it.
func (s *Shell) killTermJobs() {
	s.jobMu.Lock()
	jobs := make([]*Job, 0, len(s.termJobs))
	for _, j := range s.termJobs {
		jobs = append(jobs, j)
	}
	s.jobMu.Unlock()
	for _, j := range jobs {
		j.Kill()
	}
}

// termJobPIDs are the process IDs of the shell's BackgroundTerminal jobs
// still running.
func (s *Shell) termJobPIDs() []int {
	s.jobMu.Lock()
	defer s.jobMu.Unlock()
	pids := make([]int, 0, len(s.termJobs))
	for pid := range s.termJobs {
		pids = append(pids, pid)
	}
	slices.Sort(pids)
	return pids
}
