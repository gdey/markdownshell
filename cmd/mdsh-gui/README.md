# mdsh-gui

A desktop window for mdsh, built with [Wails v3](https://wails.io/).
It shows a markdown file, runs its `sh` and `zsh` blocks as you pick
them, streams their output under them and saves it back to the file. It
uses the same `run.Session` as `mdsh -i`.

There is no editing, no images and no `tty` blocks yet; see
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
- **Keys:** `j`/`k` select a block, `enter` runs it and `a` runs from it.
- **Settings** (mdsh › Settings… on macOS, Edit › Preferences… on
  Linux): autosave after each run, a shell for every file and the start
  folder. They are kept in `mdsh/gui.json` in the user's config folder.

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

The frontend is plain HTML, CSS and JavaScript in `frontend/`, embedded
in the binary. It needs no npm build: it calls Go by name through the
runtime Wails serves at `/wails/runtime.js`.
