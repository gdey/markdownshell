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

**Not in v1:** the GUI, running languages other than `sh`.

## Document format

Per-block options sit in the fence's info string (Pandoc-style attributes), document-wide options sit under an `mdsh:` key in YAML front matter, and output goes in an `output` fence right below its block.

````markdown
---
title: Build notes
mdsh:
  shell: /bin/sh
  on_error: continue
  step: false
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
- **Output block:** language `output`, carrying `for=<id>` and the exit code. A block in a list item or block quote gets its output inside the same container. stderr gets its own `output {for=<id> stream=stderr}` fence when shown (see Stderr). On a re-run mdsh replaces the old output, so the file stays stable. If output contains backticks, mdsh uses a longer fence. An `output` fence that isn't right after its block is a copy that mdsh refills (see Output copies).
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
| `timeout=30s` | Kill the block's shell after this long; stops the run. A `background` block is killed alone | none |
| `background` | Start the block and go on to the next; its output is kept apart, and the run waits for it at the end | off |
| `tty` | Run on the terminal, for commands that need one (`docker run -it`); the session is saved as an animated GIF | off |
| `frame=N` | With `tty`, the frame of the GIF that still views (PDF, and later the TUI and GUI) show, counting from 1 | the last |
| `out=text\|markdown\|image[:path]\|screenshot[:path]` | How output is inserted | `text` |
| `embed=inline` | With `out=image` or `out=screenshot`, write a data URI instead of a file | off |

**Output kinds (`out=`).** `text` fences the output. `markdown` inserts it raw, between `<!-- mdsh:output for="id" -->` and `<!-- /mdsh:output -->` so a re-run can replace it. `image` expects stdout to be image bytes (PNG, JPEG, GIF, SVG) and fails otherwise. `screenshot` draws stdout as a terminal-style image (ANSI escapes dropped), as SVG by default or PNG when the path ends in `.png`; PNGs use the bundled Go Mono font so they look the same everywhere. For both, mdsh saves the image to `<doc>.assets/<block-id>.<ext>`, and inserts a markdown image link between the same markers. `out=image:plots/sales.png` picks the path. Files beside the doc are the default because GitHub doesn't render `data:` image URIs; `embed=inline` writes a data URI anyway for viewers that do. Showing images inline in terminals that support it (kitty, iTerm2, sixel) is planned, not built.

**Output copies.** The output right after a block is its output: mdsh replaces it on every run, reads `exit=` from it, and adds one there if it is missing. An `output {for=id}` fence anywhere else, or an `<!-- mdsh:output for="id" -->` region (for `out=markdown`, image and recording output), is a copy: like a `{{$VAR}}` placeholder, mdsh refills it with block `id`'s output on every run, so a summary near the end of a document can repeat what an earlier block found. A copy with `stream=stderr` gets the stderr fence instead, shown by the block's usual rules. The output right after a block holds at most one stdout part and one stderr fence; a second one is a copy too. A copy is rewritten whole, keeping its own place, list item or block quote, and mdsh writes the kind of output the block has now (a fence or a region) whatever kind the copy was. When the block doesn't run (skipped, `run=` false, `--only`, an earlier stop, `--no-run`) the copy keeps what it holds. When the block has nothing to show for that part (no stdout, or stderr hidden), write-back stores an empty copy so a later run can fill it, and printing leaves it out. Copies are output: `--hide-output` and the block's `show-output=` hide them when printing. A copy whose `for=` names no block is left as it is and mdsh warns about it. While mdsh prints as it runs, a copy above its block is printed before the block runs, so it shows what it held; `-w`, `-o` and `--pdf` are always up to date.

**Conditions.** A condition is `{{$VAR}}` (mdsh's environment), `{{#block/env(VAR)}}` (a block's variable, from this run or a value stored in a placeholder), `{{#block/?}}` (a block's exit status), `{{$?}}` (the exit status of the last block before this point that ran), `{{#block/ran?}}` or `{{$ran?}}` (`true` if the block, or the last runnable block before this point, ran; `false` if it was skipped), true unless empty, `0`, `false`, `no` or `off`. `!` negates it, `==value` and `!=value` compare it as a string, and `true`/`false` are constants. Conditions drive `run=`, `show=`, `show-output=` and `show-stderr=`, and sections: everything between `<!-- mdsh:if COND -->` and `<!-- mdsh:end -->` lines (prose, blocks and output) is printed only when COND holds. Sections nest; the marker lines are never printed. Only `run=` changes what runs; the others change what is printed, and write-back keeps everything. In `run=`, `{{$?}}` is the previous block's status; in the display attributes, which are checked after the block runs, it is the block's own. `{{$ran?}}` follows the same rule. `err?` is another name for `?`; a status is true when non-zero, so `{{$err?}}` reads as "it failed". Block facts end in `?` so they never clash with shell variables. A block mdsh couldn't run (an mdsh error) has status 1. Use cases: a runbook whose diagnostics show with `DEBUG=1`, a cleanup that runs only after a failure, output shown only when a block fails, install steps shown only for the detected OS, output with secrets hidden unless asked for.

**Includes.** A block with the bare attribute `<include` holds lines from another file: `file=` (required, relative to the document's directory), `from=` and `to=` (line numbers counted from 1, inclusive; `from` defaults to the start, `0` meaning the same, and `to` to the end of the file) and `number_on` (show each line's number in the file). The fence's language is the language. mdsh reads the file on every run and writes the lines into the block's body, printed and on write-back, using a longer fence if the lines would close it; the body is mdsh's, so hand edits are replaced. `number_on` is a display hint: printed markdown prefixes the numbers, write-back stores the plain lines, and the TUI or GUI may number them their own way. An include block never runs, whatever its language, and has no name: `#id` and the step attributes (`env=`, `share=`, `run=`, `confirm`, `timeout=`, `background`, `tty`, `out=`, `on_error=`, `show-output=`, `show-stderr=`) are parse errors on it, placeholders can't name it, and it doesn't count toward `block-N`. Includes are content, not steps, so `--only`, a stop or the hide flags don't affect them. `skip` keeps the stored lines and `show=` hides the block; a missing file or a range past the end is an mdsh error, shown below the block.

**Terminal blocks (`tty`).** A block marked `tty` runs on a terminal, in its env's shell like any other block, so programs that need one (`docker run -it`, `ssh`, `sudo`, an editor, a REPL) work. Each shell of a document with `tty` blocks gets a pseudo-terminal of its own as its controlling terminal (it starts in a session of its own); a `tty` block's stdin, stdout and stderr are that terminal, and mdsh sits between it and the real one, like `script(1)`: mdsh's terminal goes raw, keys pass through, window size changes are passed on, and ctrl+c reaches the running command through the pseudo-terminal rather than mdsh. Ctrl+c ends the command, and the rest of the block goes on. Afterwards mdsh restores both terminals' settings. Everything the block shows is recorded with its timing and saved as an asciinema cast (asciicast v2, one JSON line per write), the lossless source a TUI or GUI can replay as text. mdsh also plays it through a terminal emulator into an animated GIF, for viewers that can't play a cast: a frame at most every 100ms, idle stretches capped at 2s, the last frame held 3s before it loops, and rows the session never reached left off. It is saved like `out=screenshot`, to `<doc>.assets/<block-id>.gif` (`out=recording:path` picks another path; `embed=inline` makes it a data URI), and linked between output markers. The cast goes beside it, at the same path with a `.cast` extension (with `embed=inline`, at `<doc>.assets/<block-id>.cast`). A session that shows nothing saves nothing. Still views can't animate, so the PDF shows the last frame, or frame `N` (counting from 1) with `frame=N`, written into the link as `#frame=N`; a frame past the end is the last. The TUI and GUI will read the same attribute. The block's exit status and variables count as usual. Without a tape (below), with no terminal (cron, CI, a pipe with no controlling terminal), or when mdsh is in the background, the block fails with an mdsh error. `--yes` doesn't change this: it answers mdsh's questions, not the block's own prompts. A block without `tty` that opens `/dev/tty` itself gets the shell's pseudo-terminal, which nobody is typing at, so it should be marked `tty`.

**Background blocks (`background`).** A block marked `background` starts in its env's shell the way `block &` would, and the run goes straight on to the next block. It runs in a subshell, so it sees the shell's variables, functions and working directory, but what it sets (variables, `cd`, `exec` redirections) stays its own, and placeholders can't read its variables. Its stdin is `/dev/null`. Its stdout and stderr go to files of their own, not the shell's output, so nothing it prints lands in another block's output; mdsh shows them under the block once it ends, by the usual rules. Its process ID is in the shell variable `MDSH_BG_<id>` (the id with anything but letters, digits and `_` turned into `_`), so a later block in the same env can `wait "$MDSH_BG_reader"` for it and get its exit status. mdsh checks for background blocks that have ended before each block it runs, and before a block's `run=` is checked, so `{{#id/?}}` is its exit status once it has ended (and 0 while it runs). `timeout=` counts from the block's start and kills just that block and the processes it started, not the shell, and the run goes on; the block's status is -1 and it fails with "timed out". After the last block (or a stop), mdsh waits for background blocks still running: until their `timeout=`, or for 5 seconds if they have none, and then kills them, keeping the output they had written. A background block that fails doesn't stop the run, even with `on_error: stop`, since later blocks have already run. It can't be `tty`, which is a parse error, since nobody is at its terminal. A syntax error is reported like any block's and nothing starts. A plain `&` job in a block still works, but it writes to the shell's output, so what it prints after its block ends shows up under whichever block is running then, or is lost between blocks. While mdsh prints as it runs (`confirm` or step mode), a background block that ends after later blocks were printed has its output printed then, under its name. [examples/fifo](../examples/fifo) uses background blocks to hand data between blocks through named pipes.

**Tapes.** A `tape {for=id}` fence types the input for the `tty` block `id`, in the input commands of a [VHS](https://github.com/charmbracelet/vhs) tape, so the block runs with nobody at the keyboard: in CI, under `-w`, or to regenerate its recording. `<include file=demo.tape` on the fence reads the tape from a file instead, as for any include, so an existing VHS tape works. A tape has no name and doesn't count toward `block-N`; it must name a `tty` block, and a block has at most one; anything else is a parse error. It prints like any code block, so readers see what was typed, and `show=` hides it. mdsh plays it while the block runs on an 80x24 terminal, whatever the real terminal is, so the recording comes out the same anywhere; if there is a terminal, the session is shown on it too, but keys typed there are not passed on. Commands: `Type "text"` (or `'text'`, `` `text` ``), the keys `Enter`, `Space`, `Tab`, `Backspace`, `Delete`, `Insert`, `Escape`, `Up`, `Down`, `Left`, `Right`, `Home`, `End`, `PageUp`, `PageDown` with an optional count (`Backspace 3`), `Ctrl+X`, `Alt+X`, `Ctrl+Alt+X`, `Shift+Tab`, `Sleep 500ms` (a bare number is seconds), `Wait /regex/` (until the last line, trailing spaces dropped, matches; `Wait+Screen` checks the whole screen; `Wait@5s` sets the timeout; with no regex, the wait pattern), and `Set TypingSpeed`, `Set WaitTimeout`, `Set WaitPattern`. `@speed` after a key or `Type` sets its typing speed for that line. Placeholders in a double-quoted `Type` string, `{{$VAR}}` from mdsh's environment and `{{#block/env(VAR)}}` from an earlier block (exported or not), are filled in as the tape starts, so a password or token is typed without being stored in the document; the recording holds only what the screen shows, so a prompt that turns echo off keeps the value out of it too. A placeholder whose block didn't run is an error. Strings in single quotes or backticks are typed as they are, so `Type '{{$HOME}}'` types the braces. This is mdsh's addition: VHS types every `Type` string as it is. Defaults are VHS's: 50ms per key, a 15s wait timeout and the pattern `>$`. `#` comments and blank lines are skipped, and so are the commands that shape VHS's own recording (`Output`, `Require`, `Hide`, `Show`, `Screenshot`, `Copy`, `Paste`, `Source`, `Env`, and `Set` of anything else, such as `Set Width`, which VHS measures in pixels). An unknown command is an error. Keys go to the block as soon as it starts, before the program in it may be ready, so a tape usually starts with a `Wait` or `Sleep`. If a `Wait` times out, or the block is still running 10s after the tape ends (most likely waiting for input), mdsh kills the block's shell and reports it; what was recorded so far is still saved.

**Stderr.** stderr is captured separately and shown in an `output {for=id stream=stderr}` fence only when the block fails, unless `show-stderr=` or front matter `show_stderr:` says otherwise. mdsh's own errors (timeouts, image errors) are always shown.

**Placeholders.** `{{#setup/env(NAME)}}` in the prose stands for the value `NAME` had right after block `setup` ran, exported or not (unset reads as empty). `{{$NAME}}` stands for `NAME` in mdsh's own environment. `{{#setup/?}}`, `{{$?}}`, `err?` and `ran?` stand for exit statuses and whether a block ran, as in conditions. mdsh reads each referenced variable from the block's shell once the block finishes. Printed output replaces the placeholder with the value. Write-back (`-w`) keeps the placeholder and stores the value inside it, `{{#setup/env(NAME):"world"}}`, with `"`, `\` and newlines escaped. A block that didn't run (skipped, `--only`, an earlier stop) leaves the stored value in place. Placeholders inside fenced or inline code are left alone, and one naming an unknown block is an error. `interpolate: false` under `mdsh:` turns them off.

## Environment model

Each env is one long-lived shell process. Blocks without `env=` all run in the top-level env, so variables, functions and the working directory carry over like lines of one script. A named sub-env is a separate shell stacked on its parent, and only shared vars cross the boundary.

- **Top-level env:** started once per run with the caller's environment, filtered by the front matter's `env.share` list if set.
- **Sub-env:** `env=build` creates it on first use and reuses it after, so two `env=build` blocks share state. `env=build/test` stacks `test` on `build`. `share=` is read only when the env is created.
- **Down (parent to child):** at creation, the child gets the parent's *exported* vars that pass `share`. `share=...` (or `share=*`) passes all, `-NAME` removes a name (`share=...,-TOKEN`); `share=` (empty) starts clean.
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
| `mdsh --no-run doc.md` (or `-n`) | Prose, code and the output saved in the file; nothing runs. Combines with `--hide-code` and `--hide-output`; `-o out.md` saves the printout to `out.md` (plain markdown), `-w` is an error | No |
| `mdsh -w doc.md` | Progress on stderr | Yes: output rewritten |
| `mdsh -o out.md doc.md` | Progress on stderr | No; writes what `-w` would to `out.md` (new files get the input's mode; image links are relative to `out.md`) |
| `mdsh --pdf doc.pdf doc.md` | Nothing; what it would print is laid out as `doc.pdf` (`-` for stdout). Hide flags and `--only` apply | No; with `-w` or `-o` those files are written too |
| `mdsh --only build doc.md` | That block, plus earlier blocks in its env chain so its state exists | Per flags |
| `mdsh completion bash\|zsh` | A completion script: flags, `.md` files, block ids after `--only` | No |
| `mdsh -i doc.md` | TUI (v2, not built yet) | Yes, as blocks run |

- `confirm` blocks: before running one, mdsh shows its code on the terminal (`/dev/tty`, so piping output still works) and asks `[r]un / s[k]ip / [o]nly run blocks without saved output / [s]top`, using the attribute's value as the question if given. Skip leaves the block unrun with its old output and goes on. `o` skips the block and, for the rest of the run, every asked-about block that has saved output, without asking; other blocks are still asked about. Enter means stop. Stopping leaves the block and everything after it unrun, keeps their old output, and exits 0. `--step` or front matter `step: true` asks before every block that would run, as if each were marked `confirm`. With no terminal, the run stops at the block with an error unless `--yes` is passed. The TUI will ask the same question in its own UI.
- `--hide-code` hides runnable blocks only; other code blocks are part of the prose.
- `--color=auto|always|never` controls styling, default `auto`. Auto styles only when stdout is a terminal and `NO_COLOR` is unset; otherwise it prints plain markdown. `--no-color` is short for `--color=never`.
- Execute mode writes to stdout, so `mdsh doc.md > report.md` gives a rendered report.
- `-` reads the markdown from stdin: `cat doc.md | mdsh -`.

**Shebang.** `mdsh file.md` with no subcommand means `mdsh run file.md`, so `#!/usr/bin/env mdsh` works on both Linux and macOS without `env -S`. Arguments after the file name are passed to blocks as `$1`, `$2`, and so on. mdsh skips the shebang line when parsing, drops it from printed output, and keeps it when writing back with `-w`. Trade-off: GitHub shows a shebang line as text and won't treat the front matter below it as front matter.

**Interactive mode (TUI, v2).** You type or pick a block; it runs in its env; its output block appears right under it and is saved to the file. New blocks typed at the prompt are appended to the document as fenced `sh` blocks. Re-running a block replaces its output. A `background` block is tagged `background` and shows as running in the background, with a count in the status line, while other blocks run; its output comes in when it ends, and a run from a block (`a`) waits for it at the end as the CLI does. Quitting kills background blocks still running.

## Block languages

v1 ships on `sh` plus a few markdown helpers.

**sh in v1, with extensions**

- `out=markdown` puts a block's output into the document as raw markdown, so a script can print a table or a list that renders.
- Each shell starts with helper functions: `md_h1`, `md_h2`, `md_h3` (headings), `md_table [-F SEP] [-s N]` (tab-separated lines on stdin, or split on SEP as `awk -F` does; the first line after skipping N is the header) and `md_code LANG` (stdin as a fenced block). They are plain shell functions, so blocks stay valid `sh`.

## Control socket and audit log (planned)

Not built yet. This section sets the rules before any code is written.

A block can talk back to the run that started it with `mdsh ctl`: set variables for later blocks, tick tasks, ask the user a question, and report progress. Every run also writes an audit log of what happened, socket calls included, so a run that `ctl` changed can be reviewed afterwards.

**What ctl can't do.** It can't add, remove, edit or run blocks, and it can't change prose apart from task markers. This keeps the line drawn in [Runnable blocks in generated output](#runnable-blocks-in-generated-output-2026-10-31): you can still read the file and know what can run. A `ctl` call can change *whether* a written block runs, through a variable that a `run=` condition reads, and the audit log records that.

**The socket.** At the start of a run (`mdsh doc.md`, `-w`, `-o`, reports), or of a session (`mdsh -i`, mdsh-gui), mdsh creates a Unix socket in a new temp directory with mode 0700, and removes both at the end. On Windows it is a named pipe, `\\.\pipe\mdsh-<random>`, that only the current user can open. mdsh exports two variables in every block's prelude, whatever its env and whatever `env.share` or `share=` say:

- `$MDSH_SOCK` is the socket's path.
- `$MDSH_TOKEN` is a random token for this run of this block. mdsh uses it to tell which block made a call, including a call from a process the block started in the background. A token stops working when its block ends, or for a `background` block, when that block ends.

**Protocol.** Each call opens a connection, sends one line of JSON, reads one line of JSON back, and closes. `mdsh ctl` is the client that blocks are expected to use. The protocol is versioned so other clients can be written later.

```json
{"v":1,"token":"…","cmd":"set","args":{"name":"env","value":"prod"}}
{"v":1,"ok":true}
{"v":1,"ok":false,"error":"no task matches \"deploy\"","code":2}
```

mdsh handles calls one at a time, in the order they arrive, so calls from background blocks never interleave.

**Commands.**

| Command | Does | Prints |
| --- | --- | --- |
| `mdsh ctl set NAME VALUE` | Sets `$MDSH_VAR_NAME` for later blocks, conditions and placeholders. `-` as the value reads stdin. `--secret` marks it secret | Nothing |
| `mdsh ctl unset NAME` | Removes a variable that `set` made | Nothing |
| `mdsh ctl get NAME` | Reads `$MDSH_VAR_NAME` as it is now, including values set after this block started | The value |
| `mdsh ctl check TASK` / `uncheck TASK` | Ticks or unticks a task in prose or a table cell. `TASK` is a number (as `SetTask` counts them) or a label that must match exactly one task | Nothing |
| `mdsh ctl ask QUESTION` | Asks for text. `--choice a,b,c` limits the answers, `--default V` is used when nobody can answer, `--secret` hides the typing | The answer |
| `mdsh ctl confirm QUESTION` | Asks yes or no. `--default yes\|no` | Nothing; the exit code is the answer |
| `mdsh ctl status TEXT` | Shows progress. `--step N --of M` adds a count | Nothing |

**Exit codes.** These are the same for every command, so a block can check them with `if` or `||`.

| Code | Means |
| --- | --- |
| 0 | Done, or `confirm` was answered yes |
| 1 | `confirm` was answered no |
| 2 | Refused: bad arguments, an unknown task, a token that has stopped working, or a name that an input binds |
| 3 | Not running under mdsh: `$MDSH_SOCK` is unset or nothing is listening on it |
| 4 | No answer: `ask` or `confirm` with nobody to answer and no `--default` |

`status` is the one exception to code 3. Outside mdsh it does nothing and exits 0, so a script that reports progress still runs on its own.

**Rules for variables.**

- `set` changes the variable for blocks that *start* afterwards, in every env. A block that is already running keeps the environment it started with. It can read the new value with `get`.
- `run=` and other conditions are checked just before their block runs, so they see every `set` made before that point.
- A name that a `textbox` or `choice` binds with `var=` belongs to that input. `set` on it is refused with code 2, since the input's text in the file would otherwise disagree with the variable.
- A `--secret` variable reaches blocks and conditions as usual. Placeholders print it as `***`, and write-back stores `***` in the file, never the value.

**Rules for tasks.** `check` follows the same write-back rules as a click in mdsh-gui. Under `-w`, `-i` and the GUI, the change is saved to the file. Under plain `mdsh doc.md`, it shows in what mdsh prints, and the file is not changed. In the GUI, edits you make by hand are still refused while a block runs; `ctl` edits are the exception, because they go through the session.

**Rules for questions.**

- **With someone to answer:** the GUI shows a dialog, the TUI a prompt, and the CLI asks on `/dev/tty`, as it does for `confirm` blocks.
- **Without:** this means `$MDSH_MODE` is `report`, or a CLI run with no terminal. `ask` and `confirm` take `--default`, and the log records that the default answered. With no default, the call exits 4, and mdsh shows "no answer: QUESTION" under the block as an mdsh error.
- `--yes` answers yes to `confirm` calls that have no default, the same way it does for `confirm` blocks.
- Time spent waiting for an answer counts toward the block's `timeout=`.

**Status.** In the CLI, the status line goes to stderr; in the TUI and GUI, it shows on the block's card. It replaces the previous status from the same block, and it goes away when the block ends. It is never written to the file.

**Audit log.** `--audit FILE` writes one JSON object per line, in order, for the whole run. mdsh-gui and `mdsh -i` keep a log for each run in the session, and the GUI can save one. Every event has `v`, `seq` (counting from 1), `t` (RFC 3339 with milliseconds) and `event`, plus `block` and `env` when they apply. Events:

- `run.start`: the file, a SHA-256 of its bytes, `$MDSH_MODE`, the flags and arguments, the mdsh version. `run.end`: the exit code and whether the run stopped early.
- `env.start`: an env's shell starting, with its parent and the names `share` let through (names only, no values).
- `block.start`, `block.end` (with the exit code and the time taken), `block.skip` (with the reason: `skip`, `run=` and the value it saw, `once`, `--only`, a stop or a confirm answer).
- `ctl`: every call, with the calling block, the command, its arguments, the result and the exit code. Refused calls are logged too.
- `var`: a variable changing, with the old and new values and what changed it (`ctl`, an input, a placeholder read).
- `task`: a task ticked or unticked, with its number, its label and what changed it.
- `ask`: the question, the answer, and who answered it (`user`, `default`, `yes-flag`, or `none` for code 4).
- `write`: mdsh changing the file: an output region, a task, an input, a placeholder value, or a History entry.

`--secret` values never appear in the log. Where a value would go, the log stores `{"secret":true,"hmac":"…"}`, an HMAC-SHA-256 keyed with a random key for each run. So you can tell whether two secret values in one run were the same, but a short secret can't be guessed from the log. Answers to `ask --secret` are treated the same way.

**Open questions.**

- Should `check` also reach tasks in a block's `out=markdown` output, as clicks in mdsh-gui can?
- Should `set` on a name an input binds update the input, instead of being refused?
- Does the audit log need its own viewer in mdsh-gui, or is a file enough for now?

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
- **Custom language:** dropped on 2026-10-29. Blocks are `sh` or `zsh`, with the markdown helpers.
- **Confirm:** a `confirm` block asks to run or stop; `--yes` skips the question.
- **Images:** `out=image` for image bytes, `out=screenshot` to draw text output as an image.
- **Conditions:** `run=`, `show=`, `show-output=`, `show-stderr=` and `mdsh:if` sections, driven by `{{$VAR}}`, `{{#block/env(VAR)}}` and the block facts `?` (alias `err?`) and `ran?`.
- **Stderr:** shown when the block fails, hidden otherwise; `show-stderr` overrides both ways.
- **Placeholders:** `{{#block/env(VAR)}}` in prose, on by default, `interpolate: false` to turn off.
- **Output copies:** an `output {for=id}` fence or region that isn't right after block `id` is refilled from it on every run, like a placeholder. Settled on 2026-10-29.
- **Includes:** a block marked `<include` with `file=`, `from=`, `to=` (1-based, inclusive) and `number_on`; mdsh writes the lines into the block's body, and the fence's language is the language. Include blocks never run and have no name.
- **Control socket:** blocks talk back to the run with `mdsh ctl` over a socket in `$MDSH_SOCK`, limited to vars, tasks, questions and status; every run can write a JSON Lines audit log. Settled on 2026-10-31; not built yet.

## Not pursued

Ideas we looked at and decided against, with the reasons, so they aren't
picked up again without a new argument.

### Runnable blocks in generated output (2026-10-31)

**The idea.** A block with `out=markdown` can print fenced `sh` or `zsh`
blocks. Today they land in its `<!-- mdsh:output -->` region and print as
plain code. The idea was to make them real blocks, with ids, a Run button
in the GUI and saved output, that never run on their own: only when picked
one at a time. The use case was prechecks a block could detect for the
env it runs in, such as one check per service it finds.

**Why not.**

- **You can't read the file and know what runs.** What a document can
  run would depend on what an earlier run printed. A runbook should be
  reviewable as written.
- **Ids stop being stable.** Generated ids can clash with written ones,
  and with each other. Placeholders and conditions that name a generated
  block can't be checked until its parent has run, so an unknown id would
  have to stop being an error everywhere, or the parent would have to
  declare the ids it makes.
- **Re-runs throw work away.** Re-running the parent rewrites its region,
  which drops the generated blocks, their saved output, and any edits.
- **Regions would have to nest.** Every region closes with the same
  `<!-- /mdsh:output -->` line, so saving a generated block's own
  `out=markdown` output means changing how regions are found, plus a
  depth limit for blocks that print blocks.
- **Every front end gets a new case.** The CLI, TUI, GUI, reports, the
  outline, the session's sync, and editing would all need rules for
  blocks that come and go. That is several PRs of complexity for one use
  case.
- **It's code written by code.** It would want its own confirm step
  anyway.

**Instead.** Write the prechecks as ordinary blocks and gate each one
with `run=` on an environment variable or a detected fact, as
[Optional prechecks](../README.md#optional-prechecks) shows. They stay
visible and reviewable, keep stable ids, and a full run includes them.
Generated output stays plain markdown: fences in it print as code.
