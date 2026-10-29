#!/usr/bin/env mdsh
---
title: A FIFO writer that never closes
---
# A FIFO writer that never closes

This document goes wrong on purpose, to show what mdsh does when a
reader never gets end of file. Run it and the run stops at `wait`.

A background reader, and a writer that opens the FIFO on descriptor 3
and forgets to close it. The FIFO goes in a temporary directory, since
this run never gets to clean it up.

```sh {#reader}
cd "$(mktemp -d)"
mkfifo pipe
cat pipe > got.txt &
READER=$!
```

```sh {#writer}
exec 3>pipe
echo "only line" >&3
echo "writer done"
```

```output {for=writer exit=0}
writer done
```

The shell still holds descriptor 3, so `cat` never sees end of file and
`wait` never returns. Without `timeout=` this block would hang the run.
With it, mdsh kills the block's shell after two seconds. Killing the
shell is the only way to stop a block, so the shell's state goes with
it, and the run stops here: the next block doesn't run.

```sh {#wait timeout=2s}
wait "$READER"
echo "reader exited with $?"
```

```output {for=wait exit=-1 exited}
```

```output {for=wait stream=stderr}
mdsh: timed out after 2s
```

```sh {#after}
echo "never runs"
```

A reader can't be left waiting past the end of the run, though. When
the document ends, mdsh ends each shell and everything it started, so
a forgotten background reader (or writer) doesn't keep mdsh running.
