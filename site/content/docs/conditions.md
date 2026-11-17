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
| `{{$DEBUG}}` | `DEBUG` is exposed (see below), set, and not `0`, `false`, `no` or `off` |
| `!{{$QUIET}}` | the opposite |
| `{{#detect/env(OS)}}==Linux` | block `detect` left `OS` set to `Linux` |
| `{{#build/?}}!=0` | block `build` failed; `{{$?}}` is the last block's status |
| `{{$MDSH_VAR_env}}==prod` | an [input]({{< relref "inputs" >}}) or `mdsh ctl set` left `env` as `prod` |
| `{{#login/status}}==201` | [http block]({{< relref "http" >}}) `login` got that status |
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
environment mdsh was started in, if you exposed it.

Printed output shows the value. `-w` keeps the placeholder and stores the
value inside it, `{{#setup/env(NAME):"world"}}`, so the markdown reads well
without running it and keeps the last value when the block doesn't run.

The other forms read a block's result rather than a variable:

| Form | Means |
| --- | --- |
| `{{$?}}`, `{{#build/?}}` | The exit status of the last block that ran, or of block `build`. In `run=`, `{{$?}}` is the previous block's status; in `show=`, `show-output=` and `show-stderr=` it is the block's own |
| `{{$err?}}`, `{{#build/err?}}` | The same statuses, read as true when non-zero |
| `{{$ran?}}`, `{{#build/ran?}}` | Whether the last runnable block, or `build`, ran |
| `{{$MDSH_VAR_name}}`, `{{$MDSH_MODE}}` | An input's value, or what `mdsh ctl set` set; `running`, `interactive` or `report`. mdsh's own `MDSH_` variables need no exposing |
| `{{#login/status}}`, `{{#login/json($.id)}}`, `{{#login/header(Content-Type)}}` | The status, a value from the JSON body, or a header of the response an [http block]({{< relref "http#the-response-in-prose-and-conditions" >}}) got |

## Exposing variables

<div class="note">

**Changed in 1.3.** `{{$VAR}}` reads a variable from your environment
only when you expose it. A document that used `{{$USER}}` or
`run={{$DEBUG}}` with 1.2 needs `--expose USER,DEBUG` now.

</div>

A document is code someone may have sent you, so it doesn't get your
environment for free. Name the variables it may read:

```sh
mdsh --expose DEBUG notes.md          # one; repeat the flag for more
mdsh --expose USER,DEBUG notes.md     # or a comma-separated list
MDSH_EXPOSE=DEBUG mdsh notes.md       # the same, from the environment
```

This covers `{{$VAR}}` in prose, in `run=`, `show=` and `mdsh:if`
conditions, and in tapes. mdsh's own `MDSH_` variables need nothing. If
the front matter has `env.share`, a variable must also pass it. An
unexposed `{{$VAR}}` prints as written and reads as unset, so
`run={{$DEBUG}}` is false.

Saving (`-w`, `-o`, `mdsh -i` and the desktop app) never stores a value
from your environment in the file, and drops one an older mdsh stored.
`{{#block/env(NAME)}}` placeholders are unaffected: they quote what a
block in the document set, not your environment.

## Turning placeholders off

Placeholders inside code are left alone. Turn them off with
`interpolate: false` under `mdsh:` in the front matter.
