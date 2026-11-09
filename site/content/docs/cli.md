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
| `mdsh --html doc.html doc.md` | Nothing; writes what it would print as a standalone `doc.html` | No; with `-w` or `-o` it writes those too |
| `mdsh --only build doc.md` | That block, plus what it depends on | Per flags |
| `mdsh --yes doc.md` | Runs `confirm` blocks without asking | Per flags |
| `mdsh --step doc.md` | Asks before every block: run, skip or stop | Per flags |
| `mdsh --stop-on-error doc.md` | Stops at the first block that fails, as if every block had `on_error=stop` | Per flags |
| `mdsh --expose DEBUG doc.md` | Lets `{{$DEBUG}}` read `DEBUG` from your environment; see [Conditions and placeholders]({{< relref "conditions#exposing-variables" >}}) | Per flags |
| `mdsh --audit run.jsonl doc.md` | Also writes a JSON Lines log of the run: each block started, ended or skipped and why, each variable changed; see [Talking back to mdsh]({{< relref "control#the-audit-log" >}}) | Per flags |
| `mdsh --source defs.zsh doc.md` | Sources `defs.zsh` into every shell before its first block; repeatable | Per flags |
| `mdsh --allow-outside doc.md` | Lets includes, `out=` files and report images reach outside the document's git repository, or its folder when it isn't in one; only for documents you trust | Per flags |
| `mdsh -i doc.md` | A full-screen UI: pick blocks and run them, see [Terminal UI]({{< relref "interactive" >}}) | When you save (`w`), or after each block with `--autosave` |
| `mdsh -i` or `mdsh -i ~/notes` | A start screen to pick a file under this folder, or that one | As above |
| `mdsh ctl set NAME value` | From inside a block: sets a variable for the blocks after it (`unset`, `get` and `status` too); see [Talking back to mdsh]({{< relref "control" >}}) | No |
| `mdsh lsp` | A language server for block attributes, for your editor | No |
| `mdsh completion bash\|zsh` | A completion script | No |
| `mdsh --serve [doc.md]` | Nothing; serves the notebook to a web browser at 127.0.0.1:8080, see [Browser]({{< relref "browser" >}}) | When you save in the page |
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
- `--html` does the same as an HTML page, with local images inside it and
  raw HTML from the document left out. It can't be used with `--pdf`.
- `--source FILE` is read once, when mdsh starts, so a pipe works:
  `mdsh --source <(alias -L; typeset -f) doc.md` gives blocks the aliases
  and functions of the shell you run it from. See
  [Environments]({{< relref "environments#aliases-and-functions" >}}).
- Output is styled when stdout is a terminal. `--color=never`, `--no-color`
  or `NO_COLOR` turn that off.
