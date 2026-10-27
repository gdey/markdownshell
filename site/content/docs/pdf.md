---
title: PDF output
description: Share a run as a PDF report.
weight: 55
---

`--pdf FILE` lays out what mdsh would print as a PDF: the prose, the blocks
and their output, after the run. It's pure Go, so there is nothing else to
install.

```sh
mdsh --pdf report.pdf report.md                 # run, write the PDF
mdsh --hide-code --pdf report.pdf report.md     # prose and output only
mdsh --only build --pdf build.pdf report.md     # one block and what it needs
mdsh -w --pdf report.pdf report.md              # update the file and the PDF
mdsh -n --pdf report.pdf report.md              # the output saved by -w; nothing runs
mdsh --pdf - report.md | lpr                    # PDF on stdout
```

The flags work as they do for printing: `--hide-code` and `--hide-output`
decide what's shown, `--only` what runs, and `show=`, `show-output=` and
`mdsh:if` sections still apply. With `-w` or `-o` the markdown is written as
well, and the PDF still shows the printed view.

## An example

[examples/report.md](https://github.com/gdey/markdownshell/blob/main/examples/report.md)
is a small release report. `mdsh --pdf pdf/report.pdf report.md` turns it
into
[this PDF](https://github.com/gdey/markdownshell/blob/main/examples/pdf/report.pdf):

![Page 1 of the report: a table, a block's text output in a dark box, and a markdown block drawn as a table](../../images/pdf/report-1.png)

![Page 2 of the report: a failing block with its exit status and red stderr, a screenshot drawn as text, and an include with line numbers](../../images/pdf/report-2.png)

PDFs of the other examples sit beside it:
[hello.pdf](https://github.com/gdey/markdownshell/blob/main/examples/pdf/hello.pdf)
and
[include.pdf](https://github.com/gdey/markdownshell/blob/main/examples/pdf/include.pdf).

![The include example as a PDF: each include captioned with its file and lines, and line numbers in a gutter](../../images/pdf/include-1.png)

## What goes on the page

| In the document | In the PDF |
| --- | --- |
| Prose, lists, tables, quotes, links | Laid out as on a page; headings become PDF bookmarks |
| A runnable block | A light box, captioned with its info string |
| Its output | A dark, terminal-style box |
| A failing block | `exit N` above its output, stderr in a red box |
| `out=markdown` output | Part of the document, like any other markdown |
| `out=screenshot` (SVG) | The text, in a terminal-style box, sharp and searchable |
| PNG, JPEG and GIF images | Drawn, scaled to fit the page |
| Other images (SVG, remote URLs) | Their alt text |
| An include block | Captioned with its file and lines |
| An include with `number_on` | Line numbers in a gutter beside the code |
| Front matter, `#!` line, output markers | Left out |

Pages are A4 and numbered. Text uses the Go fonts, so characters they
don't cover, such as emoji or CJK, show as boxes.
