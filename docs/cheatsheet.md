---
title: Cheat sheet
description: Every attribute, key, flag and shortcut on one page.
weight: 60
---

One page of everything: what goes in a fence, in the front matter and on
the command line, and which key does what in `mdsh -i` and the desktop
app. Each row says only what a thing is; the other pages say how it
behaves.

## Command line

| Command | Does |
| --- | --- |
| `mdsh doc.md [args…]` | Run every block, print the markdown with output under each block. Same as `mdsh run doc.md`; `#!/usr/bin/env mdsh` works. `args` reach blocks as `$1`, `$2`… |
| `mdsh -w doc.md` | Run and write the output back into the file |
| `mdsh -o out.md doc.md` | Like `-w`, into `out.md`; `doc.md` is untouched |
| `mdsh -n doc.md` (`--no-run`) | Print with the output saved by `-w`; nothing runs. `-n -o view.md` saves that printout |
| `mdsh --hide-code doc.md` | Prose and output only |
| `mdsh --hide-output doc.md` | Prose and code only; with `--hide-code` too, prose only and nothing runs |
| `mdsh --only build doc.md` | Run that block, plus the earlier blocks in its env chain |
| `mdsh --yes doc.md` | Run `confirm` blocks and `depends=` blocks without asking |
| `mdsh --step doc.md` | Ask before every block, as if each were marked `confirm` |
| `mdsh --stop-on-error doc.md` | Stop at the first failing block |
| `mdsh --expose DEBUG doc.md` | Let `{{$DEBUG}}` read your environment; repeatable or comma-separated. `MDSH_EXPOSE=A,B` does the same |
| `mdsh --source defs.zsh doc.md` | Source a file into every shell before its first block; repeatable; a pipe works |
| `mdsh --audit run.jsonl doc.md` | Also write a JSON Lines log of the run |
| `mdsh --allow-outside doc.md` | Let includes, `out=` files and report images reach outside the document's folder, or hidden files (`.env`, `.git`) in it; only for documents you trust |
| `mdsh --pdf doc.pdf doc.md` | Write what would be printed as a PDF (`-` for stdout); the hide flags, `--only` and `--step` apply |
| `mdsh --html doc.html doc.md` | The same as a standalone HTML page; not with `--pdf` |
| `mdsh --show-secrets --pdf out.pdf doc.md` | Put `secret` blocks' code and output in the report; without it they read `(secret)` |
| `mdsh --color=never doc.md` | No styling on a terminal; `--no-color` and `NO_COLOR` too (`auto`, `always`, `never`) |
| `mdsh -i doc.md` | The terminal UI; `--autosave` saves after every block that runs |
| `mdsh -i [folder]` | A start screen listing the markdown files under this folder, or that one |
| `mdsh - < doc.md` | Read the document from stdin |
| `mdsh completion bash\|zsh` | A completion script: flags, `.md` files and, after `--only`, block ids |
| `mdsh lsp` | A language server on stdin and stdout: completes and explains fence attributes, flags mistakes |
| `mdsh ctl …` | From inside a block, talk to the run (below) |
| `mdsh --version` | The version |

Exit status: 0, 1 when any block failed, 2 on an error such as a bad flag
or file.

`mdsh ctl` works only in a block mdsh is running:

| Command | Does |
| --- | --- |
| `mdsh ctl set [--secret] NAME VALUE` | Set `$MDSH_VAR_NAME` for the blocks after this one; `-` reads the value from stdin; `--secret` has mdsh-gui blur output that names it |
| `mdsh ctl unset NAME` | Remove a variable `set` made |
| `mdsh ctl get NAME` | Print `$MDSH_VAR_NAME` as it is now |
| `mdsh ctl ask [--choice a,b] [--default V] [--secret] QUESTION` | Ask for text and print the answer |
| `mdsh ctl confirm [--default yes\|no] QUESTION` | Ask yes or no; the exit code is the answer |
| `mdsh ctl status [--step N --of M] TEXT` | Show progress for this block |
| `mdsh ctl help` | List these. Exit codes: 0 done or yes, 1 no, 2 refused, 3 not under mdsh, 4 no answer |

`mdsh-gui [flags] [file.md [args…] | folder]` takes `--source FILE`,
`--only ID`, `--step`, `--stop-on-error` and `--version`; on Linux the
AppImage runs the command line tool when started with `--cli` first.

### Questions a run asks

| Prompt | Answers |
| --- | --- |
| `Run this block?` (`confirm`, `--step`, `step: true`) | `r` run · `k` skip · `o` skip this and every later block with saved output · `s` or enter stop |
| `Run build first?` (`depends=`) | `r` run · `k` go on without it · `s` or enter stop |

With no terminal the run stops at a question unless `--yes` answers it.

## Fence attributes

Attributes sit in `{…}` after the language: ```` ```sh {#build env=ci confirm="Deploy?"} ````.
Every `sh` and `zsh` block runs; other languages are left alone. Unknown
attributes are kept as written.

| Attribute | Does |
| --- | --- |
| `#id` | The block's name, for output, placeholders, `depends=` and `--only`. Saving gives an unnamed block `block-<uuid>` |
| `skip` | Don't run (picking the block by hand in `mdsh -i` or mdsh-gui still runs it) |
| `run=COND` | Run only if the condition holds; `run=once` runs it only while it has no output |
| `show=COND` | Print the code only if the condition holds |
| `show-output=COND` | Print the output only if the condition holds |
| `show-stderr[=COND]` | Show stderr always, or if the condition holds; by default only when the block fails |
| `collapse[=COND]` | In mdsh-gui, open with the code folded to one line |
| `collapse-output[=COND]` | In mdsh-gui, open with the output folded to one line |
| `nosave` | Saving keeps what the file holds: the output, a textbox's text or a choice's picks live in the session only. On an `output` fence, just that fence |
| `secret` | The code and output hold a secret: mdsh-gui and `mdsh -i` mask them until shown, and as with `nosave` the output isn't saved |
| `links` | Make `http:`, `https:` and `mailto:` addresses in the output clickable in mdsh-gui and `mdsh -i` |
| `env=NAME` | Run in a named sub-env, a shell of its own; `a/b` stacks `b` on `a` |
| `share=A,B` | Exported variables a new sub-env gets from its parent, besides the base set. `...` or `*` all, `-NAME` drops one, empty just the base set. Default: front matter `env.share`, else all |
| `on_error=stop\|continue` | Stop the run if this block fails, or go on. Default: front matter `on_error` |
| `depends=A,B` | Blocks to run first, in order; each one that hasn't run this session is asked about |
| `always` | As a dependency, run again each time a block that depends on it runs |
| `confirm[="Question?"]` | Ask before running, with this question if given |
| `timeout=30s` | Interrupt the block after this long and stop the run; its shell stays. A `background` block is killed alone |
| `background` | Start the block and go on to the next; the run waits for it at the end. `$MDSH_BG_<id>` holds its PID for `wait` |
| `tty` | Run on a terminal, for `docker run -it`, `ssh`, an editor; the session is saved as an asciinema cast |
| `frame=N` | With `tty`, or on a `cast` fence, the frame still views show and the player opens at, from 1 |
| `out=text` | Output goes in an `output` fence (the default) |
| `out=markdown` | Output is inserted as markdown, between `<!-- mdsh:output for="id" -->` and `<!-- /mdsh:output -->` |
| `out=image[:path]` | Output is image bytes (PNG, JPEG, GIF, SVG), saved beside the file and linked |
| `out=screenshot[:path]` | The text output drawn as a terminal-style image: SVG, or PNG when the path ends in `.png` |
| `out=recording:path` | With `tty`, where the cast goes; a path ending in `.gif` links an animated GIF too |
| `embed=inline` | With `out=image` or `out=screenshot`, a data URI instead of a file |

Paths default to `<doc>.assets/<id>.<ext>`; `res://name` is `name` in
that folder.

### Include blocks

```` ```go {<include file=cmd/main.go from=10 to=24 number_on} ```` fills
the block with lines from a file on every run. It never runs, has no name
and takes none of the attributes above.

| Attribute | Does |
| --- | --- |
| `<include` | Marks the block as an include |
| `file=PATH` | The file, relative to the document; `res://name` is in its assets folder (required) |
| `from=N` | First line, counting from 1 (default the first) |
| `to=N` | Last line, inclusive (default the last) |
| `number_on` | Show each line's number in the file |

### Input blocks

A `textbox` fence is a box to type in; a `choice` fence lists its options
as task items (`- [x] prod`). Both write what is picked back into the file
on save.

| Attribute | On | Does |
| --- | --- | --- |
| `var=NAME` | both | Bind the text, or the checked options one per line, to `$MDSH_VAR_NAME` for blocks and conditions |
| `lines=N` | `textbox` | The most lines it holds; 1 or less is one line |
| `lines=fit` | `textbox` | Any number of lines; mdsh-gui sizes the box to the text |
| `mode=dropdown\|radio\|multi` | `choice` | Pick one from a list (default) or buttons, or check any number |
| `placeholder="…"` | both | What an empty textbox, or a dropdown with nothing chosen, shows |
| `nosave`, `secret` | both | As above: what is typed stays in the session |

### Other fences and markers

| Fence or line | Does |
| --- | --- |
| ```` ```output {for=id exit=0} ```` | A block's output, right under it; elsewhere, a copy mdsh refills on every run. `stream=stderr` is its stderr; `nosave` keeps that fence out of the file |
| `<!-- mdsh:output for="id" -->` … `<!-- /mdsh:output -->` | The same for `out=markdown` and image output |
| ```` ```tape {for=id} ```` | Types the input for `tty` block `id`, in VHS commands: `Type "…"`, `Enter`, `Tab`, `Up`…, `Ctrl+X`, `Alt+X`, `Sleep 1s`, `Wait /regex/`, `Set TypingSpeed`, `WaitTimeout`, `WaitPattern`. Double-quoted `Type` takes placeholders. Can be an include |
| ```` ```cast ```` | An asciinema recording, played in mdsh-gui; `frame=N` opens it there |
| ```` ```d2 ````, ```` ```svg ```` | A diagram, a picture, drawn in mdsh-gui |
| `- [ ] task` | A task item mdsh-gui checks in the file when clicked; table cells starting with `[ ]` too |
| `<!-- mdsh:if COND -->` … `<!-- mdsh:end -->` | Print the stretch only if the condition holds; sections nest. It hides but doesn't stop blocks: gate them with `run=` |
| `<!-- mdsh:collapse [COND] -->` | Under a heading: mdsh-gui opens the section folded |
| `#!/usr/bin/env mdsh` | On the first line: `chmod +x` and run the file |

## Conditions and placeholders

| Form | Means |
| --- | --- |
| `{{#setup/env(NAME)}}` | The value `NAME` had right after block `setup` ran, exported or not |
| `{{#setup/env(NAME):"world"}}` | The same, with the last value stored by `-w` so the file reads without running |
| `{{$NAME}}` | `NAME` from the environment mdsh started in, if exposed with `--expose` or `MDSH_EXPOSE`; never saved. `MDSH_` variables need no exposing |
| `{{$?}}`, `{{#build/?}}` | The exit status of the last block that ran, or of block `build` |
| `{{$err?}}`, `{{#build/err?}}` | The same statuses, read as true when non-zero |
| `{{$ran?}}`, `{{#build/ran?}}` | Whether the last runnable block, or `build`, ran |
| `{{$MDSH_VAR_name}}`, `{{$MDSH_MODE}}` | An input's value; `running`, `interactive` or `report` |

A condition is a placeholder without the `:"…"` part, negated with `!` or
compared with `==` or `!=`: `run={{$DEBUG}}`, `show={{$ran?}}`,
`run={{#detect/env(OS)}}==Linux`, `show-output={{$?}}!=0`. `true`,
`false` and (in `run=` only) `once` are conditions too. A bare value is
true when set and not `0`, `false`, `no` or `off`. In `run=`, `{{$?}}` is
the previous block's status; in `show=`, `show-output=` and
`show-stderr=` it is the block's own. Placeholders in code and inline
code are left alone; `interpolate: false` turns them off in prose.

## Front matter

Under an `mdsh:` key at the top of the file; every key is optional, and
unknown ones are ignored.

```yaml
---
title: Build notes
description: What this runbook does, for the start screens
tags: [ops, prod]
mdsh:
  shell: /bin/bash
  on_error: stop
  step: false
  interpolate: true
  show_stderr: false
  terminal:
    cols: 100
    rows: 30
  env:
    share: [..., -GITHUB_TOKEN]
    source: [~/.zsh_aliases, lib/helpers.sh]
---
```

| Key | Values | Default | Does |
| --- | --- | --- | --- |
| `shell` | a POSIX shell or zsh | `/bin/sh`, or `zsh` if any block is `zsh` | The shell that runs every block, in every env |
| `on_error` | `continue`, `stop` | `continue` | Whether a failing block stops the run; a block's `on_error=` and `--stop-on-error` override it |
| `step` | `true`, `false` | `false` | Ask before each block, as if every block were marked `confirm` |
| `interpolate` | `true`, `false` | `true` | Fill in `{{…}}` placeholders in prose |
| `show_stderr` | `true`, `false` | unset | Show every block's stderr, or none; a block's `show-stderr` overrides it |
| `terminal.cols`, `terminal.rows` | cells, up to 1000 by 500 | fit the card, 24 | The size of the terminal mdsh-gui runs `tty` blocks on |
| `env.share` | names, `...`, `-NAME` | the base set | Variables from mdsh's environment that reach the top-level shell, and the default `share=` for sub-envs |
| `env.source` | file paths | none | Files sourced into every shell before its first block; `~/` is home, other relative paths are from the document |

The base set is `PATH`, `HOME`, `USER`, `LOGNAME`, `SHELL`, `TERM`,
`COLORTERM`, `LANG`, `LANGUAGE`, `LC_*`, `TZ`, `TMPDIR` and, on macOS,
`__CF_USER_TEXT_ENCODING`. `title`, `description` (else `summary`, else
`abstract`) and `tags` are read by the start screens and the Front matter
form, not by the run.

Front matter that doesn't parse, such as `title: Setup: first steps`
with its colon unquoted, doesn't stop the file opening: mdsh-gui and
`mdsh -i` show the error over it, and no block runs until it is fixed.
When a colon in a value is why, they offer to quote it. `mdsh FILE`
runs nothing and says why.

A `.mdsh.yaml` in a folder makes it a collection on mdsh-gui's start
screen: `title`, `description`, `icon` (an image in the folder or an
emoji), `color`, `background` and `tags`.

## Inside a block

| Name | Is |
| --- | --- |
| `$1`, `$2`… | The arguments after the file name |
| `$MDSH_BLOCK`, `$MDSH_FILE`, `$MDSH_DIR` | The block's id, the document's path and folder |
| `$MDSH_MODE` | `running` from the command line, `interactive` in `mdsh -i` and mdsh-gui, `report` for `--pdf` and `--html` |
| `$MDSH_VAR_name` | A `textbox` or `choice` with `var=name`, or what `mdsh ctl set` set |
| `$MDSH_BG_id` | The PID of `background` block `id` (anything but letters, digits and `_` becomes `_`), for `wait` |
| `md_h1`, `md_h2`, `md_h3` | Print a heading |
| `md_table [-F SEP] [-s N]` | TSV on stdin to a markdown table; `-F` splits as `awk -F` does, `-s` skips lines |
| `md_code` | Print a code block |

Blocks without `env=` share one shell, so variables, functions and the
working directory carry over. An `exit` ends the shell and the run. Every
env starts in the document's folder.

## `mdsh -i` keys

`?` lists these in the app, greying out the ones that do nothing right now.

| Key | Does |
| --- | --- |
| `j`/`k`, ↓/↑, tab | Next or previous block |
| `g`/`G` | First or last block |
| space, pgdn / pgup, ctrl+b | Page down / up |
| ctrl+d / ctrl+u | Half a page down / up |
| enter, `r` | Run the block |
| `a` | Run from this block to the end |
| `s` | Run the section the block is in |
| `i`, `:` | Type a new `sh` block and run it; esc cancels |
| `h` | History: a shell along the bottom whose commands are added to the file as `#history-N skip links` blocks |
| `,` | Run options: arguments, `--only`, step, stop on error |
| ctrl+c | Stop the run, keeping the shells; again kills the shell of a block that won't stop. Quits when nothing runs |
| ctrl+] | Stop a `tty` block running in its card |
| `e` | Edit the block in `$VISUAL`, `$EDITOR` or `vi`; enter runs it |
| `E` | Edit the whole file the same way, at the block |
| `F` | Fix front matter that doesn't parse, when mdsh can: quote a value with a `: ` in it |
| `y` | Copy the block's code to the clipboard (OSC 52, and the system clipboard locally) |
| `V` | Select lines of History's output (shift+↑ in the pane too): ↑/↓ `j`/`k` extend, `g`/`G` ends, `o` other end, `y` or enter copies, esc cancels |
| `J`/`K`, alt+↓/↑ | Move the block, with its output and tape, down or up |
| `u` | Undo the last move |
| `w`, ctrl+s | Save; a read-only file opens save as |
| `W` | Save as another path, relative to the file's folder |
| `R` / `M` | When the file changed on disk: reload it / keep yours |
| `p` | Create a report: PDF, HTML or markdown |
| `v` | Show the block's image or recording full screen; enter goes back |
| `o` | Show or hide the outline |
| `x` | Show or hide the selected block's secret |
| `C` / `O` | Show or hide code / output |
| `]` / `[` | Next / previous tab |
| ctrl+t | New tab, on the list of files |
| ctrl+w | Close the tab |
| `q` | Back to the list of files, or quit |
| `?` | The keys; ↑/↓ pick one, enter does it, esc closes |

In the History pane: tab completes, ↑/↓ or ctrl+p/n recall, ctrl+a/e,
ctrl+b/f, alt+b/f, ctrl+w, alt+d, ctrl+u/k, ctrl+y edit, ctrl+l clears,
esc goes back to the document, ctrl+d closes the pane. On the start
screen: type to filter, ↑/↓ or tab pick, enter opens, esc goes back,
ctrl+c quits. A `tty` block takes every key while it runs, ctrl+c
included.

## mdsh-gui

### Keys

Keys follow `mdsh -i` where they don't clash with the OS. ⌘ on macOS is
ctrl on Linux.

| Key | Does |
| --- | --- |
| `j` / `k` | Select the next / previous block |
| enter | Run the selected block, or the selected heading's section |
| `a` | Run from the selected block |
| `s` | Run the section the selection is in |
| `e` | Edit the block in the source view |
| `c` | Copy the block's code |
| `d` | Depends On…: edit the block's `depends=` as a list |
| delete, backspace | Delete the selected block |
| ⌘↩ / ⌘⇧↩ / ⌘R / ⌘. | Run Current Block / Run Section / Run All / Stop |
| ⌘⌥K | Clear All Outputs… |
| ⌘\ / ⌘⇧\ | Source View / Rendered View |
| ⌘⇧O / ⌘⇧H / ⌘⇧Y | Outline / History / Console |
| ⌘⇧M | Front Matter form; esc puts the page back |
| ⌥↑ / ⌥↓ | Move Block Up / Down past the next cell; with ⇧, past the next block |
| ⌘Z / ⌘⇧Z | Undo / Redo |
| ⌘N / ⌘T / ⌘⇧N / ⌘O | New File… / New Tab / New Window / Open… |
| ⌘S / ⌘⇧S / ⌘W / ⌘⇧W | Save / Save As… / Close Tab / Close Window |
| ⌘⇧] / ⌘⇧[ | Next / previous tab |
| ⌘, | Settings (Preferences on Linux) |
| ⌘Q | Quit (Linux; macOS has the app menu's) |
| esc | Cancel a drag |

In the outline ↑/↓ move between entries and ←/→ fold and unfold a
section; the window's tabs take the arrows, enter and delete. The
History pane takes the same line keys as `mdsh -i`'s, shift+enter for
another line, ctrl+c to stop a command (or copy selected lines),
ctrl+z to stop and keep the job, ctrl+d to close. With Vim keys on, `:w`
saves and `:RunBlock` and `:RunSection` run; in the Neovim pane `K`
explains an attribute and `zc`/`zo` fold.

### Menus

| Menu | Items |
| --- | --- |
| mdsh (macOS) | About mdsh · Settings… · Install / Uninstall Command Line Tool… · Hide · Quit |
| File | New File… · New Tab · New Window · Open… · Open Recent · Save · Save As… · Create Report… · Reload · Close Tab · Close Window · Quit (Linux) |
| Edit | Undo · Redo · Move Block Up · Move Block Down · Move Block Above Previous Block · Move Block Below Next Block · Depends On… · Front Matter · Cut · Copy · Paste · Select All · Preferences… (Linux) |
| Run | Run Current Block · Run Section · Run All · Stop · Clear All Outputs… · Run Options… |
| View | Source View · Rendered View · Outline · History · Console · Show Code · Show Output · Folding › Collapse All Blocks, Collapse All Sections, Expand All, Copy Folds to Source View, Copy Folds to Rendered View · Zoom In · Zoom Out · Actual Size · Toggle Full Screen · Show Next Tab · Show Previous Tab |
| Help | Cheat Sheet · Changelog |
| Insert (right-click in the source view) | Heading · Bold · Italic · Code · Link · Image · Bulleted List · Numbered List · Task List · Quote · Table · Code Block · sh Block · sh Block, Confirm First · zsh Block · Textbox · Choice |

Settings has six tabs: General, Look, Editing, Shell, Files and About;
About links to this cheat sheet and the changelog, which open in
read-only tabs. Right-clicking a recording offers Copy and Copy as GIF, and the History
transcript Copy and Select All.
