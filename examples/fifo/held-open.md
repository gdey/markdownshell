#!/usr/bin/env mdsh
---
title: A FIFO writer that stays open
---
# A FIFO writer that stays open

A reader on a FIFO sees end of file only when every writer has closed
it. A block that opens the FIFO with `exec 3>pipe` gives the shell
itself a descriptor for it, and the shell outlives the block, so the
FIFO stays open across blocks until something closes it.

Start a background reader, as in [handoff.md](handoff.md).

```sh {#reader}
rm -f pipe got.txt
mkfifo pipe
cat pipe > got.txt &
READER=$!
```

Open the FIFO on descriptor 3 and write a line. The block ends, but
descriptor 3 stays open in the shell.

```sh {#open}
exec 3>pipe
echo "first" >&3
```

So a later block can keep writing to it.

```sh {#more}
echo "second" >&3
```

Nothing has closed the FIFO yet, so `cat` is still waiting for more, and
a `wait` here would block for good (see
[never-closed.md](never-closed.md)). `kill -0` only checks that the
reader is still there.

```sh {#still-open}
kill -0 "$READER" && echo "reader still running"
```

```output {for=still-open exit=0}
reader still running
```

Closing descriptor 3 is the end of file. Now `wait` returns, and the
reader got both lines.

```sh {#close}
exec 3>&-
wait "$READER"
echo "reader exited with $?"
cat got.txt
rm -f pipe got.txt
```

```output {for=close exit=0}
reader exited with 0
first
second
```
