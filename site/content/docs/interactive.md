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
  with its output; a line starting with `#` becomes prose. Shift+↑ in
  the pane, or `V`, selects lines of its output, and `y` copies them.
- **Edit** a block in `$EDITOR` with `e`, or the whole file with `E`.
  `J` and `K` (or alt+arrows) move a block, and `u` undoes the move.
  `i` types a new block at a prompt.
- **See** an outline of the file with `o`, hide code or output with `C`
  and `O`, and show a block's image full screen with `v`. `tty` blocks
  run in their card and stay open for reading.
- **Reports**, with `p`: a PDF, HTML or markdown copy, as the desktop
  app's Create Report makes.
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
| enter or `r` | run the block |
| `a` | run from this block to the end |
| `s` | run the section the block is in |
| `h` | History: a shell whose commands are added to the file |
| `i` or `:` | type a new block and run it |
| `,` | run options |
| ctrl+c | stop the run; quits when nothing runs |
| ctrl+] | stop a `tty` block running in its card |
| `e` / `E` | edit the block, or the whole file, in `$EDITOR` |
| `y` | copy the block's code to the clipboard |
| `V` | select lines of History's output; `y` copies them |
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

A block picked by hand runs even if it is marked `skip` or its `run=`
condition is false; `a` and `s` honor both.

## Images and recordings

Images and recordings show under their block as a preview in colored
half blocks, in any terminal with 24-bit color (off with `NO_COLOR`).
`v` shows one full size, through the kitty, iTerm2 or sixel protocol
when mdsh recognizes the terminal, and in half blocks otherwise.

## Saving

With `--autosave`, output is saved after every block that runs;
otherwise `w` saves. A read-only file is refused before any block
runs, so nothing is lost.
