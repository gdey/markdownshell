# Output history for a block (`history=N`)

Design. Step 1 of the work order (the `at=` stamp) is built; the rest is not yet.

## The ask

Re-running a block replaces its output. Sometimes the old output is the
point: a health check run over days, a deploy whose last five results you
want to compare, a flaky test. A block option should keep the old outputs
as a history under the newest one, newest on top.

## How output works today (what the design has to fit)

- A block's output is the `output {for=id exit=N}` fence (or the
  `<!-- mdsh:output for="id" -->` region for markdown, images and casts)
  right after it, plus at most one `output {for=id stream=stderr}` fence.
  `Block.Output` is that span (`doc/doc.go`).
- A second stdout fence right after the block is not the block's: the
  parser treats it as a copy and refills it with the current output on
  every run (`doc.Copy`, "Output copies" in `docs/spec.md`).
- Write-back replaces the whole `Output` span with the new output
  (`render/render.go`, `Pieces`). Clear output deletes the span
  (`notebook/delete.go`, `render/clear.go`). Moving or deleting a block
  carries the span with it (`doc/move.go`, `doc/delete.go`). Renaming a
  block rewrites `for=` inside the span and `<doc>.assets/<id>.*`.
- Image, screenshot and cast outputs are files at a fixed path,
  `<doc>.assets/<id>.<ext>`, overwritten on every run.
- No output carries a time. The fence has `for=`, `exit=`, `exited`,
  `stream=`, `nosave`.
- mdsh-gui shows the output in the card's output part, with fold, copy
  and trash buttons, an exit badge and a stale badge. `mdsh -i` shows
  live output while running, else "saved output:" and the saved text.
- "History" is already a feature name: the History pane (a shell whose
  commands become `#history-N` blocks). The attribute can still be
  `history=`, but the UI should say "earlier runs" to keep them apart.

## Proposal

### 1. Attribute

`history=N` on the code block keeps the N most recent earlier outputs
under the current one. Bare `history` uses the document default, from
`mdsh: history: N` in the front matter, else 10. `history=0` or no
attribute is today's behaviour.

```sh {#check history=3}
curl -sf https://example.com/health
```

### 2. Storage: in the file, as marked fences right under the current output

Older outputs stay right after the current output, newest first, each
fence marked with its place in the history and when it ran:

````markdown
```output {for=check exit=0 at=2026-11-13T19:08:00Z}
ok
```

```output {for=check exit=7 history=1 at=2026-11-12T19:02:11Z}
curl: (7) Failed to connect
```

```output {for=check exit=7 history=1 stream=stderr}
curl: (7) Failed to connect
```

```output {for=check exit=0 history=2 at=2026-11-11T18:59:40Z}
ok
```
````

- `history=K` on an output fence means "the K-th run back". An entry is
  its stdout part plus its stderr fence, if any, both with the same
  `history=K`. Markdown, image and cast entries use the region form with
  the same attributes: `<!-- mdsh:output for="check" history="1" at="…" -->`.
- `at=` is the run's start time, UTC, RFC 3339 to the second. Every
  output fence and region gets it, with or without `history`, so a file
  always says when each block last ran. It goes on the entry's first
  part: the stdout fence or region, else the stderr fence. The first `-w` after the
  upgrade touches every output fence once; after that `at=` changes
  only when the block runs. An output fence without `at=` (written by
  an older mdsh) is still read, and shows as "time unknown".
- The parser extends `Block.Output` to cover the history fences, and
  adds `Block.History []Entry` (span, exit, at, stream parts). Because
  the span grows, move, delete, clear, rename and `nosave` all carry the
  history along with no new code paths. A fence with `history=` that
  isn't right after its block's output is an error, not a copy.
- On a run of a block with `history=N`: the current entry becomes
  `history=1`, every entry's K goes up by one, entries past N are
  dropped, and the new output is written on top. Every run pushes an
  entry, even one whose output is identical to the last, and even one
  that leaves no output (nothing printed, exit 0): the gap is visible
  as an empty fence with `at=`. The record of when it ran is the point.
- Assets: an entry's image, screenshot or cast is renamed
  `<doc>.assets/<id>.<at>.<ext>` when it is pushed down (`at` with `:`
  dropped, so it is a plain file name), and its link rewritten. Files
  past the cap are deleted. The current output keeps today's path, so
  links from elsewhere keep working. An `out=image:path` or
  `out=recording:path` of the block's own choosing is rotated the same
  way next to that path. Assets are part of the first cut, not a
  follow-up.
- Copies (`output {for=check}` elsewhere) show the current output, as
  now. A copy can ask for an entry with `history=K` on the copy; that is
  a small follow-up, not part of the first cut.
- `{{#check/?}}`, `run=once`, `depends=`, stale: all read the current
  output. History never affects what runs.
- `nosave`: the file keeps what it holds, the session keeps its own
  stack. `secret` masks every entry.

Why in the file and not a sidecar: the file is the notebook, and the
whole point is reading the history where the block is, on GitHub too.
Why marked fences and not a wrapper region: a fence per entry is what
`-w` already writes, the parser already walks it, and the `for=` rename
and prefix (list item, block quote) code works unchanged. A wrapper
`<!-- mdsh:history -->` region would be one more thing to parse and
would not stop an older mdsh misreading the entries anyway.

One cost to know about: an mdsh older than this feature parses the
history fences as copies and refills them all with the current output
on `-w`. That is lossy, so the changelog should say so.

### 3. Cap

`history=N` is the cap, enforced on every write-back and on save in
mdsh-gui and `mdsh -i`. Lowering N in the fence and saving trims the
file. There is no size cap beyond N; a block whose output is huge gets
N huge outputs, which is what was asked for.

### 4. Display

mdsh-gui (card):
- The output part shows the current output exactly as today.
- Under it, a fold line: "3 earlier runs", closed by default, opening
  to the entries, newest first. Each entry has a header with its exit
  badge and `at=` as a relative time ("yesterday 19:02"), the same copy
  button as the output, and a small trash for that one entry.
- The output tools gain "Clear earlier runs" next to the trash. Both
  are rendered-view edits, so Undo puts them back.
- Rendered view and source view stay in step as they do for the output
  span: typing in the source pane re-renders the entries.

mdsh -i (TUI):
- Under the saved or live output, a dim line "3 earlier runs (H shows
  them)". `H` on the block toggles the entries under the output, each
  under a dim "run 1 back, exit 7, 2026-11-12 19:02" line. Key to be
  checked against the current key table.

Printing, reports, PDF:
- `mdsh notes.md`, `--html`, `--pdf` print the current output only.
  `--history` includes the entries, each under its `at=`. Reports made
  from mdsh-gui and the TUI get a matching checkbox.
- `-w` always writes the entries (it is the file).

### 5. Clearing

- Clear output on the block (trash, TUI `d` on the output, `--clear`)
  removes the current output and every entry. One action, one undo.
- Clear earlier runs removes the entries and keeps the current output.
- The per-entry trash removes one entry and renumbers the rest.
- Clear All Outputs removes everything, as now.
- Dropping `history=` from the fence and saving removes the entries on
  the next write-back (the cap is now 0), with a warning line in the
  GUI console and TUI footer so it isn't a surprise.

### 6. Work, in order

1. `run`: stamp results with their start time. `render`: write `at=`
   on every output fence and region. Small, on its own, and useful
   alone.
2. `doc`: parse `history=` on fences into `Block.History`, extend the
   output span, reject misplaced entries. Tests for prefix (list item,
   quote), rename, move, delete.
3. `render`: rotate on write-back, enforce the cap, trim on cap change,
   rotate asset files for image, screenshot and cast outputs. `Clear`
   already covers the span.
4. mdsh-gui: card fold, per-entry copy and trash, Clear earlier runs,
   console warning. Screenshots for the PR and changelog.
5. `mdsh -i`: the fold line and `H`.
6. `--history` for printing and reports.

## Decisions (2026-11-13)

- Always push an entry, no dedupe.
- `at=` on every output, history or not.
- Bare `history` is 10, overridable by `mdsh: history: N`.
- Clear output wipes current and history; "Clear earlier runs" keeps
  the current output. To be revisited after use.
- Assets rotate in the first cut.
- An older mdsh overwriting history fences on `-w` is accepted; the
  changelog says so.
