#!/usr/bin/env mdsh
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

```sh {#setup show=false}
DIR="${DIR:-..}"
DIR="$(cd "$DIR" && pwd)"
LIMIT="${LIMIT:-90}"
```

## File systems

The file systems that hold `DIR`, your home directory and the temporary
directory, fullest first. `df -P` prints the same columns on Linux and
macOS.

```sh {#fs out=markdown timeout=10s}
df -P -k "$DIR" "$HOME" "${TMPDIR:-/tmp}" 2>/dev/null |
  awk 'NR > 1 && !seen[$6]++' |
  sort -k5 -rn |
  awk 'BEGIN { print "mount\tsize\tused\tfree\tuse%" }
       { printf "%s\t%.1f GiB\t%.1f GiB\t%.1f GiB\t%s\n", $6, $2/1048576, $3/1048576, $4/1048576, $5 }' |
  md_table
```

```sh {#full}
FULL="$(df -P -k "$DIR" "$HOME" "${TMPDIR:-/tmp}" 2>/dev/null |
  awk -v limit="$LIMIT" 'NR > 1 && !seen[$6]++ && $5 + 0 >= limit { n++ } END { print n + 0 }')"
echo "$FULL file systems at or over $LIMIT%"
```

<!-- mdsh:if {{#full/env(FULL)}}!=0 -->
> **Warning:** {{#full/env(FULL)}} file systems are at least
> {{#setup/env(LIMIT)}}% full. Look at the table above before they fill
> up.
<!-- mdsh:end -->

<!-- mdsh:if {{#full/env(FULL)}}==0 -->
No file system is over {{#setup/env(LIMIT)}}% full.
<!-- mdsh:end -->

## What uses the space

The largest entries directly under the directory looked at, in KiB.

```sh {#largest out=markdown timeout=1m}
cd "$DIR"
md_h3 "$(basename "$DIR")"
{
  printf 'entry\tKiB\n'
  du -sk -- * .[!.]* 2>/dev/null | sort -rn | head -n 8 | awk -F'\t' '{ printf "%s\t%s\n", $2, $1 }'
} | md_table
```

Total: **{{#total/env(TOTAL)}}**.

```sh {#total}
TOTAL="$(du -sk "$DIR" | awk '{ printf "%.1f MiB", $1 / 1024 }')"
echo "$TOTAL"
```

## Big files

Files over 64 KiB anywhere under the directory, largest first.

```sh {#big timeout=1m}
find "$DIR" -type f -size +64k -exec du -k {} + 2>/dev/null |
  sort -rn | head -n 10 |
  awk -v dir="$DIR/" '{ sub(dir, "", $2); printf "%6d KiB  %s\n", $1, $2 }'
echo "$(find "$DIR" -type f -size +64k 2>/dev/null | wc -l | tr -d ' ') files over 64 KiB"
```

## Temporary files

Space taken by the system temporary directory, which is often the first
thing to clean.

```sh {#tmp timeout=30s}
T="${TMPDIR:-/tmp}"
du -sk "$T" 2>/dev/null | awk '{ printf "%.1f MiB in temporary files\n", $1 / 1024 }'
```
