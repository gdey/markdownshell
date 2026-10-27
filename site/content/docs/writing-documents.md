---
title: Writing documents
description: Blocks, attributes, front matter and output kinds.
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
- Every `sh` block runs. Other languages are left alone, and `skip` turns a
  block off.
- mdsh reads only the `mdsh:` key of the front matter, so other tools' keys
  can live alongside it. Block attributes override it.
- Unknown attributes are kept untouched.

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
