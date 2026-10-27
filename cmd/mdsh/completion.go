package main

import (
	"fmt"
	"io"
	"os"

	"github.com/gdey/markdownshell/doc"
	"github.com/gdey/markdownshell/run"
)

// completionCmd prints a completion script:
//
//	source <(mdsh completion bash)
//	mdsh completion zsh > "${fpath[1]}/_mdsh"
func completionCmd(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: mdsh completion bash|zsh")
	}
	switch args[0] {
	case "bash":
		_, err := io.WriteString(stdout, bashCompletion)
		return err
	case "zsh":
		_, err := io.WriteString(stdout, zshCompletion)
		return err
	default:
		return fmt.Errorf("no completion for %q; try bash or zsh", args[0])
	}
}

// blocksCmd lists a file's runnable block ids, one per line, for
// completing --only. It is quiet on errors so completion never prints junk.
func blocksCmd(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return nil
	}
	src, err := os.ReadFile(args[0])
	if err != nil {
		return nil
	}
	d, err := doc.Parse(src)
	if err != nil {
		return nil
	}
	for _, b := range d.Blocks {
		if run.Runnable(b) {
			fmt.Fprintln(stdout, b.ID)
		}
	}
	return nil
}

const bashCompletion = `# bash completion for mdsh. Load it with:
#   source <(mdsh completion bash)
_mdsh() {
	local cur prev words cword
	if declare -F _init_completion >/dev/null 2>&1; then
		_init_completion || return
	else
		cur=${COMP_WORDS[COMP_CWORD]}
		prev=${COMP_WORDS[COMP_CWORD-1]}
		words=("${COMP_WORDS[@]}")
		cword=$COMP_CWORD
	fi

	if [[ $cword -eq 2 && ${words[1]} == completion ]]; then
		COMPREPLY=($(compgen -W "bash zsh" -- "$cur"))
		return
	fi

	case $prev in
	--color|-color)
		COMPREPLY=($(compgen -W "auto always never" -- "$cur"))
		return ;;
	--only|-only)
		local w f
		for w in "${words[@]}"; do
			[[ $w == *.md && -f $w ]] && f=$w && break
		done
		[[ -n $f ]] && COMPREPLY=($(compgen -W "$(mdsh __blocks "$f" 2>/dev/null)" -- "$cur"))
		return ;;
	-o|--pdf|-pdf)
		compopt -o filenames 2>/dev/null
		COMPREPLY=($(compgen -f -- "$cur"))
		return ;;
	esac

	if [[ $cur == -* ]]; then
		COMPREPLY=($(compgen -W "--hide-code --hide-output --no-run -n -w -o --pdf --only --stop-on-error --yes --color --no-color -i --version -h" -- "$cur"))
		return
	fi

	compopt -o filenames 2>/dev/null
	COMPREPLY=($(compgen -d -- "$cur") $(compgen -f -X '!*.md' -- "$cur"))
	if [[ $cword -eq 1 ]]; then
		COMPREPLY+=($(compgen -W "run completion" -- "$cur"))
	fi
}
complete -F _mdsh mdsh
`

const zshCompletion = `#compdef mdsh
# zsh completion for mdsh. Install it with:
#   mdsh completion zsh > "${fpath[1]}/_mdsh"
# or load it for this shell with:
#   source <(mdsh completion zsh)

_mdsh_blocks() {
	local f w
	for w in $words; do
		[[ $w == *.md && -f $w ]] && f=$w && break
	done
	[[ -n $f ]] || return 1
	local -a ids
	ids=(${(f)"$(mdsh __blocks $f 2>/dev/null)"})
	_describe -t blocks 'block' ids
}

_mdsh() {
	if (( CURRENT == 3 )) && [[ $words[2] == completion ]]; then
		_values 'shell' bash zsh
		return
	fi
	if (( CURRENT == 2 )) && [[ $words[CURRENT] != -* ]]; then
		_alternative 'commands:command:(run completion)' 'files:markdown file:_files -g "*.md"'
		return
	fi
	_arguments -s \
		'--hide-code[print prose and output, not runnable code]' \
		'--hide-output[print prose and code, not output]' \
		'(--no-run -n)'{--no-run,-n}'[print saved output without running anything]' \
		'-w[write output back into the file]' \
		'-o[write the -w result to this file]:output file:_files' \
		'--pdf[write what would be printed as a PDF]:pdf file:_files -g "*.pdf"' \
		'--only[run only this block and what it needs]:block:_mdsh_blocks' \
		'--stop-on-error[stop at the first failing block]' \
		'--yes[run confirm blocks without asking]' \
		'--color[style printed markdown]:when:(auto always never)' \
		'--no-color[same as --color=never]' \
		'-i[interactive mode]' \
		'--version[print the version]' \
		'-h[show help]' \
		'1:markdown file:_files -g "*.md"' \
		'*:block argument:_files'
}

if [[ $zsh_eval_context[-1] == loadautofunc ]]; then
	_mdsh "$@"
else
	compdef _mdsh mdsh
fi
`
