# Long output in a file, and windows into files (`out=file`, `match=`, `symbol=`)

Design. Nothing is built yet.

## The ask

A block that prints a few thousand lines, a build log or a test run,
swamps the document. Folding (`fold_output`, 25 lines by default) keeps
it out of the way in mdsh-gui, but the lines are still in the file, the
diff on every run is the whole log, and the reader can't be pointed at
the part that matters.

Two things, which fit together:

1. Send a block's long output to a file in the document's assets
   folder, and show it in the document through an include, so the
   document holds a window into the log rather than the log.
2. Let an include pick its window by content, not by line number: the
   first line matching a regexp, or a named symbol in a source file, so
   the window stays on the right lines when the file changes above it.

Decided in the thread (2026-11-23): `match=` and `symbol=` are for
include blocks only. A block's own output stays what the block printed;
when the output is in a file, the output fence *is* an include of that
file, so the windowing comes for free and no second mechanism is needed.

## How it works today (what the design has to fit)

- An include block, `` ```go {<include file=path from=10 to=24 number_on} ``,
  holds lines of another file. `doc.Include` reads `file=`, `from=`,
  `to=` and `number_on` (`doc/include.go`); `run.Lines` cuts the lines
  (`run/include.go`); `render.writeInclude` writes them as the block's
  body, numbered from `from=` when printing with `number_on`
  (`render/render.go`). The GUI cell carries `FirstLine` so its own
  numbering starts at the right line (`notebook/notebook.go`). An
  include never runs and has no name, and `doc.notForIncludes` lists the
  attributes it can't have, `out=`, `collapse-output` and `history`
  among them.
- Includes are filled in document order. `run.Includes` fills every
  include in one pass with no block run; a `Session` fills an include
  when the run reaches it (`run/session.go`, the `IsInclude` case).
- `file=res://name` is `<doc>.assets/name`. The file must be a regular
  file inside the document's root, not hidden, at most
  `docroot.MaxRead` (16 MB), unless `--allow-outside` or the GUI's
  per-document allowance names it. `run.Outside` lists what a document
  reaches outside, for that dialog.
- `out=` picks what a block's output is: text (the default), `markdown`,
  `image[:path]`, `screenshot[:path]`, `recording[:path]` for `tty`
  blocks (`run.outKind`). Files land at `<doc>.assets/<id>.<ext>` or
  the path given. `run.ownAsset` says which files mdsh owns, so
  `history=N` can rename and trim them and nothing else is ever moved.
- A block's output is the `output {for=id exit=N}` fence right under it
  (`Block.Output`), plus at most one `stream=stderr` fence. Write-back
  replaces the whole span. An `output {for=id}` fence anywhere else is
  a copy, refilled on every run (`doc.Copy`). `nosave` on an output
  fence survives write-back; nothing else on it is the author's.
- `fold_output` folds a block's *output* past N lines in mdsh-gui and
  `--serve`; unfolding by hand writes `collapse-output=false` on the
  block. Include blocks are skipped on purpose (`notebook/fold.go`):
  an include's body is code, and `collapse` folds code.
- Renaming a block rewrites `for=` in its output and copies and
  `.assets/<old>.` links in them (`doc/name.go`). `res://<old>.` is
  not rewritten today.
- Attribute values can be quoted, `confirm="Deploy to prod?"`, with
  `\"` for a quote inside (`doc.splitAttrs`, `doc.unquote`).

## Proposal

### 1. `out=file[:path]`: the output goes to a file

```sh {#build out=file}
make 2>&1
```

`out=file` writes the block's stdout to `<doc>.assets/<id>.log`, or to
the path given, `out=file:res://build.log` or `out=file:logs/build.txt`,
under the same root rules as every `out=` path. The extension is the
author's; `.log` is the default. It is a new kind in `run.outKind`,
beside `image`, `screenshot` and `recording`, and `ownAsset`'s rules
cover it as they are, so `history=N` renames and trims the file as it
does an image.

The output fence under the block becomes an include of that file:

````markdown
```output {for=build exit=0 <include file=res://build.log}
… the first 25 lines …
```
````

mdsh writes `for=`, `exit=`, `at=` and `<include file=` on every run.
The **window attributes** of section 2 (`from`, `to`, `match`, `before`,
`lines`, `until`, `number_on`) are the author's: mdsh keeps whatever
the fence had when it rewrites it, the way it keeps `nosave`. So the
reader points the block's own output at the interesting part by editing
the fence:

````markdown
```output {for=build exit=2 <include file=res://build.log match="^FAIL" before=3 lines=20}
```
````

With no window attributes the fence shows the file's first `fold_output`
lines (25 by default; the whole file when `fold_output: 0`), so a short
run reads as it does today and a long one is already a window. The
fence's body is written on every run, like an include's, so it reads on
GitHub, and `number_on` numbers from the window's first line.

Exit code, `exited`, stderr and `at=` are unchanged: stderr stays in its
own fence in the document, since it is usually short and is what a
failure shows. Only stdout goes to the file.

While the block runs, mdsh-gui streams the output into the card as it
does now. When the block ends mdsh writes the file, computes the
window, and the card shows the window with a line in its output bar,
"4,210 lines in build.log", that opens the file. `mdsh` printing a
document, `--html` and `--pdf` show the window and link the file.
`show-output=COND` applies to the window as to any output.

Copies elsewhere, `output {for=build}`, are refilled with the same
`<include file=` pointer, and their own window attributes survive the
same way. A plain `<include file=res://build.log …>` block anywhere in
the document works too; the only difference is that a copy follows the
block's `exit=` and a rename.

#### Writing the file

- The file is written only when its bytes changed, so an unchanged run
  leaves no diff and no mtime churn.
- The file is held to `docroot.MaxRead`. Output past that is dropped
  and the fence gets `truncated`, as the fence of a text output that hit
  a cap would. Better than failing the run after a long build.
- `nosave` with `out=file` writes the file under the session's
  temporary directory, not the assets folder, and the fence isn't
  saved, as `nosave` already means; the window shows in the GUI only.
- `secret` with `out=file` is refused at parse time: the file would sit
  on disk unmasked. The error names both attributes.
- Renaming the block renames the file as it renames other assets, and
  rewrites `file=res://<old>.` in the block's own output fence and its
  copies. Include blocks elsewhere that name the file are the author's
  and are left alone, with a warning naming them.

#### Not in this design

- Spilling automatically past N lines (a `spill_output:` front matter
  key). It would change what `mdsh -w` writes to disk for documents
  that never asked for it. Easy to add later on top of `out=file` if
  the explicit attribute proves too much typing.
- Cleaning up files of blocks that no longer exist. Images and casts
  have the same gap today; a `mdsh --prune-assets` that removes files
  `ownAsset` would claim for ids not in the document is its own small
  piece of work.

### 2. Windows by content: `match=`, `before=`, `lines=`, `until=`

New include attributes, on include blocks and on the output fences of
section 1:

| Attribute | Meaning |
| --- | --- |
| `match="re"` | The window starts at the first line matching `re` (Go regexp syntax, RE2, no lookaround). Quote it as any attribute; `\"` for a quote inside |
| `before=N` | Also show the N lines above the start, for context above an error. Clipped at the top of the file |
| `lines=N` | Show N lines from the start line, that line included. Clipped at the end of the file |
| `until="re"` | End the window at the first line matching `re` at or after the start line, that line included |
| `symbol=Name` | Section 3: the window is the definition of `Name` |

`from=` and `to=` keep their meaning, absolute line numbers, and can't
be combined with `match=` or `symbol=`: "first match plus `to` minus
`from` lines" reads wrong next to an include without `match=`, so the
length has its own name. `lines=` and `until=` can't be combined
either; `before=` goes with any start. With `match=` and no `lines=`
or `until=` the window runs to the end of the file, as `from=` without
`to=` does.

`number_on` numbers from the window's first line, so an error at line
3,812 of the log is numbered 3,812. `FirstLine` on the GUI cell comes
from the window, not from `from=`.

**No match.** The window is empty: mdsh warns on stderr,
`include: no line of build.log matches "^FAIL"`, writes an empty body,
and mdsh-gui badges the card as it badges a copy for a block that
doesn't exist. The run is not failed. A log that has no errors this
time is the normal case for a window on errors, and the empty fence
on GitHub says so.

**Several matches.** The first wins. `nth=N` for the Nth is an easy
addition when someone wants it; "every match" is not an include, and is
left out.

### 3. Windows by symbol: `symbol=Name`

````markdown
```go {<include file=run/history.go symbol=ownAsset number_on}
```
````

The window is the definition of `Name` in the file, found by meaning
rather than by line, so moving the function within the file changes
only the numbers in the document. Moving it to another file, or
renaming it, is a no-match warning, which is the right signal.

Three tiers, each a fallback for the next:

1. **Go, exact**, with `go/parser` from the standard library and no
   dependency. `symbol=` names a function, a method as `T.Method` or
   `(*T).Method`, a type, a const or a var; `Pkg.Name` isn't needed, the
   file is one package. The window is the declaration with its doc
   comment, `comment=false` drops the comment. A const or var inside a
   grouped declaration is its own spec lines, plus the comment above
   it. A file that doesn't parse falls to tier 2 for that include, with
   a warning.
2. **Other languages, best effort.** Chroma is already vendored for
   highlighting, and its lexers mark definition names with
   `NameFunction` or `NameClass` for most of the languages it knows.
   The start is the first line where such a token equals `Name`. The
   end is found by brace matching from the first `{` after the start,
   or, for a small list of languages that nest by indentation (Python,
   YAML, CoffeeScript, Nim, Haskell), the last line indented deeper than
   the start line, skipping blank lines. The language is the fence's.
   This will be wrong sometimes; the doc says so, and `lines=` or
   `until=` on the same include pins the end by hand when it is.
3. **No lexer, or nothing found:** no match, as in section 2.

`before=` adds context above the definition; `lines=` and `until=`
override its end. Ambiguity: the first definition in file order wins,
with a warning when there is more than one, and `T.Method` picks a
method over a function of the same name in Go.

Nested symbols, `Server.handle` for a method in a class in Python or
JS, are left out of the first cut: tier 1 doesn't need them and tier 2
can add them by nesting depth if they are missed.

Tree-sitter would be the exact answer for every language, and it is the
upgrade path behind the same attribute if tier 2 proves too weak. It is
cgo plus a grammar per language, vendored, which is a very large
commit for what the heuristic covers.

### 4. Ordering: a window into a file written by the same run

Includes are filled in document order. An include, or a copy, that
names a file an `out=file` block writes and sits *before* that block,
or in a summary that was filled before the block ran, would show the
previous run's log. So:

- A block's own output fence is computed when the block ends, as its
  output always is.
- When a block with `out=file` ends, the session refills every include
  and copy in the document whose `file=` resolves to a file that block
  owns (`ownAsset`), and emits their `Done` events, so the GUI redraws
  them. A full run (`run.Run`, `-w`) does the same after the last
  block, as one pass over the includes.
- `run.Includes` on its own (printing with `-n`) reads whatever is on
  disk, as today.

### 5. Folding

`fold_output` keeps folding a block's output by length, and the output
of an `out=file` block is its window, so a window longer than
`fold_output` folds as any long output does, and `collapse-output`
applies. Include blocks are still not folded by length: a 60-line
window that the author asked for and then folded to one line defeats
the window. `collapse` folds an include as it folds any code.

### 6. Work, in order

1. `doc`: `match`, `before`, `lines`, `until`, `symbol`, `comment`,
   `nth` (parse only, "not yet") on `doc.Include`; the pairing rules;
   `truncated` on output fences; tests.
2. `run`: `Window(data, inc)` beside `Lines`, with regexp and the
   warning result for no match; `FirstLine` from the window; the GUI
   badge; tests, including a file with CRLF line ends.
3. `run`: tier 1 of `symbol=`, Go via `go/parser`; tests over a file
   with doc comments, methods, grouped consts.
4. `run`: `out=file` in `outKind`, writing the file when changed, the
   cap and `truncated`, `nosave` and `secret` rules; `render`: the
   output fence as an include with the author's window attributes kept
   over write-back; copies; `doc/name.go` rewriting `res://<old>.`.
5. `run.Session` and `run.Run`: refilling includes that name a file the
   block that just ended owns (section 4).
6. mdsh-gui and `--serve`: the output bar's line count and link,
   streaming then windowing, the no-match badge; `--html` and `--pdf`
   linking the file; screenshots for the changelog.
7. `run`: tier 2 of `symbol=`, Chroma tokens plus brace or indentation
   ends, with its language list; tests over Python, JS, Rust, shell.
8. README (Including files, the attribute table), `docs/cheatsheet.md`,
   `examples/include.md`, CHANGELOG.

Steps 1 to 3 are useful on their own and can ship first.

## Risks

- **The output fence is now partly the author's.** Today write-back
  owns the whole span. Keeping some attributes and replacing others is
  new ground; `nosave` is the only precedent. The rule has to be one
  list in one place (`render`), and `doc`'s parser must accept an
  output fence with `<include` without reading it as an include block
  (`IsInclude` is true for it; `Runnable` already excludes includes, so
  check `Copy` detection and `fold.go`).
- **Tier 2 is a heuristic.** It will pick the wrong end for a Python
  function followed by a decorated one, a Rust `impl` with a trailing
  `where`, a JS arrow function assigned to a `const`. Saying so in the
  doc and offering `lines=`/`until=` is the mitigation; silent wrong
  windows are the failure to watch for in review.
- **Large logs and regexps.** `match=` over a 16 MB file is a linear
  scan with RE2, fine; `until=` the same. No backtracking means no
  pathological patterns.
- **Git noise moves, it doesn't vanish.** The log file changes on every
  run whose output changed, as images do now. The document's diff
  shrinks to the window. Writing only on change keeps unchanged runs
  clean.

## Decisions (2026-11-23)

- `match=` and `symbol=` are include-only. No windowing of a block's
  own text output: output is what the block printed.
- A block with `out=file` has an output fence that is an include of the
  file, with the author's window attributes kept over write-back. That
  is the "spill" and the "window" in one mechanism.
- Window length has its own attributes, `lines=` and `until=`, rather
  than reusing `to=` minus `from=`.
- `symbol=` ships Go-exact first, Chroma heuristic second, tree-sitter
  only if the heuristic proves too weak.
- Include blocks are not folded by length.
- Automatic spilling past N lines and pruning stale asset files are
  out of scope, noted as follow-ups.
