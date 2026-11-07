# mdsh-gui

A desktop window for mdsh, built with [Wails v3](https://wails.io/).
It shows a markdown file, runs its `sh` and `zsh` blocks as you pick
them, streams their output under them and saves it back to the file. It
uses the same `run.Session` as `mdsh -i`.

See [the design](../../docs/gui/design.md) for how it works and what
comes next.

```sh
mdsh-gui [flags] [file.md [args...] | folder]
```

Flags come before the file, as with `mdsh`; `mdsh-gui --help` lists them.
`--source FILE` (repeatable) sources a file into every shell of every
file opened, before its first block. It's read once at startup, so
`mdsh-gui --source <(alias -L; typeset -f) notes.md` gives blocks the
calling shell's aliases and functions. `--only`, `--step` and
`--stop-on-error` set the Run Options of the file named on the command
line.

With no file, the start screen lists the markdown files under the current
folder, or under `folder` when one is given (or the start folder from Settings, when started from Finder or
the Dock) to pick from, with the description from each file's front
matter (`description`, else `summary`, else `abstract`) under its name.
A folder with a `.mdsh.yaml` shows as a collection, with its own title,
icon, color, background and tags (see [Collections](#collections)).
The pin at the end of a file's row pins it to a Pinned list at the top,
above Recent, until its pin is clicked again. Pins are kept in the
settings file with the recent files.
Each file opens in a tab of its own, with its own shells.

- **Tabs:** New Tab (cmd/ctrl+T, or the + in the tab bar) opens the
  start screen; Open…, Open Recent and dropped files open in a new tab.
  Drag a tab to reorder it, onto another window's tab bar or page to
  move it there, or onto its own page, or out of every window, for a
  window of its own. Dropped on another app that takes text, such as a
  terminal or an editor, it gives the file's full path (or, for a
  document built into the app, its URL) and stays where it is. A block
  running in it carries on. Close Tab
  (cmd/ctrl+W, the ×, or a middle click) closes it, and the window goes
  with its last tab. View › Show Next Tab and Show Previous Tab
  (cmd/ctrl+shift+] and [) move between them.
- **Window sizes:** a file that opens in a window of its own, or in a
  window showing only the start screen, gets the size its window had
  last, and on macOS and Windows its place too. Opening it in a tab
  beside others leaves the window as it is. These are kept in the
  settings file, not in the markdown.
- **Sessions:** New Session on the start screen opens the day's file,
  such as `2026-11-02.md`, in the sessions folder (`~/.mdsh/sessions`
  unless Settings › Files names another), making it the first time as
  a copy of the session template, or with the date as its heading
  and your login shell (`$SHELL`, if zsh or bash) as its `shell:`.
  Last Session opens the file in that folder changed last. Either one
  opens with the History pane open and the keyboard in it.
- **File:** New File… (cmd/ctrl+N, or New File… on the start screen)
  offers the templates in the templates folder from Settings, if one
  is set, then asks where to save the file and opens it. Without a
  template, a new file starts with a heading named for it. New Tab, New Window (cmd/ctrl+shift+N), Open…, Open Recent, Save, Save As…,
  Create Report…, Reload, Close Tab and Close Window (cmd/ctrl+shift+W). Create Report… writes a PDF, HTML
  or markdown copy, with or without code and output, from the tab's
  run, a run of every block or the saved file, and shows the `mdsh`
  command that makes the same one. Dropping a markdown file on a window opens it. A • marks
  output that isn't saved, and closing or quitting asks first.
- **Run:** Run Current Block (cmd/ctrl+enter: the block the source
  view's cursor is in, or the selected card without the source view),
  Run Section (cmd/ctrl+shift+enter: the section of the selected
  heading, or the one the source view's cursor or the selected card is
  in), Run All (cmd/ctrl+R, the toolbar button turns into Stop while
  blocks run), Stop (cmd/ctrl+.), Clear All Outputs… (cmd/ctrl+alt+K,
  or the toolbar's eraser button) and Run Options… for the file's
  arguments, `--only`, step mode and stop on error. Clear All Outputs
  is off while there is no output to clear. It asks first, then takes every block's output, output copy and stored
  block value out of the document and forgets what ran; the shells
  keep what blocks set in them. It isn't saved until you save; with the source view
  open, undo there brings the output back.
  **The app never ends a shell without asking.** Stop, and a block's
  `timeout=`, interrupt the block, as ctrl+c would, and its shell lives
  on with the variables, functions and aliases blocks set. A block
  still running three seconds after Stop, such as a loop that ignores
  ctrl+c, gets a question: Kill Shell ends it, and its shell. Restart
  Shells (the ↻ in the History bar) asks first too. New arguments in
  Run Options are set in the shells as they are. Clear All Outputs,
  Reload and Save As keep the shells. What needs new shells, a new
  `shell:` in the front matter or the first `tty` block, waits for
  Restart Shells, and the History pane, which needs shells on a
  terminal, asks before starting them again. A block that runs `exit`
  still ends its own shell. A block marked
  `tty` runs in a terminal in its card: type at it as you would in a
  terminal. It is as wide as the card and 24 lines high, unless
  `cols:` and `rows:` under `terminal:` in the front matter say
  otherwise. When it ends the card shows the recording, as `mdsh` saves
  it. A block marked `background` starts and lets other blocks run: its
  card has a dashed edge and says it is running in the background, the
  status line counts them, and its output shows when it ends. Closing
  the window kills those still running. A running block's card has a
  spinner and a timer that counts up by tenths of a second, so a long
  block plainly hasn't hung; a block with `timeout=` shows its time
  against the timeout, as `12.3s / 1m`, with a bar under its head that
  fills toward it, turning amber at three quarters and red at nine
  tenths. When it ends the card keeps how long it took, or says it
  timed out.
- **Console** (View › Console, cmd/ctrl+shift+Y, or the toolbar's
  console button): what the app logs, errors and notes, each with its
  time and level, such as a call that failed because the file no longer
  parses. A badge on the button counts warnings and errors that came
  while it was closed. Copy takes the selected lines, or all of them;
  Clear empties it in every window. Drag its top edge to resize it.
  A warning an edit fixed stays, struck through and marked "(fixed)".
  A warning with a plain fix has a Fix button, in a tab showing its
  file: an output copy whose `for=` names no block is deleted, and an
  id in a `depends=` that names no block that runs is taken out. Fix
  says what it will do and asks first; Edit › Undo takes it back.
  Code in the app reports to it by logging with `log/slog`.
  Its **Events** tab shows the tab's file's event log, the same events
  `mdsh --audit` writes, in words: the shells starting and ending, and
  why, blocks starting, ending and being skipped, the file changing on
  disk and being reloaded, and `mdsh ctl` calls. An event that cost the
  shells what blocks set in them (a block's `exit`, Kill Shell, a
  shell found dead, Restart Shells) is in bold, counts toward the badge and
  shows in the status line. Copy there copies the events as JSON
  lines. The log keeps the last 2,000 events, and lasts as long as the
  file is open in the tab, reloads included.
- **History** (View › History, cmd/ctrl+shift+H): a terminal along the
  bottom that works like a paper tape. Closed, it is a status bar with
  the shell's prompt, the status, and buttons to stop (ctrl+c for a
  command typed in the pane), start the shells again (it asks first),
  and open it, when
  the terminal slides up out of the bar. Type a command and press enter:
  it runs in the file's shell, sharing variables and the working
  directory with the blocks (while it runs the prompt dims and the
  input shows how long it has run), and is added to the end of the file as a
  block, `#history-1 skip links`, `#history-2 skip links`…, with its
  output and exit status, failures included. `skip` keeps Run all from
  running it again, though its play button still does. `links` makes the web addresses it
  prints (`http:`, `https:` and `mailto:` only) clickable in the
  block's output, and they are in the pane too; take it off the block
  to turn them off, or add it to any block. It runs on a real terminal, so you can type at a
  prompt it shows, and a full-screen program such as `vi` or `top` is
  kept as a `tty` block with its recording. A line starting with `#` is
  added as prose, without the `#`; notes typed one after another make
  one paragraph. Shift+enter starts another line, and so does enter on
  an unfinished command, such as one ending in `\` or inside quotes.
  Up and down recall what you typed, and the input takes a shell's
  emacs keys: ctrl+a/e to the line's start or end, ctrl+b/f and alt+b/f
  by character or word, ctrl+w, alt+backspace, alt+d, ctrl+u and ctrl+k
  to kill, ctrl+y to yank it back, ctrl+d to delete (or close the pane
  when nothing is typed), ctrl+p/n for earlier commands and ctrl+l to
  clear the transcript. Tab completes a command, a
  file or a `$` variable, as the shell sees them: in its directory,
  with what blocks set. The prompt is the shell's own `PS1`, expanded
  as the shell would (zsh's `%` escapes and `precmd` hooks, bash's `\`
  escapes), with `PS2` on continuation lines, or `$` if it has none.
  Drag over the transcript to select lines and press cmd+c (ctrl+c on
  Linux and Windows), or right-click for Copy and Select All: they are
  copied as text, without colors. With lines selected, ctrl+c copies
  them rather than reaching a command that runs in the pane.
  The shells aren't interactive, so they don't read your rc file
  themselves: name it, such as `~/.zshrc`, in Settings › File the
  History pane's shell reads, and the shells read it when History is
  turned on, for your aliases, functions and prompt. ctrl+Z stops a
  command and ends its block with the stop status; `jobs`, `fg` and `bg`
  then work as in a terminal, in shells that allow it, such as dash, the
  `sh` of Debian and Ubuntu (bash and zsh have no job control unless
  interactive, so there ctrl+Z is ignored). A command ending in a
  lone `&` is added as a `background` block instead, without the `&`,
  and its output goes to the block when it ends. Drag the pane's top edge to make it taller, or
  as short as one line. The blocks are added as `zsh` when
  the file runs in zsh, else `sh`. Turning History on starts the shells
  again, on a terminal, with `TERM=xterm-256color`, unless a `tty` block
  already did.
- **Front matter:** the Front matter tab at the top of the rendered
  view (or Edit › Front Matter, cmd/ctrl+shift+M) pulls the page down
  to show a form for the file's front matter: its title, description
  (or `summary` or `abstract`, whichever it has) and tags, the `mdsh:`
  options (shell, on error, placeholders, stderr, step, terminal size)
  and `env:`'s shared variables and sourced files. Keys the form has no
  field for are edited as YAML under Other keys. A field is written to
  the file as it changes, on leaving a text field or pressing enter, as
  one edit that Undo takes back; only the keys that changed are
  rewritten, so the rest keep their place and comments. Front matter
  mdsh refuses, or Other keys that aren't YAML, are said under the form
  and change nothing. Emptying every field takes the front matter out.
  The tab, Done or escape puts the page back. A file without front
  matter shows Add front matter instead.
- **Editing** happens in the source view. The pencil (or `e`) on a block
  opens it with the cursor on the block's code, and clicking prose opens
  it on that text. New blocks are typed there too. Inside
  `{…}` the editor completes attribute names and values (for `file=`,
  the files beside the document and, as `res://` paths, those in its
  assets folder; for `out=image:`, the assets folder's),
  explains an attribute on hover and underlines mistakes. Vim key bindings are a
  setting; with them, `:w` saves and `:RunBlock` runs the block at the
  cursor, as cmd/ctrl+enter does. With them on, an experimental setting
  makes the source pane a real Neovim (0.10 or newer, with your own
  config unless you turn it off). It works as the built-in editor does:
  `:w` saves, `:RunBlock` and `:RunSection` run what the cursor is in,
  as cmd/ctrl+enter and cmd/ctrl+shift+enter do, and the ▶ in the sign
  column runs a block or section. The pane attaches mdsh's language
  server (`mdsh lsp`, run as `mdsh-gui --lsp`): typing in a fence's
  `{…}` offers the same completions, there or through a completion
  plugin such as nvim-cmp, `K` explains the attribute under the cursor,
  and mistakes show as diagnostics. Spelling follows the setting, with
  Neovim's `spell`. Headings, fences and output fold (`zc`, `zo`), as
  the file's folds say when it opens; the cursor's cell is marked and
  the panes scroll together; and the outline and a card's edit button
  move the cursor. The Insert menu puts blocks in at the cursor; the
  markdown bar is left out, as Vim has its own keys for that. It takes
  the app's colors and follows the Theme; a setting keeps the
  colorscheme from your config instead.
- **Checkboxes, textboxes and choices:** task list items (`- [ ] label`)
  in the rendered view check and uncheck in the file when clicked, and
  so do table cells that start with `[ ]` or `- [ ]` (GitHub shows
  those as text). Checkboxes in a block's output, such as
  `out=markdown`'s, work the same way until the block runs again and
  rewrites them. A
  `textbox` block (```` ```textbox {#host lines=3 placeholder="Which host?"} ````)
  is a box to type in, at most `lines` lines high (`lines=fit` takes
  any number and grows and shrinks with the text), showing `placeholder`
  while empty; what is typed goes into the block's body. Drag the grip
  under a textbox, or focus it and press down and up, to make it taller
  until the window closes; a double click puts it back. A `choice`
  block lists its options as task list items and is drawn as a
  dropdown, radio buttons or checkboxes (`mode=dropdown`, `radio` or
  `multi`). With `var=name`, blocks and conditions read the text or
  picks as `$MDSH_VAR_name`; a later input reusing a name is marked, with
  `!` in the outline, and ignored. All are edits like typing in the source
  view: Save writes them.
- **Dragging blocks and sections:** a block card's grip (⋮⋮, at the left
  of its head, shown on hover) drags the block, with its output, to a
  spot between top-level cells; a line shows where it will go, the view
  scrolls near its edges and Esc cancels. A top-level fence that doesn't
  run, such as a Go example or a diagram, has a grip in the margin to its
  left and drags the same way. In the outline each block and
  heading has a grip too: a heading's drags its whole section (prose,
  blocks, output and subsections), which goes only before a heading of
  its level or higher, or to the end. A section that only partly holds
  an `mdsh:if` section doesn't move. Nor does a block, or a section, go
  below a block whose `depends=` names it, or above one its own
  `depends=` names: the line stops at the last spot short of that, turns
  gray, and the status line says why. The keys below keep to the same
  limit. Drags are edits like the keys below, with the same warnings
  and Undo.
- **Moving blocks:** alt+↑ and alt+↓ (Edit › Move Block Up and Move
  Block Down) move the selected block, or the textbox or choice being
  used, past the cell above or below it; with shift they move it past
  the block above or below, prose and all. Its output and its tape, when
  the tape comes right after it, move with it; prose never does. Blocks
  in a list or quote don't move. In the source view the keys move the
  block the cursor is in, and anywhere else are the editor's own (move
  or copy lines). A move keeps the block's output and status; if it
  ran, the status line says it ran in the old order. When a move
  changes something that depends on order, such as a condition that now
  reads a block below it, a `$?` that now reads another block, a block
  going in or out of an `mdsh:if` section, or which input sets a shared
  `var=`, a bar says so with an Undo button. Undo takes a move back like
  any other edit, in whichever view made it. Blocks without ids get them
  first, as saving gives them.
- **Undo in the rendered view:** Edit › Undo (cmd/ctrl+Z) takes back the
  rendered view's last edit (checking a task, picking a choice, moving
  a block, or what was typed into a textbox since moving to it) and Redo
  (cmd/ctrl+shift+Z) does it again, up to 100 edits back. It is its own
  history: in the source view, and in a textbox while typing, undo is
  that editor's own. Once the file changes some other way, such as
  typing in the source view, a run writing output or Reload, the
  rendered view's earlier edits can't be undone.
- **Links** in the rendered view open when clicked: web links in the
  browser, a markdown file in a new tab (or the tab it's already open
  in), any other file as the system would open it, and `#heading`
  links scroll to the heading. Right-click a link to open or copy it.
- **Source view** (the `<>` button or cmd/ctrl+\\): the whole file in a
  markdown editor beside the rendered view. Typing re-renders after a
  pause, and the two scroll together; clicking a block lines the source
  up with it. Drag the line between them to share the width
  differently; a double click shares it evenly again. Blocks whose code didn't change
  keep their output. The play button in the gutter runs a block, or by
  a heading, its section. The
  rendered view can be turned off too (its button or
  cmd/ctrl+shift+\\), to work in the source alone.
- **Markdown bar** over the source view, and Insert in its right-click
  menu: heading, bold, italic, code, link, image, lists, quote, table,
  code block and an mdsh block. Tab moves between table cells.
- **Outline** (the sidebar button or cmd/ctrl+shift+O): headings and
  blocks with their state, to jump around in both panes. View › Show Code and Show
  Output hide those parts of the blocks.
- **Code and output:** code is highlighted in both views, with line
  numbers in the rendered view, for shell and the common languages, and
  ANSI colors in a block's output are drawn as colors.
- **Folding:** the arrow by a block's code or output folds it to its
  first line, and the arrow by a heading folds its section, down to the
  next heading of the same or a higher level. The outline's arrows fold
  sections too. `collapse`, `collapse-output` and a
  `<!-- mdsh:collapse -->` line in a section say what starts folded; they
  take conditions, so `{{$MDSH_VAR_name}}` can fold or unfold a part as an
  input changes. Folding by hand writes the fold to the file, so it
  opens that way next time: a bare `collapse` or `collapse-output` on
  the block, or a `<!-- mdsh:collapse -->` line under the heading, and
  unfolding takes it out. Undo takes a fold back. Where the file folds
  by a condition, or while a block runs, a fold by hand lasts only while
  the tab is open. The source view folds headings and fences in its
  gutter, apart from the rendered view. View › Folding collapses all
  blocks or sections, expands all, and copies the folds from one view to
  the other.
- **Images:** image, screenshot and recording output shows in the
  block, and so do images in the prose, from the document's folder.
  Click an output image to see it full size. Images from other sites
  are not loaded: opening a file never reaches the network, so a note
  with the image's alt text and host shows in its place.
- **Diagrams:** a fenced `d2` block shows as the diagram it draws, a
  fenced `svg` block as its picture, and a fenced `cast` block (an
  asciinema recording) in the cast player, with a button to switch to
  the code and back. d2 diagrams are drawn by the built-in
  [d2](https://d2lang.com) library, with its dagre layout, in a light
  and a dark theme to match the window. When a diagram has a mistake,
  the code shows with d2's message under it. Pictures are shown as
  images, so an SVG's scripts don't run and it loads nothing.
- **Keys:** `j`/`k` select a block, `enter` runs it, `a` runs from it,
  `e` edits it and `c` copies its code.
- **Copy:** the copy button on a block's card (or `c`) puts its code on
  the clipboard, as written in the file, without the fences or output.
  Other fenced code in the document shows the same button in its corner
  when the pointer is over it. A block's output has one too, in its
  corner, which copies the output as text, without its colors; it says
  Copied for a moment.
- **Secrets:** a block marked `secret` has its code and output blurred
  until you press the eye on its card; they blur again after a minute
  (Settings › Editing › Show secrets for) or when the window loses
  focus. A block whose code names a variable
  set with `mdsh ctl set --secret` has its output blurred the same way.
  Copying a secret still works: on macOS the clipboard marks it
  concealed so clipboard managers skip it, and it comes off the
  clipboard after the same time unless something else was copied.
- **Screen readers and the keyboard:** every button, pane, tab and
  field has a name, and the toolbar's buttons say whether they are on.
  Each block's card is a tab stop, named for the block, its language,
  its attributes and its state (ran, failed with its exit status,
  running, in the background, or waiting for an answer); focusing a
  card selects it, so `enter`, `a`, `e` and `j`/`k` work from there. The
  outline's entries say what they are and their state, the fold
  buttons say whether they are open, and the window's tabs take the
  arrow keys, enter to show one and delete to close it, as Settings'
  tabs take the arrows. A question a block asks (confirm, depends=)
  puts the focus on Run and is read out with its reason; the status
  line, "running", errors and the end of a run are read out too. The
  grips that size the source view, the console and the History pane
  take the arrow keys, and in the outline ↑/↓ move between entries and
  ←/→ fold and unfold a section. Edit › Depends On… (`d`) edits the
  selected block's `depends=` as a list of the blocks above it to check,
  the keyboard's way to the dependency lines. The terminals read out
  what arrives (xterm's screen reader mode), and don't blink their
  cursor under reduce-motion. The Neovim source pane is a picture to a screen
  reader; the built-in editor is a text field. Keyboard focus shows a
  ring in every look, and with the system's reduce-motion setting the
  panes and folds don't slide and the views don't scroll smoothly.
- **Settings** (mdsh › Settings… on macOS, Edit › Preferences… on
  Linux) has tabs: General for the
  view files open in (rendered, source or both; by default the view
  the last tab had) and the start folder; Look for the theme (System,
  Light or Dark), the look (Plain; Teletype: the site's typewriter
  face on fan-fold paper in the light theme and green phosphor in the
  dark; Blueprint: white lines on blue paper; Bubble Gum: Hubba Bubba
  pink with bubbles across the page, or berry in the dark; Witchcraft:
  a framed kraft-paper scroll against a sky of stars and crescent
  moons, or deep plum and gold in the dark), the text size (80% to 200%; the outline widens with it), the interface and code fonts (any installed; the code font any of one width), line spacing,
  an accent color, the History pane's colors (the window's, inverted,
  or always dark) and, for Teletype, the paper's sprocket strips,
  ruled lines and card shadows; Editing for autosave after
  each run, Vim mode and spell checking of the prose in the
  source view (on by default; not code, links or HTML); Shell for a shell for every file and the file
  the History pane's shell reads; and Files for the templates folder
  for New File…. They are kept in
  `mdsh/gui.json` in the user's config folder. Shell has one more,
  **Give blocks your shell's aliases and functions**, which adds an
  `mdsh` function to `~/.zshrc` (or `$ZDOTDIR/.zshrc`), and to
  `~/.bashrc` when there is one, between marker comments, and
  unchecking it takes it out. The function runs the CLI with
  `--source <(alias -L; typeset -f)` (in bash,
  `<(unset -f mdsh _mdsh; alias -p; declare -f)`); the app itself isn't
  started from a shell, so it can't see them. The same block loads
  `mdsh`'s tab completion, in zsh once `compinit` has run. About shows the version and
  build, the Go version and platform, where the settings file, `.zshrc`
  and the folders from Settings are, whether `zsh`, `sh`, `mdsh` and the
  shell from Settings are on the app's PATH and their versions, and
  links to the changelog, docs, source and releases; Copy puts it on
  the clipboard for a bug report. The changelog is built into the app:
  its link, or Help › Changelog, opens it in a read-only tab, with its
  screenshots, so it says what changed in the build you're running. `mdsh-gui --version` prints the same
  version: the release's, or for a `go install` or `go build` build the
  module version or the commit it was built from.

If the file changes on disk, a banner offers to reload it. Reloading
keeps the shells, and the variables, functions and aliases blocks set
in them; a change to the front matter's `shell:`, `env.share` or
`env.source` waits for Restart Shells.

## A tour

The start screen lists the runbooks in a folder, with the description
from each one's front matter. Type to filter.

![The start screen: four runbooks in ~/runbooks, each with a description](../../site/static/images/gui/start.png)

### Collections

A `.mdsh.yaml` file in a folder of runbooks turns the folder into a
collection on the start screen: a header with an icon, title,
description and tags, in the folder's color, over a background image.

```yaml
title: Deploys
description: Ship, roll back and check the prod stack.
icon: rocket.svg      # an image in the folder, or one emoji: "🚀"
color: "#3b82f6"      # #hex, rgb(), hsl() or a CSS color name
background: banner.png
tags: [ops, prod]
```

Every key is optional. Image paths are relative to the folder and must
stay inside it. A file's tags are those of the collections it is in,
from the listed folder down, plus the `tags:` in its own front matter.
The tag bar under the filter lists them all; click one, or type
`#ops`, to show only the files tagged `ops`. A header's tooltip lists
anything in `.mdsh.yaml` that was ignored, and a folder whose
`.mdsh.yaml` doesn't parse shows as a plain folder with a ⚠ that says
why.

You don't have to write the file by hand: the ⚙ beside a folder on the
start screen opens its settings, a form for the same keys, and saves
`.mdsh.yaml` for you. Clearing every field removes the file; keys and
comments the form doesn't know are kept. The gear shows only on folders
you can write to, and not on one whose `.mdsh.yaml` doesn't parse.

Run All streams each block's output in under it as it runs.

![A run in progress: the build block's output so far, with Stop in the toolbar](../../site/static/images/gui/running.png)

A `background` block runs on while the next ones do, with a dashed edge
until it ends. A `confirm` block asks in its card before it runs, and
so does a block that a block you run names in `depends=` and that hasn't
run yet.

Every block that Run all runs is drawn, even one that printing leaves
out because its `show=` is false or its `<!-- mdsh:if -->` section is
off. Such a block, and a tape hidden the same way, has a dashed edge and
a "hidden when printed" badge, and a section that holds one opens
unfolded even if the file folds it, so a file can't run code you never
saw.

![A background block still running, and a confirm block asking "Ship the site to production?" with Run, Skip and Stop](../../site/static/images/gui/confirm.png)

A block's `depends=` is drawn as lines in the page margin, from the block
up to each block it names, painted faintly behind the cards: its first
dependency in purple, then pink, teal and orange, and any past four in
gray. Each line keeps a lane of its own, in the left margin when the
lanes fit there, else the right. Selecting a block lights its lines and
outlines the blocks they lead to in the same colors, in the rendered
view and in the outline. A block in a folded section is stood for by
the section's heading, and a heading standing for more than one gets a
ring for each, the first dependency's innermost; hover a line to
read what it joins, and double-click one to go to its far end. The
lines are edited where they are drawn. Each card has two anchors on its
edge: a ring at the top, where its lines leave, and a dot at the bottom,
where lines from the blocks that depend on it arrive. Drag the ring
onto a block above to depend on it, click the dot at a line's corner to
take that dependency out, or drag the dot to another block above to
depend on that one instead, or out past the lines, away from the cards,
to tear it off. Hold a dragged line over a folded section's heading
and the section opens, so the blocks in it can be dropped on; it folds
again when the line leaves it or the drag is called off, and stays open
for the tab (the file still folds it) when you drop on a block in it.
Each writes the block's `depends=`, so Undo
takes it back, and a cycle or a block that can't be depended on is
refused with mdsh's message. The button at the top of the margin hides
the lines and shows them again.

The outline lists the headings and blocks, with a mark for each block's
state. The selected block, or the one the source pane's cursor is in,
has a play button in the outline that runs it.

Each heading with a block under it has a play button beside it that
runs its section: the blocks under it, its subsections' included, down
to the next heading of the same or a higher level. They run as Run All
runs them, so `confirm` asks, `skip`, `run=` and `run=once` hold,
`depends=` runs what a block needs first even outside the section, and
`on_error=stop` ends the run. Click a heading, or pick it in the
outline, to select it: its play button shows in the outline, and enter
or `s` runs it. `s` on a selected block runs the section it is in.
While blocks run, the toolbar lists them, the run's current step (▶)
first and then those in the background (◐); click one to go to it.

![The outline beside the document, with a check by each block that ran](../../site/static/images/gui/outline.png)

The source view shows the file in a markdown editor, with the markdown
bar over it, beside the rendered view. The two scroll together.

![The source view on the left, with line numbers and run buttons in the gutter, beside the rendered view](../../site/static/images/gui/source.png)

Image, screenshot and recording output shows in the block.

![A block with out=screenshot, and its output drawn as a picture of a terminal](../../site/static/images/gui/image.png)

A `tty` block runs in a terminal in its card.

![A tty block asking "Your name?" in a terminal in its card](../../site/static/images/gui/tty.png)

View › History opens a terminal along the bottom. Each command typed
there is added to the end of the file with its output, and each `#`
line as prose.

![The History pane under a document: notes and commands typed in the pane, and the same notes as prose and commands as blocks with their output in the file](../../site/static/images/gui/history.png)

If the file changes on disk, a banner offers to reload it.

![A banner saying disk.md changed on disk, with Reload and Keep mine](../../site/static/images/gui/changed.png)

File › Create Report… makes a PDF, HTML or markdown report, and shows
the `mdsh` command that makes the same one.

![The Create report dialog: format, what to show, where the output comes from, and the mdsh command](../../site/static/images/gui/report.png)

Tabs drag between windows, or out into a window of their own.

![Two windows: deploy.md and disk.md in one, and onboarding.md, dragged out, in another](../../site/static/images/gui/windows.png)

Settings opens in a tab, like the Changelog, and has tabs of its own
for the view files open in and the start folder; the theme, the look and its options; autosave and Vim mode; the shell; the templates folder; and an
About tab with the version, build, paths and tools.

![Settings](../../site/static/images/gui/settings.png)

![The deploy runbook in the dark theme](../../site/static/images/gui/dark.png)

![The deploy runbook in the Teletype look, on paper, with push asking to run](../../site/static/images/gui/teletype-paper.png)

![The same window in the Teletype look with the dark theme, as green phosphor](../../site/static/images/gui/teletype-phosphor.png)

![The Teletype look with the source view open beside the rendered view](../../site/static/images/gui/teletype-source-paper.png)

![The Teletype look in phosphor with the History pane open, its commands added to the file as blocks](../../site/static/images/gui/teletype-history-phosphor.png)

![Settings › Look: theme, look, text size, fonts, line spacing, accent, History pane and the paper's texture](../../site/static/images/gui/teletype-settings-look.png)

![Paper with the History pane inverted, so it shows as phosphor](../../site/static/images/gui/teletype-history-inverted.png)

![The deploy runbook in the Blueprint look: white lines on blue paper with a drafting grid](../../site/static/images/gui/blueprint-light.png)

![The Blueprint look with the dark theme, in night blue, with push asking to run](../../site/static/images/gui/blueprint-dark.png)

![The Blueprint look with the History pane open under the document](../../site/static/images/gui/blueprint-history.png)

![Settings › Look with Blueprint chosen: the sheet's grid can be turned off](../../site/static/images/gui/blueprint-settings-look.png)

![The deploy runbook in the Bubble Gum look: pink paper with bubbles, puffy cards and bubble-lettered headings, with push asking to run](../../site/static/images/gui/bubblegum-light.png)

![The Bubble Gum look with the dark theme, in berry](../../site/static/images/gui/bubblegum-dark.png)

![The Bubble Gum look with the History pane open under the document, its command added to the file as a block](../../site/static/images/gui/bubblegum-history.png)

![Settings › Look with Bubble Gum chosen: the bubbles and the shadows under cards can be turned off](../../site/static/images/gui/bubblegum-settings-look.png)

![The deploy runbook in the Witchcraft look: a framed kraft-paper scroll against a sky of stars and crescent moons, with ship asking to run](../../site/static/images/gui/witchcraft-light.png)

![The Witchcraft look with the dark theme: the scroll in deep plum, ruled in gold](../../site/static/images/gui/witchcraft-dark.png)

![Settings › Look with Witchcraft chosen: the stars and moons and the glow around cards can be turned off](../../site/static/images/gui/witchcraft-settings-look.png)

## Install

Releases have a universal macOS app (`.dmg` and `.zip`), signed with a
Developer ID and notarized from 1.4 on, and a Linux AppImage; see
[Desktop app](../../site/content/docs/desktop.md). Both carry the `mdsh`
command line tool, built from the same commit: `mdsh.app` at
`Contents/MacOS/mdsh`, the AppImage at `usr/bin/mdsh`. Settings › Shell
puts it on `PATH` (a symlink in `/usr/local/bin` on macOS, a copy in
`~/.local/bin` from an AppImage); see
[docs/design/cli-in-app.md](../../docs/design/cli-in-app.md).

## Building

mdsh-gui is its own Go module, so the `mdsh` command doesn't pull in
Wails or need cgo. It needs Go 1.27 and cgo, plus:

- macOS: the Xcode command line tools.
- Linux: GTK 4 and WebKitGTK 6, for example
  `sudo apt-get install libgtk-4-dev libwebkitgtk-6.0-dev`.
- Windows: WebView2, which ships with current Windows.

```sh
cd cmd/mdsh-gui
go build .
```

On macOS, Wails, Go and the SDK ask for different minimum macOS
versions, so the linker warns that object files were "built for newer
'macOS' version". The warnings are harmless; to build for one version
and silence them:

```sh
MACOSX_DEPLOYMENT_TARGET=13.0 CGO_CFLAGS=-mmacosx-version-min=13.0 \
  CGO_LDFLAGS=-mmacosx-version-min=13.0 go build .
```

The frontend is TypeScript in `frontend/src`, bundled into
`frontend/bundle` and embedded in the binary, so `go build` needs
nothing more. After changing the TypeScript or the Go methods the
frontend calls, regenerate the bindings and the bundle (no npm needed):

```sh
go generate
```

The Neovim source pane is `nvim --embed` with the `ext_linegrid` UI:
`nvim/` runs it and keeps its buffer in step with the notebook, and
`frontend/src/nvim.ts` draws the grid on a canvas. Its tests run a real
Neovim, from `$MDSH_TEST_NVIM` or found as the app would, and skip
without one.

The code editor is CodeMirror 6 with Vim bindings, vendored as one
bundle in `frontend/vendor/codemirror.js`. `go run ./internal/vendorjs`
rebuilds it from the versions `frontend/vendor/package-lock.json` pins,
again without npm. To type-check the TypeScript:

```sh
npx --package typescript tsc -p frontend
```

## Packages

`build/darwin/app.sh VERSION OUTDIR`, on macOS, builds the universal
`mdsh.app` and puts it in a `.dmg` and a `.zip`. It signs the app ad
hoc, or with a Developer ID and notarizes it when `MDSH_SIGN_IDENTITY`
and the `MDSH_NOTARY_*` variables are set; see
[docs/signing.md](../../docs/signing.md), which also says how the
release workflow gets them. The
`.dmg` window shows an arrow from mdsh to Applications; it is laid out
with [dmgbuild](https://github.com/dmgbuild/dmgbuild) by
`build/darwin/dmg/settings.py`, over a background drawn from
`build/darwin/dmg/background.svg` by `build/darwin/dmg/render.sh`.
`app.sh` also builds `cmd/mdsh` for both architectures into
`Contents/MacOS/mdsh` and signs it before the app.
`build/linux/appimage.sh VERSION OUTDIR`, on Linux, builds the
AppImage with `wails3 generate appimage`, which bundles GTK and WebKit's
helper processes, then adds `mdsh` and `build/linux/AppRun`, which runs
it for `--cli` or a link named `mdsh`, and packs the image again. The icon is `build/appicon.png`, drawn from
`build/appicon.svg`. `.github/workflows/gui-packages.yml` runs both;
the release workflow adds their files to the release.
