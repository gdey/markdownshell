#!/usr/bin/env mdsh
---
title: A FIFO writer that stays open
---
# A FIFO writer that stays open

A reader on a FIFO sees end of file only when every writer has closed
it. A block that opens the FIFO with `exec 3>pipe` gives the shell
itself a descriptor for it, and the shell outlives the block, so the
FIFO stays open across blocks until something closes it.

Make the FIFO and start a background reader, as in
[handoff.md](handoff.md).

```sh {#setup}
rm -f pipe
mkfifo pipe
```

```sh {#reader background}
cat pipe
```

```output {for=reader exit=0}
first
second
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
kill -0 "$MDSH_BG_reader" && echo "reader still running"
```

```output {for=still-open exit=0}
reader still running
```

Closing descriptor 3 is the end of file. Now `wait` returns, and the
reader shows both lines.

```sh {#close}
exec 3>&-
wait "$MDSH_BG_reader"
echo "reader exited with $?"
rm -f pipe
```

```output {for=close exit=0}
reader exited with 0
```
