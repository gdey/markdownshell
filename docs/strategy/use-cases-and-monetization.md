# mdsh: use cases, monetization, enterprise fit, and pitches

Status: draft, Nov 1, 2026. Strategy, not design. Nothing here changes
code. Where this doc describes the product it refers to what is on `main`
or in an open PR, and says which. Where it recommends, the recommendation
is marked and the reasons given, so j can disagree with the reason rather
than the conclusion.

The one-line position, shared with the website work in the "Plan the next
release and polish" thread:

> **mdsh turns the markdown you already have into something that runs,
> and turns each run into a record you can send.**

Everything below is a buyer, a workflow or a feature hung on that line.
The site's existing Use cases section (runbooks, reports, setup
checklists, from PR #51) stays as the public, example-backed version of
section 1. This doc is the longer, internal version with the buyers and
money attached.

## 1. Use cases

The product has four front ends on one engine, and each one changes who
can use a document and where. The use case list is organized by front
end because that is how a buyer meets the product: a platform engineer
sees the CLI first, an analyst sees the GUI first.

### What exists today (for reference)

| Capability | Where | State |
| --- | --- | --- |
| Run `sh`/`zsh` blocks in order, shared or sub-env shells, `share=` allowlist | CLI, TUI, GUI | Released (v1.0+) |
| Write output back into the file; stable diffs | CLI `-w`, TUI, GUI autosave | Released |
| Reports: `--pdf`, `--html`, `--hide-code`, `mdsh:if` sections, placeholders | CLI | Released |
| Guard rails: `confirm`, `--step`, `timeout=`, `on_error`, `run=` conditions, `run=once` | All | Released |
| `depends=` between blocks, drawn as lines and editable in the GUI | CLI, GUI | main (#91, #96) |
| Interactive blocks: `textbox`, `choice`, task checkboxes, `var=` → `$MDSH_VAR_*` | GUI (writes), CLI/TUI (reads) | main |
| `tty` blocks with recordings, `tape` for unattended input | CLI, TUI, GUI | Released |
| d2 and SVG fences rendered inline | GUI | main |
| History terminal: type commands, they become blocks | GUI | main |
| Console (app log), outline, folds, drag to reorder, undo | GUI | main |
| Browser mode: `mdsh --serve`, loopback by default, token + cookie | PRs #55, #56, #85 (stdio) | Open |
| Remote blocks over SSH: `remote` tag, one remote per file, `send=`/`fetch=` | PR #95 | Design only |
| Control socket (`mdsh ctl`) and `--audit FILE` JSON Lines log, `--secret` | `docs/spec.md` (#70, merged) | Designed, not built |
| Packages: Linux, macOS, Windows CLI; macOS and Linux GUI | goreleaser | Released; macOS app unsigned, Windows "not well tested" |

### 1.1 CLI: runbooks that are also the record

The site already covers deploys, incident triage, release checklists,
scheduled reports and setup checklists. Three use cases are not on the
site yet and should be, because they are the ones a team lead will
recognize:

- **Change requests with evidence.** A change ticket asks "what did you
  run and what happened". `mdsh --yes --pdf change-1234.pdf deploy.md`
  produces exactly that, with the commands, their output, exit codes and
  the time. With `--audit` (PR #70) it would also produce a machine
  record. This is the use case that sells to regulated teams.
- **CI jobs that humans can also run.** The same `release.md` runs in CI
  with `--yes` and on a laptop with `confirm` prompts. One file, no
  drift between "the pipeline" and "the manual fallback". mdsh's own
  release runbook is the demo.
- **Living READMEs.** `mdsh -w README.md` keeps the sample output in a
  README true. CI can run it and diff the result, so a
  README that no longer matches the code fails the build.

### 1.2 TUI: the same files over SSH, on a server, in a pager

`mdsh -i` matters less as a product on its own and more as the answer to
"but I'm on a bastion host with no GUI". The use cases are the CLI ones,
done by hand, one block at a time:

- **Incident response on the box.** Open the triage runbook on the host,
  step through it, and the output is saved as you go. When the incident
  is over, the file is the timeline.
- **Guided procedures for an operator who is not the author.** The
  author writes the prose and the blocks; the operator presses enter.
  `confirm` and `--step` give the operator a veto at every step.
- **Teaching.** A tutorial you can run inside the terminal you are
  learning, with `tty` blocks for the REPL and recordings for the write-up.

### 1.3 GUI: the notebook for people who will not open a terminal

The desktop app is the first front end that a non-engineer can use, and
that changes the buyer. New use cases the GUI makes possible:

- **Parameterized runbooks.** `textbox` and `choice` blocks with `var=`
  turn a runbook into a form: pick the environment, type the ticket
  number, press Run Section. The author controls the choices, the
  operator never edits code.
- **Analyst notebooks on shell tools.** `jq`, `curl`, `psql`, `duckdb`,
  `aws`, `gh`: an analyst who lives in these can keep a notebook of
  queries with tables rendered from `out=markdown` and charts from
  `out=image`, in a file that renders on GitHub without the app.
- **Reports with diagrams.** d2 fences draw the architecture, blocks
  fetch the numbers, `--pdf` ships it. One file for the design review.
- **Exploration that becomes a document.** The History terminal records
  what you typed as blocks. Work first, then delete what did not matter
  and write the prose around what did. This is the "notebook" story in
  the project topic, and it is the thing Jupyter does not do for shells.
- **Onboarding kits.** A checklist with task checkboxes, blocks that
  check each tool, and `mdsh:if` sections that show install steps only
  when something is missing. The new hire runs it, ticks the boxes, and
  sends the file back.

### 1.4 Browser mode and SSH (upcoming)

`--serve` (PRs #55, #56) and remote blocks (PR #95) point the same product
at two more places:

- **One notebook, run on the server, viewed on the laptop.** `--serve`
  on a host plus an SSH tunnel gives the GUI experience on a machine that
  has no display. The security section of PR #56 already describes the
  token, Origin and Host checks, and the "use a tunnel" guidance. This is
  the shortest path to "team" use without building multi-user.
- **Runbooks that reach across hosts.** With `remote` blocks, a deploy
  runbook can build locally, `send=` the artifact, and run the restart on
  the target, all in one file, with the local `confirm` in front of the
  remote step. This is what ops teams write in wiki pages today and run
  by copying into three terminals.
- **A server-side runner for scheduled reports.** `--serve=stdio` (PR
  #85) plus a cron entry gives a report that renders in a browser without
  the GUI installed anywhere.

### 1.5 Use cases to say no to, for now

- **General-purpose notebooks for Python or R.** Jupyter owns this.
  mdsh's edge is that the kernel is the shell the user already has. Other
  languages are an open question in `docs/plan.md`; the pitch should not
  promise them.
- **Workflow orchestration (DAGs on a schedule, retries, fan-out).**
  `depends=` is for ordering inside one document, not a scheduler.
  Airflow, Dagster and friends are not the competitor and should not be
  invited to be.
- **A hosted multi-tenant notebook service.** See section 2.4. The
  security model is "runs as you on your machine", and that is a feature.

## 2. Monetization options

### 2.1 The license sets the terms

`LICENSE` is MIT, copyright Gautam Dey, and every release so far,
including the GUI, has shipped under it. Three consequences:

1. **Nothing shipped can be taken back.** Anyone may redistribute and
   sell v1.2.1 and everything on `main` today. Any paid product has to be
   something new, or something that is more valuable because j runs it.
2. **A license change going forward is allowed but has a cost.** Future
   components can be under a different license (source-available, BSL,
   or proprietary), and the existing MIT code can keep its license. Doing
   this to the *core* after the fact is what cost HashiCorp, Redis and
   Elastic goodwill. Doing it to a *new, separate* component (an
   enterprise server, a sync service) is the usual and accepted form.
3. **MIT is an asset for adoption.** Platform teams can vendor it,
   security teams can read it, and it passes every open-source review a
   procurement process has. That is worth more than the revenue a
   restrictive license would protect at this stage.

Recommendation: keep the CLI, library, TUI and desktop GUI MIT. Put any
paid component in a separate module or repo with its own license from
day one, so there is never a relicensing event.

### 2.2 The options, with trade-offs

| Option | What is sold | Fits the license? | Build cost | Ops burden | Revenue shape | Risk |
| --- | --- | --- | --- | --- | --- | --- |
| **A. Open core** | Enterprise features (SSO for `--serve`, audit log sinks, policy, central settings) in a separate paid module | Yes, if the module is new and separately licensed | Medium | None | Per-seat or per-org subscription | Must keep the free product excellent or the community leaves; feature line is contentious |
| **B. Paid GUI** | The desktop app as a paid download, CLI stays free | Weak: the GUI is already MIT and released | Low | None | One-time or subscription per user | Anyone can build and ship the free GUI from source; the price buys convenience and signed binaries only |
| **C. Team / enterprise tier** | A server (the `--serve` host grown up): shared runbook library, SSO, audit, policy, RBAC on which blocks need confirm | Yes, new component | High | Medium (customers self-host) | Per-org annual contract, $5k–$50k | Long sales cycle; needs the enterprise gaps in section 3 closed first |
| **D. Hosted / serve mode** | j runs the server: a URL per team, notebooks run in a sandbox j operates | Yes | High | High (24/7, security, isolation) | Monthly per seat | The product's whole security model is "runs as you on your machine"; hosting flips that into "runs on j's machine" and makes j liable for every block |
| **E. Support and services** | Support contract, runbook authoring, onboarding workshops, custom features | Yes, no change | None | Low | Hourly or retainer, lumpy | Does not scale past j's hours; but it is revenue this quarter and the best source of use cases |
| **F. Sponsorship / donations** | GitHub Sponsors, Open Collective | Yes | None | None | Small, irregular | Not a business, but it costs nothing to switch on |

### 2.3 What to do first, and why

Recommendation, in order:

1. **Switch on F now.** GitHub Sponsors and a "Support" page on the site.
   Costs an afternoon. Signals that the project intends to last.
2. **Offer E as soon as the next release ships.** A "Get help" page:
   runbook authoring, team onboarding, priority fixes. The first three
   customers of E tell j which of A or C is real. Every paid feature in
   this doc is a guess until someone pays for it.
3. **Build toward C, not D.** The team server is the product that fits
   the engine: `--serve` already exists in PRs, `--audit` is designed,
   SSH is designed. Self-hosted keeps "runs as you" true and keeps j out
   of the liability of running customers' shells. Sell it as "the
   runbook server": one place the team's runbooks live, run from, and
   leave records in.
4. **Treat B as packaging, not product.** Signed and notarized macOS and
   Windows builds, auto-update and a nicer installer are worth charging
   for in a small way (a "supporter build" at $20–$50), but they are not
   a business. Do not gate GUI features.
5. **Do not do D** unless a customer asks for it and will pay for the
   isolation work. Sandboxing (section 3) is a precondition.

### 2.4 Where the free/paid line goes (for option A or C)

The rule that keeps the community on side: **anything one person needs
on one machine is free. Anything that exists because there is more than
one person, or more than one machine, is where the money is.**

Free, forever: the CLI, TUI, desktop GUI, reports, interactive blocks,
`--serve` on loopback for yourself, SSH remote blocks, local `--audit`
to a file.

Paid (team server or module): SSO/OIDC on `--serve`, shared runbook
libraries with permissions, audit log shipped to SIEM with signed
entries, org-wide policy (which shells, which includes, which blocks
must confirm, secrets never in write-back), central settings and
templates, approval flows ("this `confirm` needs a second person"),
scheduled runs with history, and a support SLA.

## 3. Enterprise suitability

### 3.1 What exists today

- **Two security reviews** of v1.2.1, one for the CLI and one for the
  GUI, in the project folder under `security-review/`. Both start from
  the right threat model: the user opens a file they did not write.
  Together they list 16 findings, 4 of them High. They are reproducible
  and have concrete fixes. **As of this draft none is marked fixed in
  `CHANGELOG.md`** and spot checks (`run/include.go` has no root check;
  the GUI has no Content-Security-Policy) agree. This is the first thing
  an enterprise buyer will ask about, because the reviews are good enough
  to be handed over, and the findings are the kind (secret disclosure
  without running anything) that security teams escalate.
- **A sound base.** Things the reviews checked and found right: atomic
  write-back, 0600 temp files, shell quoting of mdsh's own variables,
  goldmark in safe mode, no network listener in the CLI, process-group
  kill of background jobs. The engine is small, pure Go, with no npm at
  build time and vendored, hash-checked frontend bundles.
- **Review gates by design.** `confirm`, `--step`, `env.share`, sub-envs,
  `timeout=`, `on_error: stop`. These are the primitives an audit story is
  built on, and they already exist.
- **Reproducible releases.** goreleaser with `-trimpath`, a checksums
  file, CI on Linux and macOS with `-race`.
- **Designs that point the right way.** The control socket and audit
  log section of `docs/spec.md` (#70, merged: `--audit`, `--secret`, a
  socket that cannot add or run blocks) and PR #95 (SSH with a
  trust-boundary diagram, no option passthrough, host keys left to the
  user) are written the way an enterprise reviewer wants to read them.

### 3.2 What is missing

Grouped by what a buyer's security questionnaire asks for. "Needed for"
says which tier from section 2.4 it blocks.

| Gap | Today | What "done" looks like | Needed for |
| --- | --- | --- | --- |
| **Fix the review findings** | 16 open, 4 High (escape sequences hide code at confirm; `{{$VAR}}` leaks env into reports and write-back; include reads any file; GUI zero-click exfiltration via image URLs; hidden blocks run) | All High and Medium fixed and listed under a Security heading in the changelog; a `SECURITY.md` with a disclosure address | Free tier. Blocks everything else |
| **Code signing and notarization** | macOS app unsigned (docs tell users to bypass Gatekeeper); Windows unsigned; no cosign on archives | Apple Developer ID + notarization, Windows Authenticode, Sigstore signatures on release archives, SLSA provenance from the GitHub release workflow | Free tier for any company laptop with MDM |
| **Audit log** | Designed in PR #70 (`--audit FILE`, JSON Lines, HMAC for secrets) | Shipped in CLI and GUI; one schema; `mdsh-gui` and `--serve` write it too; later: tamper-evident chain and a SIEM sink | Free (local file); Team (shipping, retention, signed) |
| **Secrets handling** | `env.share` filters shells; placeholders and write-back do not honor it (review finding) | `--secret`/`secret:` from PR #70; placeholders resolve only allowlisted vars; write-back never stores caller-env values | Free |
| **Policy** | None. A file chooses its own `shell:`, its includes, whether blocks confirm | A policy file (org or user level): allowed shells, include root, "every block confirms unless the file is in a trusted dir", no remote blocks, no `tty`. Documents cannot loosen it | Team |
| **Sandboxing** | Blocks run as the user with the user's whole environment; `share=` is the only fence | Opt-in per-document sandbox: on macOS `sandbox-exec`, on Linux bubblewrap or Landlock, with a documented "what the sandbox does not protect" | Free (opt-in); Hosted (mandatory) |
| **SSO / identity** | Not applicable to CLI/GUI (OS identity). `--serve` has a token and cookie | OIDC on `--serve`; user name in the audit log; later RBAC on a runbook library | Team |
| **Approvals** | `confirm` asks the same person who is running | A second-person approval on marked blocks, via the control socket or the team server | Team |
| **Supply chain** | Go modules with sums; vendored JS with integrity hashes; Dependabot not checked | SBOM in each release (`syft`), govulncheck in CI, pinned actions, Dependabot | Free |
| **Windows** | CLI builds, "not well tested"; no GUI | Tested CLI on Windows with `sh` from Git for Windows or PowerShell blocks; GUI build | Enterprise desktop fleets are mostly Windows |
| **Compliance paperwork** | None | `SECURITY.md`, a threat model doc (the reviews are most of it), a data-handling statement ("mdsh sends nothing anywhere"), accessibility notes for the GUI | Team sales |
| **Versioned, documented file format** | `docs/spec.md` is thorough | A stated compatibility promise: which attributes are stable, how a v2 file opens in v1 | Any buyer who stores runbooks for years |

### 3.3 Gap list, ordered

What to do, in order, if the goal is "an enterprise could adopt the free
tier without a waiver":

1. Fix the 4 High findings (CLI #1, #2; GUI #1, #2), then the Mediums.
   Ship as a security release with a changelog entry per finding.
2. Add `SECURITY.md` and a disclosure address.
3. Sign and notarize the macOS app. Sign Windows builds. Sigstore the
   archives. Remove the "bypass Gatekeeper" instructions from the docs.
4. Ship `--audit` (PR #70) in the CLI, then the GUI.
5. Make placeholders and write-back honor `env.share`, and ship
   `--secret`.
6. SBOM and govulncheck in CI.
7. A policy file, read-only to documents.
8. Opt-in sandbox, Linux first.

Items 1 to 6 are weeks, not months, and all of them are free-tier work.
7 and 8 are where the team tier starts.

### 3.4 An honest summary for a buyer

Today: a well-designed single-user tool with a clear security model, two
public security reviews, and open findings that are being fixed. Suitable
now for individual engineers and small teams that trust their own files.
Not yet suitable for "open any runbook from the shared drive" on a
managed fleet, and not yet suitable for a central server.

## 4. Sales pitches

Each pitch is one page: who, the pain, what mdsh does about it, proof,
the ask, and the objection to expect. They share the one-line position
at the top of this doc so the website, the README and these pages say
the same thing.

### 4.1 Platform and SRE teams: "The runbook is the record"

**Who.** The team that owns deploys, on-call and the wiki page titled
"How to restart the thing". Buyer is the team lead or the head of
platform. Users are every engineer on call.

**The pain.** Runbooks live in a wiki and drift. The person on call at
3am copies commands from the page into three terminals, and the record of
what happened is a Slack thread. The change-management ticket asks for
evidence, and the evidence is a screenshot.

**What mdsh does.** The runbook is a markdown file in the repo. It
renders on GitHub like any doc. Run it and the commands execute in order,
`confirm` stops before the risky step, `timeout=` stops a hang,
`on_error: stop` stops a bad deploy. The output lands under each command
and `-w` writes it into the file, so the last run is in git. `--pdf`
makes the evidence for the ticket. With `remote` blocks (design in PR #95)
the same file builds here and restarts there. With `--audit` (PR #70)
every step, skip and answer is a JSON line for the SIEM.

**Proof.** `examples/runbooks/deploy.md` and the triage runbooks, with
their PDFs on the site. mdsh's own release runs as an mdsh file.

**The ask.** Pick one runbook that hurt last quarter. Convert it. Run it
on the next deploy with `--step`. Free, MIT, single binary, no server.

**Objection.** "Our runbooks need approvals and an audit trail." Answer:
the audit log is designed and in flight; approvals and policy are the
team server (section 2.4), and this team is who it is for. Ask them what
the approval flow should look like.

### 4.2 Data and analyst teams: "A notebook whose kernel is your shell"

**Who.** Analysts and data engineers whose real tools are `psql`,
`duckdb`, `jq`, `curl`, `aws s3`, `bq`, `gh`. Buyer is the analytics
lead. Users are the people who keep a `queries.txt`.

**The pain.** Jupyter wants Python. The shell work (pull, filter, join,
summarize) lives in a terminal history and a scratch file, and the weekly
report is rebuilt by hand. Sharing means pasting output into a doc that
goes stale the day it is sent.

**What mdsh does.** Open the desktop app. Type in the History terminal;
each command becomes a block with its output. Keep the ones that matter,
write the sentence above each. `out=markdown` turns tab-separated output
into a table, `out=image` keeps the chart, `{{#total/env(N)}}` quotes a
number in the prose. A `choice` block picks the week, a `textbox` takes
the customer id, `Run Section` refreshes everything under a heading.
`--pdf` ships the report; `--html` gives a page with the images inside.
The file is markdown, so it renders on GitHub for everyone who does not
have the app.

**Proof.** `examples/runbooks/disk-report.md` and `test-coverage.md`:
tables, conditional warnings, a PDF from one command.

**The ask.** Take one recurring report. Make it a file. Run it next week
with one click. Free, no account, works offline.

**Objection.** "We need Python cells." Answer: a block can call
`python -c` or a script today, and the output lands the same way.
Native Python blocks are an open question, not a promise. If that is the
blocker, this is not the right tool yet.

### 4.3 Solo developers and consultants: "Your notes, but they run"

**Who.** The independent developer, the consultant who sets up the same
stack for every client, the open-source maintainer with a release
checklist. Buyer and user are the same person.

**The pain.** A `notes.md` with commands you paste. A README whose
sample output stopped being true two releases ago. A client handover
doc that says "run these" and nobody can tell if they did.

**What mdsh does.** Add `#!/usr/bin/env mdsh` to the top of the notes and
`./notes.md` runs them. `mdsh -w README.md` keeps the README's output
true, and CI can fail when it is not. The setup checklist you hand a
client runs on their machine and tells them what is missing, with the
install steps shown only when needed. A `tty` block records the session
as a GIF for the write-up. One binary, MIT, works in a terminal, in the
TUI over SSH, or in the desktop app.

**Proof.** `examples/hello.md` (the README), `onboarding.md`, and the
GIF from `examples/terminal.md`.

**The ask.** Install it, run the hello example, convert one file. If it
saves an hour, sponsor the project or buy the supporter build with the
signed installer.

**Objection.** "Why not a shell script?" Answer: a script has no prose,
no record of the last run, no `confirm` in front of the dangerous line,
and nobody reads it on GitHub. A markdown file is the thing you already
write; mdsh makes it run.

## 5. Shared messaging with the website rethink

The "Plan the next release and polish" thread is redoing the site; its
plan is `docs/release/v1.3.md` (PR #106). The split: that doc owns the
site's structure and headline, this doc owns the copy. To avoid two
versions of the story:

- **The site home page carries the one-line position** from the top of
  this doc and the three buyers as three short rows (the pitch titles
  above), each linking to a use case page. The existing Use cases pages
  stay as the proof.
- **Pitches 4.1 to 4.3 are the source** for any landing copy. Edit them
  here; copy to the site.
- **Section 3.4 is the wording for a Security page** on the site, next to
  `SECURITY.md`, with a link to the two reviews once they are in the repo
  (recommended: move them to `docs/security/`, since a public review is a
  selling point only when it is public).
- **"Supporter build" and "Get help"** (section 2.3) are two pages the
  site needs if j takes those recommendations. They are small.
- The welcome template the release thread is planning (one SVG that
  explains the interface) is also the first thirty seconds of the solo
  developer pitch. Design it for that reader.

## 6. Questions for j

These change the direction, so they are worth answering before anyone
builds toward section 2:

1. **Is the goal income or adoption?** If income within a year, start
   with support (E) and the supporter build, and this doc should become
   a pricing page. If adoption first, skip to the gap list in 3.3 and
   revisit money after the security release.
2. **Is a separately licensed paid component acceptable?** Everything
   in section 2.4's paid column assumes yes. If j wants the whole thing
   MIT forever, the only revenue is E and F, and that is a fine answer.
3. **Which buyer first?** The platform/SRE pitch fits the engine best
   and is where audit, SSH and policy all point. The analyst pitch fits
   the GUI and the History terminal. Picking one decides what the next
   release polishes.
4. **Appetite for running a service?** Section 2.3 says no to hosted.
   If j wants a hosted product anyway, sandboxing moves to the top of
   the gap list and the design changes.
5. **Who fixes the security findings, and when?** Everything in section
   3 waits on this. If the next release is the security release, the
   website should say so.
