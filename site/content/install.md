---
title: Install and run
description: Install the mdsh desktop app, terminal UI and command line, and run your first document.
layout: landing
lede: "Two programs: `mdsh-gui`, the desktop app, and `mdsh`, the command line with its terminal UI, `mdsh -i`. They share one engine, so a file runs the same in both."
---

## Homebrew, on macOS and Linux

```sh
brew install --cask gdey/tap/mdsh   # the desktop app with mdsh, macOS
brew install gdey/tap/mdsh          # just mdsh and mdsh -i
```

Pick one: the desktop app carries `mdsh` and the cask puts it on your
`PATH`, so the two don't install side by side. The macOS app is signed
with an Apple Developer ID and notarized, so it opens like any other
app.

Downloaded the app instead? Settings › Shell › Command line tool puts
its `mdsh` on your `PATH`, on macOS and from the Linux AppImage.

## Linux packages

Each [release](https://github.com/gdey/markdownshell/releases/latest)
has a `.deb` and an `.rpm` for `mdsh`, and the desktop app as an
AppImage that carries its own GTK and WebKit:

```sh
sudo apt install ./mdsh_*_linux_amd64.deb     # Debian, Ubuntu
sudo dnf install ./mdsh_*_linux_amd64.rpm     # Fedora, RHEL
chmod +x mdsh-gui_*_linux_x86_64.AppImage
./mdsh-gui_*_linux_x86_64.AppImage
```

## Anything else

Archives for Linux, macOS and Windows on amd64 and arm64 are on the
[releases page](https://github.com/gdey/markdownshell/releases/latest),
with a `checksums.txt`. Or build it with Go 1.27 or newer:

```sh
go install github.com/gdey/markdownshell/cmd/mdsh@latest
```

Blocks run in `/bin/sh` unless the front matter names another shell.
The Windows builds are not well tested yet.

## Run something

| To | Run |
| --- | --- |
| Open a folder in the desktop app | `mdsh-gui ~/runbooks` |
| Open a file in the desktop app | `mdsh-gui deploy.md` |
| Open it in the terminal UI | `mdsh -i deploy.md` (`?` lists the keys) |
| Run it and print the result | `mdsh deploy.md` |
| Run it and write the output back | `mdsh -w deploy.md` |
| Run it unattended, in CI | `mdsh --yes -w deploy.md` |
| Make a report | `mdsh --pdf run.pdf deploy.md`, or `--html run.html` |
| Keep a log of what ran | `mdsh --audit run.jsonl deploy.md` |
| Let it read `$DEBUG` | `mdsh --expose DEBUG deploy.md` |
| Run it like a script | `chmod +x deploy.md && ./deploy.md` |

## Your first document

Save this as `hello.md`:

````markdown
#!/usr/bin/env mdsh
# Hello

```sh {#greet}
echo "hello from $(uname -s)"
```
````

Open it with `mdsh-gui hello.md` and press play, or run `mdsh -w
hello.md`. Either way the file gets an `output` fence under the block:

````markdown
```output {for=greet exit=0}
hello from Darwin
```
````

Run it again and the output is replaced, not added to. From here, take
the [examples tour]({{< relref "examples" >}}), or read
[Writing documents]({{< relref "docs/writing-documents" >}}).
