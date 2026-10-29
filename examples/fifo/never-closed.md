#!/usr/bin/env mdsh
---
title: A FIFO writer that never closes
---
# A FIFO writer that never closes

This document goes wrong on purpose, to show what mdsh does when a
reader never gets end of file.

Make the FIFO in a temporary directory, since this run never gets to
clean it up.

```sh {#setup}
cd "$(mktemp -d)"
mkfifo pipe
```

A background reader. `timeout=` on a `background` block kills just
that block, not its shell, and the run goes on.

```sh {#reader background timeout=2s}
cat pipe
```

```output {for=reader exit=-1}
only line
```

```output {for=reader stream=stderr}
mdsh: timed out after 2s
```

A writer that opens the FIFO on descriptor 3 and forgets to close it.
The shell still holds descriptor 3 after the block ends, so `cat` never
sees end of file.

```sh {#writer}
exec 3>pipe
echo "only line" >&3
echo "writer done"
```

```output {for=writer exit=0}
writer done
```

The run goes on regardless. A `wait "$MDSH_BG_reader"` here would block
until something killed it, so this block leaves it alone.

```sh {#after}
echo "the run went on"
```

```output {for=after exit=0}
the run went on
```

After the last block, mdsh waits for background blocks still running:
until their `timeout=`, or for 5 seconds if they have none. Then it
kills them. The reader above timed out after two seconds, and still
shows the line it got.
