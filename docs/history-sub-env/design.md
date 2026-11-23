# History on a sub-env

Status: accepted (2026-11-22). Follows [job-control/design.md](../job-control/design.md) §7.

## The ask

With the interactive shell on, the History pane is a terminal on the
document's top-level shell only. A block with `env=build` runs in a
scripted shell of its own, where nothing can be typed, so ctrl+z, `fg`
and a live prompt don't reach it. The ask is to let the pane switch to
a sub-env, which then runs interactively on its own terminal, as the
top level does.

## Today

- `env.Stack.Get` starts the top level with `Config.Interactive` and
  every sub-env with it cleared: "Only the top-level env, the one a
  terminal shows, runs interactively."
- A sub-env is created from its parent's exported variables, filtered
  by its `share=`, and starts in the document's directory.
- `run` already runs a block in whatever shell its env has, by typing
  the source line when that shell is interactive (`runBlock`,
  `captureVars`). Nothing there assumes the top level.
- The notebook (`live`), `ui` and the TUI (`liveTerm`) attach to one
  shell, found with `Session.TopShell`. Its jobs, the half-typed line
  and "a command holds the shell" are all about that one shell.
- History blocks are added with no `env=`, so a whole-file run, or
  Play, runs them in the top level.

## Proposal

### 1. Which env the pane is on

The pane is on one env at a time: the top level by default. An env
picker on the pane's bar (mdsh-gui and `--serve`) lists the top level,
then each env the document names (`env=` on any block, with its parents,
`build` before `build/test`), each marked if its shell is running. In
`mdsh -i`, `ctrl+g` then `e` opens the same list over the pane. The
choice lasts until the pane closes or the file is reopened; it is not
saved in the file.

### 2. Starting a sub-env interactively

The Stack gains a set of envs that run interactively: the top level as
today, plus each env the pane has been switched to. `Get` starts an env
in that set with `Interactive` (and `Login` as the setting says), on its
own pty, with the same integration as the top level, and with its
variables from its parent as today (from the parent's state dump when
the parent is interactive).

Switching to an env whose shell hasn't started starts it interactively.
Switching to one already running scripted needs a new shell, since a
running shell can't become interactive: the pane asks first ("build's
shell has run 3 blocks; switch it to an interactive shell? What those
blocks set there is lost"), as Restart Shells does, and the new shell
starts from the parent again. Once interactive, an env stays so until
the shells restart; switching the pane away leaves it running, with its
jobs, and switching back shows it again.

The rc file runs in each interactive shell, so it can set what the
env's `share=` kept out (a `PATH`, a token). That is the cost of a real
shell and matches the top level; the picker's note says so.

### 3. Typed commands and History blocks

A command typed while the pane is on `build` becomes a History block
with `env=build`: ```` ```sh {#history-4 env=build skip links} ````, so
Play and a whole-file run send it to the same env. Notes stay plain
prose. The block-start, recording, `tty`, stopped and resumed handling
is the top level's, per shell.

### 4. Playing while a command holds a shell

What PR 321 does for the top level applies to each interactive env:
Play on an `env=build` block while vim holds `build`'s shell stops vim,
runs the block there and brings vim back. A command holding the top
level doesn't stop a block played in `build`, which is another shell.
The side shell (alt-click) starts from the state of the block's own
env.

### 5. One pane, several shells

- **Screen.** The pane shows one shell. Each interactive shell keeps
  its screen (the vt10x screen `mdsh -i` already keeps; in mdsh-gui a
  ring of its recent output, replayed into a cleared xterm on switch),
  so switching back shows what was there.
- **Size.** Only the shell on the pane gets the pane's size; the others
  keep theirs until shown.
- **Jobs.** The bar lists the jobs of the shell on the pane; the picker
  shows a count of stopped jobs beside each other env. Restart Shells
  names the jobs of every interactive shell.
- **Recording toggle** applies to the shell on the pane.

### 6. What doesn't change

Scripted runs (`mdsh doc.md`, `-w`, `-o`, reports) never start an
interactive shell. Envs the pane hasn't visited stay scripted. Without
the interactive shell setting, there is no picker.

## Work, in order

1. **env/run:** `Stack.SetInteractive(name)` and the restart path;
   `Session.Shell(name)` in place of `TopShell`; `RunSide` from the
   block's env. Tests: a sub-env started interactively gets its share
   and runs blocks by typed lines; switching a scripted one restarts it.
2. **notebook, ui, web:** `live` keyed by shell; `SetHistoryEnv`,
   `HistoryEnvs`; History blocks get `env=`; the picker, screens and
   jobs per shell. Screenshots.
3. **tui:** the same in `mdsh -i`, with ctrl+g e. Screenshot.
4. **Docs.**

## Decisions (2026-11-22)

- The pane changes env only from the picker; selecting a block never
  moves it. A "follow the selected block's env" setting can come later.
