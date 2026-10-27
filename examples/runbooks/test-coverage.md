#!/usr/bin/env mdsh
---
title: Test coverage report
mdsh:
  on_error: stop
---
# Test coverage report

Runs mdsh's tests with coverage and writes the results here as markdown.
Run it with `mdsh -w examples/runbooks/test-coverage.md` to refresh the
report in place, or `mdsh -o coverage.md …` to keep this file as is.

The tests run from the repository root. `TestRunbooks` is skipped,
because it runs this file and would start the tests again.

```sh {#setup}
command -v go >/dev/null || { echo "go is not installed" >&2; exit 1; }
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo ../..)"
test -f go.mod
PROFILE="$(mktemp)"
go version | cut -d' ' -f3
```

## By package

```sh {#packages out=markdown timeout=5m}
go test -skip '^TestRunbooks$' -coverprofile="$PROFILE" ./... |
  awk '
    BEGIN { print "| Package | Coverage |"; print "| --- | ---: |" }
    $1 == "ok" || $1 == "?" || $1 ~ /^github.com\// {
      pkg = ($1 ~ /^github/) ? $1 : $2; sub(/^github.com\/gdey\/markdownshell\/?/, "", pkg)
      if (pkg == "") pkg = "."
      cov = "no tests"
      if (match($0, /coverage: [0-9.]+%/)) cov = substr($0, RSTART + 10, RLENGTH - 10)
      printf "| `%s` | %s |\n", pkg, cov
      n++
    }
    END { if (n == 0) { print "no packages in the go test output" > "/dev/stderr"; exit 1 } }'
```

Total, across every statement: **{{#total/env(TOTAL)}}**.

```sh {#total}
TOTAL="$(go tool cover -func="$PROFILE" | awk '$1 == "total:" { print $3 }')"
test -n "$TOTAL" && echo "$TOTAL"
```

## Least covered functions

The ten functions with the lowest coverage, ignoring ones at 100%.

```sh {#least out=markdown}
go tool cover -func="$PROFILE" |
  awk '$1 != "total:" && $3 != "100.0%" { f = $1; sub(/^github.com\/gdey\/markdownshell\//, "", f); sub(/:$/, "", f); printf "%s\t%s\t%s\n", $3 + 0, f, $2 }' |
  sort -n | head -n 10 |
  awk -F'\t' 'BEGIN { print "| Coverage | Where | Function |"; print "| ---: | --- | --- |" }
              { printf "| %s%% | `%s` | `%s` |\n", $1, $2, $3 }'
```

```sh {#cleanup show=false}
rm -f "$PROFILE"
```
