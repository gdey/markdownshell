---
title: The desktop app
description: mdsh-gui, a notebook for markdown files whose kernel is your shell. macOS and Linux.
layout: landing
lede: "`mdsh-gui` opens a markdown file as a notebook. Press play on a block and its output lands under it, in the file. It is free, it runs on macOS and Linux, and it never needs a server."
---

<p class="cta"><a class="button" href="../install/">Install it</a> <a class="button ghost" href="../examples/">Take the tour</a></p>

{{< shot name="hero" title="deploy.md · waiting for you" >}}The deploy runbook from the examples, stopped before the step that ships.{{< /shot >}}

## Start where your files are

The start screen lists the markdown files under a folder, with each
file's description under its name. Type to filter, or `#tag` for a tag.
Pin the files you come back to. A folder with a `.mdsh.yaml` shows as a
**collection**, with its own title, icon and color; the ⚙ beside it
edits those in a form. **New Session** opens a file named for the day
with the History pane ready, and **Last Session** opens the latest one.

{{< shot name="start" title="Start" >}}Pinned and recent files, then the examples folder as a collection.{{< /shot >}}

## Source and notebook, side by side

Every file opens in a tab with shells of its own. The rendered notebook
is on the right, the markdown source on the left, and they scroll
together. The source view completes block attributes, explains them on
hover, underlines mistakes, and checks the spelling of the prose; Vim
keys are a setting, and a Neovim pane is an experimental one.

In the notebook, drag a block, a fence or a whole section from the
outline to move it, fold what you aren't reading, delete a block or
clear its output, and undo any of it. Every block, its output and any
fence in the prose has a copy button. Tabs drag between windows, and a
block running in one carries on; drop a tab on a terminal to paste the
file's path.

{{< shot name="split" title="deploy.md · source and rendered" >}}The source beside the rendered view.{{< /shot >}}

## Run a block, a section, or all of it

Each block has a play button, each heading runs its section, and Run
all runs the file. Output streams in under the block, with its colors,
and a timer counts up beside it, against its `timeout=` if it has one.
A `confirm` block asks in its card, with Run, Skip and Stop. A block's
`depends=` runs what it needs first and is drawn as colored lines in
the margin, which you can drag to change. Run › Clear All Outputs
starts over, as if nothing had run. A `background` block that outlives
its run keeps a Stop on its card, and the Console's Processes tab shows
everything still running under the file's shells, as a tree, each with
how long it has run and a Kill.

{{< shot name="depends" title="guardrails.md" >}}The selected block's dependencies, drawn in the margin.{{< /shot >}}

## Keep the runs before

Each output says on its title bar how long ago the run that made it
started, and keeps counting. Mark a block `history=N` and the file
keeps its N runs before that one too, under the current output: a line
says how many, and opens to each run's output with its time and exit
status. Copy one, delete one, or clear them all; Create Report's
Earlier runs switch prints them. See
[Earlier runs]({{< relref "docs/writing-documents#earlier-runs" >}}).

{{< shot src="earlier-runs.png" paper="earlier-runs-paper.png" phosphor="earlier-runs-phosphor.png" title="health.md" >}}A block with `history=3`: today's output, then three earlier runs, one of them a failure from yesterday.{{< /shot >}}

## Talk to web services

An `http` block makes one request, written the way it goes over the
wire, and checks the response in Hurl's words. Its card shows the
status, the body and the headers in tabs, with a tick or a cross by
each check, and the body folds on its own. Captured values, such as a
token, reach the prose and the next request, and credentials are saved
masked. See [HTTP requests]({{< relref "docs/http" >}}).

{{< shot src="http-gui.png" paper="http-gui-paper.png" phosphor="http-gui-phosphor.png" title="api.md" >}}An http block whose check failed, with the body it got and the value the check wanted.{{< /shot >}}

## Keep secrets out of the file

Mark a block `secret` and its code and output are blurred until you
press the eye on its card, then masked again after a minute or when the
window loses focus. Copying still works. Mark a block, a textbox or a
choice `nosave` and it works as usual, but what it holds stays in the
session: saving leaves the file as it was, and the card says *session
only*.

{{< shot src="secrets-light.png" dark="secrets-dark.png" paper="secrets-paper.png" phosphor="secrets-phosphor.png" title="rotate.md · running" >}}A `secret` key, a `nosave` pod list, and a rollout timing itself against its one-minute timeout.{{< /shot >}}

## Work in the History pane

View › History opens a terminal along the bottom, with your shell's
prompt and completion. Each command you type is added to the file as a
block, with its output; a line starting with `#` becomes a note, and
``# ```sh`` starts a block that is added, unrun, when a line of
```` ``` ```` ends it. It is
how a document starts when you don't know yet what it will say. The
pane shows the shell's working directory, and a button beside its close
button has it fill the tab. The record button, a dot beside Stop, turns
recording off: commands still run in the file's shell, so a `cd`
carries over, but aren't added to the file. With Settings › Shell › Run
a repeated History command in its block on, typing a command again
reruns its block and keeps the output before as an earlier run.

{{< shot name="history" title="2026-11-02.md · History" >}}A session: the commands typed below became the blocks above.{{< /shot >}}

## Forms, diagrams, pictures

`textbox` and `choice` blocks are drawn as a text field, radio buttons
and checkboxes; what you pick is saved in the file and reaches the
blocks as `$MDSH_VAR_` variables ([Inputs]({{< relref "docs/inputs" >}})). `d2` fences draw diagrams and `svg`
fences pictures. `out=markdown` output is drawn as markdown, images and
terminal recordings show in the block. An image from another site, or
from outside the document's folder, waits behind a Load image button,
and the document remembers the ones you load. HTML in the markdown, such
as `<details>` or `<kbd>`, is drawn, cleaned of scripts.

{{< shot name="inputs" title="inputs.md" >}}A form written in markdown.{{< /shot >}}

{{< shot name="diagram" title="architecture.md" >}}A `d2` diagram above a table made by a block.{{< /shot >}}

## Front matter, as a form

The Front matter tab at the top of the page opens a form for the file's
title, description and tags, its `mdsh:` options, and the variables and
files its shells get. Each change is written to the file as one edit,
and Undo takes it back.

{{< shot src="front-matter-light.png" dark="front-matter-dark.png" paper="front-matter-paper.png" phosphor="front-matter-phosphor.png" title="rotate.md · Front matter" >}}The form over the top of the page.{{< /shot >}}

## Reports with one click

File › Create Report… makes a PDF, a standalone HTML page or a markdown
copy, with or without code and output, from this tab's run, a fresh
run, or the saved file. It shows the `mdsh` command that makes the same
report, for CI.

{{< shot name="report" title="report.md · Create report" >}}The Create report dialog.{{< /shot >}}

## Make it yours

Settings has tabs for General, Look, Editing, Shell, Files and About.
**Look** picks {{< looks >}}, each in light and dark, and
tunes the text size, fonts, line spacing, accent color and the History
pane's colors. About shows the build, the paths it uses and the tools
on your `PATH`, with Copy for a bug report. Help › Changelog opens what
changed in the build you're running, in a tab of its own.

{{< shot src="settings-look-paper.png" title="Settings" >}}Settings › Look, in the Teletype look.{{< /shot >}}

{{< shot src="hero-greentea.png" dark="hero-greentea-dark.png" title="deploy.md · Green Tea" >}}The deploy runbook in the Green Tea look, new in 1.7.{{< /shot >}}

Every button, pane and tab has a name for a screen reader and a key for
the keyboard; the app was checked with VoiceOver for 1.3. The full list
of menus, keys and settings is in the
[desktop app reference]({{< relref "docs/desktop" >}}).
