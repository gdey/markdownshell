---
title: mdsh
---

## A notebook you can read anywhere

An mdsh document is an ordinary markdown file. Prose explains what each
block does, the `sh` blocks run top to bottom, and the output lands right
under the block that made it. It renders on GitHub, diffs cleanly in git,
and runs from the command line.

````markdown
#!/usr/bin/env mdsh
# Hello, mdsh

```sh {#setup}
export GREETING=hello
NAME=world
echo "$GREETING, $NAME"
```
````

Run `mdsh -w hello.md` and mdsh writes the result back into the file:

````markdown
```output {for=setup exit=0}
hello, world
```
````

## What it does

- **Shared or isolated shells.** Blocks share one shell like lines of a
  script, or run in named sub-envs that only see the variables you pass
  with `share=`.
- **Output that stays put.** Re-running replaces old output, so the file
  stays stable and diffs stay small.
- **Text, markdown or images.** `out=markdown` inserts output as markdown,
  `out=image` saves image bytes next to the file, and `out=screenshot`
  draws text output as a terminal-style picture.
- **Runbooks with guard rails.** `confirm` asks before a block runs,
  `timeout=` kills a stuck one, and `run=` or `mdsh:if` sections switch
  parts on with conditions like `{{$DEBUG}}`.
- **Prose that quotes results.** Placeholders such as
  `{{#setup/env(NAME)}}` show the value a block left behind.
- **Interactive mode.** `mdsh -i` opens a document in a terminal UI: pick
  blocks and run them, type new ones at a prompt, and edit them in your
  editor. See [Interactive mode]({{< relref "docs/interactive" >}}).
- **Terminal sessions.** A `tty` block runs on a real terminal and is
  recorded as a GIF; a `tape` fence types its input so it runs unattended.
- **PDF reports.** `--pdf report.pdf` lays out the run as a PDF, in pure
  Go. See [PDF output]({{< relref "docs/pdf" >}}) for an example.
- **Executable files.** With a `#!/usr/bin/env mdsh` shebang, `./notes.md`
  just runs.

![A banner drawn by an mdsh block](images/banner.svg)

![A terminal-style screenshot made by an mdsh block](images/screenshot.png)

Both images above were drawn by blocks in
[the hello example](https://github.com/gdey/markdownshell/blob/main/examples/hello.md).
