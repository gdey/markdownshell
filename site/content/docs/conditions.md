---
title: Conditions and placeholders
description: Switch blocks and sections on and off, and quote results in prose.
weight: 40
---

## Conditions

A condition is a placeholder without the `:"..."` part, optionally compared
with `==` or `!=`, or negated with `!`:

| Condition | True when |
| --- | --- |
| `{{$DEBUG}}` | `DEBUG` in mdsh's environment is set and not `0`, `false`, `no` or `off` |
| `!{{$QUIET}}` | the opposite |
| `{{#detect/env(OS)}}==Linux` | block `detect` left `OS` set to `Linux` |
| `true`, `false` | always, never |
| `once` | in `run=` only: the block has no output yet |

Use them in `run=`, `show=`, `show-output=` and `show-stderr=`, and to show
or hide whole stretches of the document:

```markdown
<!-- mdsh:if {{$DEBUG}} -->
Prose and blocks here are printed only when DEBUG is set.
<!-- mdsh:end -->
```

`run=once` runs a block only while it has no output: none saved after it
in the file, and none from running it earlier in the same `mdsh -i` or
`mdsh-gui` session. Use it for a slow setup or a one-time fetch; delete
its output to run it again. Picking the block by hand still runs it.

Sections nest, and their comment lines never appear in printed output. Only
`run=` changes what runs; the others change what is printed, and `-w` keeps
everything in the file.

## Placeholders

`{{#setup/env(NAME)}}` in prose stands for the value `NAME` had right after
block `setup` ran, exported or not. `{{$NAME}}` stands for `NAME` in the
environment mdsh was started in.

Printed output shows the value. `-w` keeps the placeholder and stores the
value inside it, `{{#setup/env(NAME):"world"}}`, so the markdown reads well
without running it and keeps the last value when the block doesn't run.

Placeholders inside code are left alone. Turn them off with
`interpolate: false` under `mdsh:` in the front matter.
