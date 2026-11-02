# mdsh CLI v1.2.1 security review

Scope: tag `v1.2.1` (5d18124): `cmd/mdsh`, `run`, `doc`, `env`, `render`, `report`, `pdf`, `tui`, `termimg`, `tape`. GUI is reviewed separately.

Threat model: someone opens a `.md` file they didn't write. "It runs the code in it" is expected. The bugs below are things that happen **without running any block** (`-n`, `--html`, `--pdf`, `-i` before running), or that **get around the review gates** mdsh offers (`confirm`, `--step`, `env.share`).

Everything marked *Reproduced* was run against a v1.2.1 build. PoC files are in the "Repro" sections.

| # | Severity | Finding |
|---|---|---|
| 1 | High | Terminal escapes in a block body can hide code at the `confirm` / `--step` prompt |
| 2 | High | `{{$VAR}}` placeholders put any env var into output, `-w` files and reports, even with `-n`, and ignore `env.share` |
| 3 | Medium | `<include file=...>` reads any file (absolute paths, `../`, symlinks), even with `-n`, and before any confirm |
| 4 | Medium | Saved output and prose send raw escape sequences to the terminal (OSC 52 clipboard write, title, OSC 8 links), with `-n` too, and in `-i` |
| 5 | Medium | Front matter `shell:` picks the interpreter, and the confirm prompt doesn't show it |
| 6 | Low/Med | Asset writes (`out=image:PATH`, block id, `.assets` dir) can land outside the doc dir and follow symlinks |
| 7 | Low | `-n` / reports can hang or run out of memory on FIFOs and `/dev/zero` |
| 8 | Low | `--html` / `--pdf` embed any image file the doc names by absolute path |

---

## 1. Escape sequences hide code at the confirm prompt (High)

**Where:** `cmd/mdsh/main.go` `prompter.confirm` writes `b.Body` raw (`fmt.Fprintf(p.out, "...%s---\n", b.ID, saved, b.Body)`). The streamed document (`streamer`/`newPrinter`) also prints the body unfiltered, and glamour does not strip escapes. In the TUI, `tui/view.go` `clean()` keeps `0x1b` on purpose, so CSI sequences such as cursor-up and erase-line still work there.

**Impact:** `confirm` and `--step` are the way a user reviews a block before it runs. A line ending in `\r\x1b[2K` (or a cursor-up and erase) is printed and then wiped from the screen. The user approves code they could not see.

**Repro (reproduced):**
```sh
printf '```sh {confirm}\necho visible-line\ntouch /tmp/PWNED #\r\033[2K\n```\n' > hide.md
mdsh hide.md      # answer r: the screen shows only "echo visible-line", and /tmp/PWNED is created
```
I ran a cursor-up and erase variant under a pty. The raw `ESC[...` bytes reach the terminal unchanged and the hidden command runs. I didn't check how it looks in a real terminal emulator; the `\r ESC[2K` form above is the simplest version of the same trick.

**Fix:** Before showing code (prompt, stream, TUI), replace C0/C1 control characters other than `\n` and `\t` with visible escapes such as `^[` or `\x1b`. At minimum do this in the confirm path. Consider warning when a body contains control characters at all.

## 2. `{{$VAR}}` discloses the caller's environment (High)

**Where:** `run.NewLookup` resolves `{{$VAR}}` from the caller's full environment (`env.Vars(environ)`). `cmd/mdsh/main.go` passes `os.Environ()` into render and report options in **every** mode, including `-n`. With `-w`, `render` writes the value back as `{{$VAR:"value"}}`. The `-i` TUI does the same (autosave uses `Env: m.opts.Run.Environ`). `run.runTaped` expands placeholders in tapes from the same unfiltered lookup.

**Impact:** No code needs to run. `mdsh -n --html report.html notes.md` or `mdsh -w notes.md` puts `$GITHUB_TOKEN`, `$AWS_SECRET_ACCESS_KEY` and so on into a file the user is about to share or commit. The front matter `env.share` allowlist limits only what the shells see, not placeholders, so a user who relies on `share: [PATH]` is not protected. A tape can type a secret into a program under the doc's control, which also gets around `share`.

**Repro (reproduced):**
```sh
printf -- '---\nmdsh:\n  env:\n    share: [PATH]\n---\nToken: {{$SECRET_TOKEN}}\n\n```sh\necho "[$SECRET_TOKEN]"\n```\n' > s.md
SECRET_TOKEN=s3cr3t mdsh s.md         # prose prints "Token: s3cr3t"; the block prints "[]"
SECRET_TOKEN=s3cr3t mdsh -n --html - s.md | grep s3cr3t
SECRET_TOKEN=s3cr3t mdsh -w s.md && grep s3cr3t s.md   # {{$SECRET_TOKEN:"s3cr3t"}} saved
```

**Fix:** Resolve `{{$VAR}}` against the same filtered set the top-level env gets (`env.Filter(..., d.Options.Env.Share)`). Better still, make caller-env placeholders opt-in, for example only for vars listed in a new `placeholders:` / `expose:` key, or only when the user passes a flag. Do the same for tapes.

## 3. Include blocks read any file, even with `--no-run` (Medium)

**Where:** `run/include.go` `includeBlock` accepts absolute paths and joins relative ones without checking that they stay in the doc dir, and `os.ReadFile` follows symlinks. `NewSession` reads includes before any block runs or is confirmed, and `-n` reads them too ("reading a file runs nothing").

**Impact:** `-n` looks like a safe preview, but it reads `~/.ssh/id_ed25519`, `~/.aws/credentials`, `/etc/passwd` and similar files into the printed doc, the `-o` file, `-w` write-back, and `--html` / `--pdf` reports that get shared. A cloned repo can also ship a symlink that a relative include follows.

**Repro (reproduced):**
```sh
printf '```text {<include file=/etc/passwd to=3}\n```\n\n```text {<include file=../../../../etc/hostname}\n```\n' > inc.md
mdsh -n --html - inc.md | grep root:
```

**Fix:** By default, resolve the path (`filepath.EvalSymlinks`) and refuse anything outside the doc dir (or the git root). Add a flag such as `--allow-include-outside` for trusted docs. On Go 1.24+, `os.Root` / `os.OpenInRoot` handles this cleanly. Also refuse anything that is not a regular file (see #7).

## 4. Raw terminal escapes from saved output and prose (Medium)

**Where:** `render.Markdown` → `newPrinter` writes doc bytes straight to stdout, styled or not (glamour keeps them). The `-w` status path writes `render.Output` to stderr. The TUI `clean()` keeps ESC, and OSC sequences ended by `ESC \` survive it.

**Impact:** `mdsh -n evil.md` (or `-i evil.md`) can write to the clipboard with OSC 52 on terminals that allow it (kitty, WezTerm, Alacritty, iTerm2 if enabled), which sets up a paste of attacker commands. It can also set the window title, make spoofed OSC 8 hyperlinks, and move the cursor to hide text, all without running any block. The same applies to live block output, but that code is already running.

**Repro (reproduced):** a saved `output` fence containing `\033]52;c;ZXZpbAo=\007` and `\033]0;pwned\007`. Running `mdsh -n` with `--color=never` or `--color=always` emits both sequences unchanged.

**Fix:** When the destination is a terminal, strip or escape everything except SGR color codes (`ESC[...m`) in doc text and saved output. Allow SGR only in output mdsh produced itself. In the TUI, tighten `clean()` to keep only `ESC[` ... `m`.

## 5. Front matter `shell:` isn't shown at the confirm prompt (Medium)

**Where:** `doc.Options.Shell` → `run.Shell` → `env.Start` uses `exec.Command(cfg.Path)` with `Dir` set to the doc dir, so `shell: ./tool` resolves next to the doc. The syntax check (`s.path -n f`) and the `init` script both run through that binary.

**Impact:** In `--step` / `confirm` mode the user approves what looks like `sh` code, such as `echo hi`. What actually runs it is any binary the doc chose, including one shipped in the same repo, and that binary can do anything.

**Fix:** Show the shell in the first confirm prompt (and in `-i`) whenever it isn't `/bin/sh` or `zsh`. Consider allowing only names found on `PATH` or a small allowlist, and rejecting relative paths, unless a flag allows it.

## 6. Asset writes can escape the doc dir (Low/Medium)

**Where:** `run.assetPath` → `filepath.Join(opts.Dir, rel)` → `writeAsset` (`MkdirAll` + `os.WriteFile`, which follows symlinks). `rel` comes from `out=image:PATH` / `out=screenshot:PATH` / tty `out=` (it also derives `.cast`), or from the block id (`{#../../x out=screenshot}`), because ids are not validated.

**Impact:** The block code runs anyway, but the confirm prompt shows only the body, so the destination isn't reviewed. A repo can ship `doc.md.assets/x.svg -> ~/.bashrc`, and a harmless-looking `date` block with `out=screenshot` then overwrites `~/.bashrc`. `out=image` takes any output with `<svg` in its first 1 KB as an image, so the bytes written are close to arbitrary.

**Fix:** Validate block ids (`[A-Za-z0-9_.-]+`, no `..`). Refuse asset paths that resolve outside the doc dir. Open with `O_NOFOLLOW` (or `os.Root`) and write to a temp file plus rename. Show `out=` targets in the confirm prompt.

## 7. FIFOs and devices hang or exhaust memory (Low)

**Where:** `includeBlock`, `report/html.go` `images.inline`, `pdf` image loading, and `termimg.Load` all call `os.ReadFile` on any path.

**Repro (reproduced):** `<include file=fifo>` makes `mdsh -n` hang until killed. `<include file=/dev/zero>` reads until memory runs out.

**Fix:** `Lstat` first, require a regular file, and cap the size (for example 16 MB for includes and images).

## 8. Reports embed any image by absolute path (Low)

**Where:** `report/html.go` `images.inline`, and the `pdf` image loader.

**Impact:** `![](/Users/x/Desktop/Screenshot.png)` in an untrusted doc gets that image embedded in the `--html` / `--pdf` file the user then shares. Only image types are read (PNG/JPEG/GIF/WEBP, or anything with `<svg`), which limits the damage.

**Fix:** Inline only images under the doc dir (and its assets dir), using the same root check as #3.

---

## Checked and fine

- **No network surface.** v1.2.1 has no `--serve` mode or listener in the CLI or shared packages. The only `net/http` import is `DetectContentType`.
- **Temp files:** `MkdirTemp` is 0700, block and job files are 0600, the background job dir is 0700, and everything is removed on `Close`. The TUI editor temp file comes from `CreateTemp` (0600) and is removed after editing.
- **Shell quoting:** `MDSH_*` values, args and file paths go through `quote()`. `PIDVar` sanitises ids. `captureVars` names are limited by the `refBody` regex. The output marker is 96 random bits.
- **Write-back:** `writeFile` is atomic (temp file plus rename in the target dir) and keeps the mode. Renaming over a symlinked target replaces the link rather than following it.
- **HTML report:** goldmark is not in unsafe mode, so raw HTML is dropped and `javascript:` links are blanked (checked). Code and captions are HTML-escaped.
- **Background blocks:** killed by process group or tree at the end of a run (after `BackgroundWait`) or on timeout. A block that double-forks or uses `setsid` can outlive mdsh, which is expected for code that runs anyway.

## Suggested order

Fix 1 and 4 together (one sanitiser for terminal output), then 2, then 3 and 6 (one "stay inside the doc root" helper), then 5, 7 and 8.
