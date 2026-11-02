#!/usr/bin/env mdsh
---
title: Build and serve the website
mdsh:
  on_error: stop
---
# Build and serve the website

Builds the project site in `site/` with Hugo and serves it on your
machine, with live reload, so you can see a change before you push it.
Run it from anywhere in a checkout:

```text
mdsh docs/site.md        # in the terminal; Ctrl-C stops the server
mdsh -i docs/site.md     # in the terminal UI
mdsh-gui docs/site.md    # in the desktop app; Stop ends the server
```

Pick what to do and the port below. In the desktop app they are a
control and a text field; elsewhere, edit the checked item or the number.

```choice {#action var=action mode=radio}
- [x] serve
- [ ] build
```

`serve` builds the site and keeps Hugo running at the port, rebuilding as
you edit. `build` writes the finished site to `site/public/` and stops, as
the Website workflow does before it deploys.

```textbox {#port var=port lines=1 placeholder="1313"}
1313
```

## Hugo

The Website workflow builds with Hugo 0.147.9. Another version usually
works, but this says so, since layouts can break between versions.

```sh {#check}
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo ..)/site"
WANT=0.147.9
if command -v hugo >/dev/null; then
  HUGO=ok
  hugo version
  hugo version | grep -q "v$WANT" || echo "note: CI uses Hugo $WANT"
else
  HUGO=missing
  echo "hugo is not installed"
fi
export HUGO WANT
```

Without Hugo, this installs the workflow's version with `go install`,
after asking. It takes a few minutes. `brew install hugo` works too.

```sh {#install depends=check run={{#check/env(HUGO)}}==missing confirm="Install Hugo with go install?"}
command -v go >/dev/null || { echo "Go is not installed: see https://gohugo.io/installation/" >&2; exit 1; }
go install "github.com/gohugoio/hugo@v$WANT"
export PATH="$(go env GOPATH)/bin:$PATH"
hugo version
```

## Build

A clean build, the same command CI runs, so warnings and broken
templates show here first.

```sh {#build depends=check}
rm -rf public
hugo --gc --minify --printPathWarnings
echo "$(find public -name '*.html' | wc -l | tr -d ' ') pages in $(pwd)/public"
```

## Serve

Runs until you stop it. Open the address it prints; pages reload as you
save files under `site/`.

```sh {#serve depends=build run={{$MDSH_VAR_action}}==serve}
PORT="$(printf '%s' "${MDSH_VAR_port:-1313}" | tr -dc 0-9)"
echo "serving on http://localhost:${PORT:=1313}/"
hugo server --port "$PORT" --bind 127.0.0.1
```

<!-- mdsh:if {{$MDSH_VAR_action}}==serve -->
The site is at [http://localhost:{{$MDSH_VAR_port:"1313"}}/](http://localhost:{{$MDSH_VAR_port:"1313"}}/).
<!-- mdsh:end -->
