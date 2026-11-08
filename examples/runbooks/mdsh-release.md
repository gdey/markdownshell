#!/usr/bin/env mdsh
---
title: Release mdsh
mdsh:
  on_error: stop
---
# Release mdsh

The checks for releasing mdsh itself. A release happens when its
release-prep PR merges: the PR moves `[Unreleased]` in `CHANGELOG.md`
into a dated `## [X.Y.Z]` section, and once that is on main the release
workflow builds everything, tags the merge commit, publishes the release
and updates the Homebrew tap. Nobody tags by hand.

Run this on the release-prep branch before merging it, to check the
changelog is ready, or from a checkout of main:

```text
mdsh examples/runbooks/mdsh-release.md                    # check only
VERSION=v1.3.0 mdsh examples/runbooks/mdsh-release.md     # check another version
PUSH=1 mdsh examples/runbooks/mdsh-release.md             # tag by hand, if the workflow didn't
```

Without `PUSH` it only checks and changes nothing, which is how the tests
run it. `VERSION` defaults to the newest release in `CHANGELOG.md`.

The release-prep PR is ready when `CHANGELOG.md` has a dated
`## [X.Y.Z]` section, `[Unreleased]` is empty, and the compare links at
the bottom point at the new version. A minor release (`X.Y.0`) also
brings a new look, named in its changelog section. The changelog and
look checks below say so; the rows that need main (on main, matching
origin) only matter for tagging by hand.

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

## New look

Every minor release brings a new look for the desktop app (1.3 brought
Teletype and Blueprint, 1.4 Bubble Gum, 1.5 Witchcraft; see
[Adding a look](../../docs/looks.md)). For an `X.Y.0` release this
checks that a look in `cmd/mdsh-gui/frontend/looks` has
`since = "X.Y"` and that the release's changelog section names it.
Patch releases skip it.

```sh {#look}
v="${VERSION#v}"
case "$v" in
  *.0)
    minor="${v%.0}"
    looks="$(grep -l "^since = \"$minor\"$" cmd/mdsh-gui/frontend/looks/*/look.css 2>/dev/null)"
    [ -n "$looks" ] ||
      { echo "no look has since = \"$minor\": every minor release brings a new look (docs/looks.md)" >&2; exit 1; }
    section="$(awk -v v="$v" '
      /^## \[/ { p = index($0, "[" v "]") > 0; next }
      /^\[[^]]*\]: / { p = 0 }
      p' CHANGELOG.md)"
    for f in $looks; do
      name="$(sed -n 's/^name = "\(.*\)"$/\1/p' "$f" | head -n 1)"
      printf '%s\n' "$section" | grep -q "$name" ||
        { echo "the $v changelog section doesn't name its new look, $name" >&2; exit 1; }
      echo "$v brings the $name look"
    done
    ;;
  *) echo "$v is a patch release: no new look needed" ;;
esac
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

## Tag and push by hand

Only needed when the release workflow didn't tag, for instance after its
run was cancelled. Runs only with `PUSH` set, and asks first. Pushing the
tag starts the same release workflow, which builds the CLI for six
targets and the desktop app packages, publishes the release with the
notes above, and updates the Homebrew tap.

```sh {#tag run={{$PUSH}} confirm="Tag and push the release?"}
[ "$READY" = yes ] || { echo "not ready to tag: see the checks above" >&2; exit 1; }
git tag -a "$VERSION" -m "mdsh $VERSION"
git push origin "$VERSION"
echo "pushed $VERSION"
```

## After the merge

1. Watch the release workflow on the
   [Actions page](https://github.com/gdey/markdownshell/actions/workflows/release.yml).
   If the packages fail to build, nothing is tagged: fix main and run the
   workflow again from that page. If it fails after tagging, delete the
   tag and the release (`git push --delete origin vX.Y.Z`), fix main, and
   run it again.
2. Check the release on the
   [Releases page](https://github.com/gdey/markdownshell/releases) has
   the six CLI archives, the `mdsh-gui` `.dmg`, `.zip` and AppImage, and
   `checksums.txt`, and that `brew upgrade gdey/tap/mdsh` gets it.
