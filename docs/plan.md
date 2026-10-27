# mdsh plan

The roadmap for mdsh. [spec.md](spec.md) says how mdsh works; this file
says what gets built in which order and what is still open.

## Milestones

| Milestone | Scope | Status |
| --- | --- | --- |
| v1: library and CLI | Parse, run and render `sh` blocks; `mdsh run` | Merged; finishing for v1.0.0 |
| v2: interactive mode | `mdsh -i`: a TUI that runs blocks as you go | Not started |
| Custom language | A nicer block language next to `sh` | Not started, needs its own spec |
| GUI | Desktop or web front end on the same library | Not started |

## v1: library and CLI

Goal: a markdown file with `sh` blocks runs end to end from the command
line, and its output can be printed or written back into the file.

Done:

- [x] `doc`: byte-exact fence scanner, Pandoc-style attributes, YAML
      front matter under `mdsh:`, shebang line, output fences and regions.
- [x] `env`: one long-lived shell per env, stacked sub-envs, `share=`
      filtering, markdown helper functions.
- [x] `run`: run every `sh` block in order, `skip`, `on_error`,
      `timeout=`, `--only`, `confirm`, events for front ends.
- [x] `render`: printed and write-back markdown, `--hide-code`,
      `--hide-output`, prose-only mode.
- [x] CLI: `-w`, `-o`, `--color`/`--no-color`/`NO_COLOR`, `--yes`, stdin,
      `#!/usr/bin/env mdsh`, bash and zsh completions.
- [x] Images: `out=image` for image bytes, `out=screenshot` for text drawn
      as SVG or PNG, `embed=inline`.
- [x] Placeholders: `{{#block/env(VAR)}}` and `{{$VAR}}` in prose, with
      the last value stored on write-back.
- [x] Conditions: `run=`, `show=`, `show-output=`, `show-stderr=` and
      `mdsh:if` sections.
- [x] Block facts: `{{$?}}`/`{{$err?}}`, `{{$ran?}}` and their
      `{{#block/...}}` forms.
- [x] Docs: spec, README literate example, `examples/`, CHANGELOG.
- [x] CI on Linux and macOS with `go vet` and `go test -race`.

Left before tagging v1.0.0:

- [x] Merge the first PR.
- [ ] Run a few real-world files through it and fix what breaks.
- [x] Fences nested in lists or block quotes.
- [x] Release builds (goreleaser) and install instructions.

## v2: interactive mode

Goal: `mdsh -i doc.md` opens a TUI (Bubble Tea) on the same `run` engine.

- Pick or type a block; it runs in its env and its output appears under it
  and is saved to the file.
- Blocks typed at the prompt are appended to the document as `sh` blocks.
- Re-running a block replaces its output.
- `confirm` blocks ask inside the TUI instead of on `/dev/tty`.
- Inline images in terminals that support them (kitty, iTerm2, sixel).

## Custom language

Goal from the start: nicer loops, easy command execution and easy markdown
output. Starts after v1 runs real files, with its own spec covering:

- Syntax, from the sketch in [spec.md](spec.md#block-languages).
- Build our own interpreter in Go, or build on an embeddable Go language
  (Starlark, Risor) with mdsh builtins.
- How it plugs into `run` next to `sh`, and how it shares env vars.

## GUI

A desktop or web front end on the same library, consuming the same run
events as the CLI and TUI. Not designed yet.

## Open questions

- Should vars ever flow up from a sub-env (`export=`)? Dropped for v1.
- Other block languages (bash, python) through the same runner interface.
