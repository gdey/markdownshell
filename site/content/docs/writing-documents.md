---
title: Writing documents
description: Blocks, attributes, front matter, output kinds and includes.
weight: 20
---

Per-block options sit in the fence's info string, document-wide options sit
under an `mdsh:` key in YAML front matter, and output goes in an `output`
fence right below its block.

````markdown
---
title: Build notes
mdsh:
  on_error: continue
  show_stderr: false
  env:
    share: [PATH, HOME]
---

```sh {#setup}
export GREETING=hello
```

```sh {#build env=build share=GREETING}
echo "$GREETING from a sub-env"
```

```output {for=build exit=0}
hello from a sub-env
```
````

## Rules

- `#name` gives a block an id. When mdsh saves the file (`-w`, or a save in `mdsh -i` or mdsh-gui), it gives each unlabeled block an id of its own, such as `#block-0192f3a4-7c1e-7b2a-9d3e-5f6a7b8c9d0e`, so the id stays the same when blocks move. Until then an unlabeled block is `block-N` by position. Give a block a name to refer to it by something readable.
  Ids must be unique.
- Every `sh` and `zsh` block runs. Other languages are left alone, and
  `skip` turns a block off.
- Blocks run in `/bin/sh`, or in `zsh` when the document has any `zsh`
  block. `shell:` under `mdsh:` in the front matter picks another shell.
  Each env is one shell, so every block in the document runs in it.
- mdsh reads only the `mdsh:` key of the front matter, so other tools' keys
  can live alongside it. Block attributes override it. Every option is
  listed in [Front matter]({{< relref "front-matter" >}}).
- Unknown attributes are kept untouched.
- Blocks can sit inside list items and block quotes, such as a step in a
  numbered list. Their output is written inside the same item or quote.

## Block attributes

| Attribute | Meaning |
| --- | --- |
| `#id` | Block name. An unnamed block gets `block-<uuid>`, written into its fence when mdsh saves the file |
| `skip` | Don't run |
| `run=COND` | Run only if the condition holds |
| `show=COND` | Print the code only if the condition holds |
| `show-output=COND` | Print the output only if the condition holds |
| `show-stderr[=COND]` | Show stderr (by default only when the block fails) |
| `env=name` | Run in a named sub-env; `a/b` stacks `b` on `a` |
| `share=A,B` | Exported vars a new sub-env gets from its parent (default all; empty for none) |
| `on_error=stop` | Stop the run if this block fails |
| `depends=A,B` | Blocks to run first: each one that hasn't run yet is asked about |
| `confirm[="question"]` | Show the block and ask whether to run it, skip it or stop the run |
| `timeout=30s` | Kill the block's shell after this long (stops the run) |
| `tty` | Run on the terminal, for commands like `docker run -it`; the session is saved as an animated GIF |
| `frame=N` | With `tty`, the frame a PDF shows (default the last) |
| `out=markdown` | Insert output as raw markdown |
| `out=image[:path]` | Output is image bytes (PNG, JPEG, GIF, SVG): save it and link it |
| `out=screenshot[:path]` | Draw the text output as a terminal-style image |
| `embed=inline` | With `out=image` or `out=screenshot`, embed a data URI instead of a file |

Blocks marked `<include` take their own attributes; see [Including files](#including-files).

## Output kinds

`text` (the default) fences the output. `markdown` inserts it as-is between
`<!-- mdsh:output for="id" -->` and `<!-- /mdsh:output -->` comments, so a
re-run can replace it:

````markdown
```sh {#table out=markdown}
printf 'name\tvalue\nGREETING\thello\n' | md_table
```

<!-- mdsh:output for="table" -->
| name | value |
| --- | --- |
| GREETING | hello |
<!-- /mdsh:output -->
````

`image` expects stdout to be image bytes. `screenshot` draws stdout as a
terminal-style image: SVG by default, PNG when the path ends in `.png`.
Both save to `<doc>.assets/<block-id>.<ext>` unless you give a path, and
link the image between the same markers.

### Output copies

The output right under a block is its own, and mdsh replaces it on every
run. An `output {for=id}` fence anywhere else in the document, or an
`<!-- mdsh:output for="id" -->` region for `out=markdown` and image
output, is a copy: mdsh refills it with block `id`'s latest output on every
run, like a placeholder, so a summary can repeat what an earlier block
found. `stream=stderr` copies the stderr fence. A copy keeps what it holds
when its block doesn't run, `--hide-output` and `show-output=` hide copies
too, and a copy for a block that doesn't exist is left alone with a
warning.

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
is then asked about, as if it were marked `confirm`. This works the same
with `--pdf`: you answer each prompt, then mdsh writes the PDF, with saved
output for the blocks you skipped. See
[PDF output]({{< relref "pdf#stepping-through-a-run" >}}).

When a run can ask, because of `--step`, `step: true` or a `confirm`
block, mdsh prints the document as it goes rather than all at the end:
the prose and code up to each block before asking about it, and the
block's output, stderr included if it failed, as soon as it finishes.
This happens with `-w` too, so you can follow a run as it is written
back. Without asking, `-w` prints only a status line per block, followed
by the output and stderr of a block that failed.

## Dependencies

`depends=` lists blocks, by id, that a block needs to have run first:

````markdown
```sh {#build}
make
```

```sh {#deploy depends=build,login}
./deploy.sh
```
````

When `deploy` is about to run, mdsh checks `build` and then `login`, in
that order. A block that has already run in this session (in `mdsh -i`
or mdsh-gui; in a plain `mdsh` run, earlier in the run) and didn't fail
is passed over. For any other, mdsh asks
`Run build first? [r]un / s[k]ip / [s]top`:

- `r` checks that block's own `depends=` the same way, then runs it. Its
  `skip`, `run=` and `confirm` don't apply, since you just said to run it.
- `k` goes on without it.
- `s`, or just enter, runs neither it nor `deploy`.

If a dependency fails, `deploy` doesn't run. In a whole run, the run then
goes on or stops as the dependency's `on_error` says, and a block already
run as a dependency isn't run again when the run reaches it. With no
terminal the run stops with an error, unless you pass `--yes`, which runs
dependencies without asking. An id that names no block, a block that
depends on itself and a cycle are errors when the file is read. mdsh-gui
and `mdsh -i` ask the same question in their own way.

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

A `tape` fence types the input for a `tty` block, in the commands of a
[VHS](https://github.com/charmbracelet/vhs) tape, so the block runs with
nobody at the keyboard, in CI or under `-w`:

````markdown
```sh {#repl tty}
python3 -q
```

```tape {for=repl}
Wait />>>$/
Type "print(6 * 7)"
Enter
Wait />>>$/
Type "exit()"
Enter
```
````

Or read it from a file: ```` ```tape {for=repl <include file=demo.tape} ````.
mdsh supports `Type`, the named keys (`Enter`, `Tab`, `Up`, …) with a
count, `Ctrl+X` and `Alt+X`, `Sleep`, `Wait /regex/` and
`Set TypingSpeed`, `WaitTimeout` and `WaitPattern`, and skips the rest of
VHS's commands. Double-quoted `Type` strings can hold placeholders,
`{{$VAR}}` or `{{#block/env(VAR)}}`, filled in as the tape plays, for
passwords and tokens that shouldn't be written into the document. Single
quotes or backticks type the text as it is. A taped block runs on an 80x24 terminal so its recording
is the same everywhere. Start with a `Wait` or `Sleep`, since keys go out
as soon as the block starts. [examples/terminal.md](https://github.com/gdey/markdownshell/blob/main/examples/terminal.md) shows each of
these, with the recordings they make.

With no terminal (cron, CI) or with mdsh in the background, a `tty` block
without a tape fails with an error instead of hanging.

## Including files

A block marked `<include` holds lines from another file, so a doc can show
real code without a copy that goes stale. mdsh reads the file on every run
and writes the lines into the block, so the document reads well anywhere
markdown does, GitHub included.

````markdown
```go {<include file=cmd/main.go from=10 to=24 number_on}
func main() {
	...
}
```
````

| Attribute | Meaning |
| --- | --- |
| `<include` | Marks the block as an include |
| `file=path` | The file to include, relative to the document's directory (required) |
| `from=N` | First line to include, counting from 1; `0` (the default) is the start of the file |
| `to=N` | Last line to include, inclusive; the default is the end of the file |
| `number_on` | Show each line with its line number in the file |

- The fence's language is the language the lines are shown in.
- An include block never runs, even as `sh` or `zsh`, and has no name: `#id`,
  `env=`, `share=`, `run=` and the other step attributes are errors on it,
  and placeholders can't refer to it.
- The body belongs to mdsh. Each run replaces it with the file's current
  lines, so edits made by hand are lost.
- `number_on` is for display. Printed markdown starts each line with its
  number; write-back stores the plain lines, and other renderers can
  number them their own way.
- Includes are read on every run, even with `--only`, `--hide-code` or
  `--hide-output`. `skip` leaves the stored lines alone and `show=` hides
  the block. A missing file or a `from=` past the end of the file is an
  error, shown below the block.

[examples/include.md](https://github.com/gdey/markdownshell/blob/main/examples/include.md)
in the repository shows each option.
