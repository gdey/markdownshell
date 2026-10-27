# mdsh design spec

## Overview and goals

mdsh turns a markdown file into a runnable script: labeled code blocks run in order, and their output lands back in the markdown. Think of a Jupyter notebook whose file format is plain markdown.

Everything is written in Go, as one core library with three front ends built on it:

| Piece | What it does | When |
| --- | --- | --- |
| Library | Parse, run, render. All logic lives here. | v1 |
| CLI | `mdsh run file.md`: execute mode | v1 |
| TUI | Interactive mode: run blocks as you go | v2 |
| GUI | Desktop or web front end on the same library | Later |

**v1 goals**

- A markdown file with `sh` blocks runs end to end from the command line.
- Blocks can share a session or run in their own sub-env, set per block.
- Output can be printed as markdown or written back into the file.

**Not in v1:** the custom language interpreter, the GUI, running languages other than `sh`.

## Document format

Per-block options sit in the fence's info string (Pandoc-style attributes), document-wide options sit under an `mdsh:` key in YAML front matter, and output goes in an `output` fence right below its block.

````markdown
---
title: Build notes
mdsh:
  shell: /bin/sh
  on_error: continue
  interpolate: true
  show_stderr: false
  env:
    share: [PATH, HOME]
---

```sh {#setup}
export GREETING=hello
```

```sh {#build env=build share=GREETING}
echo "$GREETING from a sub-env"
```

```output {for=build exit=0}
hello from a sub-env
```
````

**Rules**

- **Label:** `#name` gives a block an id. Unlabeled blocks get `block-N` by position. Ids must be unique.
- **What runs:** every block whose language mdsh knows (`sh` in v1). Other languages are left alone. `skip` turns a block off.
- **Front matter:** standard YAML front matter at the top of the file. mdsh reads only the `mdsh:` key, so `title`, tags and other tools' keys live alongside it. A shebang line may come before the opening `---`. Block attributes override it.
- **Output block:** language `output`, carrying `for=<id>` and the exit code. A block in a list item or block quote gets its output inside the same container. stderr gets its own `output {for=<id> stream=stderr}` fence when shown (see Stderr). On a re-run mdsh replaces the old output, so the file stays stable. If output contains backticks, mdsh uses a longer fence.
- **Unknown attributes** are kept untouched, so files stay friendly to other tools.

Block attributes in v1:

| Attribute | Meaning | Default |
| --- | --- | --- |
| `#id` | Block name | `block-N` |
| `skip` | Don't run | off |
| `run=COND` | Run only if COND holds | run |
| `show=COND` | Print the code only if COND holds | shown |
| `show-output=COND` | Print the output only if COND holds | shown |
| `show-stderr[=COND]` | Show stderr | front matter `show_stderr`, else only when the block fails |
| `env=<name>` | Run in a named sub-env; `a/b` stacks `b` on `a` | top-level env |
| `share=A,B` | Exported vars a new sub-env gets from its parent; empty for none | front matter `env.share`, else all |
| `on_error=stop` | Stop the run if this block fails (`continue` overrides front matter) | front matter `on_error` |
| `confirm[="question"]` | Ask before running: run the block or stop the run | off |
| `timeout=30s` | Kill the block's shell after this long; stops the run | none |
| `out=text\|markdown\|image[:path]\|screenshot[:path]` | How output is inserted | `text` |
| `embed=inline` | With `out=image` or `out=screenshot`, write a data URI instead of a file | off |

**Output kinds (`out=`).** `text` fences the output. `markdown` inserts it raw, between `<!-- mdsh:output for="id" -->` and `<!-- /mdsh:output -->` so a re-run can replace it. `image` expects stdout to be image bytes (PNG, JPEG, GIF, SVG) and fails otherwise. `screenshot` draws stdout as a terminal-style image (ANSI escapes dropped), as SVG by default or PNG when the path ends in `.png`; PNGs use the bundled Go Mono font so they look the same everywhere. For both, mdsh saves the image to `<doc>.assets/<block-id>.<ext>`, and inserts a markdown image link between the same markers. `out=image:plots/sales.png` picks the path. Files beside the doc are the default because GitHub doesn't render `data:` image URIs; `embed=inline` writes a data URI anyway for viewers that do. Showing images inline in terminals that support it (kitty, iTerm2, sixel) is planned, not built.

**Conditions.** A condition is `{{$VAR}}` (mdsh's environment), `{{#block/env(VAR)}}` (a block's variable, from this run or a value stored in a placeholder), `{{#block/?}}` (a block's exit status), `{{$?}}` (the exit status of the last block before this point that ran), `{{#block/ran?}}` or `{{$ran?}}` (`true` if the block, or the last runnable block before this point, ran; `false` if it was skipped), true unless empty, `0`, `false`, `no` or `off`. `!` negates it, `==value` and `!=value` compare it as a string, and `true`/`false` are constants. Conditions drive `run=`, `show=`, `show-output=` and `show-stderr=`, and sections: everything between `<!-- mdsh:if COND -->` and `<!-- mdsh:end -->` lines (prose, blocks and output) is printed only when COND holds. Sections nest; the marker lines are never printed. Only `run=` changes what runs; the others change what is printed, and write-back keeps everything. In `run=`, `{{$?}}` is the previous block's status; in the display attributes, which are checked after the block runs, it is the block's own. `{{$ran?}}` follows the same rule. `err?` is another name for `?`; a status is true when non-zero, so `{{$err?}}` reads as "it failed". Block facts end in `?` so they never clash with shell variables. A block mdsh couldn't run (an mdsh error) has status 1. Use cases: a runbook whose diagnostics show with `DEBUG=1`, a cleanup that runs only after a failure, output shown only when a block fails, install steps shown only for the detected OS, output with secrets hidden unless asked for.

**Includes.** A block with the bare attribute `<include` holds lines from another file: `file=` (required, relative to the document's directory), `from=` and `to=` (line numbers counted from 1, inclusive; `from` defaults to the start, `0` meaning the same, and `to` to the end of the file) and `number_on` (show each line's number in the file). The fence's language is the language. mdsh reads the file on every run and writes the lines into the block's body, printed and on write-back, using a longer fence if the lines would close it; the body is mdsh's, so hand edits are replaced. `number_on` is a display hint: printed markdown prefixes the numbers, write-back stores the plain lines, and the TUI or GUI may number them their own way. An include block never runs, whatever its language, and has no name: `#id` and the step attributes (`env=`, `share=`, `run=`, `confirm`, `timeout=`, `out=`, `on_error=`, `show-output=`, `show-stderr=`) are parse errors on it, placeholders can't name it, and it doesn't count toward `block-N`. Includes are content, not steps, so `--only`, a stop or the hide flags don't affect them. `skip` keeps the stored lines and `show=` hides the block; a missing file or a range past the end is an mdsh error, shown below the block.

**Stderr.** stderr is captured separately and shown in an `output {for=id stream=stderr}` fence only when the block fails, unless `show-stderr=` or front matter `show_stderr:` says otherwise. mdsh's own errors (timeouts, image errors) are always shown.

**Placeholders.** `{{#setup/env(NAME)}}` in the prose stands for the value `NAME` had right after block `setup` ran, exported or not (unset reads as empty). `{{$NAME}}` stands for `NAME` in mdsh's own environment. `{{#setup/?}}`, `{{$?}}`, `err?` and `ran?` stand for exit statuses and whether a block ran, as in conditions. mdsh reads each referenced variable from the block's shell once the block finishes. Printed output replaces the placeholder with the value. Write-back (`-w`) keeps the placeholder and stores the value inside it, `{{#setup/env(NAME):"world"}}`, with `"`, `\` and newlines escaped. A block that didn't run (skipped, `--only`, an earlier stop) leaves the stored value in place. Placeholders inside fenced or inline code are left alone, and one naming an unknown block is an error. `interpolate: false` under `mdsh:` turns them off.

## Environment model

Each env is one long-lived shell process. Blocks without `env=` all run in the top-level env, so variables, functions and the working directory carry over like lines of one script. A named sub-env is a separate shell stacked on its parent, and only shared vars cross the boundary.

- **Top-level env:** started once per run with the caller's environment, filtered by the front matter's `env.share` list if set.
- **Sub-env:** `env=build` creates it on first use and reuses it after, so two `env=build` blocks share state. `env=build/test` stacks `test` on `build`. `share=` is read only when the env is created.
- **Down (parent to child):** at creation, the child gets the parent's *exported* vars that pass `share`. `share=*` passes all; `share=` (empty) starts clean.
- **Up (child to parent):** nothing in v1. Vars only flow down.
- **Never shared:** unexported shell vars, functions, aliases, and the working directory. Every env starts in the document's directory.

**How a block runs in a shell.** mdsh checks the block's syntax with `sh -n` in a separate process (a syntax error in a sourced script would end the shell). It then writes the block to a temp file and sources it with stdin from `/dev/null`, followed by a line that prints a random marker and `$?` on stdout and a marker on stderr. Output up to the markers belongs to the block; the number after the stdout marker is its exit code. stdout and stderr are captured separately.

Every block sees `$MDSH_BLOCK`, `$MDSH_FILE`, `$MDSH_DIR`, and the script's arguments as `$1`, `$2`, and so on.

**On failure:** a block that calls `exit` ends its shell, so the run stops there. A block whose last command returns non-zero is marked failed and the run continues. `on_error: stop` in front matter, `on_error=stop` on a block, or `--stop-on-error` makes any failure stop the run. mdsh exits 1 if any block failed.

## Run modes

Two modes share one engine: execute mode runs the whole file and prints markdown, interactive mode runs blocks as you enter them and writes output under each one.

| Command | Prints | Changes the file |
| --- | --- | --- |
| `mdsh doc.md` (same as `mdsh run doc.md`) | Full markdown: prose, code, output | No |
| `mdsh --hide-code doc.md` | Prose and output only | No |
| `mdsh --hide-output doc.md` | Prose and code only | No |
| `mdsh --hide-code --hide-output doc.md` | Prose only; nothing runs | No |
| `mdsh -w doc.md` | Progress on stderr | Yes: output rewritten |
| `mdsh -o out.md doc.md` | Progress on stderr | No; writes what `-w` would to `out.md` (new files get the input's mode; image links are relative to `out.md`) |
| `mdsh --only build doc.md` | That block, plus earlier blocks in its env chain so its state exists | Per flags |
| `mdsh completion bash\|zsh` | A completion script: flags, `.md` files, block ids after `--only` | No |
| `mdsh -i doc.md` | TUI (v2, not built yet) | Yes, as blocks run |

- `confirm` blocks: before running one, mdsh shows its code on the terminal (`/dev/tty`, so piping output still works) and asks `[r]un / [s]top`, using the attribute's value as the question if given. Enter means stop. Stopping leaves the block and everything after it unrun, keeps their old output, and exits 0. With no terminal, the run stops at the block with an error unless `--yes` is passed. The TUI will ask the same question in its own UI.
- `--hide-code` hides runnable blocks only; other code blocks are part of the prose.
- `--color=auto|always|never` controls styling, default `auto`. Auto styles only when stdout is a terminal and `NO_COLOR` is unset; otherwise it prints plain markdown. `--no-color` is short for `--color=never`.
- Execute mode writes to stdout, so `mdsh doc.md > report.md` gives a rendered report.
- `-` reads the markdown from stdin: `cat doc.md | mdsh -`.

**Shebang.** `mdsh file.md` with no subcommand means `mdsh run file.md`, so `#!/usr/bin/env mdsh` works on both Linux and macOS without `env -S`. Arguments after the file name are passed to blocks as `$1`, `$2`, and so on. mdsh skips the shebang line when parsing, drops it from printed output, and keeps it when writing back with `-w`. Trade-off: GitHub shows a shebang line as text and won't treat the front matter below it as front matter.

**Interactive mode (TUI, v2).** You type or pick a block; it runs in its env; its output block appears right under it and is saved to the file. New blocks typed at the prompt are appended to the document as fenced `sh` blocks. Re-running a block replaces its output.

## Block languages

v1 ships on `sh` plus a few markdown helpers. The custom language gets its own spec once v1 runs real files.

**sh in v1, with extensions**

- `out=markdown` puts a block's output into the document as raw markdown, so a script can print a table or a list that renders.
- Each shell starts with helper functions: `md_h1`, `md_h2`, `md_h3` (headings), `md_table` (tab-separated lines on stdin, first line is the header) and `md_code LANG` (stdin as a fenced block). They are plain shell functions, so blocks stay valid `sh`.

**Custom language (after v1), first sketch**

Goals: nicer loops, easy command execution, easy markdown output. A rough feel, nothing settled:

```text
files = $ ls *.go            # $ runs a command, returns its output

for f in files.lines {
    n = $ wc -l < {f}        # {x} interpolates, quoted safely
    table.row f, n.trim
}

md table("File", "Lines")   # md writes markdown into the document
```

Two ways to build it, to decide in that spec:

| Option | Upside | Downside |
| --- | --- | --- |
| Write our own lexer, parser and interpreter in Go | Syntax exactly as wanted | Most work; we own every bug |
| Build on an embeddable Go language (Starlark, Risor) with mdsh builtins | Fast to ship, proven runtime | Syntax is theirs, not ours |

## Go architecture

The runner reports events (started, stdout, stderr, done), and every front end (CLI, TUI, GUI) is a consumer of those events. The parser keeps the original bytes, so writing output back never reformats prose.

| Package | Job |
| --- | --- |
| `doc` | Split a file into shebang, front matter and fenced blocks, byte-exact; parse info-string attributes; attach existing output to its block |
| `env` | Start and drive shell processes; stack sub-envs; apply `share` |
| `run` | Walk the blocks, pick each block's env, emit events, save images |
| `render` | Turn a document plus results into markdown, honouring the hide flags |
| `cmd/mdsh` | The CLI |
| `tui` | Interactive mode (v2) |

**Dependencies.** A hand-written fence scanner instead of a full markdown parser: fences are the only syntax mdsh acts on, and owning the scanner makes byte-exact round trips easy. Fences nested in list items and block quotes are recognised, and their output is written with the same prefix (`> ` or the item's indent) so it stays inside the container. Block quotes inside list items must start within three spaces of the line, tabs don't count as indentation, and lazy continuation lines end a nested fence. Standard library `flag` for the CLI, `gopkg.in/yaml.v3` for front matter, Glamour for terminal styling, Bubble Tea for the TUI.

## Decisions

Settled on 2026-10-27.

- **Which blocks run:** every block in a known language (`sh` in v1); opt out with `skip`.
- **Attribute syntax:** Pandoc-style `{#id key=value}`.
- **Options:** YAML front matter under an `mdsh:` key.
- **Output format:** an `output` fence under each block.
- **Sharing:** vars only flow down in v1.
- **On failure:** continue by default, configurable with `on_error`; an explicit `exit` always stops the run.
- **Shell:** `/bin/sh` by default, `zsh` when the document has any `zsh` block, `shell:` in front matter to change it. `sh` and `zsh` blocks run; every block in a document runs in the same shell.
- **Custom language:** after v1.
- **Confirm:** a `confirm` block asks to run or stop; `--yes` skips the question.
- **Images:** `out=image` for image bytes, `out=screenshot` to draw text output as an image.
- **Conditions:** `run=`, `show=`, `show-output=`, `show-stderr=` and `mdsh:if` sections, driven by `{{$VAR}}`, `{{#block/env(VAR)}}` and the block facts `?` (alias `err?`) and `ran?`.
- **Stderr:** shown when the block fails, hidden otherwise; `show-stderr` overrides both ways.
- **Placeholders:** `{{#block/env(VAR)}}` in prose, on by default, `interpolate: false` to turn off.
- **Includes:** a block marked `<include` with `file=`, `from=`, `to=` (1-based, inclusive) and `number_on`; mdsh writes the lines into the block's body, and the fence's language is the language. Include blocks never run and have no name.
