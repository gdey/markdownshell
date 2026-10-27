# mdsh

mdsh runs the code blocks in a markdown file, like a notebook whose file
format is plain markdown. The documentation lives at
**[gdey.github.io/markdownshell](https://gdey.github.io/markdownshell/)**.
See [docs/spec.md](docs/spec.md) for the design and
[docs/plan.md](docs/plan.md) for the roadmap.

Here is [examples/hello.md](examples/hello.md), written the literate way: prose
explains each block, and the output under each block was written back by
`mdsh -w`.

````markdown
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
````

The two images it produced:

![banner](examples/hello.md.assets/banner.svg)

![screenshot](examples/hello.md.assets/screenshot.png)

More complete runbooks live in [examples/runbooks](examples/runbooks): cutting
a release, a system report, Linux and macOS host triage, and a test
coverage report for mdsh itself. `go test ./cmd/mdsh` runs each of them; a
name ending in `_linux.md` or `_darwin.md` runs only on that OS.

## Install

Download an archive for your system from the
[releases page](https://github.com/gdey/markdownshell/releases), unpack it,
and put `mdsh` on your `PATH`. Builds cover Linux, macOS and Windows on
amd64 and arm64; `checksums.txt` lists each archive's SHA-256. For example,
on Linux amd64:

```sh
VERSION=1.0.0
curl -LO https://github.com/gdey/markdownshell/releases/download/v$VERSION/mdsh_${VERSION}_linux_amd64.tar.gz
tar xzf mdsh_${VERSION}_linux_amd64.tar.gz mdsh
sudo mv mdsh /usr/local/bin/
mdsh --version
```

On macOS, a downloaded binary may be quarantined; `xattr -d
com.apple.quarantine mdsh` clears it. Blocks run in `/bin/sh` by default;
on Windows, set `shell:` under `mdsh:` in the front matter to a POSIX shell
such as Git Bash's `bash.exe`. The Windows builds are not well tested yet.

With Go 1.24 or newer you can build it instead:

```sh
go install github.com/gdey/markdownshell/cmd/mdsh@latest
```

## Usage

```
mdsh notes.md              # run, print markdown with output under each block
mdsh -w notes.md           # run, write output back into the file
mdsh -o out.md notes.md    # same, but into out.md; notes.md is untouched
mdsh --hide-code notes.md  # prose and output only
mdsh --hide-output notes.md
mdsh --hide-code --hide-output notes.md   # prose only, nothing runs
mdsh -n notes.md           # print with the output saved by -w; nothing runs
mdsh -n -o view.md notes.md  # save that printout to view.md instead
mdsh --pdf notes.pdf notes.md   # what it would print, as a PDF
mdsh --only greet notes.md # one block, plus what it depends on
mdsh --yes notes.md        # run confirm blocks without asking
mdsh --step notes.md       # ask before every block: run, skip or stop
```

Shell completion covers flags, `.md` files and, after `--only`, the
file's block ids:

```
source <(mdsh completion bash)                 # bash, e.g. in ~/.bashrc
mdsh completion zsh > "${fpath[1]}/_mdsh"      # zsh, then restart the shell
source <(mdsh completion zsh)                  # or zsh, after compinit in ~/.zshrc
```

Printed markdown is styled when stdout is a terminal; `--color=never`,
`--no-color` or `NO_COLOR` turn that off.

## PDF output

`--pdf FILE` lays out what mdsh would print as a PDF, so a run can be
shared as a report. The hide flags and `--only` apply, and with `-w` or
`-o` the markdown is written too:

```sh
mdsh --pdf report.pdf report.md                 # run, write the PDF
mdsh --hide-code --pdf report.pdf report.md     # prose and output only
mdsh -w --pdf report.pdf report.md              # update the file and the PDF
mdsh -n --pdf report.pdf report.md              # the output saved by -w; nothing runs
```

It's pure Go, with no external tools. Output shows in a terminal-style box,
stderr in red, `out=screenshot` output as text, and `number_on` includes
with a line-number gutter. [examples/report.md](examples/report.md) shows
each of these, and [examples/pdf](examples/pdf) holds the PDFs of it,
[hello.md](examples/hello.md) and [include.md](examples/include.md).

## Blocks

Every `sh` and `zsh` block runs, top to bottom. Blocks without `env=` share
one shell, so variables, functions and the working directory carry over.
Blocks run in `/bin/sh` unless `shell:` under `mdsh:` in the front matter
says otherwise; a document with any `zsh` block runs in `zsh` by default.

| Attribute | Meaning |
| --- | --- |
| `#id` | Block name (default `block-N`) |
| `skip` | Don't run |
| `run=COND` | Run only if the condition holds |
| `show=COND` | Print the code only if the condition holds |
| `show-output=COND` | Print the output only if the condition holds |
| `show-stderr[=COND]` | Show stderr (by default it only shows when the block fails) |
| `env=name` | Run in a named sub-env; `a/b` stacks `b` on `a` |
| `share=A,B` | Exported vars a new sub-env gets from its parent (default all; empty for none) |
| `on_error=stop` | Stop the run if this block fails |
| `confirm[="question"]` | Show the block and ask whether to run it, skip it or stop the run |
| `timeout=30s` | Kill the block's shell after this long (stops the run) |
| `tty` | Run on the terminal, for commands like `docker run -it`; the session is saved as an animated GIF |
| `frame=N` | With `tty`, the frame a PDF shows (default the last) |
| `out=markdown` | Insert output as raw markdown |
| `out=image[:path]` | Output is image bytes (PNG, JPEG, GIF, SVG): save it and link it |
| `out=screenshot[:path]` | Draw the text output as a terminal-style image: SVG, or PNG if the path ends in `.png` |
| `embed=inline` | With `out=image` or `out=screenshot`, embed a data URI instead of a file |

## Including files

A block marked `<include` holds lines from another file, so a doc can quote
real code without a copy that goes stale:

````markdown
```go {<include file=cmd/main.go from=10 to=24 number_on}
```
````

Each run writes the file's lines into the block, so it reads well on
GitHub too. `file=` is relative to the document. `from=` and `to=` are line
numbers counted from 1 and inclusive; `from` defaults to the start of the
file and `to` to the end. The fence's language is the language, and
`number_on` shows line numbers when the document is printed. An include
block never runs and has no name. [examples/include.md](examples/include.md)
shows each option.

## Confirming blocks

A block marked `confirm` pauses the run: mdsh shows its code and asks
`Run this block? [r]un / s[k]ip / [o]nly run blocks without saved output / [s]top`,
or your own question from `confirm="Deploy to prod?"`.

- `r` runs the block.
- `k` skips it and goes on. A skipped block keeps any output already saved
  in the file.
- `o` skips this block and every later one that already has saved output,
  and runs the rest. Blocks without saved output are still asked about.
  The prompt says `(has saved output)` when a block has some.
- `s`, or just enter, stops the run there. This is the safe choice.

The prompt goes to the terminal, so it works even when output is piped.
With no terminal, the run stops there with an error unless you pass `--yes`.

To step through a whole document this way, pass `--step` or set
`step: true` under `mdsh:` in the front matter. Every block that would run
is then asked about, as if it were marked `confirm`.

When a run can ask, because of `--step`, `step: true` or a `confirm`
block, mdsh prints the document as it goes rather than all at the end:
the prose and code up to each block before asking about it, and the
block's output, stderr included if it failed, as soon as it finishes.
This happens with `-w` too, so you can follow a run as it is written
back. Without asking, `-w` prints only a status line per block, followed
by the output and stderr of a block that failed.

## Terminal blocks

Some commands need a terminal: `docker run -it`, `ssh`, `sudo`, an
editor, a REPL. Mark their block `tty` and mdsh runs it on a terminal,
passing your keys through:

````markdown
```sh {#shell tty}
docker run -it --rm alpine sh
```
````

The block still runs in its env's shell, so variables and the working
directory carry over as usual. Ctrl+c goes to the command, not to mdsh,
and window resizes are passed on.

mdsh records the session as an [asciinema](https://asciinema.org) cast,
`<doc>.assets/<block-id>.cast` (or `out=recording:path` with `.cast`),
and makes an animated GIF from it beside the cast, linked below the
block, since GitHub and PDFs can't play a cast. A PDF can't animate, so it shows the last frame; set
`frame=N` to show frame `N` instead, counting from 1.

With no terminal (cron, CI) or with mdsh in the background, a `tty` block
fails with an error instead of hanging.

## Conditions

A condition is a placeholder without the `:"..."` part, optionally
compared with `==` or `!=`, or negated with `!`:

| Condition | True when |
| --- | --- |
| `{{$DEBUG}}` | `DEBUG` in mdsh's environment is set and not `0`, `false`, `no` or `off` |
| `!{{$QUIET}}` | the opposite |
| `{{#detect/env(OS)}}==Linux` | block `detect` left `OS` set to `Linux` |
| `{{$?}}==0` | the last block that ran exited 0 (see below) |
| `{{#build/?}}!=0` | block `build` ran and failed |
| `{{$err?}}`, `{{#build/err?}}` | the same statuses: true when non-zero, so when the block failed |
| `{{$ran?}}`, `{{#build/ran?}}` | the last runnable block, or block `build`, ran (not skipped) |
| `true`, `false` | always, never |

Use them in `run=`, `show=`, `show-output=` and `show-stderr=`, and to
show or hide whole stretches of the document:

```markdown
<!-- mdsh:if {{$DEBUG}} -->
Prose and blocks here are printed only when DEBUG is set.
<!-- mdsh:end -->
```

`{{$?}}` is the exit status of the last block that ran before it. In
`run=` that is the previous block, so `run={{$?}}==0` runs a block only
if the one before it succeeded. `show=`, `show-output=` and `show-stderr=`
are checked after the block runs, so there it is the block's own status:
`show-output={{$?}}!=0` shows output only on failure. `{{#build/?}}`
names the block instead. `{{$ran?}}` works the same way, so
`run=!{{$ran?}}` is a fallback for a block that was skipped, and
`{{$err?}}` is another name for `{{$?}}` that reads well as a condition.

Sections nest, and their comment lines never appear in printed output.
`show=`, `show-output=` and sections only change what is printed: `-w`
keeps everything in the file. Use `run=` to keep a block from running.

## Placeholders

`{{#setup/env(NAME)}}` in the prose stands for the value `NAME` had right
after block `setup` ran (exported or not), and `{{$NAME}}` for `NAME` in
the environment mdsh was started in. `{{#setup/?}}`, `{{$?}}`, `err?` and
`ran?` stand for exit statuses and whether a block ran, as in conditions. Printed output shows the value;
`-w` stores it in the file as `{{#setup/env(NAME):"world"}}`, so the
markdown reads well without running it and keeps the last value when the
block doesn't run. Placeholders in code blocks and inline code are left
alone. Turn them off with `interpolate: false` under `mdsh:` in the front
matter.

Every front matter option is listed on the
[Front matter](https://gdey.github.io/markdownshell/docs/front-matter/) page.

## Shell details

A block's stderr is captured separately. It is shown only when the block
fails, unless `show-stderr` (or `show_stderr: true` under `mdsh:` in the
front matter) says otherwise.

An explicit `exit` ends the shell and stops the run. Blocks can use
`$MDSH_BLOCK`, `$MDSH_FILE`, `$MDSH_DIR`, the script's arguments as `$1`…,
and the helpers `md_h1`, `md_h2`, `md_h3`, `md_table` (TSV on stdin) and
`md_code`.
