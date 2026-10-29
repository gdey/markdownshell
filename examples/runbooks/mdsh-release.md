#!/usr/bin/env mdsh
---
title: Release mdsh
mdsh:
  on_error: stop
---
# Release mdsh

The steps for releasing mdsh itself. Run it from a checkout of main:

```text
mdsh examples/runbooks/mdsh-release.md                    # check only
VERSION=v1.3.0 mdsh examples/runbooks/mdsh-release.md     # check another version
PUSH=1 mdsh examples/runbooks/mdsh-release.md             # check, then tag and push
```

Without `PUSH` it only checks and changes nothing, which is how the tests
run it. `VERSION` defaults to the newest release in `CHANGELOG.md`.

Before running it, the release-prep PR should be merged: `CHANGELOG.md`
has a dated `## [X.Y.Z]` section, `[Unreleased]` is empty, and the compare
links at the bottom point at the new version.

## Version

```sh {#version}
command -v git >/dev/null || { echo "git is not installed" >&2; exit 1; }
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo ../..)"
test -f CHANGELOG.md
if [ -z "$VERSION" ]; then
  VERSION="$(sed -n 's/^## \[\([0-9][0-9.]*\)\].*/\1/p' CHANGELOG.md | head -n 1)"
fi
VERSION="v${VERSION#v}"
export VERSION
echo "$VERSION"
```

## Changelog

These checks stop the run if they fail: the tag's release job needs a
section to use as the release notes.

```sh {#changelog}
v="${VERSION#v}"
grep -Eq "^## \[$v\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$" CHANGELOG.md ||
  { echo "no dated '## [$v]' section in CHANGELOG.md" >&2; exit 1; }
grep -q "^\[$v\]: .*/compare/.*\.\.\.$VERSION$" CHANGELOG.md ||
  { echo "no '[$v]' compare link ending in $VERSION" >&2; exit 1; }
grep -q "^\[Unreleased\]: .*/compare/$VERSION\.\.\.HEAD$" CHANGELOG.md ||
  { echo "the [Unreleased] link doesn't compare from $VERSION" >&2; exit 1; }
echo "CHANGELOG.md is ready for $VERSION"
```

The release notes, cut from `CHANGELOG.md` with the same awk as
`.github/workflows/release.yml`. This shows their sections and how many
entries each has; the whole notes are in the file it names.

```sh {#notes}
NOTES="$(mktemp)"
awk -v v="${VERSION#v}" '
  /^## \[/ { p = index($0, "[" v "]") > 0; next }
  /^\[[^]]*\]: / { p = 0 }
  p' CHANGELOG.md > "$NOTES"
grep -q . "$NOTES" || { echo "the notes for $VERSION are empty" >&2; exit 1; }
echo "notes: $NOTES"
awk '/^### / { if (h) print h ": " n; h = substr($0, 5); n = 0 }
     /^- / { n++ }
     END { if (h) print h ": " n }' "$NOTES"
```

## Ready to tag?

These depend on the checkout, so they are reported rather than stopping the
run. Tagging below needs every row to be `yes`.

```sh {#preflight out=markdown}
ROWS="$(mktemp)"
READY=yes
row() {
  printf '%s\t%s\n' "$1" "$2" >> "$ROWS"
  [ "$2" = yes ] || READY=no
}
yn() { if "$@" >/dev/null 2>&1; then echo yes; else echo no; fi; }

row "on main" "$(yn test "$(git branch --show-current)" = main)"
row "no uncommitted changes" "$(yn test -z "$(git status --porcelain --untracked-files=no)")"
row "[Unreleased] is empty" "$(yn test -z "$(awk '
  /^## \[Unreleased\]/ { p = 1; next }
  /^## \[/ { p = 0 }
  p && NF' CHANGELOG.md)")"
if git fetch -q origin main 2>/dev/null; then
  row "main matches origin" "$(yn test "$(git rev-parse HEAD)" = "$(git rev-parse FETCH_HEAD)")"
else
  row "main matches origin" "unknown (fetch failed)"
fi
if tags="$(git ls-remote --tags origin "refs/tags/$VERSION" 2>/dev/null)"; then
  row "$VERSION not tagged yet" "$(yn test -z "$tags")"
else
  row "$VERSION not tagged yet" "unknown (ls-remote failed)"
fi
if command -v goreleaser >/dev/null; then
  row "goreleaser check" "$(yn goreleaser check)"
fi
export READY

md_h3 "Checks for $VERSION"
{ printf 'check\tok\n'; cat "$ROWS"; } | md_table
echo
echo "Ready to tag: **$READY**"
```

CI on main isn't checked here: look at the latest run on the
[Actions page](https://github.com/gdey/markdownshell/actions?query=branch%3Amain)
before pushing the tag.

## Tag and push

Runs only with `PUSH` set, and asks first. Pushing the tag starts the
release workflow, which builds the CLI for six targets and the desktop app
packages, and makes a **draft** release with the notes above.

```sh {#tag run={{$PUSH}} confirm="Tag and push the release?"}
[ "$READY" = yes ] || { echo "not ready to tag: see the checks above" >&2; exit 1; }
git tag -a "$VERSION" -m "mdsh $VERSION"
git push origin "$VERSION"
echo "pushed $VERSION"
```

## After the tag

1. Watch the release workflow on the
   [Actions page](https://github.com/gdey/markdownshell/actions/workflows/release.yml).
   If it fails, delete the tag (`git push --delete origin vX.Y.Z` and
   `git tag -d vX.Y.Z`), fix main, and run this again.
2. Open the draft on the
   [Releases page](https://github.com/gdey/markdownshell/releases). Check
   it has the six CLI archives, the `mdsh-gui` `.dmg`, `.zip` and
   AppImage, and `checksums.txt`, then publish it.
