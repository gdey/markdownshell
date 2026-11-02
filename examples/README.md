---
title: A tour of mdsh
description: The examples in reading order, with a block to try each one.
---
# A tour of mdsh, in examples

An mdsh document is a markdown file whose `sh` blocks run, top to
bottom, with the output of each written back under it. The files in
this directory show what that makes possible, one idea at a time. Read
them in order: each one builds on the last, and the runbooks at the end
put everything together.

Every example is written the literate way. The prose explains what the
block under it does, and the output you see in the file is real: it was
written by `mdsh -w`. So the files read well on GitHub, and they read
even better after you have run them yourself.

## How to run an example

Run them from this directory, since includes, scripts and image paths
are relative to the file. Each file starts with `#!/usr/bin/env mdsh`,
so with `mdsh` on your `PATH` it runs like a script.

```text
mdsh hello.md              # run it and print the document with fresh output
./hello.md                 # the same, through the shebang
mdsh -w hello.md           # run it and write the output back into the file
mdsh -i hello.md           # open it in the terminal UI and run blocks as you pick them
mdsh-gui hello.md          # open it in the desktop app
mdsh --pdf hello.pdf hello.md   # run it and lay the result out as a PDF
```

`mdsh -i` and `mdsh-gui` are the best way to explore: pick a block,
press enter or click Run, and watch the output land under it. In the
terminal UI, `?` lists every key, and `o` opens an outline of the file.

## Following along in mdsh

This file is an mdsh document too. Open it with `mdsh -i README.md` or
in mdsh-gui, and each part below has a **Try it** block that runs the
example it describes, so you can read about a feature and see it run
without leaving the tour. The blocks are marked `skip`, so a run of the
whole file (`a`, Run All, or `mdsh README.md`) leaves them alone;
picking one by hand runs it. They need `mdsh` on your `PATH`.

```sh {#check skip}
command -v mdsh >/dev/null && echo "mdsh is on your PATH: $(mdsh --version)"
```

Run `mdsh-gui` with no file from this folder and its start screen
shows the tour as a collection, with `fifo/` and `runbooks/` as
collections of their own. Their look comes from the `.mdsh.yaml` file
in each folder; the ⚙ beside a folder edits it.

## 1. Hello: blocks, shells and output

**[hello.md](hello.md)** is the one to start with. It shows the basics
that every other example relies on:

- Blocks run **in one shell**, so a variable set in the first block is
  there in the second, like lines of one script.
- `env=sub share=GREETING` runs a block in a **sub-env**: a separate
  shell that only gets the exported variables you name.
- `out=markdown` puts a block's output into the document **as
  markdown**, and the `md_table` helper turns tab-separated lines into
  a table.
- `out=image` saves the bytes a block prints as an **image** next to
  the file, and `out=screenshot` draws plain text output as a
  **terminal-style picture**. Both are linked below the block, so they
  show on GitHub.
- An `<!-- mdsh:if {{$DEBUG}} -->` **section** and a `run={{$DEBUG}}`
  **condition** switch a debugging part on only when `DEBUG` is set.
  A document only sees the variables of your environment that you
  **expose**, with `--expose DEBUG` or `MDSH_EXPOSE=DEBUG`, so a file
  someone sends you can't read your tokens.
- `{{#setup/env(GREETING)}}` in the prose is a **placeholder** that
  quotes what a block left behind; `-w` stores the value inside it, so
  the prose reads well without a run.

Try `DEBUG=1 mdsh --expose DEBUG hello.md` to see the hidden section
appear, and `mdsh --hide-code hello.md` to print only the prose and
output.

```sh {#try-hello skip}
DEBUG=1 mdsh --expose DEBUG --hide-code hello.md
```

## 2. Quoting files

**[include.md](include.md)** shows a block marked `<include`: it holds
lines from another file, [include/greet.sh](include/greet.sh), and mdsh
rewrites them on every run so the quote never goes stale. `from=` and
`to=` pick a range of lines, `number_on` asks for line numbers, and the
fence's language is the language the lines are shown in. An include
never runs; it is content, not a step.

```sh {#try-include skip}
mdsh include.md
```

## 3. Guard rails for runbooks

**[guardrails.md](guardrails.md)** is a small runbook with every way of
keeping a run safe:

| Attribute | In the example |
| --- | --- |
| `on_error: stop` (front matter) | A failing step ends the run |
| `run=once` | A fetch step that runs only while it has no saved output |
| `confirm="…"` | A login step that shows its code and asks before running |
| `depends=login,fetch` | A deploy step that makes sure its prerequisites ran, asking about any that didn't |
| `timeout=10s` | A health check that is killed if it takes too long |
| `on_error=continue` | A lint step allowed to fail without stopping the run |
| `run={{#lint/?}}!=0` | A step that runs only because the lint step failed |
| `skip` | A rollback kept in the file, never run with it |
| An `output {for=deploy}` copy | A summary at the end that repeats what the deploy step printed |

Run it twice. The first `mdsh guardrails.md` asks about the login step;
`mdsh --yes guardrails.md` answers yes for you, which is how CI and the
tests run runbooks. `mdsh --only deploy guardrails.md` runs just the
deploy step, and `depends=` makes sure its login runs first. Add
`--step` to be asked before every block, and `--audit run.jsonl` to
keep a log of the run: one JSON line for each block that started,
ended or was skipped and why, each question answered, and each
variable changed.

```sh {#try-guardrails skip}
mdsh --yes --hide-code --audit "$(mktemp)" guardrails.md
```

## 4. Inputs: values picked in the document

**[inputs.md](inputs.md)** holds a host name in a `textbox` block and
an environment and a set of checks in `choice` blocks. With `var=`, each
one binds an `$MDSH_VAR_` variable that blocks, `run=` conditions,
`mdsh:if` sections and placeholders all read. In mdsh-gui the inputs
are a text field, radio buttons and checkboxes, and what you pick is
written into the file; anywhere else, edit the fence's body. A block
can bind a variable the same way with `mdsh ctl set`, which talks back
to the run over `$MDSH_SOCK`. The file ends with `collapse` and
`<!-- mdsh:collapse -->`, which make mdsh-gui open a block or a section
folded.

```sh {#try-inputs skip}
mdsh --hide-code inputs.md
```

## 5. Terminals and tapes

**[terminal.md](terminal.md)** is for commands that need a terminal: a
prompt that reads a name, a small menu in
[terminal/fruit.sh](terminal/fruit.sh), and a login that turns echo
off. A block marked `tty` runs on a real terminal, and mdsh records the
session as an asciinema cast in
[terminal.md.assets](terminal.md.assets), linked under the block, which
mdsh-gui plays a frame at a time if you like.

A `tape` fence types the block's input for it, in
[VHS](https://github.com/charmbracelet/vhs) tape commands, so the block
runs unattended: in CI, under `-w`, or to regenerate a recording.
[terminal/fruit.tape](terminal/fruit.tape) is a tape read from a file
through an include, and the login tape types a token from a placeholder,
so the secret never lands in the document or the recording. The last
block has no tape and runs only with `LIVE=1`, for you to type at.

Running the file plays the tapes and records again, so this prints the
document with the recordings it already has. `mdsh -w terminal.md`
records them afresh.

```sh {#try-terminal skip}
mdsh -n terminal.md
```

## 6. Background blocks

**[fifo/](fifo)** has three short documents about `background` blocks:
a block that starts and lets the run go on to the next one. Each uses a
named pipe to hand data from a later block to an earlier one, which is
the clearest way to see that two blocks really do run at once.

- [handoff.md](fifo/handoff.md): a background reader, a writer, and a
  `wait "$MDSH_BG_reader"` that collects the reader's exit status.
- [held-open.md](fifo/held-open.md): a writer that keeps the pipe open
  across blocks with `exec 3>pipe`, and closes it on purpose.
- [never-closed.md](fifo/never-closed.md): a writer that forgets to
  close it, and `timeout=` on the background block keeping the run from
  hanging. This one goes wrong on purpose.

The deploy runbook in part 9 uses the same idea to check links while
the next step runs.

```sh {#try-fifo skip}
mdsh --hide-code fifo/handoff.md
```

## 7. Reports: PDF and HTML

**[report.md](report.md)** is written to be read as a report. Run
`mdsh --pdf pdf/report.pdf report.md` and the PDF holds what `mdsh
report.md` would print: the prose, every block, its output, a failing
block with its exit status and red stderr, an `out=markdown` table, a
screenshot drawn as sharp text, and an include with a line-number
gutter. `--html` writes the same view as a single HTML page, and
`--hide-code` leaves the code out for readers who won't run it.

**[pdf/](pdf)** holds PDFs made this way of this report, of parts 1, 2
and 5, and of a run of the test coverage runbook in part 9. This
refreshes the report's PDF there:

```sh {#try-report skip}
mdsh --pdf pdf/report.pdf report.md >/dev/null && ls -l pdf/report.pdf
```

## 8. Working interactively

**[history.md](history.md)** is about the History pane that `mdsh -i`
(press `h`) and mdsh-gui (View › History) have: a shell at the bottom of
the screen whose commands are added to the end of the document as they
run, as `#history-N` blocks with their output, and whose `#` lines
become prose. It is the way to start a document when you don't yet know
what it will say. The file shows the shape such a session leaves
behind, and is there to add to.

```sh {#try-history skip}
mdsh --hide-code history.md
```

Open any example in the terminal UI to see the rest of it: `a` runs
from the block under the cursor, `s` runs its section, `e` edits a block
in your editor, `i` types a new block at a prompt, `p` creates a report,
and `?` lists it all. mdsh-gui does the same with a mouse, a source view
beside the rendered one, tabs for several files, and three looks in
Settings › Look: Plain, Teletype (typewriter text on fan-fold paper, or
green phosphor in the dark theme) and Blueprint (white lines on
blueprint blue).

## 9. Real runbooks

**[runbooks/](runbooks)** puts all of the above to work. Each file is a
document someone would actually keep, and `go test ./cmd/mdsh` runs
every one of them on each change, so they keep working. A name ending
in `_linux.md` or `_darwin.md` runs only on that OS.

| Runbook | What it is | Shows off |
| --- | --- | --- |
| [deploy.md](runbooks/deploy.md) | Build, check and ship a static site behind a `current` symlink, in a scratch directory | `on_error: stop`, a `background` link checker, `confirm` before shipping, a `skip` rollback, `show=false` cleanup |
| [release-checklist.md](runbooks/release-checklist.md) | Tag a release of a small Go project, in a scratch repository | Blocks inside numbered steps and block quotes, `out=markdown` notes, cleanup gated on `run={{#tag/?}}==0` |
| [mdsh-release.md](runbooks/mdsh-release.md) | How mdsh itself is released: check the changelog and the checkout, then tag and push with `PUSH=1` | `run={{$PUSH}}` with `confirm`, a preflight table, a check-only mode for tests |
| [onboarding.md](runbooks/onboarding.md) | A first-day check that a machine can build mdsh | A table of tools with versions, an `mdsh:if` section printed only when something is missing, a report meant to be sent as a PDF |
| [system-report.md](runbooks/system-report.md) | Describe a machine for a bug report | Sections and blocks gated on the detected OS, a sub-env with `share=` so tokens in your shell can't leak into the report |
| [disk-report.md](runbooks/disk-report.md) | A weekly look at disk space | `on_error: continue`, tables from `df` and `du`, a warning paragraph that appears only over a limit, `timeout=` on slow steps |
| [linux-triage_linux.md](runbooks/linux-triage_linux.md), [macos-triage_darwin.md](runbooks/macos-triage_darwin.md) | First read-only steps when a host is slow | One file per OS, blocks in block quotes and lists, `on_error: continue` so partial answers still come through |
| [test-coverage.md](runbooks/test-coverage.md) | mdsh's own test coverage, by package and least-covered function | `go test` output turned into tables with awk, `-w` to refresh the report in place |

The [use cases](https://gdey.github.io/markdownshell/use-cases/) pages
on the site walk through three of these with the PDF, HTML and markdown
reports one run of them made. The deploy runbook ships to a scratch
directory, so it is safe to run anywhere; `--yes` answers its question.

```sh {#try-deploy skip}
mdsh --yes --hide-code runbooks/deploy.md
```

## Where to find each feature

| Looking for | Example |
| --- | --- |
| Blocks that share a shell | [hello.md](hello.md), [history.md](history.md) |
| `env=` and `share=` sub-envs | [hello.md](hello.md), [runbooks/system-report.md](runbooks/system-report.md) |
| `out=markdown` and `md_table` | [hello.md](hello.md), [report.md](report.md), [runbooks/disk-report.md](runbooks/disk-report.md) |
| `out=image`, `out=screenshot` | [hello.md](hello.md), [report.md](report.md) |
| Placeholders in prose | [hello.md](hello.md), [guardrails.md](guardrails.md), [inputs.md](inputs.md) |
| `--expose`, `MDSH_EXPOSE` | [hello.md](hello.md), [terminal.md](terminal.md) |
| `run=`, `show=`, `mdsh:if` sections | [hello.md](hello.md), [guardrails.md](guardrails.md), [runbooks/system-report.md](runbooks/system-report.md) |
| `<include` blocks | [include.md](include.md), [report.md](report.md), [terminal.md](terminal.md) |
| `confirm`, `--yes`, `--step` | [guardrails.md](guardrails.md), [runbooks/deploy.md](runbooks/deploy.md) |
| `depends=` | [guardrails.md](guardrails.md) |
| `run=once` | [guardrails.md](guardrails.md) |
| `on_error`, `timeout=`, `skip` | [guardrails.md](guardrails.md), [runbooks/deploy.md](runbooks/deploy.md) |
| Output copies | [guardrails.md](guardrails.md) |
| `--audit FILE` | the Try it block of part 3 |
| `textbox`, `choice`, `var=` | [inputs.md](inputs.md) |
| `mdsh ctl set`, `$MDSH_SOCK` | [inputs.md](inputs.md) |
| `collapse`, `mdsh:collapse` | [inputs.md](inputs.md) |
| `tty` blocks, tapes, recordings | [terminal.md](terminal.md) |
| `background` blocks | [fifo/](fifo), [runbooks/deploy.md](runbooks/deploy.md) |
| `--pdf`, `--html`, `--hide-code` | [report.md](report.md), [pdf/](pdf), [runbooks/onboarding.md](runbooks/onboarding.md) |
| `#history-N` blocks from the History pane | [history.md](history.md) |
| Front matter: `on_error`, `title` | [guardrails.md](guardrails.md), [runbooks/](runbooks) |
| Blocks inside lists and block quotes | [runbooks/release-checklist.md](runbooks/release-checklist.md), [runbooks/linux-triage_linux.md](runbooks/linux-triage_linux.md) |

Every attribute and flag is described in the
[documentation](https://gdey.github.io/markdownshell/docs/), and the
[spec](../docs/spec.md) has the rules behind them.

## What is here

```text
README.md             this tour, with a block to try each part
hello.md              1. blocks, shells, output kinds, conditions, placeholders
hello.md.assets/      the image and screenshot hello.md draws
include.md            2. quoting files
include/greet.sh      the script include.md quotes and runs
guardrails.md         3. confirm, depends=, run=once, timeout=, on_error, skip, output copies
inputs.md             4. textbox and choice blocks, var=, mdsh ctl set, folding
terminal.md           5. tty blocks and tapes
terminal.md.assets/   its recordings, as asciinema casts
terminal/             the prompt script and tape it uses
fifo/                 6. background blocks and named pipes
report.md             7. a document written as a report
report.md.assets/     its screenshot
pdf/                  PDFs of hello, include, terminal, report and the test coverage runbook
history.md            8. what a History session leaves behind
runbooks/             9. complete runbooks: deploy, release, onboarding, reports, triage
```
