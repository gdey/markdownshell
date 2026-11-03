# mdsh-swift

mdsh-gui's window, written in Swift with SwiftUI, for macOS 14 and
later. It aims to match [mdsh-gui](../mdsh-gui/README.md) feature for
feature, and is a work in progress: see [What's missing](#whats-missing).

```sh
mdsh-swift [flags] [file.md [args...] | folder]
```

## How it works

The app only draws. Everything else, parsing, running blocks, editing
and saving, is the same Go code `mdsh` and `mdsh-gui` use, in mdsh-gui's
`notebook` package. The app reaches it through a small Go program,
`mdsh-swift-bridge` in [bridge/](bridge), which it starts once and talks
to in JSON lines on its stdin and stdout:

```
→ {"id": 7, "method": "runBlock", "params": {"doc": 1, "block": 0}}
← {"event": "block", "doc": 1, "data": {"kind": "stdout", "block": 0, "data": "hello\n"}}
← {"event": "ran", "doc": 1}
← {"id": 7}
```

Requests are served at the same time, so a long run doesn't hold up
Stop, or the answer to a block's confirm question. The methods follow
mdsh-gui's `App` methods ([service.go](../mdsh-gui/service.go)), minus
the window, dialog and tab bookkeeping, which the Swift app does itself;
[bridge/methods.go](bridge/methods.go) lists them. So the app adds no
logic and no file format of its own.

The settings are mdsh-gui's settings, kept in `mdsh/swift.json` beside
mdsh-gui's `gui.json`, so the two apps don't write over each other's
recent files.

- `Sources/MdshKit`: the bridge client and the types it sends. Foundation
  only.
- `Sources/MdshSwift`: the app.
- `bridge/`: the Go bridge, its own module, which uses mdsh-gui's
  packages.

## Building

It needs macOS 14 or later, Xcode 15 or later (or its command line
tools), and Go.

```sh
scripts/build-app.sh          # build/mdsh-swift.app, the bridge inside it
scripts/run.sh notes.md       # build the bridge and swift run the app
```

The app finds the bridge in `$MDSH_SWIFT_BRIDGE`, in its bundle's
Resources, beside its executable, or on `PATH`.

```sh
(cd bridge && go test ./...)
MDSH_SWIFT_BRIDGE=$PWD/build/mdsh-swift-bridge swift test
scripts/screenshots.sh build/screenshots
```

CI ([swift.yml](../../.github/workflows/swift.yml)) builds and tests both
on macOS, and keeps screenshots of the app and the app itself as
workflow artifacts.

## What's in it

- Windows and tabs: macOS window tabs, so a tab can be dragged out to a
  window of its own or into another window. New Tab, New Window, Open…,
  Open Recent, Save, Save As…, Reload, Close Tab, Close Window, and
  asking about unsaved output before a tab closes or the app quits.
  Dropping a markdown file on a window opens it.
- The start screen: filter box with `#tags`, New File… (with templates),
  Open…, New Session, Last Session, the tag bar, Pinned, Recent, files by
  folder with descriptions, collections with their icon, color and
  background, and folder settings.
- The rendered view: prose, headings that fold and run their section,
  block cards with copy, edit, delete, run from here and run, folds for
  code and output, live output with a copy button, `secret` blocks
  masked until the eye button shows them, exit status, confirm and depends
  questions in the card, task checkboxes, textboxes and choices, `tty`
  blocks in a terminal in the card, the cast player for a `tty` block's
  recording and a `cast` fence (play, pause, scrub, step, frame=, Copy
  as GIF), and drag to move a block. Keys j, k,
  enter, a, s, c, e, d, delete and alt+arrows.
- The source view, with markdown and fenced code highlighted, line
  numbers, run buttons and fold arrows in the gutter (headings,
  fences and output regions fold), the current line shaded, the
  markdown bar, ⌘↩ to run the block at the cursor, and spell
  checking. View › Folding copies folds between the two views.
- The outline, with each block's state.
- The History pane, with the shell's prompt, history, Tab completion
  and the readline keys (ctrl+w, ctrl+u, ctrl+k, ctrl+y, alt+b/f/d,
  alt+backspace, ctrl+p/n, ctrl+l, ctrl+d); the Console pane.
- The front matter form, Run Options, Create Report, Depends On, Clear
  All Outputs.
- Settings: General, Look (theme, the Plain, Teletype, Blueprint and Bubble Gum
  colors, text size, fonts, line spacing, accent, History pane colors, History prompt focus),
  Editing, Shell (with the zsh wrapper), Files, and About.

## What's missing

Next, roughly in order:

- Prose drawn by AppKit's HTML import, which leaves out task
  checkboxes in place (they are listed beside the text), and draws
  tables and images plainly.
- Live output is plain text until the block ends; ANSI colors show once
  its output is rendered.
- The `tty` terminal is plain text read by the rules mdsh uses for a
  recording's text (carriage return, backspace, erase to end of line),
  not a terminal emulator, so full-screen programs such as vim don't draw.
- Editing the dependency lines by dragging their rings and dots; Depends
  On (d) edits them. The page lays out only the cards near the screen,
  so a line to a block further away runs off the edge of the page.
- The toolbar and buttons in the Teletype, Blueprint and Bubble Gum looks (keycaps,
  lettered outlines, pills); the paper, cards and badges follow the look.
- Attribute completion, hover help and checks in the source view; Vim
  and Neovim.
- Scroll sync between the rendered and source views.
- Window sizes and places kept per file.
- `--source` on the command line.
- Links to other markdown files and to `#headings` in the rendered
  view; they open in the browser for now.
- Show in Finder after Save As.
- The cast player's copy of a frame as text, and selecting text on its
  screen: it shows the frames as pictures.
- The built-in changelog and Help › Changelog: Settings › About links to the
  changelog on GitHub rather than opening it in a read-only tab.

Images: the notebook renders local images as `/doc/<path>` links, only
for files beside the document, and the app reads those files into
`data:` URLs for AppKit's HTML import. Unlike mdsh-gui's asset server it doesn't check the folder again,
so it relies on the notebook keeping to that.
