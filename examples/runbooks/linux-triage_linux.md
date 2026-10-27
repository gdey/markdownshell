#!/usr/bin/env mdsh
---
title: Linux host triage
mdsh:
  on_error: continue
---
# Linux host triage

First steps when a Linux box is slow. Every block is read-only. A block
that fails doesn't stop the rest (`on_error: continue`), since partial
answers still help.

## Load

```sh {#load}
read one five fifteen _ < /proc/loadavg
CPUS="$(getconf _NPROCESSORS_ONLN)"
echo "load: $one $five $fifteen on $CPUS CPUs"
```

> The run queue is long when the 1-minute load is well above the CPU
> count. This block prints a warning then, and nothing otherwise.
>
> ```sh {#busy}
> awk -v cpus="$CPUS" -v l1="$one" 'BEGIN { if (l1 > 2 * cpus) print "warning: load is over twice the CPU count" }'
> ```

## Memory

```sh {#mem out=markdown}
awk '
  /^(MemTotal|MemAvailable|SwapTotal|SwapFree):/ { kb[$1] = $2 }
  END {
    printf "| | MiB |\n| --- | ---: |\n"
    printf "| Total | %d |\n", kb["MemTotal:"] / 1024
    printf "| Available | %d |\n", kb["MemAvailable:"] / 1024
    printf "| Swap used | %d |\n", (kb["SwapTotal:"] - kb["SwapFree:"]) / 1024
  }' /proc/meminfo
```

Available memory right now: **{{#mem-check/env(AVAIL_MIB)}}** MiB.

```sh {#mem-check show=false}
AVAIL_MIB="$(awk '/^MemAvailable:/ { printf "%d", $2 / 1024 }' /proc/meminfo)"
test "$AVAIL_MIB" -gt 0
```

## Busiest processes

1. By CPU:

   ```sh {#top-cpu}
   ps -eo pid,pcpu,comm --sort=-pcpu | head -n 6
   ```

2. By memory:

   ```sh {#top-mem}
   ps -eo pid,rss,comm --sort=-rss | head -n 6
   ```

## Disks

Mounts over 90% full. No output means none are.

```sh {#full timeout=10s}
df -P -x tmpfs -x devtmpfs -x overlay -x squashfs 2>/dev/null |
  awk 'NR > 1 && $5 + 0 > 90 { print $6, $5 }'
```

## Listening ports

`ss` comes with iproute2, which minimal containers may not have.

```sh {#ports}
if command -v ss >/dev/null; then
  ss -ltn | awk 'NR > 1 { print $4 }' | sort -u | head -n 10
else
  echo "ss not installed"
fi
```
