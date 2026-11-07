---
title: Talking back to mdsh
description: mdsh ctl, for a block that sets variables or reports progress, and the audit log of a run.
weight: 45
---

A block can talk back to the run that started it with `mdsh ctl`: set a
variable for the blocks after it, read one back, or say how far along it
is. And a run can write an audit log, so a run that a block changed can be
reviewed afterwards.

`mdsh ctl` can't add, edit or run blocks, and it can't change prose. You
can still read the file and know what can run; a variable it sets can
only decide *whether* a written block runs, through a `run=` condition,
and the audit log records that.

## mdsh ctl

````markdown
```sh {#pick}
latest=$(git describe --tags --abbrev=0)
mdsh ctl set release "$latest"
mdsh ctl status "found $latest"
```

```sh {#ship run={{$MDSH_VAR_release}}}
./ship "$MDSH_VAR_release"
```
````

| Command | Does |
| --- | --- |
| `mdsh ctl set NAME VALUE` | Sets `$MDSH_VAR_NAME` for blocks that start afterwards. `-` as the value reads stdin |
| `mdsh ctl set --secret NAME VALUE` | The same, and keeps the value out of the file and the log |
| `mdsh ctl unset NAME` | Removes a variable that `set` made |
| `mdsh ctl get NAME` | Prints `$MDSH_VAR_NAME` as it is now, including values set after this block started |
| `mdsh ctl status TEXT` | Shows progress for this block. `--step N --of M` adds a count |

`NAME` is letters, digits and `_`, without the `MDSH_VAR_` in front.

- `set` changes the variable for blocks that *start* afterwards, in every
  [env]({{< relref "environments" >}}). A block that is already running
  keeps the environment it started with; it can read the new value with
  `get`.
- `run=` and other [conditions]({{< relref "conditions" >}}) are checked
  just before their block runs, so they see every `set` made before it.
- A name that a [textbox or choice]({{< relref "inputs" >}}) binds with
  `var=` belongs to that input, and `set` on it is refused. The exception
  is a `nosave` input: `set` fills it for this session, and the file keeps
  its body.
- A `--secret` value reaches blocks and conditions as usual. A
  placeholder shows it as `***`, and write-back stores `***`, never the
  value.

The status line goes to stderr under `mdsh`; it is never written to the
file.

### Exit codes

| Code | Means |
| --- | --- |
| 0 | Done |
| 2 | Refused: bad arguments, a name an input binds, or a token that has stopped working |
| 3 | Not running under mdsh |

`status` is the exception to 3: outside mdsh it does nothing and exits
0, so a script that reports progress still runs on its own.

### How it reaches mdsh

At the start of a run, or of a session in `mdsh -i` or mdsh-gui, mdsh
opens a socket in a new private temp directory, and every block gets two
variables, whatever its env shares:

- `$MDSH_SOCK`, the socket's path.
- `$MDSH_TOKEN`, a token for this run of this block. It stops working
  when the block ends, so mdsh knows which block made each call, even
  from a process the block left in the background.

Only your own user can call it. On Windows there is no socket yet, so
`mdsh ctl` exits 3 there.

## The audit log

```sh
mdsh --audit run.jsonl deploy.md
jq -c 'select(.event == "block.end")' run.jsonl
```

`--audit FILE` writes one JSON object per line, in order, for the whole
run, to a file only you can read. Each has `seq`, a time `t` and an
`event`, with `block` and `env` when they apply.

| Event | Records |
| --- | --- |
| `run.start` / `run.end` | The file and a SHA-256 of it, the mode, flags and arguments, the mdsh version; then the exit code |
| `env.start` | An env's shell starting, and the names `share` let through (names, not values) |
| `block.start` / `block.end` | A block running, then its exit code and time taken |
| `block.skip` | A block not run, and why: `skip`, the `run=` condition and the value it saw, `once`, `--only`, a failed `depends=` |
| `ctl` | Every `mdsh ctl` call, refused ones too, with the block, arguments and result |
| `var` | A variable changing, with the old and new values and what changed it |
| `write` | mdsh saving the document |

A secret value never appears in the log. Where it would go, the log has
`{"secret":true,"hmac":"…"}`, keyed with a new random key each run, so
you can tell whether two secrets in one run matched without being able to
guess a short one.

`--audit` works for `mdsh`, `-w`, `-o` and reports. It is refused with
`-i` for now, and mdsh-gui doesn't save a log yet.
