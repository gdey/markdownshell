---
title: Inputs
description: Textboxes and choices, the form controls a document can carry.
weight: 22
---

Some runbooks need a value from whoever runs them: a host name, an
environment, which checks to run. A `textbox` or `choice` fence holds that
value in the document, so the file stays plain markdown. mdsh-gui draws it
as a control you type in or pick from, and writes what you enter back into
the fence. Everywhere else, including GitHub and `mdsh -i`, it shows as a
code block, and its value is whatever the body says.

{{< shot src="form-light.png" dark="form-dark.png" title="deploy-form.md" >}}A textbox, a dropdown, radio buttons and checkboxes, and a block that reads them.{{< /shot >}}

That window is this file:

````markdown
```textbox {#host var=host placeholder="Which host?"}
web-3.example.com
```

```choice {#env var=env placeholder="Environment"}
- [ ] staging
- [x] prod
```

```choice {#strategy var=strategy mode=radio}
- [x] rolling
- [ ] blue-green
```

```choice {#checks var=checks mode=multi}
- [x] health
- [ ] smoke tests
- [x] error rate
```

```sh {#plan}
echo "deploy $MDSH_VAR_strategy to $MDSH_VAR_host ($MDSH_VAR_env)"
printf '%s\n' "$MDSH_VAR_checks" | sed 's/^/  then check: /'
```
````

{{< shot src="form-source-light.png" dark="form-source-dark.png" title="deploy-form.md · source and rendered" >}}The same file with the source view open: each control is a fence.{{< /shot >}}

## Textboxes

A `textbox` fence holds text. Its body is the value.

| Attribute | Meaning |
| --- | --- |
| `var=name` | Bind `$MDSH_VAR_name` to the text, without its last newline |
| `lines=N` | The most lines it holds; 1 or less is a single line, the default |
| `lines=fit` | Any number of lines, in a box as tall as the text that grows and shrinks as you type |
| `placeholder="…"` | Shown while it is empty, in mdsh-gui only |
| `nosave` | What you type is used for this session and not saved; the body is the default |
| `show=COND` | Show it only when the condition holds |

mdsh-gui writes what you type into the body, using a longer fence if a
line would close it. Drag the grip under a textbox, or focus it and
press down and up, to make the box taller; a double click puts it back.
The height lasts until the window closes and is never saved to the file.

## Choices

A `choice` fence lists its options as task list items. The checked ones
are the value.

| Attribute | Meaning |
| --- | --- |
| `var=name` | Bind `$MDSH_VAR_name` to the checked labels, one per line |
| `mode=dropdown` | Pick one from a dropdown (the default) |
| `mode=radio` | Pick one of a group of radio buttons |
| `mode=multi` | Pick any number, as checkboxes |
| `placeholder="…"` | What a dropdown shows while nothing is picked |
| `nosave` | Picks are used for this session and not saved; the checked items are the default |
| `show=COND` | Show it only when the condition holds |

mdsh-gui writes your picks back as `[x]` and `[ ]`, and leaves any other
lines in the body alone.

## Using the value

An input with `var=name` binds `$MDSH_VAR_name`. The name is letters,
digits and `_`. Every block sees it as an exported variable, and
[conditions and placeholders]({{< relref "conditions" >}}) read it as
`{{$MDSH_VAR_name}}`, so one pick can decide what runs and what the
document says:

````markdown
```sh {#guard run={{$MDSH_VAR_env}}==prod confirm="Deploy to prod?"}
./deploy --env prod
```

<!-- mdsh:if {{$MDSH_VAR_env}}==prod -->
> **This run targets prod.**
<!-- mdsh:end -->

This run checks **{{$MDSH_VAR_host}}**.
````

A value is read from the document as it is when the block runs, so
change a pick and run again, and the blocks follow it. An input without
`var=` is just for display. Inputs never run, and they count toward
`block-N` like any block.

A block can also set a `$MDSH_VAR_` variable for the blocks after it,
with [`mdsh ctl set`]({{< relref "control" >}}). It can't set a name that
an input binds, because the input's text in the file would then disagree
with the variable. The exception is a `nosave` input: `ctl set` may fill
it, and the input then shows the new value.

## The same input in two places

Only the first input with a given `var=` binds the name. A later input
with the same `var=` is a copy of it when it is the same control: the
same kind, `lines=` or `mode=`, `placeholder=` and option labels, in the
same order. Copies show one value, so a long runbook can put the
environment picker at the top and again beside the step that uses it.
Typing or picking in either one changes both.

If the file opens with copies that disagree, mdsh warns about each one,
and mdsh-gui sets them to the first one's value. Undo takes that back.
A later input with the same `var=` that isn't the same control gets a
warning that says how it differs, and its `var=` is ignored. mdsh-gui
marks it, and the outline shows it with `!`.

## In the terminal and on the command line

`mdsh`, `mdsh -w` and `mdsh -i` read each input's value from the file.
To change a value there, edit the fence (`e` in `mdsh -i` opens the block
in `$EDITOR`) and run again. A report shows an input as the code block it
is in the file.
