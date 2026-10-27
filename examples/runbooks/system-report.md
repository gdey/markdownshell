#!/usr/bin/env mdsh
# System report

A quick report on the machine this runs on, for pasting into a bug
report. Sections that only apply to one OS show only there.

```sh {#detect}
OS="$(uname -s)"
ARCH="$(uname -m)"
echo "$OS $ARCH"
```

## Shells and tools

Which common tools are installed, and where.

```sh {#tools out=markdown}
{
  printf 'tool\tpath\n'
  for t in sh bash zsh git go make curl; do
    p="$(command -v "$t" 2>/dev/null || true)"
    printf '%s\t%s\n' "$t" "${p:-not installed}"
  done
} | md_table
```

## Disk

Free space on the root file system. `df -P` gives the same columns on
Linux and macOS.

```sh {#disk}
df -P / | awk 'NR == 2 { printf "%s used of %d MiB\n", $5, $2 / 2048 }'
```

<!-- mdsh:if {{#detect/env(OS)}}==Linux -->
## Linux details

- Kernel and distribution:

  ```sh {#linux-release run={{#detect/env(OS)}}==Linux}
  uname -r
  if [ -r /etc/os-release ]; then . /etc/os-release; echo "$PRETTY_NAME"; fi
  ```

- Memory, from `/proc/meminfo`:

  ```sh {#linux-mem run={{#detect/env(OS)}}==Linux}
  awk '/^MemTotal:/ { printf "%d MiB total\n", $2 / 1024 }' /proc/meminfo
  ```
<!-- mdsh:end -->

<!-- mdsh:if {{#detect/env(OS)}}==Darwin -->
## macOS details

- Version:

  ```sh {#mac-release run={{#detect/env(OS)}}==Darwin}
  sw_vers -productName
  sw_vers -productVersion
  ```

- Memory, from `sysctl`:

  ```sh {#mac-mem run={{#detect/env(OS)}}==Darwin}
  echo "$(( $(sysctl -n hw.memsize) / 1048576 )) MiB total"
  ```
<!-- mdsh:end -->

## Environment

Only the variables named here reach this block, so the report doesn't
leak tokens or other secrets from your shell.

```sh {#env env=report share=HOME,SHELL,LANG}
for v in HOME SHELL LANG GITHUB_TOKEN; do
  eval "val=\${$v:-}"
  if [ -n "$val" ]; then echo "$v is set"; else echo "$v is not set"; fi
done
```
