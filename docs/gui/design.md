# mdsh-gui design

Status: design, Oct 28, 2026. Tracks the `gui` branch.

## Goals

mdsh-gui is a desktop notebook for mdsh files. You open a markdown runbook, run its blocks, edit it, and turn the result into a report, without a terminal. It does what `mdsh -i` does, adds what a window does better (editing, images, reports), and keeps the file format exactly as the CLI writes it.

- **Same files, same results.** A file saved by the GUI runs and prints the same in `mdsh`, and the reverse. All logic stays in `doc`, `run` and `render`, and the GUI adds no file format of its own.
- **Edit where you read.** Prose is edited in place in the rendered view. A split view shows the markdown source beside it, kept in sync, for when you want to see the code.
- **Reports are a first-class output.** One action turns a run into a PDF, HTML or markdown copy to share.

Not goals for v1: multiple documents in one window (use one window per file), a plugin system, remote kernels, or languages beyond what `run` supports today (`sh`, `zsh`).

## Features

Every feature of `mdsh` and `mdsh -i` gets a place in the GUI. New in the GUI: save as, create report, a start screen, and editing the whole document. M1 to M6 are the milestones below.

| Feature | Today (CLI / TUI) | In the GUI | When |
| --- | --- | --- | --- |
| Run one block, run from here, run all | `enter`, `a` in `-i`; `mdsh file.md` | Buttons per block and in the toolbar | Done (#28) |
| Stream output, stop | live in `-i`, `ctrl+c` | Live under the block, Stop button | Done (#28) |
| confirm and step mode | prompt in `-i`, `--step`, `--yes` | Question under the block's code: Run / Skip / Only unsaved / Stop | Done, add Only unsaved in M1 |
| mdsh:if sections, placeholders | printed by `render` | Same rules, redrawn after each run | Done |
| Save | autosave in `-i`, `w`, `-w` | Save (cmd+S), opt-in autosave, a dirty marker | M1 |
| Save as | `-o file` | Save As… (cmd+shift+S); the window then follows the new file | M1 |
| File changed on disk | `w` twice to overwrite | Banner: Reload / Keep mine | M1 |
| Edit one block | `e` opens `$EDITOR` | Edit the code in place in the block | M2 |
| Add a block | `i` / `:` prompt | Add a block below this one, or at the end | M2 |
| Edit the whole document | `$EDITOR` outside mdsh | Click prose to edit it in place; a split view puts the source beside the rendered doc | M3 |
| Images, screenshots, recordings | half-block preview, `v` | Real images in the output, click to open full size | M4 |
| Create report | `--pdf`, `-o`, `--hide-code`, `--hide-output`, `--no-run` | Report… dialog: format (PDF, HTML, markdown), hide code, hide output, run first or use saved output | M5 |
| tty blocks | hand over the terminal | A terminal pane in the window (xterm.js over a pty) | M6 |
| Arguments `$1…` | trailing CLI args | Command line on launch, and an Arguments field | M1 |
| `--only`, `--stop-on-error`, `--shell` | flags | Settings (mdsh › Settings…) for defaults; Run menu for this file | M1 |
| Open recent, drag a file in, file association | none | Start screen listing `.md` files under the current folder (shown when no file is given), Open Recent, drag and drop, file association | M1 |

## Screens

There is one window per document, with the rendered view always present and the source optionally beside it. Native menus carry every action, so each one has a shortcut.

These are mockups, not the built app, shown as they would look on a Mac. The toolbar holds only what changes while you work. Run all turns into Stop while blocks run, and it goes back when they finish. Rendered and split use GitHub's file and `<>` icons. The sidebar icon opens and closes the outline. Open, Save, Save As and Create Report live in the File menu, and settings live in **mdsh › Settings…** (cmd+,). Block actions are icons: pencil to edit, play to run, play-to-bar to run from here.

![Start screen: markdown files in the current folder and below, grouped by folder, with recent files first, a filter box, and block counts](picker.png)

*No file given.* The window lists markdown files under the folder it started in, with recent files first, and you click one to open it. The file names here are made up. When the app starts from Finder or the Dock there is no useful current folder (it is /), so it lists the start folder from Settings. That setting defaults to the last folder you used, and the first time to ~/Documents on macOS or your home folder elsewhere.

![Rendered view while running: Stop in the toolbar, the outline marking disk with a question mark, one paragraph in its inline editor, and the disk block with its confirm question and info tooltip](notebook.png)

*Rendered view, while running.* `disk` is marked `confirm`, so its question (the block's own `confirm=` text, or "Run this block?") sits right under its code, and the view scrolls to it. Hovering over the ⓘ icon explains why it is waiting. The first paragraph is open in its inline markdown editor.

![Split view at rest: Run all in the toolbar, the File menu open with Open, Save, Save As and Create Report, the source on the left and the rendered doc on the right](split.png)

*Split view, at rest.* Nothing is running, so the toolbar shows Run all, and the File menu is open. The cursor is on line 19, and the same section is highlighted on the right.

![Create report dialog: format PDF, HTML or Markdown, what to show, where the output comes from, and where to save](report.png)

*Create report.* It maps onto existing flags, and the dialog shows the matching `mdsh` command.

1. **Rendered view** (always on; #28 has the read-only start). Prose is rendered, and runnable blocks are cards with Run, Run from here, an exit status and output below. Click a paragraph, heading, list or table to edit it in place. The edit happens on the markdown of that one top-level block, so the rest of the file stays byte for byte as it was. Block code is edited in its card. A confirm question appears in the block's own card, under its code, and the view scrolls to it. A sticky banner at the top is kept for the file-changed notice.
2. **Split view** (toggle, cmd/ctrl+\). The whole file's markdown in a code editor (CodeMirror 6) on the left, with markdown and shell highlighting and mdsh attributes like `{#id confirm}` highlighted. The rendered view is on the right. The two scroll together and stay in sync: typing in the source re-renders after a short pause, and editing in the rendered view updates the source. Output from blocks whose code did not change is kept, as `Session.Sync` does for `-i`.
3. **Outline** (toggle). A narrow list of headings and blocks to jump around long runbooks. Each block shows its state: ✓ ran, ? waiting for a confirm answer, ✗ failed.
4. **Report dialog.** A sheet with format, hide code, hide output, "run all first" or "use saved output", and a destination. It writes the file, then offers to reveal it.
5. **Settings (mdsh › Settings…, cmd+,).** App-wide defaults such as autosave, shell, theme, Vim mode and the start folder (last used, or a fixed folder). Per-file run options (arguments, `--only`, stop on error, step mode) go in the Run menu.

Menus: **File** (New, Open, Open Recent, Save, Save As, Create Report, Reload, Close) · **Edit** (Undo, Find) · **Run** (Run Block, Run From Here, Run All, Stop, Step Mode) · **View** (Split View, Outline, Show Code, Show Output, Zoom).

**Block attribute helper.** The `{#id …}` part of a fence gets help in both the block card and the split view's source. Typing inside the braces offers completions for attribute names and their known values (`out=` text, markdown, image, screenshot; `on_error=` stop, continue; env names already in the file; `{{#block/…}}` and `{{$VAR}}` in conditions), and hovering an attribute shows its one-line meaning and default. Invalid values are underlined as errors, and unknown attributes and include-only ones on a runnable block as warnings, since mdsh keeps them untouched. The block editor also has a sliders icon that opens a form over the attributes: language, id, every flag as a checkbox, confirm and show-stderr as a checkbox with an optional value, and a field for each valued attribute (with the choices or env names offered). Include blocks get their own fields. Each change edits the opening fence in the editor, so the form and the text never disagree, and the attribute order and any unknown attributes stay as they were. Both read one attribute table exported by `doc` (library change 5), so the GUI never keeps its own copy of the spec.

**Markdown bar.** The split view's source and the inline prose editor have a slim bar of common markdown actions, like GitHub's comment editor: heading, bold, italic, inline code, link, image, bulleted, numbered and task lists, quote, table, fenced code block, and an mdsh block (a shell fence with an `{#id}` attribute, with confirm as an option). Each button inserts or wraps markdown at the cursor or selection, so the text stays plain markdown. Table inserts a small header-and-row skeleton, and pressing tab inside a table moves between cells. Every button has a shortcut where a common one exists (cmd/ctrl+B, I, K, E). Block code cards don't get the bar, since their content is code.

**Vim mode** (Settings, off by default). Every place you edit text is a CodeMirror 6 editor: the split view's source, a block's code in its card, and the inline prose editor. So one setting turns on Vim keys in all three, using the vendored `@replit/codemirror-vim` extension (normal, insert and visual modes including visual block, counts, operators and text objects, registers, marks, macros, `.` repeat, search, and `:s`). With Vim mode on, `esc` goes to normal mode instead of closing the editor, `:w` applies the edit and saves the file, `:q` closes the editor, and `:wq` or `:x` does both (Done and cmd/ctrl+enter also apply and close). Changing the setting takes effect at once in open editors.

Shortcuts follow the TUI where they don't clash with the OS. `j`/`k` move between blocks, `enter` runs, `a` runs from here, and `e` edits, but only when no text field is focused. Cmd/ctrl+S, cmd/ctrl+shift+S, cmd/ctrl+R (run all), cmd/ctrl+. (stop), and cmd/ctrl+\ (split view).

## Architecture

The GUI is a thin layer: the frontend draws, `main.App` binds, and `notebook` holds the state. Running, rendering and PDF stay in the shared library, so the CLI, the TUI and the GUI cannot drift apart.

- **Frontend** (`cmd/mdsh-gui/frontend`). TypeScript, bundled by esbuild's Go library from go generate, with no npm. CodeMirror, used by every editor, is a vendored pre-built bundle, `frontend/vendor/codemirror.js`, with the Vim extension included. `go run ./internal/vendorjs` rebuilds it without npm: it downloads the exact versions `frontend/vendor/package-lock.json` pins from the npm registry, checks their integrity hashes, bundles them with esbuild and copies their type declarations and licenses beside it. The app's own code bundles into `frontend/bundle/` and imports the vendored bundle as a separate file, so the two change independently. Typed bindings come from `wails3 generate bindings -ts -b -names -i`, which import the runtime Wails serves at `/wails/runtime.js`. CI checks that the vendored bundle matches its lock and the bindings and bundle are generated, and tsc type-checks against the vendored types. The frontend listens for events: `block` (started, stdout, stderr, done), `confirm`, `opened`, `saved`, `file-changed`, `menu` (a menu command for the window in front), `settings` and, later, `tty`. Events reach every window, so each keeps those its own window sent.
- **`main.App`** (the Wails service). One per window. Menus, dialogs (open, save as, report destination), window title and dirty marker. No logic beyond calling `notebook`.
- **`notebook`** (no Wails import, tested on its own). The open document, its `run.Session`, results, dirty state, and the file's mtime for change detection. It gains `SaveAs`, `Report`, `SetBlock`, `AddBlock`, `SetSource` (split view), SetRange (inline prose edits) and a pty for tty blocks.
- **Shared library.** Today `doc`, `run`, `render` and `pdf`. Five changes are needed, each a small PR to main that the GUI then uses:
  1. Move the report pipeline that `cmd/mdsh` builds inline for `--pdf`, `-o`, `--hide-code` and `--hide-output` into a library function, so the CLI and the Report dialog share it.
  2. A way to carry results across a whole-document re-parse: match blocks by id and body, as `-i` does for one block with `SetBlock` + `Sync`.
  3. Image links: the asset server serves files under the document's folder at `/doc/…`, and only that folder, so saved and new images show.
  4. tty blocks: `run.Options.TTY` takes an `*os.File` already. The GUI passes the far end of a pty and streams the near end to xterm.js. No change to `run` is expected, but it needs checking.
  5. Attribute table: `doc` exports its block attributes (name, value kind, allowed values, default, one-line help) as data, and a function that formats attributes back into an info string while keeping order and unknown ones. A test checks the table against the attribute table in `docs/spec.md`, so it can't drift from the spec. (#31.)

**Editing prose in place.** Each rendered prose block carries the byte range of its markdown in the file. goldmark records source lines for every block, so `notebook` can return them with the cells. Clicking a block swaps it for a small markdown editor holding just those bytes. On blur, `notebook.SetRange` splices the new text in, re-parses, and redraws. A rich-text editor that writes the whole file back would reformat untouched markdown and break the byte-for-byte guarantee, so v1 does not use one (agreed with j on Oct 28).

![mdsh-gui architecture: 3 GUI layers over the shared library](architecture.png)

Only `notebook` and below hold logic. The CLI and TUI use the same library, so a fix there reaches all three.

## Decisions

j answered the open questions on Oct 28.

| Question | Decision |
| --- | --- |
| What is a report? | An export of the run: PDF, HTML or markdown, with hide code and hide output options, like `--pdf` and `-o`. |
| Frontend build | TypeScript with no npm. esbuild's Go library bundles it from `go generate`. CodeMirror is a vendored pre-built bundle in `frontend/vendor/`, rebuilt from a package lock by a Go tool. Typed bindings come from `wails3 generate bindings -ts`. `tsc` type-checks in CI only, through npx. |
| Autosave | Off by default: save when asked, with a dirty marker. Autosave is a setting. |
| Windows or tabs | One window per file for now. Keep the window state separate from the app state so tabs can be added later. |
| Packaging | Later. The targets are `.app` and `.dmg` for macOS, Flatpak for Linux, and a Windows installer much later. |
| Platforms | macOS and Linux. Windows is not supported for now. |
| Inline prose editing | A small markdown editor on that block's source, not a rich-text editor. |
| Block attribute helper | Completion and hover help inside `{…}`, plus a form on the block card. Both use an attribute table exported by `doc`. |
| Markdown bar | Insert-at-cursor buttons for headings, lists, tables, blocks and so on. The text stays plain markdown; it is not WYSIWYG. |
| Vim key bindings | A setting, off by default. It covers all editors, since they are all CodeMirror. |

## Milestones

Each milestone is one PR into `gui`, one commit each, with tests passing. Library changes go to main first and reach `gui` through a rebase. `gui` merges to main after M5. M6 can follow on main if tty blocks are not needed for a first release.

1. **M0, Scaffold.** Notebook view, run, stream, confirm, save. (#28.)
2. **M1, Files.** (#30.) Save As, a dirty marker and opt-in autosave, a file-changed banner, Open Recent, drag and drop, arguments and run settings, native menus and shortcuts.
3. **M2, Block editing.** Edit a block in place (pencil icon or `e`) and add a block at the end of the file, using `SetBlock` / `AppendBlock` + `Session.Sync`, as `-i` does. The editor holds the whole block, fences included, so the attributes are edited where they are written; the block's old output stays until it runs again. The block editor is CodeMirror from the vendored bundle, with the Vim mode setting. This brings in the frontend build. It also adds the block attribute helper (completion, hover help and the form), which needs library change 5 on main first. Adding a block anywhere but the end needs a library change (an insert next to `AppendBlock`) and comes later.
4. **M3, Split view and prose editing.** The source pane (CodeMirror), click-to-edit prose in the rendered view, and the markdown bar for both. It needs library change 2 (results carried across a re-parse) on main first.
5. **M4, Images.** Serve the document's folder and show image, screenshot and recording output inline. Library change 3.
6. **M5, Reports.** The Report dialog for PDF, HTML and markdown. It needs library change 1 (a shared report pipeline) on main first.
7. **M6, tty blocks.** A pty and xterm.js pane. Library change 4, if any.

After M5: merge `gui` into main. Packaging comes later: `.app` and `.dmg` for macOS, Flatpak for Linux, and a Windows installer much later.
