---
title: Terminal UI
description: mdsh -i, the notebook in a terminal, over SSH or anywhere without a desktop.
weight: 16
---

`mdsh -i` is the desktop app's notebook in a terminal: the same
documents, the same engine, and the same output saved to the file. It
is the answer to "but I'm on a bastion host": open the runbook on the
box, step through it, and the file is the timeline when you're done.

```sh
mdsh -i                         # a start screen for this folder
mdsh -i ~/runbooks              # ...or that one
mdsh -i deploy.md               # open a file
mdsh -i --yes deploy.md         # run confirm blocks without asking
mdsh -i deploy.md staging       # block arguments work as usual: $1 is staging
```

{{< shot src="tui-confirm.png" title="mdsh -i deploy.md" term=true >}}The deploy runbook in the terminal, asking before it ships. The blocks above ran, and their output streamed in under them.{{< /shot >}}

## What it does

- **A start screen** lists the markdown files under a folder, recent
  files first; type to filter. Each file opens in a **tab** of its own
  (`ctrl+t` for a new one, `]` and `[` to move between them).
- **Run** a block with enter, from it to the end with `a`, or its
  section with `s`. `confirm` blocks and the blocks `depends=` names ask
  in the block's card. `,` sets the run options: arguments, step mode,
  stop on error.
- **History**, with `h`: a shell along the bottom with your shell's own
  prompt, completion and emacs line keys. Each command is added to the file as a block,
  with its output; a line starting with `#` becomes prose, and
  ``# ```sh`` opens a block typed line by line, added to the file when a
  ```` ``` ```` line ends it. ctrl+o in the pane turns recording off,
  so commands run in the shell without being added to the file, until
  ctrl+o again or the pane closes. The pane's rule shows the shell's
  working directory, and `Z` has the pane fill the screen. Shift+↑ in
  the pane, or `V`, selects lines of its output, and `y` copies them.
  With `--interactive-shell` (experimental), the pane is a terminal on
  the shell, which reads your rc files: every key goes to it, so vim,
  ctrl+z, `fg` and `jobs` work, the rule lists its jobs, and ctrl+g
  gives the keys back to the file. `@` then moves the pane to another
  env the file names, such as `env=build`, whose shell runs
  interactively too; commands typed there become `env=build` blocks.
- **Edit** a block in `$EDITOR` with `e`, or the whole file with `E`.
  `J` and `K` (or alt+arrows) move a block, and `u` undoes the move.
  `i` types a new block at a prompt.
- **See** an outline of the file with `o`, hide code or output with `C`
  and `O`, and show a block's image full screen with `v`. `tty` blocks
  run in their card and stay open for reading.
- **HTTP requests.** An [`http` block]({{< relref "http" >}}) shows
  its response's status, body and checks under it; `H` switches to the
  headers and `Y` copies the request as `curl`.
- **When it ran.** A saved output's label says how long ago it ran,
  "saved output, 3 mins ago:", and a block run this session has it
  after its time in the header. A block with `history=N` says under its
  output how many earlier runs it keeps, and `b` shows each one with
  how far back it is, its exit status and when it ran. Running such a
  block several times before saving keeps every run.
- **Reports**, with `p`: a PDF, HTML or markdown copy, as the desktop
  app's Create Report makes; its Earlier runs switch adds the runs kept
  by `history=N`.
- **Save** with `w`, Save As with `W`. If the file changes on disk, a
  banner says so, and `R` reloads it or `M` keeps yours.

`?` lists every key, and greys out the ones that do nothing right now,
with the reason.

{{< shot src="tui-history.png" title="mdsh -i architecture.md · History" term=true >}}The History pane: `cd` and `ls` typed at the bottom became the blocks `history-1` and `history-2` above.{{< /shot >}}

## Keys

| Key | Does |
| --- | --- |
| `j`/`k`, arrows | next or previous block |
| `g`/`G` | first or last block |
| space, pgup, ctrl+d/ctrl+u | scroll |
| enter or `r` | run the block; on a block running in the background, stop it |
| `a` | run from this block to the end |
| `s` | run the section the block is in |
| `h` | History: a shell whose commands are added to the file |
| `P` | Processes: what runs under the file's shells, as a tree, with how long each has run; `x` kills the one picked and what it started |
| `i` or `:` | type a new block and run it |
| `,` | run options |
| ctrl+c | stop the run, keeping the shells and what blocks set there; again kills the shell of a block that won't stop. Quits when nothing runs |
| ctrl+] | stop a `tty` block running in its card |
| `e` / `E` | edit the block, or the whole file, in `$EDITOR` |
| `y` | copy the block's code to the clipboard |
| `Y` | copy an `http` block's request as a `curl` command |
| `H` | show an `http` block's response headers, or its body again |
| `b` | show or hide the block's earlier runs, kept by `history=N` |
| `x` | show or hide a `secret` block's code and output |
| `F` | fix front matter that doesn't parse, when mdsh can |
| `V` | select lines of History's output; `y` copies them |
| `Z` | History fills the screen; esc gives the file its place back |
| `J`/`K`, alt+↓/↑ | move the block down or up |
| `u` | undo the last move |
| `w`, ctrl+s / `W` | save / save as |
| `R` / `M` | reload from disk / keep yours |
| `p` | create a report |
| `o`, `C`, `O`, `v` | outline, code, output, image |
| `]`/`[`, ctrl+t, ctrl+w | tabs |
| `q` | close the file, or quit |
| `?` | every key, with what it does now |

## Running blocks

Blocks keep their shells between runs, the same as in one `mdsh` run,
so running a block again runs it in the shell as the earlier runs left
it, and replaces its output. Stopping a block starts its shell over.

A running block's header counts up how long it has run, against its
`timeout=` when it has one (`running… 12s / 1m`), and keeps the time
once it ends (`✓ 1.4s`). The time is for this session; it isn't saved.

A block picked by hand runs even if it is marked `skip` or its `run=`
condition is false; `a` and `s` honor both.

## Status in your terminal

`mdsh -i` tells the terminal it runs in what it is doing, with the
[Program Status Protocol](https://mitchellh.com/writing/program-status-osc7501)
(OSC 7501): working while a block runs, blocked while a card waits for
your answer, done or error once the run ends, and idle again at your
next key. A terminal that shows program status, such as Ghostty, lists
mdsh and what it waits on in its tab or inbox; one that doesn't ignores
the reports. `mdsh doc.md` reports the same way.

The window title names the file, marked while a run is going:
`[50%] deploy.md - mdsh` with a block's `mdsh ctl status --step` as the
percent, `[running]` without steps, `[waiting]` while a card asks,
`[failed]`, and `[done]` until your next key. Quitting puts your
terminal's title back, where it keeps a title stack. Ghostty, Windows
Terminal, VS Code and ConEmu also show a progress bar on the tab or
taskbar (OSC 9;4), and `mdsh doc.md` shows it too, gone when it exits.

## Images and recordings

Images and recordings show under their block as a preview in colored
half blocks, in any terminal with 24-bit color (off with `NO_COLOR`).
`v` shows one full size, through the kitty, iTerm2 or sixel protocol
when mdsh recognizes the terminal, and in half blocks otherwise.

## Saving

With `--autosave`, output is saved after every block that runs;
otherwise `w` saves. With `--history-reuse`, a command typed in History
again runs its `#history-N` block again, keeping the run before under
it as an earlier run, rather than adding another block. A read-only file is refused before any block
runs, so nothing is lost.
