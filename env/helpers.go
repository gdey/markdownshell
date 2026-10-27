package env

// helpers are shell functions defined in every shell, for blocks that
// print markdown (out=markdown).
const helpers = `
md_h1() { printf '# %s\n\n' "$*"; }
md_h2() { printf '## %s\n\n' "$*"; }
md_h3() { printf '### %s\n\n' "$*"; }

# md_table turns tab-separated lines on stdin into a markdown table.
# The first line is the header.
md_table() {
	awk -F '\t' '
		{ gsub(/\|/, "\\|"); line = "|"; for (i = 1; i <= NF; i++) line = line " " $i " |"; print line }
		NR == 1 { line = "|"; for (i = 1; i <= NF; i++) line = line " --- |"; print line }
	'
	printf '\n'
}

# md_code LANG prints stdin as a fenced code block.
md_code() { printf '~~~~%s\n' "${1-}"; cat; printf '~~~~\n\n'; }
`
