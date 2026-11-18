#!/usr/bin/env mdsh
---
title: A History session
---
# A History session

Not every document starts as a plan. `mdsh -i` and mdsh-gui have a
History pane: a shell along the bottom of the screen whose commands are
added to the end of the document as they run, each as a `#history-N`
block with its output. A line typed there that starts with `#` becomes
prose. So you can poke at a problem the way you would in a terminal,
and come out with a document that tells what you did.

Open this file with `mdsh -i history.md` and press `h`, or open it in
mdsh-gui and choose View › History, to add to it. The rest of this file
is the shape such a session leaves behind: commands typed at the pane,
notes typed as `#` lines, and the output each command printed. The
History shell is the document's own, so the directory and variables
carry from one command to the next, as they do between blocks.

```sh {#history-1}
ls *.md
```

```output {for=history-1 exit=0}
README.md
guardrails.md
hello.md
history.md
include.md
inputs.md
report.md
terminal.md
```

hello.md is the one to start with; count its blocks.

```sh {#history-2}
grep -c '^```sh' hello.md
```

```output {for=history-2 exit=0}
7
```

The runbooks are in a folder of their own.

```sh {#history-3}
cd runbooks
ls | head -n 3
```

```output {for=history-3 exit=0}
deploy.md
disk-report.md
linux-triage_linux.md
```

```sh {#history-4}
basename "$PWD"
```

```output {for=history-4 exit=0}
runbooks
```

History blocks are ordinary blocks: `mdsh history.md` runs them from
the top, `mdsh -w` refreshes their output, and they can be renamed,
moved or deleted like any other. A block that is moved out of the run
of history blocks gets an id of its own.

A whole block can be typed in the pane too, to keep a step for later
rather than run it now. A line such as ``# ```sh {#count-runbooks}``
opens it, and the `#` makes it a comment should it reach the shell. The
prompt becomes ```` ```sh> ```` while the block is open, the lines typed
are kept rather than run, and a line of ```` ``` ```` adds the block to
the end of the file just as it was typed, without running it. ctrl+c
drops it partway through. This one was typed that way:

```sh {#count-runbooks}
ls *.md | wc -l
```

The rule over the pane's input shows the shell's working directory,
here `runbooks` after the `cd` above, and follows each `cd`. In
`mdsh -i`, `Z` has the pane fill the screen and esc gives the document
its place back; mdsh-gui has a button for it beside the one that closes
the pane.
