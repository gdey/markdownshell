# tmux in the History pane: a recording per pane, played with its splits

Design, Nov 16, 2026; decisions settled Nov 22, 2026. Not built.

## The ask

The History pane is a shell: what you type there becomes a `#history-N`
block at the end of the document, with its output. tmux already runs in
it (since 1.4), but what mdsh sees then is tmux's one composed screen,
borders and status line included. The ask is for mdsh to understand
tmux: record each session's panes on their own, add those recordings to
the history, and play them back the way tmux showed them, splits and all.

Two readings of "tmux integration" fit the ask, and the design covers
both with one mechanism:

1. **tmux inside the pane.** You type `tmux` in the History pane (or it
   starts that way), split and work in it, and the document gets the
   recording of each pane, laid out as it was.
2. **Your own tmux outside.** The document watches a tmux session you
   already have in Terminal or iTerm2 and records what happens there,
   so a notebook can be kept of work done in a plain terminal.

Reading 1 is recommended first; the recorder is the same for both, so 2
is a follow-up rather than a different design (see "Where tmux lives").

## Which "history" this is

"History" names two things in mdsh now:

- **The History pane** (mdsh-gui's pane at the bottom of the window, and
  the same pane in `mdsh -i`): a terminal where a command typed runs in
  the document's shell and is added as a `#history-N` block
  (`history/`, `cmd/mdsh-gui/notebook/history.go`, `tui/history.go`).
- **Output history** (`history=N` on a block, PRs #270, #276 and #279):
  a block's earlier runs kept under its current output, stamped `at=`.

This design is about the **History pane**. It touches output history
only where a recording is an asset that gets rotated with the other
earlier runs, and the UI words follow
[output-history/design.md](../output-history/design.md): "earlier runs"
for `history=N`, "History" for the pane.

## How it works today (what the design has to fit)

- **A History command runs on a terminal.** Turning the pane on gives
  each shell a pseudo-terminal (`run.Options.PTY`); a command typed is
  appended as a block and run with `RunTyped`. Output goes to the pane's
  xterm.js as `tty` events; keys go back through `Type`
  (`notebook/terminal.go`, `frontend/src/history.ts`).
- **A full-screen command becomes a `tty` block** whose output is an
  asciinema cast (`termrec`), saved at `<doc>.assets/<id>.cast` and
  linked between output markers as an image. mdsh-gui swaps the image
  for a player (`frontend/src/cast.ts`): play, pause, scrubber, frame
  stepping, text selection, GIF on the clipboard, `frame=N` written
  back. `termrec` draws the cast as a GIF or a still through vt10x for
  HTML, PDF and `mdsh -i`.
- **One terminal per document.** The pane and `tty` blocks share
  `notebook.terminal`: one size, one key stream, one output stream. The
  front matter's `terminal: cols/rows` fixes its size.
- **tmux runs there**, as any full-screen program does, since the shell's
  terminal is opened by its own name (`env/shell.go`). The recording is
  tmux's composed screen: one cast, borders drawn as text, the status
  line on the last row.
- **The pane reads an rc file** (`Settings › Shell`, `HistoryRC`) when
  it turns on, so prompts and functions look like the user's shell. That
  hook is where shell-side marks (below) can be added without asking the
  user to edit anything.
- **`mdsh -i`** has the same pane in text, with the same `#history-N`
  rule; its recordings preview as a still of the last frame.
- **Casts are capped**: 1000 columns, 500 rows, 100,000 writes, 16 MB,
  in `termrec.ReadCast` and the player, from the v1.5 security review.

## What tmux offers

Three ways to see inside a tmux session, from the outside:

| Way | What it gives | What it costs |
| --- | --- | --- |
| **Control mode** (`tmux -C attach`, since 1.8; iTerm2's integration is built on it) | A text protocol on one socket: `%output %paneid data` for every pane of the session as it is written, `%layout-change` whenever a window's layout changes, `%window-add`, `%window-close`, `%session-changed`, `%pane-mode-changed`, `%exit`; any tmux command can be sent and its reply read between `%begin` and `%end`. Since 3.2, `refresh-client -f pause-after` and `%extended-output` carry flow control and the age of each write. | Octal-escaped output to decode; timestamps are the receiver's, not tmux's (fine at the 100 ms frame rule); one more client on the session. |
| **`pipe-pane -o 'cmd'`** | Raw bytes of one pane to a command. | No timing, no layout, one hook per pane (`set-hook after-split-window`), and the user sees the hook. |
| **`capture-pane -p -e`** and **`list-panes -F`** | A pane's screen with its colors, and every pane's place (`pane_left`, `pane_top`, `pane_width`, `pane_height`, `pane_active`); `#{window_layout}` is the whole window's layout in one string, such as `bb62,159x48,0,0{79x48,0,0,1,79x48,80,0,2}`. | A snapshot, not a stream. |

Control mode gives per-pane output and layout changes in one stream and
needs nothing from the user's tmux configuration, so it is the recorder.
`capture-pane` fills in the screen a pane already shows when recording
starts mid-session, and `list-panes` or `#{window_layout}` give the
layout at that moment. `pipe-pane` is not needed.

tmux is not vendored and never will be: mdsh runs the tmux on the PATH
(`tmux -V`, 3.2 or newer for the flow-control flags; older still works
without them). Without tmux the pane is what it is today.

## Proposal

### 1. Where tmux lives: mdsh starts it, in the pane

When tmux is wanted (Settings › Shell › "History pane runs tmux", or
front matter `mdsh: history: tmux: true`; with the setting on, typing
`tmux` in the pane does the same, see Decisions), mdsh starts a tmux server of its own on a
socket named for the document, `tmux -L mdsh-<hash> new-session -d`, with
the document's shell as the pane's command, and then:

- the History pane's terminal attaches to it the ordinary way
  (`tmux -L … attach`), so the pane shows tmux as it does now, drawn by
  tmux, with the user's own `~/.tmux.conf`, keys and status line;
- mdsh attaches a **control-mode client** to the same session
  (`tmux -L … -C attach`) and records from it.

Nothing in the visible pane changes: it is still one xterm.js showing
what tmux draws. Splits are tmux's business live, and the GUI's business
only on playback (section 4). This keeps the live pane out of the design
and is what makes the first cut small.

A private server, rather than the user's default one, means the
document's session can't collide with their work, can be killed when the
document closes, and gets `remain-on-exit` and the hooks mdsh needs
without touching their configuration. The user's `~/.tmux.conf` is
still read (it is per server, from the same file).

Reading 2 of the ask, watching an outside tmux session, is the same
control-mode client pointed at the user's server (`tmux -C attach -t
work`), with no pane of mdsh's own. It is a follow-up: the recorder,
the files and the player are shared, and only the way in differs (a
"Record a tmux session…" command listing `tmux list-sessions`).

### 2. What the recorder keeps

The control client decodes the stream into one recording with several
tracks:

- **One pane track per pane**, each an asciicast v2 recording exactly as
  `termrec.Recording` is today: cols, rows, start time, writes with
  their offsets. A pane's first write is its `capture-pane -p -e`
  screen if it had one when recording started. A pane resize (from a
  split or a drag) is an asciicast `r` event, `"80x24"`, which the
  player and `termrec` learn to apply.
- **One layout track per window**: `[t, "layout", "<window_layout>",
  "<active pane>"]` on every `%layout-change`, and the first one at the
  start from `#{window_layout}`.
- **One session track**: which window is current (`[t, "window", "@1"]`
  on `%session-changed`, `%window-add` and the like), and when a pane or
  window closed.

The `%output` text is octal-escaped; decoding gives the bytes the pane's
program wrote, so each pane track plays in the existing player with no
change. Timestamps come from the control client's clock on receipt;
with 3.2's `%extended-output` the age is subtracted so a burst that
tmux held back lands where it happened.

Caps follow the cast caps, per pane and in total: a pane past 1000 by
500, or a recording past 100,000 writes or 16 MB across its panes, stops
recording and says so in the pane, as the player refuses an oversized
cast now.

### 3. What the document gets

Two kinds of block, so the document reads like a notebook and still
holds the whole session:

**The session block.** The command that started tmux is a `#history-N`
block as today, marked `tty` because it took over the screen, with the
recording as its output:

````markdown
```sh {#history-7 tty at=2026-11-16T22:10:53Z}
tmux
```
<!-- mdsh:output for="history-7" at="2026-11-16T22:10:53Z" -->
![](notes.assets/history-7.tmux/)
<!-- /mdsh:output -->
````

The output is a **folder**, `<doc>.assets/<id>.tmux/`, holding
`session.jsonl` (the layout and session tracks, and the list of panes
with their window and start times), and `%1.cast`, `%2.cast`, … one
asciicast v2 per pane. A folder, and not one file, so that:

- each pane's cast is a plain asciicast that `asciinema play`, the
  existing player, `termrec`'s GIF and the PDF still read unchanged;
- renaming the block renames one path (`<id>.tmux` → `<new>.tmux`), and
  output history rotates it as one asset, `<id>.<at>.tmux`, with the
  rules in [output-history/design.md](../output-history/design.md);
- `embed=inline` isn't offered for it (a session is too big to inline);
  the folder is the only form.

`out=recording:path` names the folder instead of a cast. A path ending
in `.gif` asks for one composite GIF (section 5) beside the folder, as
it does for a cast today.

**Command blocks.** Each command run in a pane is added as its own
`#history-N` block with its text output, in the order the commands
ended, across panes:

````markdown
```sh {#history-8 pane=%2 at=2026-11-16T22:11:04Z}
go test ./...
```
```output {for=history-8 exit=1 at=2026-11-16T22:11:04Z}
--- FAIL: TestFoo
```
````

mdsh learns where commands start and end from **shell marks** the rc
file adds: the pane's shell emits OSC 133 prompt marks (`A` prompt, `B`
command start, `C` output start, `D;<exit>` command end; the FinalTerm
convention that iTerm2, kitty, WezTerm, Ghostty and VS Code read) and,
on `B`, a private `OSC 7777;mdsh;cmd=<base64>` carrying the command
line, through zsh's `preexec`, bash's `DEBUG` trap and fish's
`fish_preexec`. mdsh strips both from the pane track so the recording is
clean, and cuts the pane's output between `C` and `D` into the block's
output, with the exit status from `D`. The marks are added by the same
rc hook the pane uses today, so the user edits nothing; the pane's
shell is set with `default-shell` and `default-command` on mdsh's
server, so new panes get the marks too. A pane whose shell has no marks
(a program that isn't a shell, or an outside session without the hook)
gets no command blocks, only its track in the session block.

A full-screen command in a pane (vim) is a command block marked `tty`,
whose output is a link into the session recording, `#pane=%2&from=…`,
rather than a cast of its own, so the file doesn't hold the bytes twice.

Two things change from today's History commands, and the doc should
say so where it describes the pane: a command in a tmux pane runs in
**that pane's shell**, not the document's shell, so a variable it sets
isn't seen by blocks run later from the cards; and re-running the
document re-runs command blocks in the document's shell as it does any
`#history-N` block, which is the same rule as today but matters more
when the commands were typed in several panes with their own
directories. `pane=` is kept on the block as a record, and is ignored by
the runner.

### 4. Playing it back: the split player

The cast player (`frontend/src/cast.ts`) grows a mode for a `.tmux/`
output:

- It reads `session.jsonl` and every pane cast, and keeps one xterm.js
  per pane, each driven by the pane player that exists today (frames by
  the 100 ms rule, writes that change nothing make no frame).
- One clock, one play button, one scrubber and one frame step drive all
  the panes; frames are the union of the panes' frames and the layout
  changes.
- The layout track places the panes. tmux's layout string is parsed
  (`WxH,x,y` cells, `{…}` side by side, `[…]` stacked, a pane id at the
  leaves); each pane's xterm is sized to its cell and drawn at its
  place, with a one-cell border between panes in the look's border
  color and the active pane's border highlighted, as tmux draws it. A
  layout change between frames re-places the panes; a resize event in
  a pane's cast resizes its xterm.
- Windows show as tabs above the player, the current one as the
  session track says at that frame; stepping the scrubber switches tabs
  when the recording did. The status line is not drawn: it was tmux's,
  not the panes'.
- Text in any pane can be selected and copied; the copy button copies a
  composite GIF; `frame=N` is written back as today, counting composite
  frames; the player opens on `#pane=%2&from=…` links at that pane and
  time.

The pane is the live tmux, so **the History pane itself needs no
splits**: the ask's "the history pane would have to be able to do
splits" is met by the player in the document, and by tmux drawing the
live pane. An iTerm2-style native split of the live pane (each tmux pane
in its own xterm.js, keys sent with `send-keys`) is possible on the same
control-mode client, and is a later step if wanted; it is listed under
Not in this design.

### 5. Stills and GIFs

`termrec` learns to draw a session: it plays each pane cast into its own
vt10x screen, lays the screens out by the layout string at that time,
draws borders, and emits a composite frame. The still (PDF, `mdsh -i`'s
preview, the last frame in `mdsh notes.md`'s HTML report) and the GIF
(`out=recording:….gif`, HTML reports) come from that with no new
drawing code beyond placement. `frame=N` counts composite frames, so a
PDF and the player agree.

### 6. `mdsh -i`

The TUI's History pane runs tmux now as the GUI's does; it gets the same
recorder (the control-mode client is in `history/`, not in the GUI) and
writes the same blocks. Playback in the TUI is the composite still, as
casts are today; the TUI gains no split player.

### 7. Settings, spec, security

- Settings › Shell gains "History pane runs tmux" (off by default) and
  shows the tmux found on the PATH with its version, or "tmux isn't
  installed".
- Front matter: `mdsh: history: tmux: true` asks for it per document.
- Spec: the `tty` section gets the `.tmux/` output, `pane=`, the shell
  marks, and the two changes under section 3.
- The control-mode socket is mdsh's own, mode 0700 in the user's tmux
  socket folder, named for the document, and killed with the shells on
  close or Restart Shells. A recording is data as a cast is: the
  player applies the cast caps per pane and in total, and the OSC
  marks are parsed with the same care `docs/term-escapes` gives the
  pane's escapes. The command line in the `cmd=` mark is base64 and
  bounded, and is written to the file as block text, which is already
  shown as code and never run on read.

## Decisions

Settled Nov 22, 2026, each as recommended:

1. **Where tmux lives first.** mdsh's own server in the pane (section
   1); watching an outside session is the follow-up (work order step 8).
   The other order would give the notebook-of-a-plain-terminal use
   first, but every pane's commands would then depend on the user adding
   the marks to their own shell.
2. **How tmux is turned on.** Both ways: the setting and the front-matter
   key turn it on explicitly, and with the setting on, typing `tmux` in
   the pane is noticed and run on mdsh's server rather than the user's
   default one. With the setting off, `tmux` runs as any command does
   today, as one composed cast.
3. **Command blocks and the session block.** Both, in the first cut: the
   marks reuse the rc hook, and the cut into blocks is a small parser. A
   shell without the marks still gives a complete session block.
4. **A folder, not one file.** Each pane stays a plain asciicast that
   the existing player, `termrec` and `asciinema` read; the folder is
   one asset to rename and rotate. A single `<id>.tmuxcast` with a pane
   id on each event was the alternative, and nothing else could read it.
5. **Every window.** Every window of the session is recorded, not only
   the one on screen; it costs nothing in control mode, and the player
   shows them as tabs.

## Not in this design

- Native splits in the live History pane (one xterm.js per tmux pane,
  keys through `send-keys`), iTerm2-style. Same client, later step.
- Replaying a recording into a live tmux (a "resume" of the session).
- A `tmux` block language, or running blocks inside tmux panes.
- screen, zellij and other multiplexers; zellij has no control mode.
- Recording mouse input or `%input`; the pane tracks are output only.

## Work order

1. `history/tmuxctl`: the control-mode client. Start a server, attach
   in control mode, decode `%output`, `%extended-output`,
   `%layout-change`, `%window-*`, `%session-changed`, `%exit`; send
   commands and read replies. Tests against a real tmux where one is on
   the PATH, skipped otherwise. The first thing the spike checks is the
   claim the design rests on: that a control client gets `%output` for
   every pane of the session, not only the window on screen, and that
   `%extended-output`'s age is usable for timing. If it only covers the
   current window, the recorder switches each pane's flag on with
   `refresh-client -A`.
2. `termrec`: a `Session` type (pane recordings, layout and session
   tracks), `WriteSession`/`ReadSession` to the `.tmux/` folder, the `r`
   event, and the caps.
3. `notebook`, `tui`: start tmux on the document's server when asked,
   attach the pane, record, write the session block on exit. The
   `History pane runs tmux` setting and the front-matter key.
4. The shell marks in the rc hook, the `cmd=` mark, and the cut into
   command blocks with `pane=` and `at=`.
5. `termrec`: composite stills and GIFs, used by the PDF, HTML reports
   and `mdsh -i`.
6. mdsh-gui: the split player, tabs for windows, `frame=N`, the copy
   button, links into the recording. Screenshots in the PR and the
   CHANGELOG.
7. Output history: rotation of the `.tmux` folder as an asset.
8. Follow-up: watch an outside tmux session.
