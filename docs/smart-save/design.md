# Smart saves, and `.mdsh` bundles with their own history

Design. Nothing is built yet. Exploratory: it compares the ways to do
each part and recommends one, and ends with the choices that are j's.

## The ask

Two things, which turn out to be one:

1. **Know when to save.** mdsh-gui holds what you type in memory and
   writes the file on cmd+S, when you close the tab, or, with Autosave
   on, after a block runs. Nothing is written while you type, so a
   crash loses the typing, and the moments that *are* written are the
   ones a run picked, not the ones that read well in `git diff`. j
   asked how hard it would be to have a small local model whose only
   job is to say "now is a good time to save".
2. **A `.mdsh` bundle with version control built in.** A document, its
   assets and its history in one thing that opens as one file. Start on
   git, then maybe something better: a store that knows what a block,
   a run and an edit are, in a SQLite database made for this app. With
   "a bit of intelligence" that writes what each change was and knows
   what is worth keeping and what is noise.

They are one thing because "a good time to save" and "a good place to
cut a version" are the same judgement, and the second is where a model
earns its keep. The first is mostly mechanics. This doc argues that and
says what to build in which order.

## How it works today (what the design has to fit)

**Where changes come from.** The notebook (`notebook.Notebook`) holds
the document and changes it from several places:

- Typing in the source pane. CodeMirror sends line edits after a
  typing pause (`App.Edit`, `notebook.Edit`, `web/src/app.ts`
  `typingPause()`), or the whole text (`App.SetText`). An edit that
  doesn't parse is refused and the document is left as it was
  (`doc.SetSource`), so the notebook never holds an unclosed fence
  from the pane; the pane does, until it closes.
- Rendered-view edits: checking a task, typing in a `textbox` (saved
  to the notebook 500 ms after the last key, `app.ts`), picking a
  `choice`, folding, dragging a block, deleting one, editing
  `depends=`. Each is one `notebook.change` that Undo takes back, with
  typing in one box coalesced into one.
- A run ending: the block's output is written into the source
  (`Notebook.fold`), unless the block is `nosave`.
- The History pane: a typed command becomes a `#history-N` block.
- Neovim, with Use Neovim on, which writes the file itself.

**Where saves come from.** `Notebook.Save` writes `d.Src` to the file
(`notebook.write`), renames blocks that need an id, tidies history
assets, clears `dirty` and takes the file's stamp. It is called by
cmd+S and Vim's `:w` (`App.Save`), by the close prompt ("Unsaved
output", `docTab.askToSave`), and by `afterRun` when Autosave is on,
and then only if the file didn't change on disk meanwhile (`Changed`).
`dirty` is "the document differs from what was last saved", whatever
changed it; its name predates typing in the pane. The TUI is the same
shape: `w`/ctrl+s, or `AutoSave` after each block.

**What a save must not do**, whoever decides it: write a `nosave`
block's output or a `nosave` input's value, write a `secret` block's
output unmasked, write over a file that changed on disk without asking
(the banner), save while a run is writing output (`ErrBusy`), or save
a document that doesn't parse (it can't: the notebook never holds one).

**Assets.** `<doc>.assets/` holds images, casts and, with
[spilled output](../spilled-output/design.md), logs. `run.ownAsset`
says which files are mdsh's. A document is already two things on disk.

**The assistant.** [diagnose.md](../diagnose.md) designs, not yet
built, one `diag.Backend` interface with two backends: `anthropic` and
`openai`, the second being any Chat Completions server, Ollama,
llama.cpp's `llama-server`, LM Studio, `kronk server`. It settles
where a token lives, that a document can't pick a backend, that the
user previews what goes to a remote model, and that "mdsh sends
nothing anywhere" stays true until the user turns a backend on. Any
model this design uses goes through that interface; this doc adds no
second way to reach one.

## Part 1: when to save

### Taking the question apart

"Is this a good time to save?" is three questions with three kinds of
answer:

| Question | Kind | Who can answer it |
| --- | --- | --- |
| Is the document whole? Fences closed, front matter parses, attribute quotes balanced | Syntax | `doc.Parse`, already run on every edit |
| Is the author between things? Hands off the keys, cursor left the block, pane lost focus, a run is about to start, the tab is closing | Mechanics | Timers and events the GUI already has |
| Is this a moment worth keeping? Would someone reading the history want a line here | Judgement | A person, or a model with the diff in front of it |

The first two decide whether a *save* is safe and unobtrusive. The
third decides where a *version* starts and ends, and what to call it.
A save that is safe and unobtrusive is good enough for a file on disk:
the file is a cache of the document, and nothing reads the file's
history except git, which has its own idea of a version (a commit)
that the user makes on purpose. The judgement question only matters
once mdsh keeps a history itself, which is Part 2. So Part 1 can be
mechanics, and the model moves to where there is time to think.

### Option A: heuristics

A `save.Judge` fed what the editor knows:

```go
// Signal is what the editor knows when a save is considered.
type Signal struct {
	Event       Event         // Typed, LeftBlock, FocusLost, BeforeRun, RunEnded, Closing, Tick
	Idle        time.Duration // since the last keystroke
	InFence     bool          // the cursor is inside a code fence
	FenceClosed bool          // and that fence has its closing line
	Running     bool          // a block is running (saving would wait anyway)
	Changed     bool          // the file changed on disk since it was read
	Dirty       bool
}

// Judge says whether to save now, and why, for the status line.
type Judge interface {
	Save(Signal) (ok bool, why string)
}
```

The rules, in order; the first that applies wins:

1. Never while `Running`, when `Changed` (the banner asks), or when not
   `Dirty`.
2. Always on `Closing`, `BeforeRun` and `RunEnded`, as Autosave does
   now. Before a run the file on disk is what the run reads for
   includes, so it should be current.
3. On `FocusLost` (the window or the pane), and on `LeftBlock` (the
   cursor moved out of the fence it was editing): save if `Idle` is
   past a short pause, 2 s.
4. On `Tick`, every few seconds: save if `Idle` is past the long pause,
   5 s by default, and the cursor isn't inside an open fence.
5. Inside an open fence, or with the pane's text not yet applied to
   the notebook: don't. The pane applies text after `typingPause()`,
   so this is the one case the notebook's own check doesn't cover.

Cost: a few hundred lines of Go in `notebook` or a small `save`
package, a setting for the two pauses, tests over a scripted sequence
of signals. No dependency. The TUI gets the same judge with fewer
signals.

What it brings: no lost typing after a crash, a file that is current
when a run reads it, and saves that land at pauses rather than
mid-word. What it can't do: tell a pause for thought from a pause
because you're done, or say what changed. Nothing at save time needs
either.

### Option B: a local model, in process, through yzma

j's preference, with Kronk below. A small model loaded by mdsh-gui
itself and asked, with the recent edits and the cursor's surroundings,
"save now?".

**Runtime.** llama.cpp is the one engine that runs every small model
on every platform mdsh ships on. [yzma](https://github.com/hybridgroup/yzma)
(Hybrid Group, Apache 2.0) is Go bindings to it over `purego` and
`ffi`: "CGo is not needed", a plain `go build`, which keeps the
`CGO_ENABLED=0` server build and the Windows CLI build as they are.
The Go side is a vendorable Go module. The C++ side is llama.cpp's
prebuilt shared libraries, loaded at run time with `llama.Load(path)`,
fetched per platform by the `yzma` tool or by the application itself
at first use, each file checked against the release's SHA-256, with a
tagged yzma pinned to a llama.cpp release. Linux amd64 and arm64 with
CUDA, Vulkan, HIP, ROCm and SYCL; macOS arm64 with Metal; Windows
amd64. It wraps "over 96%" of llama.cpp, including embeddings and the
grammar sampler, which is how a model is held to answering `yes` or
`no` and nothing else. It is young (a few hundred stars, a steady
re-pin to each llama.cpp release) and its API tracks llama.cpp rather
than promising stability, so the version pairing is ours to manage.

Compared with the cgo bindings (`goccy/go-llama`,
`dianlight/gollama.cpp`, the `go-skynet` forks), yzma is the only one
that fits this repo: nothing C in `vendor/`, no C++ toolchain in `go
build`, and the native code arrives the way a model does, as a
checked download the user agreed to.

**Models.** What fits "small, local, a yes/no", all GGUF:

| Model | Weights (Q4/Q8) | RAM resident | Yes/no on a 1k-token prompt, M-series / x86 laptop | Fit |
| --- | --- | --- | --- | --- |
| Gemma 3 270M | ~170 MB / ~300 MB | ~0.4 GB | ~0.1–0.3 s / ~0.5–1.5 s | Made for fine-tuned classification; weak at following a judgement prompt as shipped |
| SmolLM2 360M | ~230 MB / ~400 MB | ~0.5 GB | similar | Same |
| Qwen3 0.6B | ~400 MB / ~650 MB | ~0.9 GB | ~0.3–0.6 s / ~1–3 s | Follows a prompt; Kronk's manual names it for small or CPU-only machines; still shaky on "is this a pause" |
| Gemma 3 1B, Llama 3.2 1B | ~700 MB / ~1.1 GB | ~1.5 GB | ~0.5–1 s / ~2–5 s | Good enough to summarise a diff in one line |
| 3–4B (Qwen3 4B, Gemma 3 4B, Phi-4 mini) | 2.5–3 GB | 3–5 GB | 1–3 s / 5–15 s | Writes a good change line; reasons about what is noise |

The timings are my estimates from running these classes of model, not
measurements in mdsh; they are the right order of magnitude for a
design and would be measured before any of this ships.

**What it brings** to the save question: a judgement call on the
author's state from the text of the edits, with no server to run.
**What it costs**: two downloads on first use, the llama.cpp libraries
(tens of MB) and a model (200 MB to 1 GB), neither of which can be in
the repo or the app; half a gigabyte or more of RAM while the model is
resident, or a load of a second or two per question; and a second
thing to configure beside the assistant, unless it *is* the assistant
(option B2). And the answer isn't good yet: a model under 1B
parameters, as shipped, is not a reliable judge of a question this
fuzzy, and fine-tuning one for it needs labelled examples of good and
bad save moments that don't exist until the heuristics have run for a
while and logged them. For a yes/no that heuristics get right nearly
every time, it is a lot of machine for the last few percent, today.

### Option B2: Kronk, as the SDK in process or as a server

[Kronk](https://github.com/ardanlabs/kronk) (Ardan Labs) is "a Go SDK
and model server for hardware-accelerated local inference", on
llama.cpp through yzma, and j's preference for how a model reaches
mdsh. It is two things, and both fit:

- **`kronk server`**, on `127.0.0.1:11435`, with OpenAI-compatible Chat
  Completions, Responses and embeddings endpoints and an
  Anthropic-compatible Messages endpoint. diagnose.md already names it
  as the first `openai` backend to try. The user installs Kronk
  (`brew install ardanlabs/kronk/kronk`), pulls a model from its
  catalog (`kronk model pull …`), and mdsh talks HTTP to localhost:
  nothing new in the binary, no download mdsh is responsible for, the
  RAM is the server's, and the model is shared with every other tool
  on the machine that speaks the same API.
- **The SDK**, `github.com/ardanlabs/kronk`, embedded in mdsh-gui: the
  same yzma underneath, plus what option B would have to build, the
  library install (`kronk libs`), the model catalog and pull, and a
  typed API for text, streaming, tool calls, embeddings, reranking and
  "typed Decision questions" (`/v1/decide` on the server), which is
  exactly the shape of "save now?": a question, a fixed set of
  answers, one of them back. Vendored, this is a `local` backend of
  the assistant beside `openai` and `anthropic`, with no server for
  the user to run. Its vendored size and its license need checking
  before that commit (the README pages read state neither; yzma's is
  Apache 2.0).

So the ladder is: `kronk server` over the `openai` backend first,
since diagnose.md gets it for free; the SDK as an in-process `local`
backend second, when mdsh-gui should work with no server running; raw
yzma only if the SDK gets in the way. In every rung the model is
optional: a user who installs nothing gets every feature of Part 1 and
the template lines of Part 2.

### Option B3: Apple's on-device model

macOS 26 on Apple silicon ships a ~3B model behind the Foundation
Models framework: no download, no RAM beyond what the system already
spends, guided generation into a typed struct (a `@Generable` Swift
type), a 4096-token context. mdsh-gui already has a little
Darwin-only code (`cmd/mdsh-gui/spell_darwin.go`, `fonts_darwin.go`);
this would be a small Swift helper or dylib behind an `apple` backend,
local, no token. It is the best fit for a Mac user who installed
nothing, and nothing at all for Linux, Windows, `--serve` and the CLI.
With Kronk preferred, this is a later backend for the no-install Mac
case, not part of this design's path.

### Option C: hybrid, which is the recommendation

Heuristics decide **when to save**, the file on disk. A model, when one
is configured, decides **what to say about a version** and helps
decide **what is noise**, in Part 2, off the save path: it runs after
the save, can take seconds, and if it is slow, absent or wrong the
version still exists with a template description. The model never
blocks a save and never stops one.

This puts the model where its two real strengths are, reading a diff
and writing a line about it, and keeps it out of the one place where
latency and a wrong answer would be felt on every keystroke. The model
is Kronk's, over the `openai` backend or the embedded SDK; when the
judge is later given a model to consult (decision 2), Kronk's Decision
question is the call to make, with a short timeout and the rules as
the fallback.

### Option D: a remote model

Through the `anthropic` backend, the same role as C's model, with a
better answer. The problem is consent: diagnose.md shows the user what
goes to a remote model before it goes, every time, and a preview on
every version is unusable. So a remote model can write version
descriptions only under a setting that says so in words ("send changed
lines to Anthropic to describe versions"), with the same redaction the
diagnose bundle gets (`secret` blocks, env values, the user's
patterns), and never for a document outside a trusted folder. Whether
that setting should exist at all is j's call (decision 3).

### Learning the thresholds later

The pauses in option A are guesses. The honest way to a "smart" judge
without a language model is to log, per save, the signal that fired
and whether the user hit Undo or edited the same line within a few
seconds, and tune the pauses from that; a weighted sum of the signals
fitted to that log is a classifier, and it is 2 KB. That is a
follow-up once there is data, not a first step.

## Part 2: the `.mdsh` bundle

### What a bundle is

A **`.mdsh` bundle** is a folder with the `.mdsh` extension:

```
deploy.mdsh/
  document.md        the document as today, the assets linked by relative path
  assets/            what <doc>.assets/ holds now
  history/           Part 2's history: a bare git repository, later a database
  bundle.json        {"format": 1, "mdsh": "1.4.0"}
```

A folder, not an archive or a database file, because:

- `document.md` is still a markdown file. `cat`, `grep`, an editor,
  GitHub's renderer when the bundle is in a repo, and mdsh v1.x all
  read it. Nothing about the document is locked up.
- macOS shows a folder with a registered extension as one file (a
  package, `LSTypeIsPackage` in the app's document types), which is
  the "opens as one thing" j asked for; Linux and Windows file
  managers show a folder, and `mdsh deploy.mdsh` opens it either way.
- A zip has to be rewritten whole on every save, and a database file
  hides the markdown.

The loose form, `deploy.md` + `deploy.md.assets/`, stays and stays the
default for a new file: it is what the README teaches and what a repo
of runbooks already holds. `mdsh bundle deploy.md` makes a bundle;
`mdsh unbundle deploy.mdsh` makes the loose form; Save As offers both.
A bundle is the loose form plus `history/` and a manifest, so there is
one document model, one `notebook`, and the bundle is a question of
paths (`docroot` already knows the document's root and assets folder).

### History, phase 1: git

A bare git repository at `history/`, written by mdsh on every version
cut, never by the user, with one commit per version and the version's
description as the message. `document.md` and `assets/` are its
working tree, so `git --git-dir=deploy.mdsh/history log -p` works for
anyone who wants it, and the UI's history pane is `git log`, `git
show` and `git checkout -- document.md` with buttons.

Two ways to drive git from Go:

| | Shell out to `git` | `go-git` (pure Go) |
| --- | --- | --- |
| Dependency | None in the binary; needs git installed. On a Mac without the Command Line Tools, running `git` pops Apple's install dialog, which mdsh-gui must not do on a save | Vendored, pure Go, a few MB of `vendor/`; its own commit, per the project's rules |
| `CGO_ENABLED=0` | Fine | Fine |
| Speed on a 1 MB document | A process per commit, ~20–50 ms | In process, ~5–10 ms |
| Risk | Version drift, the dialog | Edge cases of a reimplementation; fine for a repo only mdsh writes |

`go-git`, for the dialog alone. The repository is mdsh's; it needs
add, commit, log, show, and checkout of one file, all of which go-git
has had for years.

**Not `.git/`.** The folder is `history/`, a bare repository, so a
bundle committed to the user's own repository is plain files to the
outer git, not a nested repository or a submodule. The outer repo will
see `history/objects/…` change on every version, which is churn; the
README suggests `*.mdsh/history/` in `.gitignore` for bundles kept in
a repo, with the trade that the history then lives on one machine.
Where the history should live when the document is in a repository is
decision 6.

### History, phase 2: a store that knows what a block is

Git stores snapshots of text and knows nothing else. Every question a
runbook's author actually asks is about blocks and runs:

- Every run of `#deploy`, with its exit code, how long it took, and
  its output, across months. `history=N` keeps the last N in the file;
  this is all of them, out of the file.
- What changed in `#build`'s code between the run that passed and the
  one that failed.
- Which versions were typing, which were runs, which were a task being
  checked, which were the History pane.
- The output of a block that is `nosave` or past the size cap: kept
  here, out of the document, or not kept at all, by rule.

`doc.SetSource` already keeps a block's identity across edits and
moves, so the notebook knows, for every change, which block it was
and what kind of change. A store that records that is the "not as
context agnostic as git" j asked for. A sketch, in SQLite:

```sql
CREATE TABLE version (
  id INTEGER PRIMARY KEY, at TEXT NOT NULL, parent INTEGER,
  kind TEXT NOT NULL,          -- typed | run | input | move | history | import | manual
  summary TEXT NOT NULL,       -- the template line, always present
  described TEXT,              -- the model's line, when one wrote it
  described_by TEXT,           -- backend/model, or NULL
  source BLOB NOT NULL         -- sha256 of the document's text, in blob
);
CREATE TABLE blob (hash BLOB PRIMARY KEY, size INTEGER, data BLOB);   -- deflate, content addressed
CREATE TABLE block_at (version INTEGER, block TEXT, code BLOB, attrs TEXT, PRIMARY KEY (version, block));
CREATE TABLE run (
  id INTEGER PRIMARY KEY, version INTEGER, block TEXT, started TEXT, ended TEXT,
  exit INTEGER, stdout BLOB, stderr BLOB, truncated INTEGER        -- hashes into blob, or NULL
);
CREATE TABLE edit (version INTEGER, block TEXT, kind TEXT, first INTEGER, last INTEGER);
CREATE VIRTUAL TABLE text USING fts5(summary, described, stdout, content='');
```

Snapshots are content addressed so a version that changed one line
costs the whole text once, deflated, which for a 100 KB document and
a thousand versions is tens of MB, fine; a delta scheme is an
optimisation for later. Outputs are hashed, so a hundred runs with
the same output store it once, and an output over a cap stores its
hash, its first lines and `truncated`.

**Engine.** The schema is the design; the engine is a choice:

| Engine | cgo | `vendor/` | Note |
| --- | --- | --- | --- |
| `modernc.org/sqlite` | No | Large, tens of MB of transpiled C, slow first build | The usual pure-Go answer; `zombiezen.com/go/sqlite` is a nicer API on top of it |
| `mattn/go-sqlite3` | Yes | Small | Breaks the `CGO_ENABLED=0` server build and the Windows CLI build |
| None: JSON Lines log + `blob/` folder | No | Nothing | Zero dependency, `jq`-readable; every query is a scan; FTS is `grep` |

The honest middle: a `history.Store` interface with the queries above,
a first implementation on the JSON Lines log (the versions, runs and
edits of one document are thousands of rows, not millions), and
`modernc.org/sqlite` behind the same interface when a query needs an
index or search. j wants SQLite, and it is the right end state for
full-text search over months of output; whether to start there is
decision 5.

**Phase 1 and 2 together.** The git repository in phase 1 is the
`source` snapshots of phase 2 with a format other tools read. The
simplest sequence is git first, then the store beside it, then the
store alone with an `mdsh history export --git` for anyone who wants
a repo. Or skip git: the store's snapshots can always be exported as
commits, and a store that is one file is simpler to carry than a bare
repository. Decision 5 again.

### Where the intelligence goes

Three jobs, each with a rule first and a model second:

**1. Where a version starts and ends.** Rules: a run ending is a
version; a `BeforeRun` save is one if anything changed since the last;
typing is one version until the author has been idle for a long pause,
5 minutes, or runs something; an input (task, textbox, choice) is its
own version, coalesced like Undo coalesces; Closing cuts one. The
judge's `why` is the version's `kind`. A model adds nothing here that
the user wouldn't rather set.

**2. What the version was.** The *template* line is computed from the
structured diff and is always there and always true: "Ran #deploy,
exit 0, 3.2 s", "Edited #build (4 lines), ran it, exit 2", "Checked 2
tasks under Rollout", "Typed 31 lines in Why we do this". The *model*
line, when a backend is configured, is one sentence of intent from the
template line, the changed lines and the heading above them, with the
redaction rules of diagnose.md and the local-only default: "Switch the
build to the release target and check it still links". It is stored
beside the template line, labelled with the backend and model, and
shown in a lighter face. Asked off the save path, after the version
is cut; a failure or a timeout leaves the template line. `Describer`
is one method on top of `diag.Backend`:

```go
// Describer writes one line about a version, from its template line,
// the changed lines with the heading above each, and the runs in it.
type Describer interface {
	Describe(ctx context.Context, v Delta) (string, error)
}
```

**3. What to track.** Rules: track the document's text, every block's
code and attributes, every run's exit, duration and output up to a
cap; don't track `nosave` output or input values, or `secret` output
(the document can't hold it unmasked and the history mustn't either);
keep an asset file by hash, once; mark as **noise** the output of a
block whose code and exit didn't change but whose output did on every
run, and keep only its latest, unless the block says `history`. A
model can propose a block is noise sooner, from what the output looks
like (timestamps, progress bars, a `date` line), as a suggestion the
user accepts once per block, which writes `track=false` or
`track=output` on the fence so the decision is in the file and
reviewable, not in the model. These are the rules I'd start with;
decision 7 asks whether they are the right ones.

### What the user sees

- **mdsh-gui.** A **History** entry in View, a drawer beside the
  outline listing versions newest first: the time, the kind's icon, the
  line (the model's when there is one, the template's under it), the
  blocks touched. Clicking shows the diff in the source pane's look;
  Restore puts that version's text in the document as one Undo-able
  edit. A run's row opens its output. Search over lines and output.
  The status bar's "saved" becomes "saved · v128" and the Autosave
  setting becomes a Save section: pauses, versions on/off, describe
  with the assistant on/off.
- **TUI.** `H` lists versions in the preview pane; `enter` shows the
  diff, `r` restores.
- **CLI.** `mdsh history doc.mdsh` lists, `mdsh history --show N`
  prints a diff or a run's output, `mdsh history --restore N`,
  `mdsh history export --git DIR`. `mdsh -w` on a bundle cuts a
  version per run as the GUI does.

## Work, in order

1. **Save judge** (option A): `save.Judge` with the rules above, the
   two pauses as settings, wired to `Edit`, `SetText`, focus and tick
   events in `ui`, and to the TUI. Tests over signal sequences. This is
   useful on its own and ships first; it changes nothing on disk but
   when it is written.
2. **Bundle paths**: `docroot` and `notebook` open a `.mdsh` folder as
   a document whose assets folder is `assets/`; `mdsh bundle` and
   `unbundle`; Save As to a bundle; the macOS document type; `--serve`
   and `--html` on a bundle. No history yet.
3. **Versions**: `history.Store` interface; the version rules of job 1;
   the template lines of job 2; the tracking rules of job 3 (`track=`
   on the fence, `doc.attrspec`). First implementation per decision 5.
4. **History UI**: the drawer, the TUI pane, `mdsh history`. Restore as
   an Undo-able edit. Screenshots for the changelog.
5. **Describer**: on top of the assistant's `diag.Backend` once that is
   built, `kronk server` over `openai` first; the setting; the
   lighter-face line. The Kronk SDK as a `local` backend, in its own
   vendor commit, per decision 8.
6. **SQLite** (if not already in step 3): `modernc.org/sqlite` in its
   own vendor commit, `history.Store` on it, FTS over lines and output,
   search in the drawer.
7. **Later, if wanted**: an `apple` backend; thresholds tuned from a
   save log; `mdsh history export --git`.

## Risks

- **Saving while typing changes what Autosave means.** Today a file is
  written when a run ends; after step 1 it is written whenever the
  author pauses. A user who edits a file that git watches will see the
  file change more often, though each change is a whole thought. The
  pauses are settings, and "save only after runs" stays as a setting
  for anyone who wants today's behaviour.
- **A model in the loop is a latency and a cost.** Kept off the save
  path by design; the risk is someone later putting it on. The
  `Describer` has no way to say "don't save".
- **Two documents on disk.** A bundle's `document.md` and an outer
  tool editing it (Neovim, git checkout) while mdsh-gui has it open is
  the same case as today's `Changed` banner, with `history/` to keep
  consistent: a version is cut from what the notebook holds, never from
  what the file holds, so an outside edit shows up as an `import`
  version when the user reloads.
- **Vendor size.** `go-git` is a few MB; `modernc.org/sqlite` is tens;
  the Kronk SDK and yzma are to be measured. Each goes in its own
  commit, per the project's rules, and the `CGO_ENABLED=0` build stays:
  yzma is `purego`, and nothing here is cgo.
- **Native code at run time.** yzma loads llama.cpp's shared library
  from a download, checked against the release's SHA-256. That is code
  mdsh didn't build running in mdsh-gui's process, which the security
  review of the release that ships the SDK rung should look at: where
  the library lives (`~/.kronk/libraries/…` when Kronk manages it),
  its mode, and that mdsh never loads one from a document's folder.
- **History holds output.** Which can hold secrets a block didn't mark
  `secret`. The history lives inside the bundle, with the document,
  under the same file permissions, and is never sent anywhere; the
  Describer gets changed *lines* under the diagnose redaction, not run
  output. Still, a history is more to leak than a document, and the
  security review of the release that ships it should look at the
  store's mode (0600) and at what `mdsh bugreport` includes.
- **The "noise" rule misfires.** A block whose output legitimately
  changes every run with the same code (a status check) would be
  marked noise and keep only its latest. `history` on the block
  overrides it, and the rule only applies after three such runs.

## Decisions for j

1. **Save while typing, on by default?** Step 1 as described saves
   after a 5 s pause and on leaving a block. The alternative is to keep
   today's save-after-runs as the default and offer the pauses as an
   option. I'd turn it on: the lost-typing case is the worse one.
2. **The model's role.** This doc keeps a model out of the save
   decision and uses it only to describe versions and suggest noise.
   The alternative is to also ask Kronk's Decision question on `Tick`,
   "save now? yes/no", with a 300 ms timeout and the rules as the
   fallback. I'd not do it until the save log shows the rules getting
   it wrong, and then do it, since the plumbing is one call.
3. **Remote models for descriptions.** Allow the `anthropic` backend to
   describe versions under an explicit setting, with redaction, or keep
   descriptions local-only. I'd start local-only and add the setting if
   asked.
4. **Bundle shape.** A folder package, as above, or a single SQLite
   file holding the markdown too. The folder keeps the markdown
   readable by everything; the single file is one thing to copy. I'd
   do the folder.
5. **History engine for the first cut.** (a) go-git bare repo, then
   SQLite beside it; (b) straight to `modernc.org/sqlite`; (c) a JSON
   Lines log behind `history.Store`, SQLite when search needs it. I'd
   do (b): it is where this ends, j asked for it, and the vendor cost
   is paid once; (a) buys a format other tools read, which `export
   --git` gives later.
6. **History and the user's git.** When a bundle sits in a repository,
   does `history/` get committed (full history travels, outer repo
   churns), get ignored (history per machine), or move out of the
   bundle to `~/.mdsh/history/<id>/` (bundle stays small, history
   doesn't travel)? I'd commit it and document the `.gitignore` line;
   SQLite in a repo diffs badly but a runbook's history is small.
7. **Tracking rules.** The rules in "What to track" and `track=` on
   the fence: right set, or should the first cut track everything and
   add rules from use?
8. **Kronk: server first, or SDK first?** The server rung costs
   nothing and needs the user to run `kronk server`; the SDK rung
   needs a vendor commit (size and license to check) and gives a
   no-server mdsh-gui. I'd ship the server rung with the assistant and
   add the SDK when the first user asks why they need a server. Apple's
   on-device model stays a later backend for the no-install Mac case.
