#!/usr/bin/env mdsh
---
title: Handing data between blocks through a FIFO
---
# Handing data between blocks through a FIFO

Blocks without `env=` run one after another in the same shell. A block
marked `background` starts and then the run goes straight on to the
next block, so it can read from a named pipe (a FIFO) that later
blocks write to.

First make the FIFO, in a block of its own: a background block runs
alongside the blocks after it, so if it made the FIFO itself, the
writer could get there first and write to a plain file instead.

Opening the FIFO for reading waits until something opens it for
writing, so the reader is a `background` block; without that, it would
wait forever and the run would never get to the writer. Whatever it
prints is kept apart and shows up under it, once it ends.

```sh {#setup}
rm -f pipe
mkfifo pipe
```

```sh {#reader background}
cat pipe
```

```output {for=reader exit=0}
hello through the fifo
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

The reader is a job of this shell, and mdsh keeps its process ID in
`$MDSH_BG_reader`, so a later block can `wait` for it. Once it has seen
end of file it ends, and `wait` gives its exit status.

```sh {#collect}
wait "$MDSH_BG_reader"
echo "reader exited with $?"
rm -f pipe
```

```output {for=collect exit=0}
reader exited with 0
```
