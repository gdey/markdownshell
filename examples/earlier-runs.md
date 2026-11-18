#!/usr/bin/env mdsh
---
title: Earlier runs
---
# Earlier runs

A block's output is what it printed the last time it ran. Mark the
block `history=N` and mdsh keeps the runs before that one too: each run
moves the output the file held down under the new one, and keeps up to
N of them, newest first. It is for a check you run now and then, where
"what did it say last week?" matters as much as what it says now.

The block below rolls a die, and fails when it rolls a 1, so its
earlier runs differ. It keeps three of them. Run it a few times with
`mdsh -w earlier-runs.md`, or with enter in `mdsh -i` (then `w` to
save), and watch the fences under it move down.

```sh {#roll history=3}
n=$(( $(od -An -N1 -tu1 /dev/urandom) % 6 + 1 ))
echo "rolled a $n"
[ "$n" -ne 1 ] || { echo "a one: try again" >&2; exit 1; }
```

```output {for=roll exit=1 exited at=2026-11-18T07:10:04Z}
rolled a 1
```

```output {for=roll stream=stderr}
a one: try again
```

```output {for=roll exit=0 at=2026-11-18T07:10:03Z history=1}
rolled a 4
```

```output {for=roll exit=0 at=2026-11-18T07:10:02Z history=2}
rolled a 2
```

```output {for=roll exit=0 at=2026-11-18T07:10:01Z history=3}
rolled a 5
```

Each output fence says when its run started with `at=`. The ones under
the current output also say how far back they are with `history=K`, 1
for the run just before. A run that failed keeps its exit status and
its stderr. Saving holds the history to N, so lowering N, or dropping
`history=`, trims it.

mdsh-gui shows the earlier runs under the block's output, behind a line
that says how many there are, each with how long ago it ran. `mdsh -i`
shows them with `b`. A plain `mdsh earlier-runs.md` prints only the
current output; `--history` prints the earlier runs too, and `--html`
and `--pdf` take it:

```sh {#try-history-flag skip}
mdsh -n --history earlier-runs.md
```

A bare `history` keeps 10 runs, or the number under `history:` in the
front matter's `mdsh:` key.
