---
title: Getting started
description: Install mdsh and run your first document.
weight: 10
---

## Install

Download an archive for your system from the
[releases page](https://github.com/gdey/markdownshell/releases), unpack it,
and put `mdsh` on your `PATH`. Builds cover Linux, macOS and Windows on
amd64 and arm64; `checksums.txt` lists each archive's SHA-256. For example,
on Linux amd64:

```sh
VERSION=1.0.0
curl -LO https://github.com/gdey/markdownshell/releases/download/v$VERSION/mdsh_${VERSION}_linux_amd64.tar.gz
tar xzf mdsh_${VERSION}_linux_amd64.tar.gz mdsh
sudo mv mdsh /usr/local/bin/
mdsh --version
```

On macOS, a downloaded binary may be quarantined; `xattr -d
com.apple.quarantine mdsh` clears it. Blocks run in `/bin/sh` by default;
on Windows, set `shell:` under `mdsh:` in the front matter to a POSIX shell
such as Git Bash's `bash.exe`. The Windows builds are not well tested yet.

With Go 1.24 or newer you can build it instead:

```sh
go install github.com/gdey/markdownshell/cmd/mdsh@latest
```

## Your first document

Save this as `hello.md`:

````markdown
#!/usr/bin/env mdsh
# Hello

```sh {#greet}
echo "hello from $(uname -s)"
```
````

Then run it:

```sh
mdsh hello.md        # print the markdown with output under each block
mdsh -w hello.md     # write the output back into hello.md
chmod +x hello.md && ./hello.md   # or run it directly
```

After `mdsh -w`, the file has an `output` fence under the block:

````markdown
```output {for=greet exit=0}
hello from Linux
```
````

Run it again and mdsh replaces that output instead of adding more.

## Shell completion

Completion covers flags, `.md` files and, after `--only`, the file's block
ids:

```sh
source <(mdsh completion bash)              # bash, e.g. in ~/.bashrc
mdsh completion zsh > "${fpath[1]}/_mdsh"   # zsh, then restart the shell
```
