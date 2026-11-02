#!/usr/bin/env mdsh
---
title: Guard rails
mdsh:
  on_error: stop
---
# Guard rails

A runbook needs more than blocks that run in order. It needs to stop
when a step fails, ask before the risky step, run a slow step only once,
and show at the end what the steps found. This document uses each of
those; run it with `mdsh guardrails.md` and answer the question, or with
`mdsh --yes guardrails.md` to answer yes to everything, as CI would.

`on_error: stop` in the front matter makes any failing block end the
run; a block can opt out with `on_error=continue`, as one does below.

## A step that only runs once

`run=once` runs a block only while it has no saved output. Once `-w`
has written its output, later runs skip it, so a slow fetch or a setup
step happens once. Delete the output fence to run it again. In `mdsh -i`
and mdsh-gui, picking the block by hand always runs it.

```sh {#fetch run=once}
echo "fetched 3 items"
```

```output {for=fetch exit=0}
fetched 3 items
```

## A step that asks first

`confirm` shows the block and asks whether to run it, skip it or stop
the run. The attribute's value is the question; without one, mdsh asks
"Run this block?". With no terminal, the run stops here unless `--yes`
is passed.

```sh {#login confirm="Log in to the demo?"}
TOKEN=demo-token
echo "logged in"
```

```output {for=login exit=0}
logged in
```

## A step with dependencies

`depends=` names blocks that must have run first, in order. One that
has run in this session and didn't fail is passed over; for any other,
mdsh asks `Run login first?`. So `mdsh --only deploy guardrails.md`
still logs in before it deploys. A dependency that fails keeps this
block from running.

```sh {#deploy depends=login,fetch}
echo "deploying with ${TOKEN:-no token}"
```

```output {for=deploy exit=0}
deploying with demo-token
```

## A step that can't hang

`timeout=` kills the block's shell when the time is up and stops the
run, so a stuck command can't hold a runbook forever. This one finishes
well inside its limit.

```sh {#health timeout=10s}
sleep 0.2
echo "healthy"
```

```output {for=health exit=0}
healthy
```

## A step that is allowed to fail

`on_error=continue` on a block overrides the front matter: this check
fails, mdsh shows its exit status and its stderr, and the run goes on.

```sh {#lint on_error=continue}
echo "linting"
echo "warning: 2 unused variables" >&2
false
```

```output {for=lint exit=1}
linting
```

```output {for=lint stream=stderr}
warning: 2 unused variables
```

A later block can read that status: `{{#lint/?}}` is `lint`'s exit
status, so this block runs only because `lint` failed. `{{$?}}` would
read the status of whichever block ran last.

```sh {#on-failure run={{#lint/?}}!=0}
echo "lint failed with status {{#lint/?}}, filing a note"
```

```output {for=on-failure exit=0}
lint failed with status {{#lint/?}}, filing a note
```

Placeholders in code are left alone, which is why the block above
printed the braces: the shell saw them, mdsh didn't touch them. Prose
is different. The lint step exited with **{{#lint/?:"1"}}**, and the deploy
step with **{{#deploy/?:"0"}}**.

## A step kept for reference

`skip` keeps a block in the document without running it. A rollback is
the usual case: it is here to copy into a shell when something goes
wrong, never run as part of the runbook.

```sh {#rollback skip}
echo "rolling back to the previous release"
```

## Summary

An `output` fence that isn't right under its block is a copy: mdsh
refills it from that block on every run, so a summary at the end can
repeat what an earlier step printed.

```output {for=deploy exit=0}
deploying with demo-token
```
