# Program status in the terminal (OSC 7501)

Design. Step 1 (mdsh and `mdsh -i` reporting their own state) is built;
steps 2 to 5 are not yet.

## The ask

Mitchell Hashimoto's [Program Status Protocol][post] (OSC 7501, spec
revision 0.4, 2026-10-10) lets a program tell its terminal what it is
doing: idle, working (with a percentage), done, blocked on the user (for
permission, a question or a credential) or failed, each with a one-line
message. The terminal shows that where it likes: a tab, a notification,
an inbox of running programs. Can mdsh use it in the History pane and in
tty shells?

[post]: https://mitchellh.com/writing/program-status-osc7501

## The protocol, in short

A report is `ESC ] 7501 ; key=value:key=value ESC \`. The terminal keeps
one *record* per `id`, a `/`-separated path; no id is the root, the
program itself. Each report replaces its record whole.

| key | values |
| --- | --- |
| `state` (required) | `idle`, `working`, `done`, `blocked`, `error`, `clear` |
| `kind` (with blocked) | `permission`, `question`, `auth` |
| `progress` (working, blocked) | 0 to 100; absent is indeterminate |
| `app` | a stable name, 32 bytes of `[A-Za-z0-9_.+-]`; children inherit it |
| `title`, `msg` | base64 UTF-8, one line, no control characters |

`working` and `blocked` are dropped by the terminal when the process
exits or the shell prints its next prompt (OSC 133 A); `done` and `error`
stay until replaced or cleared, so a program that finishes should report
one of them just before it exits. Feature detection is `OSC 7501 ; ?`,
answered by a terminal that speaks it. A whole sequence is at most 4096
bytes, a message 2048 decoded, a title 192; a terminal keeps at least 64
records. Everything in a report is untrusted input: a terminal refuses a
report whose decoded text has a control character and never reads
meaning into `msg`.

## Where mdsh sits

mdsh is on both sides of a pseudo-terminal:

- As a **program**, `mdsh doc.md` and `mdsh -i` run inside the user's
  terminal and can report their own state.
- As a **terminal**, the History pane (the interactive shell's pty, whose
  stream `env/marks.go` already scans for OSC 133), foreground `tty`
  blocks and background `tty` blocks (`env/tty_unix.go` forwards their
  pty to an `env.Terminal` and a recorder) can take reports from the
  programs blocks run. The GUI draws these with xterm.js and the TUI with
  vt10x; both ignore an OSC they don't know, so a report is lost today
  rather than garbled.
- Scripted blocks run on pipes, where a well-behaved program emits no
  OSC. `mdsh ctl status` is how such a block reports.

## Step 1: mdsh reports its own state (built)

Package `termstatus` builds the sequences (`Report.Sequence`) and keeps a
program's root record (`Tracker`), writing a report only when it changes.
A nil tracker does nothing, so callers with no terminal keep the same
calls.

| mdsh | report |
| --- | --- |
| a block starts | `working`, `progress` = blocks done over blocks that would run (none with `--only` or in `mdsh -i`), `msg` the block's label |
| `mdsh ctl status` | `working`, the block's own `--step/--of` as progress when given, `msg` "label: text" |
| a `confirm=` block, step mode, a `depends=` question, env.source approval | `blocked:kind=permission`, "Run deploy?" |
| `mdsh ctl ask`, `mdsh ctl confirm` | `blocked:kind=question`, or `auth` when the answer is secret; `msg` the question |
| the question answered | the last `working` report again (or `idle` before any) |
| run ended, nothing failed | `done`, "3 blocks ran" |
| a block failed, or the run stopped with an error | `error`, "deploy failed, exit 7" |
| interrupted (ctrl+c) | `idle`: the user is here |
| `mdsh -i` gets a key after done or error | `idle`: the result was seen |
| `mdsh -i` quits | `clear` |

The CLI writes to `/dev/tty`, as its questions do, so a run whose stdout
is piped or written to the file (`-w`) still reports; with no terminal
nothing is written. `mdsh -i` writes to its stdout, under bubbletea, as
the clipboard's OSC 52 already does; one tracker is shared by every tab.
`mdsh --serve` and mdsh-gui are not terminal programs, so they don't
report; the window title and dock will show the same states once step 4
gives them the records.

Code: `termstatus/`, `cmd/mdsh/status.go` (`trackStatus` wraps the
`run.Options` hooks), `tui/status.go`.

## Step 2: a scanner and record store in `env`

Widen `markScanner` (OSC 133 only today) into an OSC scanner that also
takes 7501: parse per the spec (grammar, limits, base64, control
character refusal, id hierarchy), keep records per pty with the lifetime
rules (drop working/blocked at a block's end and at the History shell's
next mdsh-tagged OSC 133 A), answer the `?` query on the pty master
before vt10x or xterm.js see it, and hand `env` an event for each change.
Every pty mdsh owns goes through one place in `env`, so History,
foreground tty and background tty blocks get it together, in the GUI,
the TUI and `--serve`. Records are scoped to the pty they arrived on, so
one block can't set another's state. Scripted blocks: strip a stray 7501
from output, where `termsafe.Styled` would show it as visible text.

## Step 3: `mdsh -i` shows block status and forwards it

Card header: the running badge gains the record's state, "working 42%",
"waiting: permission", with `msg`. The History bar's jobs badge and the
Processes panel (`P`) get a state column. `mdsh -i` re-emits each
block's records upward as children of its own, `id=<block-id>` (made a
valid segment with `termstatus.Name`), keeping the block's `app`, so the
outer terminal reads "mdsh › terraform: blocked, permission"; a block's
end clears its children. The spec doesn't cover nested consumers; its
hierarchical ids fit, and a terminal that shows only root records still
sees mdsh's own state.

## Step 4: mdsh-gui shows block status

A status chip on the card head: spinner with percentage for working,
amber "Waiting: permission" with the message for blocked, red for error,
a check for done. The same chip on background tty cards, where a blocked
card is the one that needs you. The History bar's jobs badge, a Processes
column, and a window title prefix the way `•` marks a dirty file, so the
dock and tab bar say a block needs you. Light and dark screenshots in the
PR and the changelog, per the project rules.

## Step 5: `mdsh ctl status` for scripted blocks

`mdsh ctl status` exists (`--step N --of M TEXT`). Add `--state`
(`working`, `blocked`, `done`, `error`) and `--kind`, so a plain `sh`
block on a pipe gets the same chip as a tty block's program, and step 3
forwards it upward as that block's child record.

## Not now

- Markers for blocked moments in the cast player: casts keep the raw
  bytes, so the reports are there. Skip until asked.
- OSC 9;4 (ConEmu progress): the spec allows mapping it to the root
  record. Easy to add to the step 2 scanner later.
- OS notifications on blocked, as a setting, after the chips exist.

## Caveats

- Terminal support is Ghostty (via libghostty, PR #14560) and the
  author's Rex. Nothing from iTerm2, kitty, WezTerm, Terminal.app or
  Windows Terminal yet. A terminal that doesn't know the sequence ignores
  it, so reporting costs nothing elsewhere.
- Program support is proof-of-concept forks of Terraform, Claude Code,
  Codex and Homebrew, so steps 2 to 4 show little until programs in
  blocks emit. That is why step 1 and `ctl status` matter first.
- The spec is the author's revision 0.4 and still moving (0.4 removed
  terminfo). Parse tolerantly, as it says: unknown keys ignored, malformed
  pairs skipped.
- tmux drops an OSC it doesn't know. mdsh under tmux reaches the outer
  terminal only with `allow-passthrough on` and DCS wrapping, which mdsh
  doesn't do (nor for OSC 52). A program inside tmux inside a tty block
  has the same problem. Unsupported, and said so.
