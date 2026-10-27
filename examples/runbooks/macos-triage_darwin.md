#!/usr/bin/env mdsh
---
title: macOS host triage
mdsh:
  on_error: continue
---
# macOS host triage

First steps when a Mac is slow. Every block is read-only, and a failing
block doesn't stop the rest.

## Machine

```sh {#machine}
echo "$(sw_vers -productName) $(sw_vers -productVersion) ($(uname -m))"
CPUS="$(sysctl -n hw.ncpu)"
echo "$CPUS CPUs, $(( $(sysctl -n hw.memsize) / 1073741824 )) GiB memory"
```

## Load

```sh {#load}
sysctl -n vm.loadavg | tr -d '{}' | awk '{ print "load:", $1, $2, $3 }'
```

> On Apple silicon, efficiency cores count toward `hw.ncpu`, so a load a
> little above the CPU count is normal.
>
> ```sh {#busy}
> one="$(sysctl -n vm.loadavg | awk '{ print $2 }')"
> awk -v cpus="$CPUS" -v l1="$one" 'BEGIN { if (l1 > 2 * cpus) print "warning: load is over twice the CPU count" }'
> ```

## Memory pressure

`vm_stat` counts pages; the page size is in its first line.

```sh {#mem out=markdown}
vm_stat | awk '
  NR == 1 { for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+$/) page = $i }
  /Pages free/     { free = $3 }
  /Pages active/   { active = $3 }
  /Pages wired/    { wired = $4 }
  END {
    printf "| | MiB |\n| --- | ---: |\n"
    printf "| Free | %d |\n", free * page / 1048576
    printf "| Active | %d |\n", active * page / 1048576
    printf "| Wired | %d |\n", wired * page / 1048576
  }'
```

## Busiest processes

1. By CPU:

   ```sh {#top-cpu}
   ps -Ao pid,pcpu,comm -r | head -n 6
   ```

2. By memory:

   ```sh {#top-mem}
   ps -Ao pid,rss,comm -m | head -n 6
   ```

## Disk

```sh {#disk timeout=10s}
df -P / | awk 'NR == 2 { print $5, "used on /" }'
```
