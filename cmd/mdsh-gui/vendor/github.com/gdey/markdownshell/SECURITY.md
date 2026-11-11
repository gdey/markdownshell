# Security policy

## Supported versions

Security fixes go into the latest minor release of mdsh and mdsh-gui.
Older releases don't get fixes; upgrade to the latest one.

| Version | Supported |
| ------- | --------- |
| 1.4.x   | Yes       |
| < 1.4   | No        |

## What counts as a security bug

mdsh runs the code in a markdown file; that is what it is for. Running
a file you don't trust runs its code, the same as running a shell
script. A security bug is something a file can do **beyond** that, for
example:

- anything that happens without running a block: opening it in
  `mdsh -i` or mdsh-gui, `mdsh -n`, `--html`, `--pdf`, or `-w`
- getting around a review gate: `confirm`, `--step`, the questions about
  dependencies, or front matter `env.share`
- reading or writing files outside what the document's blocks do
- sending data off the machine without a block running

## Reporting a vulnerability

Please report it privately, not in a public issue: use
[Report a vulnerability](https://github.com/gdey/markdownshell/security/advisories/new)
on the repository's Security tab. Include the mdsh version
(`mdsh --version`), your OS, and a document that shows the problem.

You should hear back within a week. Once a fix is released, the
advisory is published and the CHANGELOG lists the fix under a Security
heading, crediting you if you'd like.

## Past reviews

The reviews are in [docs/security](docs/security): for v1.4.0,
[the whole surface](docs/security/v1.4.0.md); for v1.2.1,
[the CLI and shared packages](docs/security/cli-v1.2.1.md) and
[mdsh-gui](docs/security/gui-v1.2.1.md). The CHANGELOG notes each
finding as it is fixed.
