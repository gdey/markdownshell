# mdsh-gui

A desktop window for mdsh, built with [Wails v3](https://wails.io/).
It shows a markdown file, runs its `sh` and `zsh` blocks as you pick
them, streams their output under them and saves it back to the file. It
uses the same `run.Session` as `mdsh -i`.

There are no images, reports or `tty` blocks yet; see
[the design](../../docs/gui/design.md) for what comes next.

```sh
mdsh-gui [file.md [args...]]
```

With no file, the window lists the markdown files under the current
folder (or the start folder from Settings, when started from Finder or
the Dock) to pick from. Each file opens in a window of its own, with its
own shells.

- **File:** New Window, Open…, Open Recent, Save, Save As…, Reload and
  Close Window. Dropping a markdown file on a window opens it. A • marks
  output that isn't saved, and closing or quitting asks first.
- **Run:** Run All (cmd/ctrl+R, the toolbar button turns into Stop while
  blocks run), Stop (cmd/ctrl+.) and Run Options… for the file's
  arguments, `--only`, step mode and stop on error.
- **Editing** happens in the source view. The pencil (or `e`) on a block
  opens it with the cursor on the block's code, and clicking prose opens
  it on that text. New blocks are typed there too. Inside
  `{…}` the editor completes attribute names and values, explains an
  attribute on hover and underlines mistakes. Vim key bindings are a
  setting. Cmd/ctrl+click on a link opens it.
- **Source view** (the `<>` button or cmd/ctrl+\\): the whole file in a
  markdown editor beside the rendered view. Typing re-renders after a
  pause, and the two scroll together; clicking a block lines the source
  up with it. Blocks whose code didn't change
  keep their output. The play button in the gutter runs a block. The
  rendered view can be turned off too (its button or
  cmd/ctrl+shift+\\), to work in the source alone.
- **Markdown bar** over the source view, and Insert in its right-click
  menu: heading, bold, italic, code, link, image, lists, quote, table,
  code block and an mdsh block. Tab moves between table cells.
- **Outline** (the sidebar button or cmd/ctrl+shift+O): headings and
  blocks with their state, to jump around in both panes. View › Show Code and Show
  Output hide those parts of the blocks.
- **Images:** image, screenshot and recording output shows in the
  block, and so do images in the prose, from the document's folder.
  Click an output image to see it full size.
- **Keys:** `j`/`k` select a block, `enter` runs it, `a` runs from it and
  `e` edits it.
- **Settings** (mdsh › Settings… on macOS, Edit › Preferences… on
  Linux): autosave after each run, Vim mode, a shell for every file,
  the start folder and the theme (System, Light or Dark). They are kept in `mdsh/gui.json` in the user's
  config folder.

If the file changes on disk, a banner offers to reload it.

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

The frontend is TypeScript in `frontend/src`, bundled into
`frontend/bundle` and embedded in the binary, so `go build` needs
nothing more. After changing the TypeScript or the Go methods the
frontend calls, regenerate the bindings and the bundle (no npm needed):

```sh
go generate
```

The code editor is CodeMirror 6 with Vim bindings, vendored as one
bundle in `frontend/vendor/codemirror.js`. `go run ./internal/vendorjs`
rebuilds it from the versions `frontend/vendor/package-lock.json` pins,
again without npm. To type-check the TypeScript:

```sh
npx --package typescript tsc -p frontend
```
