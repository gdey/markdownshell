# mdsh interface guidelines

Status: draft, Nov 2, 2026. These are the human interface guidelines for
everything a person uses in mdsh: the command line (`mdsh`), the terminal
notebook (`mdsh -i`, "the TUI") and the desktop app (`mdsh-gui`, "the GUI").
They describe what the three do today, set the rules a change should follow,
and list where the three disagree so the gaps can be closed one at a time.

The rules are for people writing PRs. Where a rule and the code differ, the
code is the bug, unless the rule is changed here first. The last section is
the follow-up list; a PR that fixes an item there deletes it.

Companions: [spec.md](spec.md) is the file format, [gui/design.md](gui/design.md)
is the GUI's design, and the accessibility audit lives in the project's
`gui-a11y/accessibility-list.md`.

## Principles

1. **One file, three doors.** A runbook behaves the same whichever way it is
   opened. The CLI, TUI and GUI share `doc`, `run`, `render` and `report`,
   so logic lives there and the three only draw. A feature added to one
   surface is noted for the other two (see Parity).
2. **Nothing runs without being shown.** Code is visible before it runs,
   including code the printed document hides. A question about running
   shows the code it is about.
3. **Prefer undo to a question.** Ask before an action only when it cannot
   be undone after the person sees the result. Everything else happens at
   once and offers Undo.
4. **Say what happened, where it happened.** A result sits by the block it
   belongs to. Feedback for an action sits where the person is looking.
   Problems the person did not cause go to the Console, with a count, and
   never interrupt.
5. **Keys first, then the mouse.** Every action has a key or a menu item,
   every state has a text equivalent, and the help lists both.
6. **Terminal words, plain words.** Use the vocabulary in Terminology below,
   in the same spelling and case in all three surfaces, the docs and the
   site.

## What exists today

| | CLI `mdsh` | TUI `mdsh -i` | GUI `mdsh-gui` |
| --- | --- | --- | --- |
| Run one, from here, all, section | flags, `--only` | `enter`, `a`, `s`, run all is `a` on the first block | card buttons, Run menu, `enter`, `a`, `s`, cmd/ctrl+R |
| Stop | ctrl+c | ctrl+c; ctrl+] for a tty block | Stop button, cmd/ctrl+. |
| Confirm, step | prompt on the terminal, `--step`, `--yes` | prompt in the block's card: r/y run, k skip, o only unsaved, s/n/enter/esc stop | question in the card: Run, Skip, Only unsaved, Stop |
| Block result | printed document, stderr line per failure | `✓`, `✗ exit N`, `✗ stopped`, `running…`, `skipped` in the header | `exit N` badge and a red edge, outline marks ✓ ✗ ? ▶ ◐ |
| Action feedback | stderr | one-line status under the document | status text in the toolbar, polite live region |
| App problems, warnings | `mdsh: file: warning: …` on stderr | status line | Console pane (View › Console, cmd/ctrl+shift+Y) with a badge counting unseen warnings and errors |
| Blocking error | exit code and stderr | status line | native error dialog titled "mdsh" |
| Save | `-w`, `-o` | `w`, ctrl+s; `--autosave` saves after each block | cmd/ctrl+S, Save As…, autosave is a setting, off by default; dirty dot |
| Quit with unsaved output | n/a | press q (or ctrl+w) twice; status says "w saves" | dialog: Save / Don't Save / Cancel, Save is the default |
| File changed on disk | n/a | `R` reloads, `M` keeps yours; `R` twice when unsaved | banner: Reload / Keep mine |
| Edit | `$EDITOR` outside mdsh | `e` block, `E` file in `$EDITOR` | source view beside the rendered view, or Neovim |
| Move a block | no | `J`/`K`, alt+↓/↑; `u` undoes | alt+↓/↑, drag; cmd/ctrl+Z undoes, banner with Undo |
| Delete a block | no | no | trash on the card, delete/backspace; asks, then banner with Undo |
| Clear all outputs | no | no | cmd/ctrl+alt+K; asks |
| Report | `--pdf`, `--html`, `-o` | `p` form | File › Create Report… dialog |
| History shell | no | `h` pane | cmd/ctrl+shift+H pane |
| Outline | no | `o` | cmd/ctrl+shift+O, sidebar icon |
| Help | `-h`, `mdsh ctl help` | `?` panel, grouped, greyed with a reason when a key does nothing | menus with accelerators; tooltips name the key |
| Themes | terminal colors | light and dark adaptive colors | System / Light / Dark, Looks: Plain, Blueprint, Teletype; text size, fonts, accent |

## Errors, warnings and status

There are four places a message can go. Pick by who caused it and whether the
person must act.

1. **The block.** A block's own outcome: exit status, stderr, "stopped",
   "skipped", a confirm question, a warning from its attributes. It stays
   with the block, in its header or under its code, and is part of the
   outline's state marks. It is never only in a status line or the Console.
2. **The status line** (TUI: the line under the document; GUI: the toolbar's
   status text). The result of the action the person just took: "saved",
   "moved #build; u undoes", "a block is running; ctrl+c stops it",
   "report saved to ~/x.pdf", "edit: …". One line, lower case unless it
   starts with a name, no trailing period. It is replaced by the next
   message and never needs a click to dismiss. It is a polite live region in
   the GUI.
3. **The Console** (GUI) and the stderr stream (CLI). Problems the person did
   not ask about now and can read later: a `depends=` that names an unknown
   block, a settings file that failed to save, a parse warning, a refused
   run when the file is read-only. A Console entry carries a level, a time,
   and the file and line it is about when there is one, so a click goes
   there. While the pane is closed the toolbar badge counts unseen warnings
   and errors, and the button's label says so for screen readers. The TUI
   has no Console; its status line carries these today (see Follow-ups).
4. **A dialog.** Only for an error that stops what the person asked for and
   has no place on the screen to go, such as a file that cannot be opened
   from the start screen. Title "mdsh", the error text as the message, one
   OK button. A dialog is never used for a block's exit status, a warning,
   or anything the person can keep working through.

Rules:

- A message says what did not happen and why, in that order, in the words
  of the action: `not run: notes.md is read-only`, `edit: $EDITOR is not
  set`. It does not start with "Error:" and does not echo a Go error wrapped
  in another.
- One problem goes to one place, plus the Console when it is worth keeping.
  A run that is refused shows in the status line *and* the Console; it does
  not also open a dialog.
- Errors stay visible. A status line message that reports a failure is kept
  until the next action, not timed out. A Console entry is kept until
  cleared.
- Warnings are not errors. A warning lets the run go on; an error stops the
  block it is about and no more than `on_error=` or `--stop-on-error` says.
  Use the levels the same way everywhere: `warn` in the Console, `warning:`
  on stderr, a yellow mark in a card.
- The CLI's exit codes are part of the interface: 0 ran, 1 a block failed,
  2 mdsh could not do what was asked (bad flag, missing file). Keep them.

## Confirmations and destructive actions

Classify every action before deciding how to guard it.

| Kind | Examples | Guard |
| --- | --- | --- |
| Undoable at once | move a block, edit the source, change a setting, fold | None. Do it, show the result, offer Undo (cmd/ctrl+Z, `u`, a banner with Undo) |
| Undoable with effort | delete a block, clear all outputs, reload a changed file | One question with the thing named: "Delete #build?" The safe button is the default |
| Runs code | run a `confirm` block, run all, run from here | The block's own question, in its card, with its code visible. The block's own Run button is the answer, so a single confirm block runs at once |
| Loses work | close with unsaved output, overwrite a changed file, quit | Ask with Save as the default and Cancel available. Never Don't Save as the default |
| Not undoable outside mdsh | the commands a block runs | mdsh does not guess; `confirm` on the block is the author's tool, `--step` the reader's |

Rules:

- A question names the thing: the block id, the file name. "Delete #build?"
  not "Are you sure?".
- Escape and ctrl+c cancel; they never run anything. Enter picks the
  default, and the default is the choice that loses nothing: Cancel, Save,
  Stop. In a confirm question Enter means Stop, as on the command line.
- The GUI asks with a sheet or a native dialog and the TUI with a line, but
  both offer the same choices in the same order and the same words.
- A press-twice guard in the TUI (q twice, R twice) is a question too: the
  status line must say what the second press does and what the alternative
  is ("output is not saved; q again quits anyway, w saves").
- An action that asked once does not ask again for the same thing in the
  same session: answering a confirm for a block answers it for that run.
- After an undoable action the banner or status line names the undo key.
- A guarded action is disabled, with the reason in its tooltip or the help
  panel, while a block is running, rather than failing after a click.

## Keyboard and focus

Single keys mean the same thing in the TUI and the GUI, and the GUI adds the
platform's modifier shortcuts for what a menu owns.

| Key | Meaning | Where |
| --- | --- | --- |
| j / k, ↓ / ↑ | next and previous block | both |
| enter | run the selected block (or section heading) | both |
| a | run from the selected block to the end | both |
| s | run the selected block's section | both |
| e | edit the selected block | both |
| ? | help | TUI; GUI has menus (see Follow-ups) |
| esc | cancel, close the panel in front | both |
| ctrl+c | stop the run; in the TUI also quit when idle | both |
| J / K, alt+↓ / ↑ | move the block down or up | both |
| u / cmd+Z | undo | TUI / GUI |
| d | depends on… | GUI |
| delete, backspace | delete the block (asks) | GUI |

Rules:

- A single-letter key acts only when no text field, editor, terminal or
  dialog has the focus. In the GUI that check is one list; add new editors
  to it.
- Modifier shortcuts follow the platform and the menu, and never shadow an
  OS or text-editing shortcut (cmd+C, cmd+V, cmd+Q, cmd+W keep their
  meaning). A shortcut that conflicts on Linux gets a different key, not a
  different meaning.
- A new key is added to the TUI help panel, the GUI menu or tooltip, the
  site's keys page and the table above in the same PR.
- The TUI help panel is the model: keys are grouped (Move, Run, Edit, File,
  View, Tabs), a key that does nothing now is greyed with the reason, and
  the what-text matches the status line it produces.
- Focus follows the action. Running puts the selection on the running
  block; a confirm question takes the focus to its Run button and shows a
  visible waiting edge; a closed dialog returns the focus to the control
  that opened it; a new tab focuses its content.
- Selection and focus are the same thing in the rendered view: the selected
  card is focusable, has a focus ring, and the keys act on it.
- Every GUI control is reachable with tab, in reading order, and dialogs
  trap the focus. Panels (outline, History, Console) are toggles with
  `aria-expanded`, and their grips take arrow keys.

## Accessibility baseline

What every surface must keep (PRs #145, #146, #154), and what a new control
must have before it merges.

- **Text for every state.** A mark (✓ ✗ ? ▶ ◐ •) always has a text
  equivalent in the element's label or next to it: "ran", "failed, exit 1",
  "waiting for an answer", "running now", "output not saved". The TUI
  prints the words beside the mark. Color alone never carries meaning.
- **Contrast.** 4.5:1 for text and 3:1 for control edges and focus rings,
  in light and dark, in every Look. Measure a new Look or token before it
  ships; the harness has the check. Primary buttons pick black or white
  text by the accent's luminance (`--accent-fg`, `--bad-fg`).
- **Focus is visible** on every focusable thing (`:focus-visible` ring in
  the accent, dashed in Blueprint), including focus set by code.
- **Roles and names.** Icon buttons have `aria-label`s that name the action
  and the key, toggles have `aria-pressed` or `aria-expanded`, groups have
  names, the tab bar is a tablist, the outline entries say their level,
  the status line and banners are live regions (`status` for news, `alert`
  for a banner that needs an answer), the Console log is `role=log`.
- **Reduced motion** switches off pane slides, smooth scrolling, the
  dependency drag animation and terminal cursor blink.
- **Text size** goes to 200% without clipping; the toolbar wraps, names
  truncate with a tooltip.
- **Pointer targets** are at least 24px.
- **Keyboard path for every pointer gesture.** Drag to move has alt+↑/↓;
  drag to resize has arrow keys on the grip; dependency rings have the
  Depends on… dialog (`d`).
- **Screen readers.** The hidden live region announces a run's progress
  and confirm questions. Known gaps: the Neovim pane's canvas and TUI
  programs inside a tty block. A new pane that draws with a canvas needs a
  text mirror or is called out as a gap on the site.
- **TUI.** Everything is text, so it is readable by a terminal screen
  reader, provided states are words and the `?` panel reads in order.

## Terminology

Use these words, with this casing, in UI text, docs, the site, commit
messages and the changelog.

| Say | Not | Meaning |
| --- | --- | --- |
| file, document; runbook for the idea | notebook, script | the markdown file that is open; "runbook" is what the README, site and start screen call one |
| block | cell, step, snippet | a fenced code block; "runnable block" when it matters |
| run, ran, running | execute, evaluate | what happens to a block |
| Run all, Run from here, Run section, Run block | Execute all, Play | the four run actions, in this order and casing |
| Stop | Cancel, Kill, Abort | ending a run; Cancel is for dialogs |
| output | result, stdout (unless stdout is meant) | what a block printed |
| exit N | failed with code N, status N | a failed block's state |
| confirm block, confirm question | prompt, approval | a block marked `confirm` and what it asks |
| Skip, Only unsaved | Pass, No | the confirm answers |
| step mode | debug mode | asking before every block |
| section | chapter, group | a heading and what is under it |
| include block | import | a block whose lines come from a file |
| tty block, terminal | pty, console block | a block that takes the terminal |
| History (pane) | shell, REPL, console | the pane whose commands are added to the file |
| Console (pane) | log, debug output | the GUI pane that shows what the app logs |
| Outline | sidebar, TOC | the headings and blocks list |
| source view, rendered view | editor, preview | the two panes |
| save, Save As…, autosave, "output not saved" | write, dirty, modified | saving |
| report | export | a PDF, HTML or markdown copy of a run |
| session | daily note | a file named for the day in the sessions folder |
| Look | theme, skin | Plain, Blueprint, Teletype; "theme" is System, Light, Dark |
| depends on | requires, needs | the `depends=` attribute |
| collection | folder, workspace | a folder with an `.mdsh.yaml` |

Style: sentence case for buttons, menu items and titles ("Run from here",
"Clear all outputs", "Create report"); lower case status lines; an ellipsis
on an item that opens a dialog ("Save As…", "Report…"); keys written as the
TUI help does: `enter`, `esc`, `ctrl+c`, `cmd/ctrl+S`, `alt+↓`. Error text
names the thing and the reason, no exclamation marks, no "please".

## Theming and contrast tokens

The GUI's colors are semantic tokens on `:root` in `style.css`, overridden
per scheme (`data-scheme`), per Look (`data-look`), and for the History pane.
Code uses the token for the meaning, never a hex value.

| Token | Meaning | Also |
| --- | --- | --- |
| `--bg`, `--fg`, `--muted`, `--border`, `--hover`, `--code-bg` | surfaces and text | `--muted` must reach 4.5:1 on `--bg` |
| `--accent`, `--accent-fg`, `--link`, `--selection`, `--active-line` | the primary color, text on it, focus rings | the accent is a setting |
| `--ok`, `--bad`, `--bad-fg`, `--warn-bg`, `--warn-border` | ran, failed, warning | the only colors allowed to carry state |
| `--control` | mdsh's own marks: attributes, `mdsh:if`, placeholders | |
| `--dep-1…4`, `--dep-more` | dependency lines | |
| `--syn-*` | the editors' syntax colors | |
| `--ansi-0…15` | ANSI colors in output | |
| `--font-ui`, `--font-code`, `--text-scale` | type | Teletype starts at 115% |
| `--term-bg`, `--term-fg` | tty blocks and the History terminal | |

The TUI has the same roles as `lipgloss.AdaptiveColor` pairs: `accent`,
`runningCol`, `okCol`, `failCol`, `dimCol`, and styles built from them
(`promptStyle`, `stderrStyle`, `bannerStyle`). Running is yellow, ran is
green, failed is red, dim is for status and tags, in both.

Rules:

- A new color is a new token with a name for its meaning, set for light and
  dark and every Look, measured for contrast, before it is used.
- State colors (`--ok`, `--bad`, running) match between the GUI and the
  TUI in hue; the TUI's green and red already equal the GUI's.
- A Look changes texture, type and decoration; it does not change what a
  state color means or hide a state mark.
- System theme follows the OS, and the History pane's scheme is its own
  setting because terminals are usually dark.
- Diagrams and images that ship in light and dark variants switch with the
  scheme (`figure.diagram`).

## Security conventions in the UI

The v1.2.1 reviews in [security/](security/) set these; a change keeps them.

- **Hidden code is still drawn.** A block the printed document leaves out
  (`show=`, an off `mdsh:if` section, a hidden tape) is drawn in the GUI
  with a dashed edge and a "hidden when printed" badge whose tooltip says
  why, and a section holding one opens unfolded. The TUI shows a block's
  code while asking even with Code switched off, and the CLI's question
  shows the code the stream left out. Run all and Run from here run hidden
  blocks, so they must be visible.
- **Control characters are shown, not sent.** Code shown in a question or
  a header prints `\r` and escapes as `^M^[[2K`, with a warning, so code
  cannot hide itself from the person deciding to run it. Saving keeps the
  bytes as written.
- **Nothing loads from the network on open.** An image from another host
  (`https:`, `//host`, `file:`) is drawn as a note with its alt text and
  host instead of fetched, with no button to load it. Images beside the
  file and `data:` images show. Every page carries a Content-Security-Policy
  that loads only from the app, and files beside the document are served
  with one that loads and runs nothing. A new pane or feature that needs a
  remote load is a design decision, not a CSP exception.
- **Secrets are opt-in.** `{{$VAR}}` reads an environment variable only
  when it is exposed (`--expose`, `MDSH_EXPOSE`, front matter `env.share`),
  and saving never stores a caller's value. A placeholder that is not
  exposed prints as written; the UI does not hint at the value.
- **Links are intercepted.** A click on a link in the rendered view goes
  through the app (`OpenLink`), which opens only `http`, `https` and
  `mailto` targets in the browser, and the webview never navigates.

## GUI and TUI parity

Every feature of the CLI has a place in the TUI and the GUI. A feature may
land in one first, but the PR says where it goes in the others, and the
Features table in `gui/design.md` and the table at the top of this doc are
updated.

Expectations:

- Same keys for the same single-letter actions (table above).
- Same words in the same order for a question's answers.
- Same block states, marks and words.
- Same guard for the same action: if the GUI asks, the TUI asks; if the TUI
  offers undo, the GUI offers undo.
- Same defaults where a terminal allows them. Autosave is off in both and
  switched on by `--autosave` in the TUI and a setting in the GUI; the TUI
  flag and the GUI setting should keep the same name.
- The CLI is the reference for flags and exit codes; a GUI dialog that maps
  onto flags shows the matching command (the Report dialog does).

## Follow-ups: where the three disagree today

Not 1.3-gating. Each is a small PR; delete the line when it is done.

1. **Run errors in the GUI go only to the Console.** A refused run ("not
   run: notes.md is read-only", a parse error on run) should also land in
   the status line, by rule 2 above. Keep the Console entry and badge. (j,
   Nov 2: leave as is for now.)
2. **The TUI has no Console.** Warnings from `depends=` and parse warnings
   go to the status line and are overwritten by the next message. Either
   keep a short log the `?` panel can open, or let the status line stack
   unread warnings with a count, as the GUI badge does.
3. **The CLI prints warnings as `mdsh: file: warning: …` and errors as
   `mdsh: …`**; the GUI Console and the TUI say `edit: …`, `move: …`. Agree
   on the `action: reason` shape everywhere and drop the `mdsh:` prefix
   from messages shown inside mdsh.
4. **Accent colors differ**: the TUI's accent is purple (`#5A3FC0` /
   `#B4A2FF`), the GUI's is blue (`#0969da` / `#2f81f7`) and the GUI uses
   purple for `--control`. Pick one meaning per hue.
5. **Quit with unsaved output**: the TUI asks by pressing twice, the GUI
   with a Save / Don't Save / Cancel dialog. The TUI should offer the same
   three answers on one line ("output is not saved: w saves, q quits anyway,
   esc stays"), so the words match.
6. **Delete a block and Clear all outputs** exist only in the GUI. Add `x`
   (delete, with the status line's undo) and a clear-outputs key to the
   TUI, or decide they are GUI-only and say so in Parity.
7. **Help**: the TUI has `?`; the GUI has menus and tooltips but no one
   panel listing the single-letter keys. Add a Keyboard Shortcuts item
   (cmd/ctrl+/) that shows the TUI's table.
8. **Report** is `p` in the TUI and a menu item with no shortcut in the
   GUI. Give it cmd/ctrl+shift+E or make `p` work in the GUI.
9. **History and Outline** are `h` and `o` in the TUI and modifier
   shortcuts in the GUI; `h` and `o` are free in the GUI's key map and could
   mirror the TUI.
10. **Depends on…** (`d`) exists only in the GUI. The TUI has no way to
    edit `depends=` short of `e`.
11. **State marks**: the GUI outline uses ▶ and ◐ for running now and in
    the background and the TUI writes "running…" and "running in the
    background…". Add the marks to the TUI header beside the words.
12. **Dialog return focus**: some GUI dialogs return the focus to body.
    Listed in the accessibility audit; covered by the focus rule above.
13. **Contrast in the Looks**: Blueprint light muted text and failure red,
    Teletype borders, are under the baseline. Listed in the accessibility
    audit with the values to use.
14. **Terminology drift**: the README's first line says "notebook", the
    GUI start screen says "Open a runbook", and the GUI design doc says
    "notebook" for the app. Sweep UI text, docs and the site with the
    Terminology table.

## Checklist for a PR that touches the UI

- Which of the four places does each new message go to, and why?
- Does it draw code or load content a document controls? Check the
  security conventions.
- Is the action undoable? If not, what does the question name, and which
  button is the default?
- What key does it have in the TUI, in the GUI, and is it in the help, the
  menu and the site?
- Does every new state have words, not only a color or a mark?
- Are the new colors tokens, set for light, dark and each Look, and
  measured?
- Does the other surface get the feature, or a line in Follow-ups?
- Screenshots of the change are in the PR description (project rule).
