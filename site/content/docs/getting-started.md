---
title: Getting started
description: Install mdsh and run your first document.
weight: 10
---

## Install

```sh
brew install --cask gdey/tap/mdsh   # the desktop app, macOS
brew install gdey/tap/mdsh          # mdsh and mdsh -i, macOS and Linux
```

Linux packages, the AppImage, archives for every platform and building
from source are on [Install and run]({{< relref "/install" >}}).

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

Open the same file with `mdsh-gui hello.md` or `mdsh -i hello.md` and
press play on the block, or enter.

From here, the [examples tour]({{< relref "/examples" >}}) walks through
what a document can do, one example at a time, and the pages in this
section go into each part.

## Shell completion

Completion covers flags, `.md` files and, after `--only`, the file's block
ids:

```sh
source <(mdsh completion bash)              # bash, e.g. in ~/.bashrc
mdsh completion zsh > "${fpath[1]}/_mdsh"   # zsh, then restart the shell
```
