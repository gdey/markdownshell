package env

// helpers are shell functions defined in every shell, for blocks that
// print markdown (out=markdown).
const helpers = `
md_h1() { printf '# %s\n\n' "$*"; }
md_h2() { printf '## %s\n\n' "$*"; }
md_h3() { printf '### %s\n\n' "$*"; }

# md_table [-F SEP] [-s N] turns lines on stdin into a markdown table.
# The first line is the header. Fields are split on tabs, or on SEP as
# awk -F takes it: -F ' ' splits on runs of blanks. -s N skips the first
# N lines.
md_table() {
	_md_fs='\t' _md_skip=0 OPTIND=1
	while getopts F:s: _md_opt; do
		case $_md_opt in
		F) _md_fs=$OPTARG ;;
		s) _md_skip=$OPTARG ;;
		*) echo 'usage: md_table [-F SEP] [-s N]' >&2; return 2 ;;
		esac
	done
	case $_md_skip in
	''|*[!0-9]*) echo "md_table: -s takes a number of lines, not $_md_skip" >&2; return 2 ;;
	esac
	awk -F "$_md_fs" -v skip="$_md_skip" '
		NR <= skip { next }
		{ line = "|"; for (i = 1; i <= NF; i++) { f = $i; gsub(/\|/, "\\|", f); line = line " " f " |" }; print line }
		NR == skip + 1 { line = "|"; for (i = 1; i <= NF; i++) line = line " --- |"; print line }
	'
	printf '\n'
}

# md_code LANG prints stdin as a fenced code block.
md_code() { printf '~~~~%s\n' "${1-}"; cat; printf '~~~~\n\n'; }
`
