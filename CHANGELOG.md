# Changelog

All notable changes to mdsh are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions
follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `mdsh` command: runs the `sh` blocks in a markdown file top to bottom and
  prints the markdown with each block's output below it. `mdsh file.md`
  is short for `mdsh run file.md`, so `#!/usr/bin/env mdsh` makes a
  markdown file executable; extra arguments reach blocks as `$1`, `$2`, …
- `-w` writes output back into the file; `-o out.md` writes it to another
  file instead. Writing back twice gives the same file.
- `--hide-code`, `--hide-output` (both: prose only, nothing runs),
  `--only BLOCK`, `--stop-on-error`, `--yes`, `-` to read from stdin.
- Styled terminal output with `--color=auto|always|never`, `--no-color`
  and `NO_COLOR`.
- Front matter options under `mdsh:`: `shell`, `on_error`, `interpolate`,
  `show_stderr` and `env.share`.
- Block attributes: `#id`, `skip`, `env=` (stacked sub-envs), `share=`,
  `on_error=`, `timeout=`, `confirm`, `run=`, `show=`, `show-output=`,
  `show-stderr=`, `out=markdown|image|screenshot` and `embed=inline`.
- Blocks without `env=` share one long-lived shell; sub-envs only receive
  the exported variables listed in `share=`.
- Placeholders in prose: `{{#block/env(VAR)}}` and `{{$VAR}}`, with the
  last value stored in the file on write-back.
- Block facts: `{{$?}}` (alias `{{$err?}}`) for the last block's exit
  status, `{{$ran?}}` for whether it ran, and `{{#block/?}}`,
  `{{#block/err?}}` and `{{#block/ran?}}` for a named block, in
  conditions and prose.
- Conditions (`{{$VAR}}`, `{{#block/env(VAR)}}`, `!`, `==`, `!=`) for
  `run=`, `show=`, `show-output=`, `show-stderr=` and
  `<!-- mdsh:if COND -->` … `<!-- mdsh:end -->` sections.
- `out=image` saves image output; `out=screenshot` draws text output as a
  terminal-style SVG or PNG.
- Shell helpers for markdown output: `md_h1`–`md_h3`, `md_table`, `md_code`.
- `mdsh completion bash|zsh`.
- Fences inside list items and block quotes run like any other block;
  their output is written inside the same list item or quote.
- Design spec in `docs/spec.md`, roadmap in `docs/plan.md`, and a
  literate example in `examples/hello.md`.
