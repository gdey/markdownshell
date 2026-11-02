---
title: The examples tour
description: A guided tour of mdsh in nine parts, in the examples folder of the repository.
layout: landing
lede: "The fastest way to see what mdsh does is to run it. The repository's `examples/` folder is a tour in nine parts, from a two-block hello to whole runbooks, and every part has a **Try it** block that runs the example it describes."
---

## Open it

```sh
git clone https://github.com/gdey/markdownshell
cd markdownshell/examples
mdsh-gui .          # the desktop app's start screen, for this folder
mdsh -i             # or the same in the terminal
```

The start screen shows the tour as a collection, and `fifo/` and
`runbooks/` as collections of their own, each with a title, icon and
color from the `.mdsh.yaml` in its folder. The ⚙ beside a folder edits
that file in a form. Pin the files you come back to; they stay at the
top.

{{< shot name="start" title="Start" >}}The start screen on the examples folder: pinned and recent files, then the tour as a collection.{{< /shot >}}

Open `README.md` first. It is the tour itself, and an mdsh document
too: each Try it block is marked `skip`, so Run all leaves them alone,
and picking one runs it. Everything also works from the command line:

```sh
mdsh hello.md                 # run it and print the document with fresh output
./hello.md                    # the same, through the shebang
mdsh -w hello.md              # run it and write the output back
mdsh --pdf hello.pdf hello.md # the run, as a PDF
```

## The nine parts

| Part | File | What it shows |
| --- | --- | --- |
| 1 | `hello.md` | Blocks that share a shell, sub-envs with `share=`, `out=markdown` tables, `out=image` and `out=screenshot`, an `mdsh:if` section, placeholders in the prose |
| 2 | `include.md` | A block that quotes lines from another file and keeps the quote current |
| 3 | `guardrails.md` | `on_error: stop`, `run=once`, `confirm`, `depends=`, `timeout=`, `skip`, and an output copy at the end |
| 4 | `inputs.md` | `textbox` and `choice` blocks with `var=`, `mdsh ctl set`, and sections that open folded |
| 5 | `terminal.md` | `tty` blocks on a real terminal, recorded as GIFs, and tapes that type for them |
| 6 | `fifo/` | `background` blocks handing data through a named pipe, and a `timeout=` that saves a hung run |
| 7 | `report.md` | A document written to be read as a PDF or HTML report |
| 8 | `history.md` | What a History session leaves behind |
| 9 | `runbooks/` | Whole runbooks: deploy, release, onboarding, disk and system reports, triage, test coverage |

## Things to try

**Ship the site, carefully.** Open `runbooks/deploy.md` and press Run
all. It builds a small site, checks its links in the background while
it measures it, and stops to ask before it ships. Everything happens in
a scratch directory, so it is safe anywhere.

{{< shot name="hero" title="deploy.md" >}}The deploy runbook waiting at its confirm block.{{< /shot >}}

**Watch the dependencies.** In `guardrails.md`, select the `deploy`
block. Lines in the margin run up to the `login` and `fetch` blocks it
names in `depends=`. Run `deploy` alone and mdsh asks to run `login`
first.

{{< shot name="depends" title="guardrails.md" >}}A block's `depends=`, drawn as lines in the margin.{{< /shot >}}

**Fill in a form.** `inputs.md` has a host name, an environment and a
set of checks. Change them in the app and run the section: the blocks
below follow what you picked, and the file keeps it.

**Make a report.** File › Create Report… on `report.md`, or from a
terminal:

```sh
mdsh --pdf report.pdf report.md
mdsh --html report.html --hide-code report.md
```

**Start your own.** New Session on the start screen opens a file named
for the day with the History pane ready. Type commands; each one lands
in the file as a block with its output.

{{< shot name="history" title="2026-11-02.md" >}}A session file: commands typed into the History pane at the bottom become blocks above.{{< /shot >}}

## Where each feature is shown

The tour's own README has a full table of which example shows which
attribute and flag:
[examples/README.md](https://github.com/gdey/markdownshell/blob/main/examples/README.md#where-to-find-each-feature).
The [use cases]({{< relref "use-cases" >}}) pages show three of the
runbooks with the reports one run of them made.
