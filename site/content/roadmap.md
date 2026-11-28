---
title: Roadmap
description: What is designed and coming next for mdsh.
layout: landing
lede: "What is shipping is on the home page. These are designed, written down in the repository, and not in a release yet. No dates."
---

## Blocks over SSH

Run a document's blocks on the machine they belong to. A deploy runbook
builds here, sends the artifact, and restarts the service there, all in
one file, with the local `confirm` in front of the remote step. The
design is in
[pull request #95](https://github.com/gdey/markdownshell/pull/95),
including where the trust boundary sits.

## A notebook in the browser, built in

`mdsh --serve` opens a notebook in a browser, from a machine that has no
display, over an SSH tunnel. It listens on loopback only, with a token
and a cookie. In 1.7 it is in an mdsh built with `-tags serve`, and
experimental in the desktop app behind a setting; release builds leave
it out. HTTPS for serving beyond this machine is in
[pull request #295](https://github.com/gdey/markdownshell/pull/295).
See [Browser]({{< relref "docs/browser" >}}).

## An MCP server

A way for an AI assistant to open a document and work with it through
mdsh, inside the same guard rails a person has. The design is in
[docs/mcp/design.md](https://github.com/gdey/markdownshell/blob/main/docs/mcp/design.md).

## Recordings in the History pane

The History pane's shell running in a tmux of its own, recorded: each
pane as a terminal recording, played back with its splits and windows,
and each command typed cut out as a block of its own. The design, with
its open questions, is in
[pull request #283](https://github.com/gdey/markdownshell/pull/283).

## Next, in the app

A Welcome document that explains the window, one icon set, and the
accessibility work that continues past the 1.3 pass. The full list is
in [docs/release/v1.3.md](https://github.com/gdey/markdownshell/pull/106).
