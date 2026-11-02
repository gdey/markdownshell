#!/usr/bin/env mdsh
---
title: Inputs
---
# Inputs

Some runbooks need a value from whoever runs them: a host name, an
environment, which checks to run. A `textbox` or `choice` block holds
that value in the document, so the file is still plain markdown, and
mdsh-gui draws it as a control to type in or pick from. Everywhere
else, the fence shows as code, and the value is whatever the body says.

With `var=name`, the input binds `$MDSH_VAR_name` for every block,
condition and placeholder. Edit the bodies below, or change them in
mdsh-gui, and run the file again to see the blocks follow.

## A textbox

`lines=` is how many lines it holds, and `placeholder=` is shown while
it is empty, in mdsh-gui only.

```textbox {#host var=host lines=1 placeholder="Which host?"}
staging.example.com
```

## Choices

A `choice` block lists options as task list items, and the checked ones
are the value. `mode=radio` picks one of a group, `mode=dropdown` (the
default) picks one from a list, and `mode=multi` any number.

```choice {#env var=env mode=radio}
- [ ] staging
- [x] prod
```

```choice {#checks var=checks mode=multi}
- [x] disk
- [ ] memory
- [x] network
```

## Blocks that read them

A multi-choice value is one label per line.

```sh {#plan}
echo "host: $MDSH_VAR_host"
echo "env: $MDSH_VAR_env"
printf '%s\n' "$MDSH_VAR_checks" | sed 's/^/check: /'
```

```output {for=plan exit=0}
host: staging.example.com
env: prod
check: disk
check: network
```

Conditions read the same variables, so a block can run only for one
pick, and a section can show only then.

```sh {#guard run={{$MDSH_VAR_env}}==prod}
echo "prod: a real runbook would ask for a second confirmation here"
```

```output {for=guard exit=0}
prod: a real runbook would ask for a second confirmation here
```

<!-- mdsh:if {{$MDSH_VAR_env}}==prod -->
> **This run targets prod.** The paragraph you are reading is inside an
> `mdsh:if` section, so it is printed only when prod is picked.
<!-- mdsh:end -->

Prose can quote an input too: this run checks **{{$MDSH_VAR_host:"staging.example.com"}}**.

## Variables a block sets

A block can bind a variable too, for the blocks after it, with
`mdsh ctl set`. It talks back to the run that started it over
`$MDSH_SOCK`, so it needs `mdsh` on your `PATH`, and works on macOS and
Linux. The variable is `$MDSH_VAR_target` from the next block on, and
conditions read it like any input's. A name an input binds, such as
`host`, is refused, so the file and the variable can't disagree.

```sh {#derive}
mdsh ctl set target "$MDSH_VAR_host:443"
mdsh ctl get target
```

```output {for=derive exit=0}
staging.example.com:443
```

```sh {#connect run={{$MDSH_VAR_target}}}
echo "would connect to $MDSH_VAR_target"
```

```output {for=connect exit=0}
would connect to staging.example.com:443
```

## Folding

A long document opens tidier when the parts most people skip start
folded. These only change how mdsh-gui first draws the file: `collapse`
on a block folds its code to one line, `collapse-output` folds its
output, and an `<!-- mdsh:collapse -->` line folds the heading section
it is in. Folding by hand in mdsh-gui writes these for you.

### Details
<!-- mdsh:collapse -->

```sh {#details collapse}
echo "this block opens folded in mdsh-gui"
echo "so does the section it is in"
```

```output {for=details exit=0}
this block opens folded in mdsh-gui
so does the section it is in
```
