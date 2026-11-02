---
title: Security
description: What a document can and can't do, the public reviews, and how to report a problem.
layout: landing
lede: "mdsh runs the code in a markdown file; that is what it is for. Running a file you don't trust runs its code, as running a shell script would. What a file must not do is anything beyond that, and that is what mdsh guards."
---

## What opening a file can't do

- **Nothing runs until you say so.** Opening a file in mdsh-gui or
  `mdsh -i`, printing it with `mdsh -n`, or making a PDF or HTML report
  of its saved output runs no block.
- **It can't read your environment.** `{{$VAR}}` in a document reads a
  variable from your environment only when you expose it with
  `--expose VAR` or `MDSH_EXPOSE`, so a file someone sent you can't
  print `$GITHUB_TOKEN` into a report. Saving never writes a caller's
  value into the file.
- **It can't hide code from you.** `confirm` and `--step` show a
  block's code with control characters made visible, so an escape
  sequence can't erase the line you are being asked about. The app
  draws blocks that a printout would leave out, with a badge, and opens
  any folded section that holds one.
- **It can't reach the network to be drawn.** The desktop app loads no
  remote images and runs under a Content Security Policy; SVG pictures
  are shown as images, so their scripts don't run.
- **It can't widen what a block sees.** Sub-envs get only the variables
  `share=` names, and the front matter's `env.share` can narrow a whole
  file.

## Reviews

mdsh v1.2.1 had two security reviews, one of the command line and
shared packages and one of the desktop app, both starting from the
case that matters: you open a file you did not write. They are public:
[CLI review](https://github.com/gdey/markdownshell/blob/main/docs/security/cli-v1.2.1.md),
[desktop app review](https://github.com/gdey/markdownshell/blob/main/docs/security/gui-v1.2.1.md).
All four High findings are fixed in 1.3, and the
[changelog]({{< relref "changelog" >}}) lists each fix under a Security
heading.

## What it is suited for today

A single-user tool with a clear model: blocks run as you, on your
machine, with your permissions. It suits engineers and teams who trust
the files they run, as they trust their shell scripts. It is not a
sandbox: a block can do anything you can. For an audit trail,
`--audit run.jsonl` writes one JSON line for every block that started,
ended or was skipped, every question answered and every variable
changed.

## Reporting a problem

Please report a vulnerability privately, through
[Report a vulnerability](https://github.com/gdey/markdownshell/security/advisories/new)
on the repository's Security tab, not in a public issue. The
[security policy](https://github.com/gdey/markdownshell/blob/main/SECURITY.md)
says what counts and what to expect.
