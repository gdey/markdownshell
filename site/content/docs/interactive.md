---
title: Interactive mode
description: Open a document in a terminal UI and run its blocks as you go.
weight: 52
---

`mdsh -i notes.md` opens the document full screen, with its prose styled
and each block in a box. Pick a block and run it; its output streams in
under it and is saved to the file as soon as it finishes.

```sh
mdsh -i notes.md                # open it
mdsh -i --yes notes.md          # run confirm blocks without asking
mdsh -i --step notes.md         # ask before each block when running with a
mdsh -i notes.md staging        # block arguments work as usual: $1 is staging
```

## Keys

| Key | Does |
| --- | --- |
| `j`/`k`, arrows, tab | next or previous block |
| `g`/`G` | first or last block |
| space, pgup/pgdn, ctrl+u/ctrl+d | scroll |
| enter or `r` | run the block |
| `a` | run the block and every one after it, like a normal run |
| `i` or `:` | type a command and add it to the document as a block |
| `e` | edit the whole block in your editor |
| `v` | show the block's image or recording full screen; enter goes back |
| `w` | save now |
| ctrl+c | stop the running block; quits when nothing runs |
| `q` | quit |

## Running blocks

Blocks keep their envs between runs, the same as in one `mdsh` run, so
running a block again runs it in the env as the earlier runs left it, and
replaces its output. Stopping a block with ctrl+c starts its env over with
a fresh shell.

A block picked by hand runs even if it is marked `skip` or its `run=`
condition is false; `a` honors both. `confirm` blocks, and every block
under `--step` or `step: true` when running with `a`, ask in the footer
with the same choices as on the command line: run, skip, skip blocks
that already have output, or stop. `--stop-on-error` works as usual.

## Adding and editing blocks

`i` (or `:`) opens a prompt at the bottom. Type a command and press enter:
it is added to the end of the document as an `sh` block and runs. Esc
cancels.

`e` opens the selected block, fences and attributes included, in
`$VISUAL`, `$EDITOR` or `vi`. The edit is saved to the file when the
editor exits, and enter runs it. Saved output follows the block if you
change its id.

## Terminal blocks

A [`tty` block]({{< relref "writing-documents" >}}) gets the whole
terminal while it runs, and the UI comes back when you press enter after
it ends. A taped block's run is shown the same way.

## Images and recordings

Images and recordings show under their block as a preview in colored half
blocks, in any terminal with 24-bit color (off with `NO_COLOR`). `v` shows
one full size, through the kitty, iTerm2 or sixel protocol when mdsh
recognizes the terminal (kitty, Ghostty, iTerm2, WezTerm, foot, mlterm,
contour, or `sixel` in `TERM`), and in half blocks otherwise. SVG images
show only as a link.

## What else shows

Include blocks show the file's lines, numbered when they have
`number_on`. `out=markdown` output is drawn as markdown, and placeholders
in the prose show their current values.

## Saving

Output is saved to the file after each block, and `w` saves at any time.
If the file changes on disk while it is open, mdsh won't save over it
until you press `w` twice.
