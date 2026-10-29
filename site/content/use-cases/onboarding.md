---
title: Setup checklists
description: Onboarding and machine checks that tell people what is missing.
weight: 30
---

A setup guide is a list of things to install and check. As an mdsh
document, the checks run: a new teammate runs the file, reads what is
missing, and sends the report to whoever helps them, instead of pasting
command output into a chat.

- **Say what is wrong, not just that something failed.** A check that
  prints "missing" is easier to act on than a red exit code, and
  `on_error: continue` runs every check even when one fails.
- **Keep secrets out of the report.** Print whether something is set, not
  its value, or run a block in a sub-env with `share=` so it sees only the
  variables you name.
- **Show the fix only when it's needed.** An `mdsh:if` section holds the
  install instructions and is printed only when a tool is missing.

## Sample: new developer setup

[onboarding.md](https://github.com/gdey/markdownshell/blob/main/examples/runbooks/onboarding.md)
checks the tools needed to work on mdsh and their versions, whether git
has a name and email, whether there is an SSH key, and that mdsh builds
from the checkout. It installs nothing.

```sh
mdsh --pdf setup.pdf onboarding.md
```

The report from one run:
[PDF](../../samples/onboarding.pdf) ·
[HTML](../../samples/onboarding.html) ·
[markdown](../../samples/onboarding.md)

![Page 1 of the setup report: a table of tools with the version found, the version needed and ok beside each](../../images/pdf/onboarding-1.png)
