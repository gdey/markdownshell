package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// shellKind is how mdsh hooks into an interactive shell.
type shellKind int

const (
	kindSh   shellKind = iota // POSIX sh: dash, ash, bash or ksh as sh
	kindBash                  // bash
	kindZsh                   // zsh
)

// kindOf picks the integration for the shell at path by its name.
// Anything that isn't bash or zsh gets the POSIX one, which needs only
// $ENV and PS1. fish and other shells that aren't POSIX are an error.
func kindOf(path string) (shellKind, error) {
	switch base := filepath.Base(path); {
	case strings.HasPrefix(base, "zsh"):
		return kindZsh, nil
	case strings.HasPrefix(base, "bash"):
		return kindBash, nil
	case base == "fish" || base == "nu" || base == "elvish" || base == "xonsh" || strings.HasPrefix(base, "pwsh") || base == "csh" || base == "tcsh":
		return 0, fmt.Errorf("%s isn't a POSIX shell; mdsh can't run it interactively", base)
	}
	return kindSh, nil
}

// dumpAwk writes a state record from what the hook passes it in its
// environment: __mdsh_s and the rest, the jobs file, wanted variables
// as __mdsh_v_NAME, and every other exported variable.
const dumpAwk = `function esc(v) { gsub(/\\/, "\\\\", v); gsub(/\n/, "\\n", v); return v }
BEGIN {
	print "state"
	print "status " ENVIRON["__mdsh_s"]
	print "pwd " esc(ENVIRON["__mdsh_pwd"])
	print "oldpwd " esc(ENVIRON["__mdsh_old"])
	print "pid " ENVIRON["__mdsh_pid"]
	if (ENVIRON["__mdsh_jl"] == "1") {
		print "jobs"
		while ((getline l < jobs) > 0) print "job " l
	}
	for (k in ENVIRON) {
		if (k ~ /^__mdsh_v_/) print "var " substr(k, 10) "=" esc(ENVIRON[k])
		else if (k !~ /^__mdsh_/ && k != "AWKPATH" && k != "AWKLIBPATH") print "env " k "=" esc(ENVIRON[k])
	}
	print "end"
}`

// dumpFunc is the shell function that writes a state record to the
// fifo, given the status. It runs in the shell itself in bash and zsh,
// so jobs -l sees the job table; jobsKnown is "0" for sh, where it runs
// in the prompt's command substitution, a subshell with no jobs.
func dumpFunc(jobsKnown string) string {
	return `__mdsh_dump() {
	jobs -l >"$__mdsh_dir/jobs" 2>/dev/null
	(
		if [ -s "$__mdsh_dir/want" ]; then
			while IFS= read -r __mdsh_n; do
				case $__mdsh_n in ''|[0-9]*|*[!A-Za-z0-9_]*) continue ;; esac
				eval "if [ -n \"\${$__mdsh_n+x}\" ]; then __mdsh_v_$__mdsh_n=\"\${$__mdsh_n}\"; export __mdsh_v_$__mdsh_n; fi"
			done <"$__mdsh_dir/want"
		fi
		__mdsh_s=$1 __mdsh_pwd=$PWD __mdsh_old=${OLDPWD-} __mdsh_pid=$$ __mdsh_jl=` + jobsKnown + `
		export __mdsh_s __mdsh_pwd __mdsh_old __mdsh_pid __mdsh_jl
		exec awk -v jobs="$__mdsh_dir/jobs" "$__mdsh_awk"
	) >"$__mdsh_dir/state" 2>/dev/null
}
`
}

// common is the start of every integration: where mdsh's files are, the
// dump, and that it is on.
func common(dir, jobsKnown string) string {
	return "__mdsh_dir=" + quote(dir) + "\n" +
		"__mdsh_awk=" + quote(dumpAwk) + "\n" +
		"MDSH_INTEGRATED=1; export MDSH_INTEGRATED\n" +
		"MDSH_TMP=" + quote(dir) + "; export MDSH_TMP\n" +
		dumpFunc(jobsKnown)
}

// initLine sources init.sh, mdsh's own start-up script: the positional
// parameters, helpers and Config.Init.
const initLine = `[ -r "$__mdsh_dir/init.sh" ] && . "$__mdsh_dir/init.sh"` + "\n"

// bashRC is the file bash reads in place of ~/.bashrc (--rcfile). It
// reads the user's own files, as bash would have, then hooks in:
// PROMPT_COMMAND ends a command (D) and sends the state, PS1 marks the
// prompt (A, B), and PS0, or on bash before 4.4 a DEBUG trap, marks
// where the command's output starts (C).
const bashRC = `if [ -n "${MDSH_LOGIN-}" ]; then
	unset MDSH_LOGIN
	[ -r /etc/profile ] && . /etc/profile
	if [ -r ~/.bash_profile ]; then . ~/.bash_profile
	elif [ -r ~/.bash_login ]; then . ~/.bash_login
	elif [ -r ~/.profile ]; then . ~/.profile
	fi
else
	[ -r ~/.bashrc ] && . ~/.bashrc
fi
__mdsh_precmd() {
	local s=$? h
	# Only as the prompt's hook, not called from a script.
	[ "${#FUNCNAME[@]}" -gt 1 ] && return $s
	h=$(HISTTIMEFORMAT= history 1 2>/dev/null)
	case $h in
	*'. "'"$__mdsh_dir"/*)
		h=${h#"${h%%[! ]*}"}
		history -d "${h%% *}" 2>/dev/null ;;
	esac
	__mdsh_dump "$s"
	printf '\033]133;D;%s;mdsh\007' "$s"
	return $s
}
__mdsh_wrap() {
	case $PS1 in
	*'133;A;mdsh'*) ;;
	*) PS1='\[\033]133;A;mdsh\007\]'"$PS1"'\[\033]133;B;mdsh\007\]' ;;
	esac
	__mdsh_at_prompt=1
}
if [[ "$(declare -p PROMPT_COMMAND 2>/dev/null)" == "declare -a"* ]]; then
	PROMPT_COMMAND=(__mdsh_precmd "${PROMPT_COMMAND[@]}" __mdsh_wrap)
else
	PROMPT_COMMAND="__mdsh_precmd${PROMPT_COMMAND:+$'\n'$PROMPT_COMMAND}"$'\n'"__mdsh_wrap"
fi
if (( BASH_VERSINFO[0] > 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] >= 4) )); then
	PS0="${PS0-}"'\033]133;C;mdsh\007'
else
	__mdsh_debug() {
		[ -n "${__mdsh_at_prompt-}" ] || return 0
		[ -n "${COMP_LINE-}" ] && return 0
		case $BASH_COMMAND in __mdsh_*) return 0 ;; esac
		__mdsh_at_prompt=
		printf '\033]133;C;mdsh\007'
	}
	trap '__mdsh_debug' DEBUG
fi
`

// zshHooks is sourced by the .zshrc shim after the user's .zshrc.
const zshHooks = `setopt interactive_comments
PROMPT_EOL_MARK=''
__mdsh_precmd() {
	local s=$?
	# Only as the prompt's hook, not called from a script.
	(( ${#funcstack} > 1 )) && return $s
	__mdsh_dump $s
	print -rn -- $'\e]133;D;'$s$';mdsh\a'
	return $s
}
__mdsh_wrap() {
	[[ $PS1 == *'133;A;mdsh'* ]] || PS1=$'%{\e]133;A;mdsh\a%}'$PS1$'%{\e]133;B;mdsh\a%}'
}
__mdsh_preexec() {
	local t=${1//\\/\\\\}
	t=${t//$'\n'/\\n}
	{ print -r -- cmd; print -r -- "text $t"; print -r -- end } >"$__mdsh_dir/state" 2>/dev/null
	print -rn -- $'\e]133;C;mdsh\a'
}
__mdsh_addhistory() {
	[[ $1 != '. "'$__mdsh_dir/* ]] || return 1
	# Every line, as a comment runs no preexec: mdsh reads a note so.
	local t=${1%$'\n'}
	t=${t//\\/\\\\}
	t=${t//$'\n'/\\n}
	{ print -r -- cmd; print -r -- "text $t"; print -r -- end } >"$__mdsh_dir/state" 2>/dev/null
	return 0
}
precmd_functions=(__mdsh_precmd $precmd_functions __mdsh_wrap)
preexec_functions+=(__mdsh_preexec)
zshaddhistory_functions+=(__mdsh_addhistory)
`

// zshShim is one of the files in the ZDOTDIR mdsh starts zsh with. It
// sources the user's file of the same name from their own ZDOTDIR, or
// $HOME, then points ZDOTDIR back at mdsh's, so zsh reads the next shim.
// The last file zsh reads (.zshrc, or .zlogin in a login shell) leaves
// ZDOTDIR as the user had it.
func zshShim(dir, name string) string {
	var b strings.Builder
	if name == ".zshenv" {
		b.WriteString(`if [[ -n ${MDSH_USER_ZDOTDIR+x} ]]; then __mdsh_uz=$MDSH_USER_ZDOTDIR; else __mdsh_uz=; fi
unset MDSH_USER_ZDOTDIR
`)
	}
	b.WriteString(`if [[ -n $__mdsh_uz ]]; then ZDOTDIR=$__mdsh_uz; else unset ZDOTDIR; fi
[[ -f ${ZDOTDIR:-$HOME}/` + name + ` ]] && source ${ZDOTDIR:-$HOME}/` + name + `
__mdsh_uz=${ZDOTDIR-}
ZDOTDIR=` + quote(dir) + "\n")
	if name == ".zshrc" {
		b.WriteString(common(filepath.Dir(dir), "1"))
		b.WriteString(zshHooks)
		b.WriteString(initLine)
		b.WriteString("[[ -o login ]] || { if [[ -n $__mdsh_uz ]]; then ZDOTDIR=$__mdsh_uz; else unset ZDOTDIR; fi; unset __mdsh_uz; }\n")
	}
	if name == ".zlogin" {
		b.WriteString("if [[ -n $__mdsh_uz ]]; then ZDOTDIR=$__mdsh_uz; else unset ZDOTDIR; fi\nunset __mdsh_uz\n")
	}
	return b.String()
}

// shRC is the file a POSIX sh reads through $ENV. It reads the user's
// own $ENV, if they had one, then marks the prompt through PS1: sh has
// no hook that runs before a prompt but PS1's expansion, which runs
// __mdsh_prompt in a subshell, nor one before a command, so the end of
// the echoed line stands in for C.
const shRC = `if [ -n "${MDSH_USER_ENV-}" ]; then
	ENV=$MDSH_USER_ENV
	unset MDSH_USER_ENV
	[ -r "$ENV" ] && . "$ENV"
else
	unset ENV
fi
__mdsh_prompt() {
	__mdsh_s=$?
	__mdsh_dump "$__mdsh_s"
	printf '\033]133;D;%s;mdsh\007\033]133;A;mdsh\007' "$__mdsh_s"
}
__mdsh_b=$(printf '\033]133;B;mdsh\007')
PS1='$(__mdsh_prompt)'"${PS1-\$ }$__mdsh_b"
unset __mdsh_b
`

// writeIntegration writes the integration files for kind into dir and
// returns the arguments to start the shell with, after its path, and
// the variables to add to its environment.
func writeIntegration(kind shellKind, dir string, cfg Config, init string) (args, env []string, err error) {
	write := func(name, text string) error {
		return os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600)
	}
	if err := write("init.sh", init); err != nil {
		return nil, nil, err
	}
	switch kind {
	case kindBash:
		if err := write("bashrc", common(dir, "1")+bashRC+initLine); err != nil {
			return nil, nil, err
		}
		args = []string{"--rcfile", filepath.Join(dir, "bashrc"), "-i"}
		if cfg.Login {
			env = append(env, "MDSH_LOGIN=1")
		}
	case kindZsh:
		zdot := filepath.Join(dir, "zdot")
		if err := os.Mkdir(zdot, 0o700); err != nil {
			return nil, nil, err
		}
		for _, n := range []string{".zshenv", ".zprofile", ".zshrc", ".zlogin"} {
			if err := os.WriteFile(filepath.Join(zdot, n), []byte(zshShim(zdot, n)), 0o600); err != nil {
				return nil, nil, err
			}
		}
		args = []string{"-i"}
		if cfg.Login {
			args = append(args, "-l")
		}
		if v, ok := lookup(cfg.Env, "ZDOTDIR"); ok {
			env = append(env, "MDSH_USER_ZDOTDIR="+v)
		}
		env = append(env, "ZDOTDIR="+zdot)
	default:
		if err := write("shrc", common(dir, "0")+shRC+initLine); err != nil {
			return nil, nil, err
		}
		args = []string{"-i"}
		if cfg.Login {
			args = append(args, "-l")
		}
		if v, ok := lookup(cfg.Env, "ENV"); ok && v != "" {
			env = append(env, "MDSH_USER_ENV="+v)
		}
		env = append(env, "ENV="+filepath.Join(dir, "shrc"))
	}
	return args, env, nil
}

// lookup finds key in a KEY=value list; the last one wins, as in exec.
func lookup(environ []string, key string) (string, bool) {
	v, ok := "", false
	for _, kv := range environ {
		if k, val, found := strings.Cut(kv, "="); found && k == key {
			v, ok = val, true
		}
	}
	return v, ok
}

// setEnv is environ with key set to value, replacing it if it is there.
func setEnv(environ []string, kv string) []string {
	key, _, _ := strings.Cut(kv, "=")
	out := make([]string, 0, len(environ)+1)
	for _, e := range environ {
		if k, _, _ := strings.Cut(e, "="); k != key {
			out = append(out, e)
		}
	}
	return append(out, kv)
}
