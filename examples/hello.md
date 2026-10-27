#!/usr/bin/env mdsh
---
title: Hello, mdsh
---
# Hello, mdsh

First we decide what to say and who to say it to. `export` makes
`GREETING` visible to any sub-env we start later; `NAME` is a plain shell
variable, so it stays in this shell.

```sh {#setup}
export GREETING=hello
NAME=world
echo "$GREETING, $NAME"
```

```output {for=setup exit=0}
hello, world
```

Blocks without `env=` all run in the same shell, like lines of one
script, so this block still sees `NAME`. It counts the letters in it:
`printf` avoids a trailing newline, `wc -c` counts bytes, and `tr`
strips the padding some `wc`s add.

```sh {#count}
printf '%s' "$NAME" | wc -c | tr -d ' '
```

```output {for=count exit=0}
5
```

A sub-env is a separate shell that only receives the exported variables
named in `share=`. `GREETING` comes through. `NAME` doesn't, because it
was never exported, so `${NAME:-unset}` falls back to `unset`.

```sh {#isolated env=sub share=GREETING}
echo "GREETING=$GREETING NAME=${NAME:-unset}"
```

```output {for=isolated exit=0}
GREETING=hello NAME=unset
```

A block can also write markdown. `out=markdown` puts its output into the
document as-is, and `md_table` turns tab-separated lines (header first)
into a table.

```sh {#table out=markdown}
printf 'variable\tvalue\nGREETING\t%s\nNAME\t%s\n' "$GREETING" "$NAME" | md_table
```

<!-- mdsh:output for="table" -->
| variable | value |
| --- | --- |
| GREETING | hello |
| NAME | world |

<!-- /mdsh:output -->

A block's output can also be an image. With `out=image`, the block
prints image bytes (PNG, JPEG, GIF or SVG), and mdsh saves them next to
this file in `hello.md.assets/` and links them here. This block draws an SVG banner with plain `printf`:
the bar is 12 pixels wide per letter of the greeting, using shell
arithmetic `$(( ))`.

```sh {#banner out=image}
w=$(( ${#GREETING} * 12 ))
printf '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="40">'
printf '<rect width="%d" height="40" rx="6" fill="#4f7cff"/>' "$w"
printf '<text x="%d" y="26" font-family="sans-serif" font-size="16" fill="#888">%s, %s</text>' "$(( w + 8 ))" "$GREETING" "$NAME"
printf '</svg>\n'
```

<!-- mdsh:output for="banner" -->
![banner](hello.md.assets/banner.svg)
<!-- /mdsh:output -->

To take a picture of plain text output instead, use `out=screenshot`:
whatever the block prints is drawn as a terminal-style image. It's an
SVG by default; ending the path in `.png` asks for a PNG. Here
`printf '%-10s'` pads each name to 10 columns so the values line up.

```sh {#screenshot out=screenshot:hello.md.assets/screenshot.png}
printf '%-10s %s\n' GREETING "$GREETING" NAME "$NAME"
```

<!-- mdsh:output for="screenshot" -->
![screenshot](hello.md.assets/screenshot.png)
<!-- /mdsh:output -->

Some parts are only for debugging. Everything between the
`mdsh:if` and `mdsh:end` comments below is printed only when `DEBUG` is
set (try `DEBUG=1 ./hello.md`), and `run={{$DEBUG}}` keeps the block
from running otherwise. Its stderr is shown too, because of
`show-stderr`; normally stderr only appears when a block fails.

<!-- mdsh:if {{$DEBUG}} -->
**Debug:** the shell is **{{#shell/env(SHELL_NAME):"sh"}}**.

```sh {#shell run={{$DEBUG}} show-stderr}
SHELL_NAME=$(ps -p $$ -o comm= 2>/dev/null || echo unknown)
echo "running in $SHELL_NAME" >&2
```

```output {for=shell stream=stderr}
running in sh
```
<!-- mdsh:end -->

Prose can quote what a block left behind: `setup` set the greeting to
**{{#setup/env(GREETING):"hello"}}** and greeted **{{#setup/env(NAME):"world"}}**.
It can also quote the environment mdsh was started in: this ran as
**{{$USER:"j"}}**.
