# mdsh-gui

A desktop window for mdsh, built with [Wails v3](https://wails.io/).
It shows a markdown file, runs its `sh` and `zsh` blocks as you pick
them, streams their output under them and saves it back to the file. It
uses the same `run.Session` as `mdsh -i`.

This is an early start: there is no editing, no images and no `tty`
blocks yet.

```sh
mdsh-gui [file.md [args...]]
```

With no file, use **Open…** to pick one.

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
