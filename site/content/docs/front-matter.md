---
title: Front matter
description: Every document-wide option under the mdsh key.
weight: 25
---

Document-wide options go under an `mdsh:` key in YAML front matter at the
very top of the file. mdsh reads only that key, so other tools' keys (such
as `title:`) can sit alongside it. Every option is optional.

```yaml
---
title: Build notes
mdsh:
  shell: /bin/bash
  on_error: stop
  step: false
  interpolate: true
  show_stderr: false
  terminal:
    cols: 100
    rows: 30
  env:
    share: [PATH, HOME]
    source: [~/.zsh_aliases]
---
```

The front matter starts with a `---` line on the first line of the file (or
right after a `#!` line) and ends with a `---` or `...` line. Without a
closing line, the `---` is just a thematic break.

## Options

| Key | Values | Default | What it does |
| --- | --- | --- | --- |
| `shell` | path to a shell | `/bin/sh`, or `zsh` if any block is `zsh` | The shell that runs every block, in every env |
| `on_error` | `continue`, `stop` | `continue` | Whether a failing block stops the run |
| `step` | `true`, `false` | `false` | Ask before running each block, as if every block were marked `confirm` |
| `interpolate` | `true`, `false` | `true` | Whether `{{...}}` placeholders in prose are filled in |
| `show_stderr` | `true`, `false` | unset | Show every block's stderr, or none of it |
| `terminal.cols`, `terminal.rows` | cells | fit the window, 24 | In mdsh-gui, the size of the terminal `tty` blocks run on |
| `env.share` | list of variable names, `...`, `-NAME` | all | Variables passed from mdsh's environment into the shell, and the default `share=` for sub-envs |
| `env.source` | list of file paths | none | Files sourced into every shell before its first block, such as aliases and functions |

### `shell`

The shell binary that runs blocks. It must be a POSIX shell or zsh, since
mdsh defines its helpers (`md_table` and friends) as POSIX shell functions.
When it's unset, a document with any `zsh` block runs in `zsh` and every
other document runs in `/bin/sh`. On Windows, point it at a POSIX shell such
as Git Bash's `bash.exe`.

### `on_error`

With `continue`, a failing block is reported and the run goes on. With
`stop`, the run ends at the first block that exits non-zero, and later
blocks don't run. Any other value is an error.

A block's own `on_error=` attribute overrides this, and the
`--stop-on-error` flag overrides both.

### `step`

`step: true` runs the document a step at a time: before each block that
would run, mdsh shows its code and asks whether to run it, skip it, skip it
only if it already has saved output, or stop. It's the same prompt as a
`confirm` block's; see [Confirming blocks]({{< relref "writing-documents#confirming-blocks" >}}).
Blocks that wouldn't run anyway (`skip`, a false `run=`, non-shell blocks)
aren't asked about.

The `--step` flag turns it on for one run without editing the file, and
`--yes` answers run to every question. It works with `--pdf` too: mdsh asks
about each block, then writes the PDF.

### `terminal`

In mdsh-gui a `tty` block runs on a terminal in its card, as wide as the
card and 24 lines high. `cols:` and `rows:` under `terminal:` set that
size instead, so the block, and its recording, come out the same in any
window; a terminal wider than its card scrolls. The History pane takes
the width too, while its height is what you drag it to. Either may be
left out, and 0 is the same. `cols` goes up to 1000 and `rows` to 500.
`mdsh` itself runs `tty` blocks on the terminal it is in, at that
terminal's size.

### `interpolate`

Placeholders such as `{{#setup/env(NAME)}}` and `{{$USER}}` in prose are
filled in by default. `interpolate: false` leaves them as written, which
helps when a document needs to show `{{...}}` literally. Placeholders in
code blocks and inline code are never touched.

This only affects prose. Conditions in `run=`, `show=` and `mdsh:if`
sections still work.

### `show_stderr`

When unset, a block's stderr is shown only when the block fails.
`show_stderr: true` shows every block's stderr; `show_stderr: false` hides
it even for failed blocks. A block's own `show-stderr` attribute overrides
this.

### `env.share`

Which variables from the environment mdsh was started in reach the
top-level shell. Leave it out to pass everything. A list passes only those
names, `[]` passes nothing (not even `PATH`), and `...` (or `*`) in the
list passes everything. A name with a leading `-` is left out even when
`...` or the plain name is also in the list, so `[..., -USER]` passes
everything except `USER`. Removals only subtract: `[-USER]` on its own
passes nothing, so add `...` when you mean "all but".

The same list is the default `share=` for sub-envs: a block that creates a
sub-env without its own `share=` gets only these exported variables from
its parent. See [Environments]({{< relref "environments" >}}).

```yaml
mdsh:
  env:
    share: [PATH, HOME, USER]
```

```yaml
mdsh:
  env:
    share: [..., -AWS_SECRET_ACCESS_KEY, -GITHUB_TOKEN]
```

### `env.source`

Files sourced into every shell mdsh starts, the top level and each
sub-env, before the shell's first block. Use it for aliases and functions
that blocks call. A path starting with `~/` is in your home directory, and
other relative paths are relative to the document. A missing file stops
the run before any block runs. A file's exit status is ignored, but one
that ends the shell, with `exit`, is an error.

```yaml
mdsh:
  env:
    source: [~/.zsh_aliases, lib/helpers.sh]
```

Files given with `--source` are sourced after these. See
[Aliases and functions]({{< relref "environments#aliases-and-functions" >}}).

## Unknown keys

Keys under `mdsh:` that mdsh doesn't know are ignored, so a typo such as
`on_eror:` silently has no effect. Invalid YAML in the front matter is an
error.
