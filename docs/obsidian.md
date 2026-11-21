# mdsh in Obsidian: feasibility

Status: research only, nothing built. Written 2026-10-31 for j's question "how can we embed the GUI version into Obsidian?" Updated the same day with j's answers on `sh` fences and distribution, and to build on the `server` branch (`ui`, `serve`, `mdsh --serve`).

## Short answer

The Wails app itself can't be embedded: Obsidian plugins are JavaScript running in Obsidian's own Electron window, with no Go runtime and no way to host a second webview app inside a pane. What can be done is an **Obsidian plugin that talks to `mdsh --serve=stdio`** (option A below). Obsidian already renders the markdown and owns the editor, so the plugin only adds run buttons, output, confirm prompts and stop. The Go side is the `ui` and `serve` packages from the `server` branch, which `mdsh --serve` already uses for the browser. It would be desktop only, because Obsidian mobile can't start processes, and mdsh needs a shell anyway.

## What Obsidian gives a plugin

From `obsidian.d.ts` (obsidianmd/obsidian-api):

- `registerMarkdownCodeBlockProcessor(lang, fn)` renders a fenced block in Reading view and Live Preview, and `registerMarkdownPostProcessor` decorates rendered HTML. `ctx.getSectionInfo(el)` gives the block's line range, which is how we'd read the `{#id …}` info string.
- `registerEditorExtension(ext)` adds CodeMirror 6 extensions to the editor (Source mode and Live Preview). Obsidian provides `@codemirror/*` itself, so a plugin marks them external and uses Obsidian's copies.
- `vault.process(file, fn)` edits a file atomically, and the `Editor` API edits the open buffer without moving the cursor.
- Custom panes (`ItemView`), modals, commands, a ribbon icon and a settings tab.
- On desktop, Node is available (`require("child_process")`), so a plugin can spawn a process. Such a plugin must set `isDesktopOnly: true`.

## Options

### A. Plugin + mdsh helper over stdio (recommended)

`mdsh --serve=stdio` speaks newline-delimited JSON on stdin/stdout. It is the transport `mdsh --serve` already has on the `server` branch (#52–#54), without HTTP: the same call-by-name dispatch over `ui.App` that `serve/call.go` does for `POST /wails/call`, with events (`block`, `confirm`, `opened`, …) written as notifications instead of sent over the websocket, and `ui.Host`'s dialogs (`Ask`, `OpenFiles`, `SaveFile`) sent to the plugin as requests it answers. The plugin spawns it once per vault and opens a tab per note, so shell state carries between blocks, which is the main point of mdsh. No token is needed: only the parent process holds the pipes. See "The stdio entry point" below.

Plugin side:
- A run button on each mdsh block: a markdown post-processor in Reading view, and a CodeMirror 6 extension in Live Preview and Source mode (see "`sh` fences" below).
- Streaming output under the block while it runs. When it ends, the plugin writes the new text back through the `Editor` API, or `vault.process` if the file isn't open.
- A modal for `confirm`, commands for Run All, Stop and Run Options, and a status bar item that counts background blocks.
- Attribute completion, hover help and lint in the editor.
- Later: tty blocks in a modal with xterm.js, and Create Report.

Pros: real sessions, the same behaviour as the GUI, the browser and `mdsh -i`, and one code path in Go. Cons: a protocol to keep stable. Most work is on the TS side.

### B. Plugin shells out to `mdsh -w --only <id>` per block

Each run starts `mdsh -w --only id file.md`, and Obsidian picks up the rewritten file.

Pros: no Go changes, so an MVP takes a few days. Cons: there's no session. `--only` re-runs the env chain each time, so `cd`, variables and background blocks don't carry over between clicks. There are no live `confirm` prompts (they go to `/dev/tty`, which there isn't), no tty blocks, and the file is written behind Obsidian's buffer, so it reloads while the user may be typing. This is fine as a throwaway prototype, but it isn't "the GUI in Obsidian".

### C. WASM build of the Go core

Go's `js/wasm` and `wasip1` can't start processes or open ptys, and `env` is built on `os/exec` and `creack/pty`. Running blocks would mean sending every process call back through Node, which amounts to rewriting `env`. Only `doc` (parsing, attribute checks) and `render` would port cleanly. That could serve lint and completion without a process, but it doesn't solve running. Not worth it on its own.

### D. Keep mdsh-gui separate and link to it

Add an Obsidian command to open the note in mdsh-gui (`mdsh-gui <path>`) and rely on the file-changed banner both ways. It takes about an hour. It's not embedding, but it's a useful stopgap.

## What's reusable

| Piece | Reuse in the plugin |
| --- | --- |
| `notebook` and `ui` (Go, root module on `server`) | Yes. `ui.App` is already Wails-free behind `ui.Host`; the stdio server is a third `Host` next to Wails and HTTP. |
| `serve/call.go` dispatcher | Yes, if it is split from the HTTP handler so stdio and HTTP share it. |
| `frontend/src/api.ts` | It's the seam: swap the Wails `App` bindings for a JSON-RPC client with the same method names, and `editor.ts`, `terminal.ts` and `report.ts` compile unchanged. |
| `rungutter.ts` | Almost as is (CM6 state/view only). |
| `editor.ts` (attr completion, hover, lint) | Yes, as editor extensions. It uses `@codemirror/autocomplete` and `lint`, which Obsidian ships, but this needs checking. Drop the Vim import, since Obsidian has its own Vim mode. |
| `terminal.ts` (xterm.js) | Yes, for tty blocks. Bundle xterm into the plugin. |
| `report.ts` | Partly. Rewrite the dialog as an Obsidian `Modal`. |
| `app.ts`, `window.ts`, `markdown.ts`, `settings.ts`, `theme.ts`, tabs, outline, markdown bar | No. Obsidian already does tabs, rendering, outline, editing, themes and settings. |

## What has to change

1. **Who owns the file.** `notebook` reads and writes the file itself and checks its mtime. In Obsidian the buffer is the truth. The bridge should take text in (`SetText`) and hand the new text back (or a list of edits) instead of saving. The plugin applies it as an editor transaction so the cursor and undo history survive. This is the biggest Go change. `Save`, `fold` and the stamp checks would get a mode with no write.
2. **Module layout.** Done on the `server` branch: #52 moved `notebook`, `settings`, `library` and the frontend into the root module, so `mdsh` can serve without Wails.
3. **Rendering.** The GUI renders cells to HTML in Go (`n.html`). In Obsidian we'd drop that and let Obsidian render. We'd only need block positions, state and output. `Cells` would grow a lighter variant.
4. **Images.** Output images are links to `<doc>.assets/…` relative to the note, which Obsidian resolves. We'd drop the GUI's `/doc/` asset URL rewriting in bridge mode.
5. **Output regions.** `<!-- mdsh:output -->` comments are hidden in Reading view. Output fences show as ordinary code blocks, so the post-processor should style them as output and tie them to their block.
6. **PATH.** Electron apps started from the Dock don't get the login shell's PATH. The plugin needs a setting for the mdsh path and should start the bridge through a login shell.

## The stdio entry point

Agreed with the HTTP server work (`server` branch, #52–#56). It will be built on main once `server` merges.

- **Flag.** `--serve=stdio` is a special value of the existing `--serve[=host:port]` flag, checked before the address is parsed. It's a bool-style flag, so `--serve stdio` with a space doesn't parse: it must be `--serve=stdio`. `mdsh-gui` takes the same flag and runs without a window when given it, so every binary we ship can be the plugin's backend.
- **Dispatcher.** `methods(*ui.App)` and `invoke` in `serve/call.go` don't depend on HTTP, and names ending in `ui.App.<Method>` already resolve. stdio is a second transport in package `serve`, not a new package.
- **Tabs.** `ui` gets a call's tab from an unexported context key that `ui.TabCalls` sets from a header. stdio needs an exported `ui.WithTab(ctx, id)`.
- **Dialogs.** `Ask`, `OpenFiles`, `SaveFile` and `ChooseFolder` go through serve's `wait`/`cancelAsks` to a window's connections. If a connection becomes an interface with `push([]byte)`, a stdio session is one window whose connection writes to stdout, and `wait`, `Ask` and `promptPath` stay as they are.
- **Frames.** One JSON object per line, in the shapes serve already uses. Calls are `{"id","name","args","tab"}`, answered by `{"id","result"}` or `{"id","error"}` (the `id` is new, for matching replies to calls). Events are `{"name","data","sender"}`, and tab events have `data: {"tab","data"}`. Questions are `mdsh:ask` and `mdsh:prompt` events, answered by `{"id","value"}`. A plugin that bundles `web/` with a small shim can then reuse the `runtime.js` logic.
- **Process.** No token, since only the parent process holds the pipes. stderr is for logs only, and EOF on stdin ends the server and its shells.
- **File ownership.** Obsidian also needs a tab mode that doesn't own its file: `SetText` in, and the saved text sent back as an event instead of written, since the open note is the source of truth. This is a `ui`/`notebook` change and conflicts with nothing open on `server`.

## Open questions and uncertainty

- **`sh` fences (decided: post-processor).** A code block processor would own every `sh` fence in the vault, so we don't register one. Instead: in Reading view, `registerMarkdownPostProcessor` finds `pre > code` elements, calls `ctx.getSectionInfo(el)` for the section's source lines, and reads the fence line itself. Only fences whose info string has mdsh attributes (`{#id …}`), or that sit in a note with `mdsh:` front matter, get a run button and output styling, and everything else keeps Obsidian's normal rendering. Reading the raw line means we don't depend on whether Obsidian keeps `{…}` in the HTML (probably not: the class is likely just `language-sh`). Two catches: per `obsidian.d.ts`, post-processors run in Reading view only, so Live Preview and Source mode need a CM6 extension (`registerEditorExtension`, `editorLivePreviewField` to tell the modes apart) that scans the document for the same fences and adds a run widget and output decorations; the existing `rungutter.ts` is the start of that. And `getSectionInfo` can return null (e.g. in embeds), where the plugin shows no button. Not yet tried in Obsidian.
- **Distribution (decided).** The user installs `mdsh` themselves, and the plugin only needs its path (a setting, defaulting to `mdsh` on a login-shell PATH). Every binary we ship takes `--serve=stdio`, so the plugin works with whichever one is installed.
- **Prior art.** The "Execute Code" community plugin already runs fenced blocks via `child_process` and shows output under them. It's worth reading for how it handles Live Preview and output, though it doesn't keep sessions or write output back to the file.
- I haven't tested any of this in Obsidian. The effort figures below are estimates.

## Rough effort

| Option | MVP | Notes |
| --- | --- | --- |
| D. Open in mdsh-gui | ~1 hour | Command + path setting |
| B. Shell out per block | 2–4 days | No sessions, no confirm, no tty |
| A. stdio + plugin | 1.5–2.5 weeks | `--serve=stdio` on the `serve` dispatcher plus text in/out (~2–4 days Go), plugin run/output/confirm/stop for both views (~1–1.5 weeks), tty + reports later (~1 week) |
| C. WASM | Not recommended | Can't run shells |

## Recommendation

Go with A, in steps that each land on their own:

1. After `server` merges to main: add `ui.WithTab`, make serve's connection an interface, and add the stdio transport and `--serve=stdio` to `mdsh` and `mdsh-gui`, tested from Go.
2. Add the "tab doesn't own its file" mode to `ui`/`notebook` (text in, text back).
3. Build an Obsidian plugin (a separate repo, or `obsidian/` here): post-processor in Reading view, CM6 extension in Live Preview and Source, streamed output, write-back via the Editor API, confirm, stop and Run All.
4. Later: attribute completion and lint (reusing `editor.ts`), tty blocks (reusing `terminal.ts`) and reports.

D is worth doing now as a stopgap. Skip B unless a throwaway demo is wanted first.
