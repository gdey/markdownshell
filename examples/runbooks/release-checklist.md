#!/usr/bin/env mdsh
---
title: Cut a release
mdsh:
  on_error: stop
---
# Cut a release

This runbook walks through tagging a release of a small Go project. It
works in a scratch repository under `$TMPDIR`, so running it changes
nothing on your machine. Point `REPO` somewhere real to use it for real.

## Before you start

The steps need `git`. This block fails, and stops the run, if it's missing.

```sh {#tools}
command -v git >/dev/null || { echo "git is not installed" >&2; exit 1; }
git --version | cut -d' ' -f1-3
```

## Steps

1. Make a scratch repository with a few commits. The commit dates are
   fixed so the notes below come out the same every run.

   ```sh {#repo}
   export REPO="$(mktemp -d)"
   cd "$REPO"
   git init -q -b main
   git config user.name "Release Bot"
   git config user.email "bot@example.com"
   export GIT_AUTHOR_DATE="2026-01-02T03:04:05Z" GIT_COMMITTER_DATE="2026-01-02T03:04:05Z"
   for msg in "Add parser" "Fix: handle empty input" "Add --version flag"; do
     echo "$msg" >> CHANGES
     git add CHANGES
     git commit -q -m "$msg"
   done
   git log --oneline | wc -l | tr -d ' '
   ```

2. Pick the version. The last tag decides it; with no tags yet, this is
   the first release.

   ```sh {#version}
   last="$(git describe --tags --abbrev=0 2>/dev/null || echo none)"
   if [ "$last" = none ]; then VERSION=v1.0.0; else VERSION="$last-next"; fi
   echo "last tag: $last, next: $VERSION"
   ```

3. Check the working tree is clean. A dirty tree stops the run here.

   ```sh {#clean}
   test -z "$(git status --porcelain)" && echo clean
   ```

4. Write the release notes as a table, one row per commit.

   ```sh {#notes out=markdown}
   md_h3 "Changes in $VERSION"
   { printf 'commit\tsubject\n'; git log --reverse --format='%h%x09%s'; } | md_table
   ```

5. Tag it.

   > Tags are public once pushed. This runbook never pushes, so the tag
   > stays in the scratch repository.
   >
   > ```sh {#tag}
   > git tag -a "$VERSION" -m "Release $VERSION"
   > git tag --list
   > ```

This run tagged **{{#version/env(VERSION)}}**.

## Clean up

The scratch repository is removed only if every step above worked, so a
failed run leaves it behind for a look.

```sh {#cleanup run={{#tag/?}}==0}
cd /
rm -rf "$REPO"
test ! -e "$REPO" && echo removed
```
