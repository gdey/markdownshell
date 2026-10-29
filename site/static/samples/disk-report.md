---
title: Disk usage report
mdsh:
  on_error: continue
---
# Disk usage report

A weekly look at disk space: how full each file system is, and what is
using the space in one directory. Every block only reads. Set `DIR` to
the directory to look into; it defaults to this repository's `examples`
directory so the report is quick to run.


## File systems

The file systems that hold `DIR`, your home directory and the temporary
directory, fullest first. `df -P` prints the same columns on Linux and
macOS.

<!-- mdsh:output for="fs" -->
| mount | size | used | free | use% |
| --- | --- | --- | --- | --- |
| / | 252.0 GiB | 9.1 GiB | 29.3 GiB | 24% |

<!-- /mdsh:output -->

```output {for=full exit=0}
0 file systems at or over 90%
```


No file system is over 90% full.

## What uses the space

The largest entries directly under the directory looked at, in KiB.

<!-- mdsh:output for="largest" -->
### examples

| entry | KiB |
| --- | --- |
| pdf | 356 |
| terminal.md.assets | 36 |
| runbooks | 36 |
| hello.md.assets | 16 |
| fifo | 16 |
| terminal | 12 |
| report.md.assets | 8 |
| include | 8 |

<!-- /mdsh:output -->

Total: **0.5 MiB**.

```output {for=total exit=0}
0.5 MiB
```

## Big files

Files over 64 KiB anywhere under the directory, largest first.

```output {for=big exit=0}
    76 KiB  pdf/terminal.pdf
    76 KiB  pdf/hello.pdf
    72 KiB  pdf/test-coverage.pdf
    68 KiB  pdf/report.pdf
4 files over 64 KiB
```

## Temporary files

Space taken by the system temporary directory, which is often the first
thing to clean.

```output {for=tmp exit=0}
53.3 MiB in temporary files
```
