#!/usr/bin/env mdsh
---
title: Handing data between blocks through a FIFO
---
# Handing data between blocks through a FIFO

Blocks without `env=` run one after another in the same shell, so a
job one block starts in the background is still running while the
blocks below it run. That makes a named pipe (a FIFO) a way to hand
data from later blocks to an earlier one.

First make the FIFO and start a reader on it. Opening a FIFO for
reading waits until something opens it for writing, so the reader has
to go in the background (`&`); in the foreground this block would
wait forever. `$!` keeps the reader's process ID for later.

```sh {#reader}
rm -f pipe got.txt
mkfifo pipe
cat pipe > got.txt &
READER=$!
echo "reader started"
```

```output {for=reader exit=0}
reader started
```

A block further down writes to it. `> pipe` opens the FIFO, the write
goes through, and the redirection closes it again when `echo` ends.
That close is the reader's end of file.

```sh {#writer}
echo "hello through the fifo" > pipe
echo "writer done"
```

```output {for=writer exit=0}
writer done
```

The reader is a job of this shell, so a later block can `wait` for it.
Once it has seen end of file it exits, and `got.txt` holds what came
through. Without the `wait`, this block could read `got.txt` before
`cat` had written it.

```sh {#collect}
wait "$READER"
echo "reader exited with $?"
cat got.txt
rm -f pipe got.txt
```

```output {for=collect exit=0}
reader exited with 0
hello through the fifo
```
