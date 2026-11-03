#!/bin/sh
# screenshots.sh takes screenshots of build/mdsh-swift.app, for the UI
# parity check: the start screen, a file, a file after Run all, and the
# looks. It runs on a copy of the examples, with settings of its own.
#
#   scripts/screenshots.sh outdir
set -eu
cd "$(dirname "$0")/.."
out=${1:-build/screenshots}
mkdir -p "$out"
out=$(cd "$out" && pwd)
bundle=$PWD/build/mdsh-swift.app
work=$(mktemp -d)
cp -R ../../examples "$work/examples"

# shot NAME SETTINGS-JSON AUTORUN ARGS... runs the app, which draws its
# window into out/NAME.png and quits (see Snapshot in MdshSwiftApp.swift).
# $show, if set, is MDSH_SWIFT_SHOW: panes and sheets to open. It starts
# the app with open(1), so it joins the login session as an app from
# Finder would.
show=
shot() {
	name=$1 settings=$2 autorun=$3
	shift 3
	printf '%s\n' "$settings" > "$work/settings.json"
	open -n -W -g \
		--env MDSH_SWIFT_SETTINGS="$work/settings.json" \
		--env MDSH_SWIFT_AUTORUN="$autorun" \
		--env MDSH_SWIFT_SHOW="$show" \
		--env MDSH_SWIFT_SNAPSHOT="$out/$name.png" \
		--env MDSH_SWIFT_SNAPSHOT_DELAY=6 \
		--stdout "$out/$name.log" --stderr "$out/$name.log" \
		"$bundle" --args "$@" &
	pid=$!
	# Give it 30 seconds before giving up on it.
	i=0
	while kill -0 "$pid" 2>/dev/null && [ $i -lt 30 ]; do sleep 1; i=$((i + 1)); done
	if kill -0 "$pid" 2>/dev/null; then
		pkill -f "$bundle/Contents/MacOS/mdsh-swift" || true
		kill "$pid" 2>/dev/null || true
	fi
	wait "$pid" 2>/dev/null || true
	show=
	if [ -f "$out/$name.png" ]; then echo "took $name"; else echo "no $name:"; cat "$out/$name.log" 2>/dev/null || true; fi
}

shot start-light '{"theme":"light"}' "" "$work/examples"
shot start-dark '{"theme":"dark"}' "" "$work/examples"
shot doc-light '{"theme":"light"}' "" "$work/examples/hello.md"
shot doc-ran-light '{"theme":"light"}' 1 "$work/examples/hello.md"
shot doc-ran-dark '{"theme":"dark"}' 1 "$work/examples/hello.md"
shot tty-light '{"theme":"light","look":"tty"}' 1 "$work/examples/hello.md"
shot tty-dark '{"theme":"dark","look":"tty"}' 1 "$work/examples/hello.md"
shot blueprint-light '{"theme":"light","look":"blueprint"}' 1 "$work/examples/hello.md"
shot gum-light '{"theme":"light","look":"gum"}' 1 "$work/examples/hello.md"
shot gum-dark '{"theme":"dark","look":"gum"}' 1 "$work/examples/hello.md"
shot gum-confirm '{"theme":"light","look":"gum"}' 1 "$work/examples/guardrails.md"
show=settings:look shot settings-look-gum '{"theme":"light","look":"gum"}' "" "$work/examples"
shot inputs '{"theme":"light"}' "" "$work/examples/inputs.md"
shot split '{"theme":"light","view":"both"}' "" "$work/examples/hello.md"
show=sourcefold shot source-fold '{"theme":"light","view":"source"}' "" "$work/examples/hello.md"
show=sourcefold shot source-tty '{"theme":"dark","look":"tty","view":"both"}' "" "$work/examples/hello.md"
shot confirm '{"theme":"light"}' 1 "$work/examples/guardrails.md"
show=outline shot confirm-outline '{"theme":"light"}' 1 "$work/examples/guardrails.md"
show=history shot history-open '{"theme":"light"}' "" "$work/examples/history.md"
show=history shot history-cursor '{"theme":"light","look":"tty","historyFocus":"cursor"}' "" "$work/examples/history.md"
show=console,outline shot console-outline '{"theme":"light"}' 1 "$work/examples/hello.md"
show=report shot report '{"theme":"light"}' "" "$work/examples/report.md"
show=runoptions shot run-options '{"theme":"light"}' "" "$work/examples/hello.md"
show=frontmatter shot front-matter '{"theme":"light"}' "" "$work/examples/hello.md"
for tab in general look editing shell files about; do
	show=settings:$tab shot settings-$tab '{"theme":"light"}' "" "$work/examples"
done
printf '# Broken\n\n```sh {#deploy depends=nope}\necho deploying\n```\n' > "$work/examples/broken.md"
show=console shot console-error '{"theme":"light"}' 1 "$work/examples/broken.md"
printf '# Notes\n\n```sh {#build}\necho built\n```\n\n```output {for=old-build}\nold\n```\n' > "$work/examples/fixable.md"
show=console,fixfirst shot console-fixed '{"theme":"light"}' "" "$work/examples/fixable.md"
show=depends shot depends-on '{"theme":"light"}' "" "$work/examples/guardrails.md"
show=clearall shot clear-all '{"theme":"light"}' 1 "$work/examples/hello.md"
show=filechanged,selectlast shot file-changed '{"theme":"light"}' "" "$work/examples/hello.md"
show=newfile shot new-file "{\"theme\":\"light\",\"templateFolder\":\"$work/examples\"}" "" "$work/examples"
printf '# Release\n\n- [x] Tag the release\n- [ ] Write the notes\n- [ ] Tell the team\n\n```sh {#tasks out=markdown}\nprintf -- "- [ ] check %%s\\n" disk memory\n```\n' > "$work/examples/tasks.md"
shot tasks '{"theme":"light"}' 1 "$work/examples/tasks.md"
for tab in shell files; do
	show=settings:$tab shot settings-$tab-values "{\"theme\":\"light\",\"shell\":\"zsh\",\"historyRC\":\"~/.zshrc\",\"templateFolder\":\"$work/examples\",\"sessionFolder\":\"$work/sessions\"}" "" "$work/examples"
done
show=depsel shot dep-lines '{"theme":"light"}' "" "$work/examples/guardrails.md"
show=delete shot delete-block '{"theme":"light"}' "" "$work/examples/hello.md"
printf -- '---\ntitle: Shared\nmdsh:\n  env:\n    share: ["...", -AWS_SECRET_ACCESS_KEY]\n---\n# Shared\n\n```sh {#who}\necho "$HOME"\n```\n' > "$work/examples/shared.md"
shot env-banner '{"theme":"light"}' "" "$work/examples/shared.md"
shot inputs-ran '{"theme":"light"}' 1 "$work/examples/inputs.md"
printf '# Session\n\n```textbox {#token var=TOKEN nosave}\nfrom the file\n```\n\n```sh {#now nosave}\necho "token is $MDSH_VAR_TOKEN"\n```\n' > "$work/examples/nosave.md"
shot nosave '{"theme":"light"}' 1 "$work/examples/nosave.md"
printf '# Keys\n\n```sh {#plain}\necho visible\n```\n\n```sh {#key secret}\necho s3cr3t-token\n```\n' > "$work/examples/secret.md"
show=select:plain shot secret '{"theme":"light"}' 1 "$work/examples/secret.md"
shot tty-running '{"theme":"light","look":"tty"}' 1 "$work/examples/guardrails.md"
shot cast-output '{"theme":"light"}' "" "$work/examples/terminal.md"
printf '# A recording\n\n```cast\n{"version": 2, "width": 30, "height": 4}\n[0.1, "o", "$ echo hi\\r\\n"]\n[0.5, "o", "hi\\r\\n"]\n[0.9, "o", "$ "]\n```\n' > "$work/examples/castfence.md"
shot cast-fence '{"theme":"dark"}' "" "$work/examples/castfence.md"
show=settings:look shot settings-look-tty '{"theme":"light","look":"tty"}' "" "$work/examples"
show=settings:look shot settings-look-blueprint '{"theme":"light","look":"blueprint"}' "" "$work/examples"
show=settings:about shot settings-about-tty '{"theme":"dark","look":"tty"}' "" "$work/examples"
shot terminal-block '{"theme":"light"}' 1 "$work/examples/terminal.md"
printf -- '# Move\n\n```sh {#fetch}\necho fetched\n```\n\n```sh {#deploy run={{#fetch/?}}==0}\necho deployed\n```\n' > "$work/examples/move.md"
show=movefirstdown shot move-banner '{"theme":"light"}' "" "$work/examples/move.md"
show=select:banner shot svg-block '{"theme":"light"}' 1 "$work/examples/hello.md"
show=foldersettings shot folder-settings '{"theme":"light"}' "" "$work/examples"
cat > "$work/examples/diagrams.md" <<'EOF2'
# Diagrams

```d2
fetch -> build -> deploy: runs
```

```svg
<svg xmlns="http://www.w3.org/2000/svg" width="160" height="40"><rect width="160" height="40" rx="6" fill="#4f7cff"/></svg>
```

```d2
fetch -> {
```
EOF2
shot diagrams '{"theme":"light"}' "" "$work/examples/diagrams.md"
shot diagrams-dark '{"theme":"dark"}' "" "$work/examples/diagrams.md"

# A block that ran again leaves the blocks that depend on it stale.
cat > "$work/examples/stale.md" <<'EOF2'
# Stale

```sh {#a}
echo a
```

```sh {#b depends=a}
echo b
```

```sh {#c depends=b}
echo c
```
EOF2
shot stale '{"theme":"light"}' rerun "$work/examples/stale.md"
shot stale-tty '{"theme":"dark","look":"tty"}' rerun "$work/examples/stale.md"

# The run timer: a block that took a moment, and one running against
# its timeout, then one its timeout ended.
cat > "$work/examples/timer.md" <<'EOF2'
# Timers

```sh {#quick}
sleep 1
```

```sh {#slow timeout=10s}
sleep 30
```
EOF2
shot run-timer '{"theme":"light"}' 1 "$work/examples/timer.md"
cat > "$work/examples/timeout.md" <<'EOF2'
# Timeout

```sh {#quick}
echo hi
```

```sh {#stuck timeout=1s}
sleep 30
```
EOF2
shot timed-out '{"theme":"light"}' 1 "$work/examples/timeout.md"

# Two inputs sharing a var= are copies; a copy that disagrees is set to
# the first's value, with a warning.
cat > "$work/examples/copies.md" <<'EOF2'
# Copies

```choice {#env var=env mode=radio}
- [ ] staging
- [x] prod
```

Later, the same choice again:

```choice {#env2 var=env mode=radio}
- [x] staging
- [ ] prod
```
EOF2
shot var-copies '{"theme":"light"}' "" "$work/examples/copies.md"

# A History prompt drawn to the pane's width: a rule across it.
cat > "$work/rule.rc" <<'EOF2'
PS1='$(printf "%*s" "${COLUMNS:-20}" "" | tr " " "-")
$ '
EOF2
show=history shot history-rule "{\"theme\":\"light\",\"historyRC\":\"$work/rule.rc\"}" "" "$work/examples/history.md"
