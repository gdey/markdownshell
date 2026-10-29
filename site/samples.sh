#!/bin/sh
# Regenerates the sample reports on the Use cases pages: runs each runbook
# once, then writes the same run as PDF, HTML and markdown into
# static/samples, plus a PNG of each PDF's first page into
# static/images/pdf. Needs mdsh on PATH and pdftoppm (poppler).
set -eu
cd "$(dirname "$0")"
runbooks=../examples/runbooks
out=static/samples
mkdir -p "$out" static/images/pdf

# disk-report is a report for people who won't run it, so it leaves the
# code out.
for name in deploy disk-report onboarding; do
	flags=
	[ "$name" = disk-report ] && flags=--hide-code
	# Run next to the runbook, so relative paths in it still work.
	run="$runbooks/.sample-$name.md"
	(cd "$runbooks" && mdsh --yes -o ".sample-$name.md" "$name.md" >/dev/null)
	mdsh -n $flags --pdf "$out/$name.pdf" "$run"
	mdsh -n $flags --html "$out/$name.html" "$run"
	mdsh -n $flags --no-color -o "$out/$name.md" "$run"
	rm -f "$run"
	pdftoppm -r 90 -png -f 1 -l 1 -singlefile "$out/$name.pdf" "static/images/pdf/$name-1"
done
