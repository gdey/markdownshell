---
title: Environments
description: How blocks share state, and how sub-envs keep it apart.
weight: 30
---

Each env is one long-lived shell process. Blocks without `env=` all run in
the top-level env, so variables, functions and the working directory carry
over like lines of one script.

- **Sub-envs.** `env=build` creates a separate shell on first use and reuses
  it after, so two `env=build` blocks share state. `env=build/test` stacks
  `test` on `build`.
- **What crosses over.** When a sub-env is created it gets the parent's
  *exported* variables that pass `share=`. `share=...` (or `share=*`)
  passes all, `share=...,-TOKEN` passes all but `TOKEN`, and `share=`
  (empty) starts clean. Variables only flow down.
- **Never shared.** Unexported variables, functions, aliases and the
  working directory. Every env starts in the document's directory.

````markdown
```sh {#setup}
export GREETING=hello
NAME=world
```

```sh {#isolated env=sub share=GREETING}
echo "GREETING=$GREETING NAME=${NAME:-unset}"
```

```output {for=isolated exit=0}
GREETING=hello NAME=unset
```
````

## Failures

A block that calls `exit` ends its shell, so the run stops there. A block
whose last command returns non-zero is marked failed and the run continues,
unless `on_error=stop`, `on_error: stop` in front matter, or
`--stop-on-error` says otherwise. mdsh exits 1 if any block failed.

## What blocks can use

Every block sees `$MDSH_BLOCK`, `$MDSH_FILE`, `$MDSH_DIR`, and the script's
arguments as `$1`, `$2` and so on. The helpers `md_h1`, `md_h2`, `md_h3`,
`md_table` (TSV on stdin) and `md_code` print markdown.
