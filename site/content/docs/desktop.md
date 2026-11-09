---
title: Desktop app
description: mdsh in a window of its own, on macOS and Linux.
weight: 15
---

`mdsh-gui` opens markdown files in a desktop window and runs their blocks
as you pick them, like the [terminal UI]({{< relref "interactive" >}})
with a mouse. It uses the same engine as `mdsh`, so a document runs the
same way in both, and what it saves is what `mdsh -w` would write.

## Install

On macOS with Homebrew:

```sh
brew install --cask gdey/tap/mdsh
```

Otherwise, each [release](https://github.com/gdey/markdownshell/releases)
has the desktop app beside the command line archives:

| File | For |
| --- | --- |
| `mdsh-gui_VERSION_macos_universal.dmg` | macOS 13 or newer, Apple silicon and Intel. Open it and drag mdsh to Applications. |
| `mdsh-gui_VERSION_macos_universal.zip` | The same app, zipped. |
| `mdsh-gui_VERSION_linux_x86_64.AppImage` | Linux on x86-64. Make it executable and run it; it carries its own GTK and WebKit. |

The macOS app is signed with an Apple Developer ID and notarized, so
Gatekeeper opens it without asking, and the `mdsh` inside it runs from a
terminal too. Releases before 1.4 aren't: open one once, then choose
Open Anyway in System Settings › Privacy & Security.

On Linux:

```sh
chmod +x mdsh-gui_*_linux_x86_64.AppImage
./mdsh-gui_*_linux_x86_64.AppImage notes.md
```

### The command line tool

The app carries `mdsh`, the command line tool, at the same version, so
the one download gives both. Settings › Shell has a Command line tool
row that puts it on your `PATH`:

- **macOS.** Install links `/usr/local/bin/mdsh` to the `mdsh` inside
  the app, so it updates with the app and goes away with it. It asks for
  an administrator password when you can't write `/usr/local/bin`. The
  app menu has the same step, as Install Command Line Tool…. If the app
  moves, it points the link at its new place the next time it opens.
  The Homebrew cask links it for you, into Homebrew's `bin`, and so
  conflicts with the `mdsh` formula: install one or the other.
- **Linux.** Install copies `mdsh` out of the AppImage to
  `~/.local/bin/mdsh`, with a `.mdsh-from-appimage` file beside it that
  says the app put it there. When you run a newer AppImage, it copies
  the new `mdsh` over it. If `~/.local/bin` isn't on your `PATH`, the
  row shows the line to add to your shell's startup file. Without
  installing, the image runs `mdsh` itself when started with `--cli`
  first, or through a link to it named `mdsh`:

  ```sh
  ./mdsh-gui_*_linux_x86_64.AppImage --cli runbook.md
  ```

Install does nothing when another `mdsh` is already on your `PATH`,
such as the formula's or a `.deb`'s, and says where it is. Remove takes
away only the link or copy the app made.

## Using it

```sh
mdsh-gui                  # the start screen, for the current folder
mdsh-gui ~/runbooks       # the start screen, for that folder
mdsh-gui notes.md staging # open a file; $1 is staging
```

For a tour with pictures, see [the desktop app]({{< relref "/app" >}}).
This page is the reference.

{{< shot name="hero" title="deploy.md" >}}A runbook waiting at a `confirm` block, with Run, Skip and Stop in its card.{{< /shot >}}

- **Start screen.** The markdown files under a folder, with pinned
  files, then recent ones, first, and each file's front matter
  `description` under its name. Type to filter, or `#tag` to show only
  files with that tag. The pin at the end of a row keeps a file at the
  top. A folder with a `.mdsh.yaml` (`title`, `description`, `icon`,
  `color`, `background`, `tags`) shows as a collection with a header of
  its own; the ⚙ beside a folder you can write to edits those keys in a
  form and saves the file.
- **Sessions.** New Session opens the day's file, such as
  `2026-11-02.md`, in the sessions folder (`~/.mdsh/sessions` unless
  Settings › Files names another), made from the session template the
  first time (with no template, it runs in your login shell, zsh or
  bash), with the History pane open. Last Session opens the file
  there changed last.
- **Tabs and windows.** Each file opens in a tab with shells of its own.
  Drag a tab to reorder it, onto another window to move it there, or
  away from the tab bar for a window of its own; a block running in it
  carries on. Drop a tab on another app, such as a terminal, to hand it
  the file's full path. Each file remembers its window size.
- **Running.** Every block has run and run-from-here buttons, every
  heading a play button for its section, and Run all runs the document.
  Run Current Block is cmd/ctrl+enter, Run Section cmd/ctrl+shift+enter,
  Run All cmd/ctrl+R and Stop cmd/ctrl+. . Output streams in under the
  block, with ANSI colors. A running block's card shows a spinner and a
  timer, `12.3s / 1m` with a bar that fills toward its `timeout=`, and
  keeps how long it took once it ends. Running blocks are listed in the
  toolbar, and a run returns focus to the block it started from. Run › Clear All
  Outputs (cmd/ctrl+alt+K, or the eraser in the toolbar) takes all
  output out of the document, as if nothing had run.
- **Copying.** Each block's card has a copy button for its code (or
  press `c`), its output has one in its top right corner, and other
  fenced code shows one on hover. Output is copied as text, without
  its colors. Drag over the History pane's output to copy lines of it.
- **Secrets.** A `secret` block's code and output are blurred, on its
  card and in the source pane, until you press the eye on its card, and
  masked again after a minute or when the window loses focus. A `nosave` card carries a *session only*
  badge: it runs, but its output isn't saved to the file.
- **Front matter.** The Front matter tab at the top of the page (Edit ›
  Front Matter, cmd/ctrl+shift+M) opens a form for the title,
  description, tags, the `mdsh:` options and `env:`, with any other
  keys as YAML. Each change is one edit Undo takes back.
- **Questions.** A `confirm` block asks in its card, scrolled to the
  middle of the view. A block whose
  `depends=` names one that hasn't run asks to run it first. A `tty`
  block runs in a terminal in its card and is recorded; a `background`
  block runs on, with a dashed edge, while the next ones do.
- **Dependencies.** `depends=` is drawn as colored lines in the margin,
  from a block up to each block it names; select a block to bring its
  lines forward, and drag a line's end to change what it depends on. A
  name that is no block is warned about and underlined. Folded sections
  show a ring where a line enters them, and the lines can be hidden.
- **Editing in the notebook.** Undo and redo; move blocks with
  alt+arrows, by dragging a card, a fence, or a section in the outline;
  delete a block or clear its output. A move never puts a block above
  one it depends on. Task lists, textboxes and choices write back into
  the file. Folds you make are saved in the file.
- **Source view** (the `<>` button or cmd/ctrl+\\). The whole file in a
  markdown editor beside the rendered view, scrolling with it. It
  completes block attributes, explains them on hover, underlines
  mistakes and checks the prose's spelling, with Vim keys as a setting.
  An experimental Neovim pane can take its place.
- **History** (View › History, cmd/ctrl+shift+H). A terminal along the
  bottom, with your shell's prompt, completion and emacs line keys
  (ctrl+a, ctrl+w, ctrl+y, alt+b…). Commands are added to
  the file as `#history-N` blocks with their output, with `skip`, so
  Run all doesn't run them again, and `links`, so the web addresses they
  print are clickable; `#` lines become
  notes. A history block deletes without asking, and becomes a plain
  block once moved.
- **Console** (View › Console, cmd/ctrl+shift+Y). Errors and notes from
  the app, with a count on the toolbar; a line about a place in a file
  jumps to it. A warning an edit fixed is struck out, and one with a
  plain fix, such as a `depends=` naming no block, has a Fix button.
  Its Events tab is the file's event log: shells starting and ending,
  and why, blocks running, the file changing on disk, `mdsh ctl` calls.
  Whatever cost the shells what blocks set in them stands out there and
  in the status line. The app never ends a shell without asking: Stop
  and `timeout=` interrupt the block, as ctrl+c would, and only Kill
  Shell, offered when a block won't stop, and Restart Shells, which
  asks first, start a shell afresh.
- **Outline** (cmd/ctrl+shift+O). Headings, blocks and controls, with a
  play button on the selected block.
- **Pictures.** Images, screenshots and recordings show in the block.
  `d2` fences draw diagrams, `svg` fences pictures, and `cast` fences
  (asciinema recordings) play in the cast player, as do `tty` blocks'
  recordings: play, pause, step a frame, select text and copy it, or
  copy the recording as a GIF. Images from other
  sites are never loaded.
- **Reports.** Save writes the output into the file, and File › Create
  Report… makes a PDF, HTML or markdown report, and shows the `mdsh`
  command that makes the same one.
- **Cheat sheet and changelog.** Help › Cheat Sheet opens the
  [cheat sheet]({{< relref "cheatsheet" >}}) in a read-only tab, and
  Help › Changelog the changelog of the build you're running, with its
  screenshots. Settings › About links to both.

{{< shot name="outline" title="deploy.md · outline" >}}The outline beside the notebook.{{< /shot >}}

## Settings

Settings (mdsh › Settings… on macOS, Edit › Preferences… elsewhere) has
six tabs:

| Tab | Holds |
| --- | --- |
| General | the view a file opens in, the start folder |
| Look | {{< looks >}}; light, dark or the system's; text size, fonts (any installed), line spacing, accent, the History pane's colors, and each look's own textures, such as the Teletype paper's sprockets, rules and shadows |
| Editing | autosave after each run, Vim keys, spell checking, how long a secret stays shown, the Neovim pane (experimental) |
| Shell | a shell for every file, the file the History shell reads, a switch that gives blocks your shell's aliases and functions and turns on tab completion, and Install for the command line tool |
| Files | the templates folder for New File…, the sessions folder and template |
| About | the build, the paths it uses and the tools on your `PATH`, with Copy for a bug report, and a link to the changelog |

{{< shot src="settings-look-paper.png" title="Settings" >}}Settings › Look, in the Teletype look.{{< /shot >}}

## Keys and screen readers

Keys follow `mdsh -i` where they can: `j`/`k` select a block, enter runs
it, `a` runs from it and `e` edits it. Every button, pane and tab has a
name for a screen reader and can be reached from the keyboard.

## Build it

The app is its own Go module, in `cmd/mdsh-gui`, built with
[Wails v3](https://wails.io/). On Linux it needs GTK 4 and WebKitGTK 6.0
(`libgtk-4-dev` and `libwebkitgtk-6.0-dev` on Debian and Ubuntu):

```sh
cd cmd/mdsh-gui
go build .
```

`build/darwin/app.sh` and `build/linux/appimage.sh` make the release
packages.
