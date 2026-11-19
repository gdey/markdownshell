# Job control: run the shell interactively on its terminal

Design. Nothing here is built yet. It follows from the ask to use mdsh as
a primary terminal, where ctrl+z, `fg`, `bg` and `jobs` have to work the
way they do in a terminal, and from the choice (2026-11-19) of option A:
run the env's shell as an interactive shell on its own pseudo-terminal
and learn where commands start and end from shell integration marks, as
VS Code, iTerm2, WezTerm and Kitty do.

## The ask

mdsh should be usable as the terminal you live in. Shell history today
is something that scrolls away; in mdsh every command typed becomes a
block with its output, so an exploration turns into a documented path.
That only works if mdsh gives up nothing a terminal has: suspending a
job with ctrl+z and picking it up later with `fg`, backgrounding a job
and foregrounding it again, `jobs`, the shell's own prompt, rc files,
completion and line editing. And all of it still has to be recorded into
the document: what was typed, what it showed, how it ended.

## How the shell runs today (what the design has to fit)

`env.Shell` (`env/shell.go`) starts the shell as a **non-interactive**
process reading commands from a pipe on stdin, with stdout and stderr on
pipes of their own. A block is written to a temp file and the shell is
fed a line that sources it, followed by `printf` lines that write a
random marker and `$?` on stdout and the marker on stderr. `demux`
(`env/demux.go`) cuts the streams at the markers, so each block's stdout
and stderr are captured apart and the exit status is exact. `Shell.Run`
is also how mdsh asks the shell questions: the prompt (`PS1`, `PS2`,
expanded by `history.promptScript`), completion candidates, exported
variables for a new sub-env, and the variables placeholders read.

With `Config.TTY` the shell also gets a pseudo-terminal as its
controlling terminal, in a session of its own, but it still reads
commands from the pipe. A `tty` block or a History command (`RunTTY`,
`RunTerminal`) is sourced with its three descriptors redirected to the
terminal (`0<>/dev/pts/N 1>&0 2>&0`), with mdsh copying keys in and
output out, and a marker written to the terminal as well so mdsh knows
when the block's output is all read.

Job control is where this breaks down:

- A non-interactive shell has job control off. `set -m` turns it on in
  dash, which then runs each command in a process group of its own and
  gives it the terminal. bash honours `set -m` only when interactive,
  and zsh takes the terminal for its jobs only when interactive. So
  `jobControl()` and `jobsOn()` turn job control on for dash alone, and
  for bash and zsh mdsh ignores `SIGTSTP`, `SIGTTIN` and `SIGTTOU`
  around the block (`trap '' TSTP TTIN TTOU`) so that ctrl+z, which the
  terminal delivers to the whole foreground group, shell included, does
  not stop the shell. Programs inherit the ignore, so vim and less can't
  suspend.
- A program that stops itself with `kill(0, SIGTSTP)`, as nvim does
  after ctrl+z, stops the shell too on macOS, and nothing ever resumes
  it. PR 309 adds a watcher that sends `SIGCONT` to a stopped shell.
  It stops the hang; it doesn't give bash or zsh job control.
- `zsh -i` on the pipes doesn't work either. A quick test in the ctrl+z
  thread hung at startup; j's own test, through a wrapper script, got
  past startup and stalled at the first command typed, with Stop and
  Processes doing nothing. Whatever the exact point, an interactive
  shell is built around its terminal: it reads commands from it, draws
  its prompt on it (zsh prints `PS1` to stderr when that is where the
  terminal is), and with job control on it puts each command in a
  process group of its own and makes that the terminal's foreground
  group. Today's `interrupt` sends `SIGINT` to the shell's process
  group, which an interactive shell ignores at its prompt and which no
  longer holds the command, so Stop can't reach what is running. The
  marker scheme itself isn't the blocker: `demux` matches a marker
  anywhere in the stream, prompt text before it or not. The fix is to
  stop driving the shell through pipes at all, which is this design.
- Even in dash, a job stopped during one block is in the shell's job
  table, but mdsh knows nothing about it: the next block's `fg` works
  by accident, and the UI can't say a job is stopped.

Two other things the design has to keep working: `shell:` in the front
matter (and the Run Options shell) is a path to a binary, passed to
`exec.Command` as the whole argv, so `shell: zsh -i` doesn't parse into
arguments and fails to start (so does `check`, which runs
`<shell> -n file`). And PR 308 (`background tty`) starts a second shell
of the same kind on a pty of its own and carries the parent shell's
state over through a dump file; it touches the same pty code.

## Proposal

### 1. Two ways to drive a shell

`env.Shell` gets a second mode, chosen in `Config`:

- **Scripted** (today): commands on a pipe, markers on pipes, stdout and
  stderr apart. Used when there is nobody at a terminal: `mdsh file.md`,
  `-w`, CI, reports, tapes without a terminal, and every sub-env (see 7).
- **Interactive** (new): the shell runs with `-i` on its pty, reads
  commands from it, draws its own prompt, and mdsh types into it. Used
  by any session that has a terminal to show: mdsh-gui, `mdsh --serve`
  and `mdsh -i`, whenever their History pane is on, and for every block
  that session runs in the top-level env.

The scripted mode changes nothing, so batch runs are not at risk. The
rest of this document is about the interactive mode.

### 2. Starting the shell

The shell is started as `<path> -i` on the pty (`Setsid`, `Setctty`),
with stdin, stdout and stderr all the pty, so it is a plain terminal
session: job control is on, the shell owns the terminal, and the
foreground process group is whatever the shell hands it to. `shell:`
stays a path. mdsh adds `-i` itself, and a login flag (`-l`) when the
setting asks for it, since on macOS `PATH` often comes from
`/etc/zprofile`. `zsh -i` written into `shell:` stays an error; the
message should say that mdsh runs the shell interactively on its own.

The shell reads its own rc files, the way it does in Terminal.app or
iTerm2. The History pane's `HistoryRC` setting (a file to source, with
its output dropped) goes away, replaced by the login toggle. mdsh adds
its integration **after** the user's rc files, by the hook each shell
has for a terminal to do that:

| shell | how the integration loads | command start / end marks | command text |
| --- | --- | --- | --- |
| zsh | `ZDOTDIR=$MDSH_TMP/zdot`; its `.zshenv` restores the user's `ZDOTDIR` and sources their `.zshenv`, its `.zshrc` sources their `.zshrc` (and `.zprofile`/`.zlogin` for a login shell), then adds `precmd` and `preexec` hooks with `add-zsh-hook` | `preexec` prints C; `precmd` prints D with `$?`, then A, and `PS1` ends with B | `preexec`'s first argument, the line as typed |
| bash | `--rcfile $MDSH_TMP/bashrc`, which sources the user's `.bashrc` (or the profile files for a login shell) and appends to `PROMPT_COMMAND` | `PROMPT_COMMAND` prints D and A; `PS0` prints C (bash 4.4+); on bash 3.2 (macOS `/bin/bash`) a `DEBUG` trap prints C once per command line, the way bash-preexec does | `fc -ln -1` in the hook, or the screen text between B and C when `HISTCONTROL` dropped it |
| sh (dash, busybox ash, bash as `sh`) | `ENV=$MDSH_TMP/shrc`, which POSIX interactive shells read; it sources the user's `$ENV` if they had one | `PS1` itself: `\e]133;D;$?\a\e]133;A\a` followed by the old `PS1`. dash 0.5.12 expands `$?` in `PS1`; so does bash. No preexec exists, so C is where the echoed command line ends | the screen text between B and C |

The marks are **OSC 133** (`ESC ] 133 ; X BEL`): `A` prompt start, `B`
prompt end (input starts), `C` command output starts, `D;status`
command ended. It is the FinalTerm scheme that every terminal with shell
integration already uses, so a user whose rc files already emit it (p10k,
iTerm2's integration, Kitty's) breaks nothing: mdsh's hooks skip theirs
when `$MDSH_INTEGRATED` is set, and mdsh's reader takes the first mark
of each kind per command and ignores repeats. mdsh's own marks carry an
extra field, `D;status;mdsh`, so it can tell its own apart when it has
to.

A prototype in this thread's container (bash 5.2 and dash 0.5.12 on a
Python pty; not committed) checked the mechanics:

- `sleep 100`, ctrl+z → `[1]+ Stopped`, D with 148 (128 + `SIGTSTP`;
  146 on macOS, where `SIGTSTP` is 18), prompt back. `jobs`, `bg`, `fg`
  behave as in a terminal, and ctrl+c on the foregrounded job gives
  D with 130.
- bash runs the rest of a list after a job stops: `sleep 100; echo x`
  prints `x` after `Stopped`, and so does a sourced file. The block's
  status is then the last command's, not 148, so "a job of this block
  was stopped" has to come from the `jobs` dump (see 4), not the status.
- In dash a ctrl+c at the prompt throws away the rest of the line, so a
  mark typed after the command can be lost. Only marks from `PS1` or
  `precmd` are reliable; the next A mark alone still ends the command,
  with the status unknown.
- bash's bracketed paste puts a multi-line paste into one edit buffer
  and prints a C mark per line, with reverse-video echo. Typing a
  block's body at the prompt is not the way to run a block (see 3).
- `. file` from the prompt works with job control: ctrl+z stops the
  command, the shell goes on.

### 3. Running a block: type one line, source the body

mdsh runs a block from the document by typing at the prompt, since an
interactive shell takes commands from nowhere else. It never types the
body. The body, with the same prelude it gets today (`$MDSH_BLOCK`,
the control-socket token, `MDSH_BG_*` for background blocks), is written
to `$MDSH_TMP/<id>.sh` as now, syntax-checked with `<shell> -n` as now,
and then one line is typed:

```
. "$MDSH_TMP/setup.sh"
```

One line means one A/B/C/D cycle, one echo to strip, no quoting of the
body, no bracketed paste, and no partial runs: an interactive shell
executes a pasted body line by line, so a syntax error on line 3 would
come after lines 1 and 2 had run. Sourcing keeps `return`, `cd`,
variables and functions in the shell as today. The pane shows the typed
line; a nicer spelling (a function `mdsh-run setup` defined by the
integration, which the pane could render as "▶ setup") is cosmetics for
later.

mdsh types only when the shell is at a prompt (the last mark seen is A
or B and nothing has been typed since). While a command is in the
foreground, Play says the shell is busy and names the command, as
Processes does, and offers to interrupt it; it does not queue. If the
user has started typing a line in the pane, Play asks them to finish or
clear it first; mdsh can't see the editor's buffer, but it knows which
keys it forwarded since the last B.

Run All types the blocks one at a time, waiting for each D, as `RunFrom`
does today. `confirm`, `depends=`, `run=`, `once`, `skip`, `on_error`,
`timeout=` and the audit log work before and after the typed line
exactly as now; only `runBlock`'s middle changes.

What mdsh learns from the marks:

- **Exit status** is the number in D. A command that ends without a D
  (a shell that died, a lost mark) gets status -1 with "status unknown".
- **Output** is the bytes between C and D, taken from the pty stream,
  not from the screen. The recorder (`termrec`) keeps the stream with
  timing as it does for `tty` blocks; the output fence gets the stream
  with escapes stripped (`termsafe`), so long outputs are not cut to the
  screen's rows. If the stream switched to the alternate screen, the
  block is kept as a recording, as `RunTyped` already decides.
- **The echo** between B and C is dropped from the output and kept as
  "what was typed" for History (see 5).

### 4. State without `Ask`: a dump at every prompt

Today mdsh asks the shell things through `Shell.Run`. In interactive
mode nothing can be run invisibly, so the shell tells mdsh instead. The
`precmd` hook (or the `PS1` expansion, for sh) writes a small dump to a
fifo mdsh keeps open at `$MDSH_TMP/state`, after D and before A:

- `$?`, `$PWD`, `$OLDPWD`, `$$`;
- `jobs -l` (job number, pgid, state, command line), so mdsh knows what
  is stopped and what runs in the background;
- `export -p` (or `env -0`), so a sub-env can be started with the
  parent's exported variables without asking;
- the variables mdsh asked for, named in `$MDSH_TMP/want`, which mdsh
  writes before typing a block: the names placeholders and conditions
  read (`s.wanted[b.ID]` today), exported or not;
- the command text, when the shell knows it (`preexec`'s argument,
  `fc -ln -1`), escaped as VS Code does so newlines and `;` survive.

A fifo rather than a file so the exported environment, which can hold
secrets, never sits on disk. mdsh holds the read end open, as it holds
the pty master, so the hook's write never blocks. The dump costs one
`jobs` and one `export -p` per prompt, a few milliseconds.

What goes away with `Ask`: `history.AskPrompt`, `promptScript` (the
shell draws its prompt), `Notebook.Complete` and the History pane's own
completion (the shell completes), `Shell.Exported` (the dump has it).
`Ask` stays for scripted shells.

### 5. The History pane is a terminal

The pane stops being an mdsh line editor in front of a shell and becomes
a terminal emulator showing the shell: xterm.js in mdsh-gui and `--serve`
(the card terminal from PR 308 is the same component), and the in-card
terminal view in `mdsh -i`. Keys go to the pty; zle or readline edits
the line, completion is the shell's, the prompt is the user's own, with
its theme, right prompt and all.

mdsh watches the marks:

- At **C**, the command has started. mdsh appends `#history-N` to the
  document with the command text (from the dump, else the screen text
  between B and C) and shows it running, as `RunTyped` does today.
- At **D**, the block gets its output and status. A `history=N` block
  rotates its runs; `HistoryReuse` finds the earlier block by its body
  as now.
- A line that is only a comment (`# why I did this`) becomes prose, as
  today. zsh needs `setopt interactive_comments` for that, which the
  integration sets; zsh and bash still add the line to history, so the
  dump carries it to mdsh. There is no C for it, so mdsh reads it at
  the next A.
- The record button still turns recording off (`RunUnrecorded` today):
  mdsh simply doesn't add the block.
- A fenced block pasted into the pane (`` ```sh {#id} `` … `` ``` ``) is
  still caught by mdsh before it reaches the pty, as the 1.7 feature
  does, and run as in 3.
- `exit` ends the shell, the pty hits EOF, and `Stack.Get` starts a new
  one as now; `env.end` is logged. zsh and bash refuse the first `exit`
  while jobs are stopped ("you have stopped jobs"), which the pane shows
  as any output.

Blocks Played from the document run in the same pane, and the user can
type into them while they run, which is what a terminal does. Every
block in an interactive session is in effect a `tty` block; the `tty`
attribute keeps its meaning for scripted runs (needs a terminal, else
fails), and the write-back still adds it to a History block that took
the screen, so the file replays the same way without a pane.

### 6. Stopped and background jobs in the document

The document records what happened, with no new syntax:

````markdown
```sh {#history-7}
nvim notes.md
```

<!-- mdsh:output for="history-7" exit="148" -->
![recording](notes.md.assets/history-7.cast)
<!-- /mdsh:output -->

```sh {#history-8}
make test
```

```output {for=history-8 exit=0}
ok  	./...	0.41s
```

```sh {#history-9}
fg
```

<!-- mdsh:output for="history-9" exit="0" -->
![recording](notes.md.assets/history-9.cast)
<!-- /mdsh:output -->
````

- A block whose command was stopped ends with `exit=148` (146 on macOS)
  and the output it had shown. Its card shows a **Stopped** badge, with
  the job number and command from the dump, as long as the job exists.
  A block that started a background job (`&`, or `bg` later) shows a
  **Running in background** badge the same way; the pane shows the
  job's later output where the shell puts it, between prompts, which
  mdsh attaches to no block (it is in the pane's scrollback and in the
  session's recording, not in the file). This is what a terminal does
  and what `background` blocks were made to avoid; a user who wants the
  output kept marks the block `background`.
- `fg`, `bg`, `jobs`, `kill %1`, `wait %1`, `disown` are commands like
  any other and become `#history-N` blocks. `fg`'s output is what the
  resumed program shows from then on, so a full-screen program's session
  is split across the block that started it and each block that resumed
  it. A follow-up could join them (the dump says which job `fg` resumed,
  so the resumed block could gain `resumes=history-7` and the player
  could play the pieces in a row); it is not in the first cut.
- Replaying such a file with `mdsh file.md` runs the blocks in order in
  a scripted shell: `nvim` needs `tty`, which write-back added; `fg`
  with no stopped job fails with the shell's own message and status 1,
  which is the truth. The docs should say that job-control blocks are a
  record of a session, not steps to repeat, and that `skip` is the way
  to keep them out of a run.
- Restart Shells, Kill Shell and closing the file end the shell; its
  pty closes, and the kernel sends `SIGHUP` (and `SIGCONT`) to its jobs.
  When the dump lists jobs, the confirmation names them: "2 stopped
  jobs (nvim, sleep) will be ended".
- **Stop** (the button, or `timeout=`) writes ctrl+c (`0x03`) to the
  pty, so the terminal's line discipline delivers `SIGINT` to the
  foreground group, exactly as a key would; after `interruptFor` mdsh
  kills the foreground process group, which the pty names
  (`TIOCGPGRP`), which is more exact than today's `killChildren`; after
  `KillAfter`, the shell. Stopped jobs are never signalled by a Stop.

### 7. Sub-envs

A sub-env (`env=build`) has no pane, so it stays a scripted shell, run
as today, with its exported variables taken from the parent's last dump
instead of from `Exported`. A later step can let the pane switch to a
sub-env, which would then be started interactively on its own pty; the
`Stack` can hold both kinds.

PR 308's background terminal starts a second shell from a state dump
made through `Run`; with an interactive parent, the dump comes from the
fifo of 4 instead, and nothing else there changes.

### 8. stdout and stderr

On a terminal there is one stream. Today a History command's output is
already the terminal text (`RunTyped` sets `r.Stdout` from the
recording), and a `tty` block's is a cast. The interactive mode makes
that true for every block the session runs: `output` gets the merged
stream, `show-stderr` and `stream=stderr` have nothing to show, and
`stderr` in a condition is empty. For a notebook that is a terminal
this is right: what you saw is what is saved, in the order you saw it.

Keeping them apart is possible and worth a later step: the typed line
becomes `. "$MDSH_TMP/setup.sh" 2>/dev/pts/M`, where `M` is a **second
pty** mdsh opens for the shell, never its controlling terminal. mdsh
reads both masters and shows both in the pane in arrival order, and
records them apart. `isatty(2)` stays true, so progress bars and colour
on stderr don't change their behaviour as they would on a pipe or a
file, and `TIOCGWINSZ` on fd 2 answers. The costs: interleaving across
two ptys is only as exact as scheduling makes it (as in any terminal
multiplexer), a command typed by the user in the pane gets no
redirection, so only Played blocks are split, and the `2>` on the typed
line shows in the pane. The default should stay merged; `stderr: split`
in the front matter, or a setting, turns the second pty on. Scripted
runs keep the exact separation they have.

### 9. mdsh-gui, `--serve` and `mdsh -i`

All three share `notebook`, so most of this lands once:

- `notebook` stops owning a prompt, a line editor and completion for the
  pane and owns a terminal (`run.Options.Terminal`, already there)
  whose output it fans out to the UI and to the marks reader. `Enter`,
  `EnterUnrecorded`, `Prompt`, `Complete` go; `Type(keys)` and
  `SetHistory` stay; new events say a command started (with its text)
  and ended, and carry the job table for badges.
- **mdsh-gui / serve:** the pane is an xterm, as the PR 308 card is.
  The settings lose "History rc file" and gain "Login shell". The pane's
  separator shows `$PWD` from the dump, as now.
- **mdsh -i:** the pane draws the terminal with the in-card view from
  PR 308 and takes keys while focused; `ctrl+g` gives them back to the
  file. The pane's own `histKey` editor, `complete`, `columns`, prompt
  drawing (`tui/history.go`) go. `a` to run a block types the line of 3.
- **Processes** lists jobs from the dump beside the shell's children.

Screenshots are part of each PR that changes a UI, per the project
rules; the design has none.

### 10. Work, in order

1. **`env`: interactive mode.** `Config.Interactive`, start with `-i`
   on the pty, the integration files per shell under a new
   `env/integration` (or next to `shellrc`), the OSC 133 reader in
   place of `demux` for the pty, the state fifo, `RunLine` (type a line,
   wait for D, return status and the C..D stream) and `Jobs()`. Tests
   on Linux and macOS for sh, bash (3.2 and 5) and zsh: status, output,
   ctrl+z, `fg`, `bg`, `exit`, a lost mark, a slow rc file, a user rc
   file that emits OSC 133 itself. `jobsOn()`/`jobControl()` and the
   `TSTP` ignores go once nothing scripted needs them; PR 309's
   `SIGCONT` watcher stays, since a scripted shell can still be stopped
   by its command.
2. **`run`: use it.** A `Session` with a `Terminal` starts the top-level
   env interactively; `RunBlock`, `RunTyped` and `RunUnrecorded` all go
   through `RunLine`; results come from the marks and the dump;
   `Exported` reads the dump. `Stop` sends ctrl+c and kills the
   foreground group. The audit log gains `job.stopped`,
   `job.backgrounded` and `job.resumed` events from the dump diff.
3. **`notebook`, mdsh-gui, `--serve`, `mdsh -i`:** the pane as a
   terminal, History blocks from marks, notes, record toggle, badges,
   busy and half-typed-line refusals, Restart Shells naming jobs. The
   settings change. Screenshots in the PRs and the changelog.
4. **Docs:** spec (`How a block runs in a shell`, `Terminal blocks`,
   History), cheat sheet, site pages, changelog, and a note that
   job-control blocks are a record.
5. **Later:** `stderr: split` with a second pty (8); `resumes=` joining
   a job's recordings (6); the pane switching to a sub-env (7); a
   friendlier spelling of the typed line (3).

Ship step 1 and 2 behind a setting, "Interactive shell (experimental)",
off by default, for one minor release, then make it the default and keep
the setting as a way back for a release more.

## Risks

- **rc files.** The shell now runs the user's whole `.zshrc` or
  `.bashrc`, with oh-my-zsh, nvm, p10k and the rest: startup can take a
  second or two (the pane should show the shell starting), rc files that
  print, prompt themes that redraw (transient prompts, p10k's instant
  prompt writes before the hooks load). The ZDOTDIR and `--rcfile`
  shims are what VS Code ships and are well trodden, but each theme that
  misbehaves is a bug report. Mitigation: the marks reader is tolerant
  (first mark of a kind wins, A alone ends a command), and a Settings
  switch runs the shell with `-f`/`--norc` for a bare shell.
- **Shell versions.** macOS ships bash 3.2 (no `PS0`, `DEBUG` trap
  instead) and zsh 5.9; Linux has dash as `sh`, with `$?` in `PS1` only
  since dash 0.5.11; busybox ash is untested. The test matrix has to
  cover them, in CI on both OSes.
- **Signal numbers.** `SIGTSTP` is 20 on Linux and 18 on macOS, so a
  stopped command's status is 148 or 146. The badge comes from the job
  table, not the number.
- **Half-typed lines.** mdsh can't see zle's buffer, so Play's "finish
  the line first" check is a heuristic on forwarded keys; a user who
  moved the cursor with the mouse or a widget could fool it, and the
  typed line would be appended to theirs. The `-n` check won't catch
  that. Low harm (the line runs or errors in the pane, visibly), but it
  should be in the tests.
- **Output capture.** Taking C..D from the stream rather than the screen
  is a change for History commands, whose text today comes from the
  screen (`shown.Text()`); escape-stripped stream text differs from
  screen text for programs that redraw lines (progress bars) and for
  zsh's `%` partial-line mark (`PROMPT_EOL_MARK`, which the integration
  should set empty). Keep the screen text as a fallback for the
  comparison in tests.
- **Secrets.** `export -p` at every prompt passes the environment over
  the fifo, never to disk; the pane's recording holds what was on
  screen, as today. The `want` file names variables only.
- **Lost stdout/stderr separation** in interactive sessions (8), and
  `show-stderr`/`stream=stderr` doing nothing there until the second
  pty lands. The spec has to say so.
- **A busy shell.** Play while a command runs in the pane is refused;
  users who run a server in the pane and then Play a block will hit
  this at once. The message should point at `&`, ctrl+z then `bg`, or
  a `background` block.
- **Scripted mode drift.** Two modes in `env` means two code paths to
  test. Keeping every sub-env scripted, and every batch run, keeps the
  old path exercised.

## Decisions (2026-11-19)

- Option A: interactive shell on its pty with shell integration marks,
  not a second attempt at `set -m` in a non-interactive shell.
- `shell:` stays a path; mdsh adds `-i` (and `-l` by setting). `shell:
  zsh -i` is not supported and should error clearly.
- A block is run by typing `. "$MDSH_TMP/<id>.sh"`, never by typing its
  body.
- Interactive sessions record one stream per block; separate stderr via
  a second pty is a follow-up behind a setting.
- Sub-envs and batch runs stay scripted.
- No new document syntax for jobs in the first cut.
