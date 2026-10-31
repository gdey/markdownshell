# Changelog

All notable changes to mdsh are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions
follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `mdsh-gui`: the active block (the selected card, or the block the
  source pane's cursor is in) has a play button by its name in the
  outline, which runs it as the card's play button does. Moving the
  cursor into a block in the source pane now selects its card.
- `mdsh-gui` shows which blocks are running: the toolbar lists the
  run's current step (▶, or ? while it waits for an answer) and each
  block running in the background (◐), and a click on one jumps to
  it. The outline marks them the same way, instead of … for both.
- `mdsh-gui` **Run › Clear All Outputs** takes every block's output out
  of the document, empties output copies and the stored values of
  block placeholders, and starts the shells again, as if nothing had
  run. Includes keep their lines. It isn't saved until you save, and
  with the source view open, undo there brings the output back.

## [1.2.1] - 2026-10-29

Three changes to `mdsh-gui`: links in a document now work, a setting
picks the view files open in, and the macOS `.dmg` shows how to install.

### Added

- `mdsh-gui` setting **Open files in**: the rendered view, the source
  view, or both side by side. The default, the view the last tab had,
  keeps the 1.2.0 behavior. It applies when a file opens in a tab, not
  on a reload or when a tab moves to another window.

### Changed

- The macOS `.dmg` opens on a window laid out for installing: mdsh's
  icon and the Applications folder on either side of an arrow, with
  notes on getting past Gatekeeper for the unsigned app (Open Anyway,
  or `xattr`). The desktop docs now lead with Open Anyway, since macOS
  15 dropped right-click › Open.

### Fixed

- `mdsh-gui` links in the rendered view are followed when clicked,
  instead of opening the source editor. Web and mail links open in the
  system browser, a markdown link opens in a tab (or brings forward the
  tab it's already in), other local files open as the system would open
  them, and `#heading` links scroll to the heading. Links have their own
  right-click menu (Open Link, Copy Link), and nothing can navigate the
  window away from the app, which used to leave the tab blank.

## [1.2.0] - 2026-10-29

The desktop app: `mdsh-gui` opens documents in a window, with packages for
macOS and Linux. Blocks can also run in the background, output can be
copied elsewhere in a document, and reports come as HTML too.

### Desktop app (`mdsh-gui`)

- `mdsh-gui`, a start on a desktop window for mdsh built with Wails v3.
  It shows a document, runs blocks as you pick them, streams their output
  and saves it. It is its own Go module under `cmd/mdsh-gui`.
- `mdsh-gui` files and menus: one window per file, a start screen listing
  the markdown files under the current folder, native menus and
  shortcuts, Save As, Open Recent, Reload, drag and drop, a dirty marker
  with a prompt before closing unsaved output, a banner when the file
  changes on disk, run options (arguments, only, step mode, stop on
  error) and a Settings window (autosave, shell, start folder).
- `mdsh-gui` start screen shows each file's front matter description
  (`description`, else `summary`, else `abstract`) under its name, and
  the filter matches it too.
- `mdsh-gui` tabs: each file opens in a tab with its own shells. Tabs
  drag to reorder, onto another window to move there, or away for a
  window of their own, and a run carries on through a move.
- `mdsh-gui` packages: releases have an unsigned universal macOS app
  (`.dmg` and `.zip`) and a Linux AppImage, and `mdsh-gui --version`.
- `mdsh-gui folder` lists the markdown files under that folder on the
  start screen, in place of the current folder.
- `mdsh-gui` block editing: edit a block in a CodeMirror editor, fences
  and attributes included, or add one at the end. The editor completes
  attribute names and values, explains them on hover, underlines
  mistakes and has a form over the attributes. Vim key bindings are a
  setting. The frontend is now TypeScript, bundled by esbuild from
  `go generate`, with CodeMirror vendored without npm.
- `mdsh-gui` source view: edit the whole file in a markdown editor
  beside the rendered view, or on its own; blocks whose code didn't
  change keep their output, and a gutter button runs a block. It
  replaces the block card editor and Add block: the pencil and clicks
  on prose go to the source, and clicking a block lines the two up. A
  markdown bar, and Insert in the right-click menu, add headings,
  lists, tables, links and mdsh blocks. An outline drawer lists the headings
  and blocks with their state, and View › Show Code and Show Output hide
  those parts. A Theme setting picks System, Light or Dark, with code
  colors that read well in both. A confirm block's own Run button
  runs it without asking again.
- `mdsh-gui` images: image, screenshot and recording output, and images
  in the prose, show inline from the document's folder, which the window
  serves and nothing outside it. Clicking an output image shows it full
  size.
- `mdsh-gui` reports: File › Create Report… writes a PDF, HTML or
  markdown report of the window's run, a run of every block, or the
  saved file, with code and output shown or hidden. It shows the
  matching `mdsh` command, and offers to show the file in its folder.
- `mdsh-gui` tty blocks: a block marked `tty` runs in an xterm.js
  terminal in its card, sized to fit, with keys going to the block. Its
  recording replaces the terminal when it ends.

### Running documents

- A block marked `background` starts and the run goes on to the next
  block. Its output is kept apart from other blocks', its process ID is
  in `$MDSH_BG_<id>` for `wait`, `timeout=` kills just that block, and
  after the last block mdsh waits for it (until its timeout, or 5s) and
  then kills it. `run.Result.Running`, `Session.Reap` and `Session.Wait`
  let front ends follow background blocks, and `env.Shell.Background`
  starts one. `mdsh -i` tags them, shows them as running in the
  background while other blocks run, counts them in the status line,
  and fills in their output when they end.
- `examples/fifo` hands data between blocks through named pipes: a
  background reader fed by later blocks, a writer held open across
  blocks, and one never closed, which only a timeout ends. Tests in
  `run` pin down what happens in each case.
- Output copies: an `output {for=id}` fence, or an `mdsh:output` region,
  that isn't right after block `id` is a copy of its output, refilled on
  every run like a placeholder. `stream=stderr` copies the stderr fence.
  A copy keeps its content when the block doesn't run, and one for a
  block that doesn't exist is a warning. `doc.Document.Copies` lists them
  and `doc.Document.Warnings` holds the warnings, which the CLI prints.

### Reports

- `--html FILE` writes what mdsh would print as a standalone HTML page,
  styled like the PDF, with local images inside it. `--html -` writes it
  to stdout.
- The `report` package makes the reports `--pdf`, `--html` and `-n -o`
  write (PDF, HTML or the printed markdown), so other tools such as
  mdsh-gui make the same ones.

### Library

- `run.Options.Terminal` runs blocks marked `tty` on a terminal the
  caller draws itself, such as a pane in a window, with keys on a
  channel and the size it reports. `env.Shell.RunTerminal` does the
  running.
- `doc.BlockAttrs` describes every block attribute (kind, allowed
  values, default, one-line help) for tools that help write them, with
  `AttrSpec.Check` to validate a value. `doc.SplitInfo` edits a fence
  info string in place, keeping unknown attributes and their order.
- `doc.Document.SetSource` parses an edited file again, keeping each
  unchanged block's identity, and `run.Session.Sync` then keeps their
  results, so a whole-file edit only clears the output of blocks that
  changed.

### Site and examples

- A Use cases section on the site: runbooks, reports and setup
  checklists, each with a sample report in PDF, HTML and markdown. New
  runbooks in `examples/runbooks` back them: `deploy.md`,
  `disk-report.md` and `onboarding.md`. `site/samples.sh` regenerates
  the samples.
- `examples/runbooks/mdsh-release.md`, the runbook for releasing mdsh:
  it checks `CHANGELOG.md` and the checkout, shows the release notes'
  sections, and with `PUSH=1` tags and pushes the release.

### Changed

- The output right after a block holds at most one stdout part and one
  stderr fence. A second `output` fence for the same block used to be
  replaced along with the first; it is now a copy.
- mdsh now needs Go 1.27 or newer to build, since the desktop app's
  Wails v3 needs Go 1.26 or newer and the CLI and app share one Go.

### Fixed

- When a block's `timeout=` stops the run, mdsh says it timed out,
  instead of blaming `exit` or `set -e`.

## [1.1.0] - 2026-10-28

Interactive mode: `mdsh -i` opens a document in a terminal UI and runs its
blocks as you go.

### Interactive mode

- `mdsh -i file.md`, interactive mode: a terminal UI (Bubble Tea) that
  shows the document, runs blocks as you pick them, streams their output
  under them and saves it to the file. `a` runs from the cursor to the
  end; `confirm` and step mode ask inside the UI.
- `i` (or `:`) types a command at a prompt; it is added to the end of the
  document as an `sh` block and run.
- `tty` blocks and taped blocks run on the terminal: the UI steps aside
  while they run and comes back on enter.
- Images and recordings show under their block as half-block previews,
  and `v` shows one full size through the kitty, iTerm2 or sixel
  protocol.
- `e` opens the selected block, fences and attributes included, in
  `$VISUAL`, `$EDITOR` or `vi` and saves the edit to the file. Saved
  output follows a changed id.

### Library

- `run.Session` runs a document's blocks one at a time while keeping
  their shells alive; `run.Run` is now built on it. `run.Session.Sync`
  catches a session up with edits to the document, and
  `run.Options.TakeTTY` lets a front end hand the terminal over.
- `doc.AppendBlock` adds a block to the end of a document, and
  `doc.Document.BlockText` and `SetBlock` read and replace a block while
  every block keeps its identity.
- The new `termimg` package draws images in a terminal.

### Shell helpers and examples

- `md_table -F SEP` splits fields on SEP as `awk -F` does (`-F ' '` is
  runs of blanks), and `md_table -s N` skips the first N lines.
- `examples/pdf/test-coverage.pdf`, the PDF of a run of the test coverage
  report runbook.

### Fixed

- The last thing a `tty` block or tape shows, after a quiet spell, is no
  longer sometimes cut off from the terminal and the recording.
- Stopping a block (`timeout=`, ctrl+c) no longer waits for a child that
  outlives the kill, such as a `sleep`: mdsh stops reading the shell's
  output instead of waiting for it to close.
- An env whose shell exited starts a fresh shell for the next block run
  in it.

## [1.0.0] - 2026-10-28

The first release: a markdown file with shell blocks runs end to end from
the command line, and its output is printed or written back into the file.

### Running documents

- `mdsh` runs the `sh` blocks in a markdown file top to bottom and prints
  the markdown with each block's output below it. `mdsh file.md` is short
  for `mdsh run file.md`, so `#!/usr/bin/env mdsh` makes a markdown file
  executable; extra arguments reach blocks as `$1`, `$2`, …
- `zsh` blocks run too. A document with any `zsh` block runs in `zsh`
  unless `shell:` is set.
- Blocks without `env=` share one long-lived shell; sub-envs only receive
  the exported variables listed in `share=`.
- `env.share` and `share=` accept `...` for all variables (like `*`) and
  `-NAME` to leave a variable out, so `[..., -USER]` passes all but `USER`.
- Fences inside list items and block quotes run like any other block;
  their output is written inside the same list item or quote.
- Include blocks: a block marked `<include` with `file=`, `from=`, `to=`
  and `number_on` holds lines from another file, written into its body on
  every run. Include blocks never run and have no name.
- `tty` block attribute: the block runs on a terminal, so commands that
  need one, such as `docker run -it`, work, and ctrl+c reaches them
  rather than mdsh. The session is saved as an asciinema cast and as an
  animated GIF linked below the block; a PDF shows its last frame, or
  the one `frame=N` picks. With no terminal the block fails with an
  error, unless it has a tape. Shells now run in their own process group.
- `tape {for=id}` fences type the input for a `tty` block, in the
  commands of a VHS tape, inline or from a file with `<include`, so the
  block runs unattended.
- Shell helpers for markdown output: `md_h1`–`md_h3`, `md_table`, `md_code`.

### Command line

- `-w` writes output back into the file; `-o out.md` writes it to another
  file instead. Writing back twice gives the same file.
- `--no-run` (`-n`) prints the document with the output saved in it by
  `-w`, without running anything. It combines with `--hide-code` and
  `--hide-output`, and `-o out.md` saves the printout to a file.
- `--hide-code`, `--hide-output` (both: prose only, nothing runs),
  `--only BLOCK`, `--stop-on-error`, `--yes`, `-` to read from stdin.
- Step-wise runs: `--step` or front matter `step: true` asks before every
  block. The confirm prompt can run, skip, skip blocks that already have
  saved output, or stop.
- Runs that ask (`--step`, `step: true`, `confirm` blocks) print the
  document as they go, so each prompt follows the prose and code it is
  about and a failed block's output and stderr show before the next one.
  This works with `-w` too. Without prompts, `-w` prints a failed block's
  output and stderr after its status line.
- When a run stops early, mdsh says why, before and after the rest of
  the document: the block ended its shell (`exit`, `set -e`), failed
  with `on_error: stop`, or you stopped at a prompt.
- `--pdf FILE` lays out what mdsh would print as a PDF, in pure Go. The
  hide flags, `--only` and the rest apply; with `-w` or `-o` the markdown
  is written as well. `--pdf -` writes the PDF to stdout.
- Styled terminal output with `--color=auto|always|never`, `--no-color`
  and `NO_COLOR`.
- `mdsh --version` and `mdsh completion bash|zsh`.

### Document format

- Front matter options under `mdsh:`: `shell`, `on_error`, `interpolate`,
  `show_stderr`, `step` and `env.share`.
- Block attributes: `#id`, `skip`, `env=` (stacked sub-envs), `share=`,
  `on_error=`, `timeout=`, `confirm`, `run=`, `show=`, `show-output=`,
  `show-stderr=`, `out=markdown|image|screenshot` and `embed=inline`.
- `out=image` saves image output; `out=screenshot` draws text output as a
  terminal-style SVG or PNG.
- Placeholders in prose: `{{#block/env(VAR)}}` and `{{$VAR}}`, with the
  last value stored in the file on write-back.
- Block facts: `{{$?}}` (alias `{{$err?}}`) for the last block's exit
  status, `{{$ran?}}` for whether it ran, and `{{#block/?}}`,
  `{{#block/err?}}` and `{{#block/ran?}}` for a named block, in
  conditions and prose.
- Conditions (`{{$VAR}}`, `{{#block/env(VAR)}}`, `!`, `==`, `!=`) for
  `run=`, `show=`, `show-output=`, `show-stderr=` and
  `<!-- mdsh:if COND -->` … `<!-- mdsh:end -->` sections.

### Releases and docs

- Release archives for Linux, macOS and Windows (amd64 and arm64), built
  by goreleaser when a `v*` tag is pushed. Windows builds are not well
  tested yet.
- A documentation site in `site/`, the design spec in `docs/spec.md`, the
  roadmap in `docs/plan.md`, and examples: a literate `examples/hello.md`,
  `examples/report.md` with PDFs in `examples/pdf`, and runbooks in
  `examples/runbooks` (release checklist, system report, Linux and macOS
  triage, test coverage report), run as tests on the OS each is for.

[Unreleased]: https://github.com/gdey/markdownshell/compare/v1.2.1...HEAD
[1.2.1]: https://github.com/gdey/markdownshell/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/gdey/markdownshell/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/gdey/markdownshell/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/gdey/markdownshell/releases/tag/v1.0.0
