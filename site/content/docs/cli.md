---
title: Command line
description: Every way to run a document.
weight: 50
---

| Command | Prints | Changes the file |
| --- | --- | --- |
| `mdsh doc.md` | Prose, code and output | No |
| `mdsh --hide-code doc.md` | Prose and output | No |
| `mdsh --hide-output doc.md` | Prose and code | No |
| `mdsh --hide-code --hide-output doc.md` | Prose only; nothing runs | No |
| `mdsh --no-run doc.md` (or `-n`) | Prose, code and the output saved by `-w`; nothing runs | No |
| `mdsh -n -o view.md doc.md` | Nothing; saves what `-n` prints to `view.md` | No |
| `mdsh -w doc.md` | Progress on stderr | Yes: output rewritten |
| `mdsh -o out.md doc.md` | Progress on stderr | No; writes what `-w` would to `out.md` |
| `mdsh --pdf doc.pdf doc.md` | Nothing; lays out what it would print as `doc.pdf` | No; with `-w` or `-o` it writes those too |
| `mdsh --only build doc.md` | That block, plus what it depends on | Per flags |
| `mdsh --yes doc.md` | Runs `confirm` blocks without asking | Per flags |
| `mdsh --step doc.md` | Asks before every block: run, skip or stop | Per flags |
| `mdsh -i doc.md` | A full-screen UI: pick blocks and run them, see [Interactive mode](#interactive-mode) | Yes: output saved after each block |
| `mdsh completion bash\|zsh` | A completion script | No |
| `mdsh --version` | The version | No |

- `mdsh doc.md` is short for `mdsh run doc.md`, so `#!/usr/bin/env mdsh`
  works on Linux and macOS. Arguments after the file name reach blocks as
  `$1`, `$2` and so on.
- `-` reads from stdin: `cat doc.md | mdsh -`.
- Execute mode writes to stdout, so `mdsh doc.md > report.md` gives a
  rendered report.
- `--pdf` respects `--hide-code`, `--hide-output`, `--only` and the rest:
  the PDF shows what would be printed. `--pdf -` writes it to stdout. See
  [PDF output]({{< relref "pdf" >}}) for what goes on the page and an example.
- Output is styled when stdout is a terminal. `--color=never`, `--no-color`
  or `NO_COLOR` turn that off.

## Interactive mode

`mdsh -i notes.md` opens the document full screen, with its prose styled
and each block in a box. Pick a block and run it; its output streams in
under it and is saved to the file as soon as it finishes. Blocks keep
their envs between runs, the same as in one `mdsh` run, so running a
block again runs it in the env as the earlier runs left it, and replaces
its output.

| Key | Does |
| --- | --- |
| `j`/`k`, arrows, tab | next or previous block |
| `g`/`G` | first or last block |
| space, pgup/pgdn, ctrl+u/ctrl+d | scroll |
| enter or `r` | run the block |
| `a` | run the block and every one after it, like a normal run |
| `i` or `:` | type a command; enter adds it to the end of the document as an `sh` block and runs it, esc cancels |
| `v` | show the block's image or recording full screen; enter goes back |
| `w` | save now |
| ctrl+c | stop the running block; its env starts over with a fresh shell. Quits when nothing runs |
| `q` | quit |

A block picked by hand runs even if it is marked `skip` or its `run=`
condition is false; `a` honors both. `confirm` blocks, and every block
under `--step` or `step: true` when running with `a`, ask in the footer
with the same choices as on the command line. `--yes`, `--stop-on-error`
and block arguments work as usual. A `tty` block gets the whole terminal
while it runs, and the TUI comes back when you press enter after it ends;
a taped block's run is shown the same way. Images and recordings
show under their block as a preview in colored half blocks (any terminal
with 24-bit color; off with `NO_COLOR`). `v` shows one full size, through
the kitty, iTerm2 or sixel protocol when mdsh recognizes the terminal
(kitty, Ghostty, iTerm2, WezTerm, foot, mlterm, contour, or `sixel` in
`TERM`), and in half blocks otherwise. SVG images show only as a link. Include blocks show the file's lines,
numbered when they have `number_on`. `out=markdown` output is drawn as
markdown, and placeholders in the prose show their current values. If the file changes on disk while it
is open, mdsh won't save over it until you press `w` twice.
