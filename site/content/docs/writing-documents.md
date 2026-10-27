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

- `#name` gives a block an id. Unlabeled blocks get `block-N` by position.
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
| `#id` | Block name (default `block-N`) |
| `skip` | Don't run |
| `run=COND` | Run only if the condition holds |
| `show=COND` | Print the code only if the condition holds |
| `show-output=COND` | Print the output only if the condition holds |
| `show-stderr[=COND]` | Show stderr (by default only when the block fails) |
| `env=name` | Run in a named sub-env; `a/b` stacks `b` on `a` |
| `share=A,B` | Exported vars a new sub-env gets from its parent (default all; empty for none) |
| `on_error=stop` | Stop the run if this block fails |
| `confirm[="question"]` | Show the block and ask whether to run it or stop the run |
| `timeout=30s` | Kill the block's shell after this long (stops the run) |
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

## Confirming blocks

A block marked `confirm` pauses the run: mdsh shows its code and asks
`Run this block? [r]un / [s]top`, or your own question from
`confirm="Deploy to prod?"`. Pressing enter stops, which is the safe choice.
The prompt goes to the terminal, so it works even when output is piped. With
no terminal, the run stops there with an error unless you pass `--yes`.

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
