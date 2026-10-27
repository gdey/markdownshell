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
| `mdsh --only build doc.md` | That block, plus what it depends on | Per flags |
| `mdsh --yes doc.md` | Runs `confirm` blocks without asking | Per flags |
| `mdsh completion bash\|zsh` | A completion script | No |
| `mdsh --version` | The version | No |

- `mdsh doc.md` is short for `mdsh run doc.md`, so `#!/usr/bin/env mdsh`
  works on Linux and macOS. Arguments after the file name reach blocks as
  `$1`, `$2` and so on.
- `-` reads from stdin: `cat doc.md | mdsh -`.
- Execute mode writes to stdout, so `mdsh doc.md > report.md` gives a
  rendered report.
- Output is styled when stdout is a terminal. `--color=never`, `--no-color`
  or `NO_COLOR` turn that off.
