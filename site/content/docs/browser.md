---
title: Browser
description: The desktop app's notebook in a web browser, served by mdsh --serve.
weight: 54
---

`mdsh --serve` shows the same notebook as the
[desktop app]({{< relref "desktop" >}}) in a web browser. You get the
rendered prose, blocks that stream their output as they run, the source
view, `tty` blocks in a terminal, reports and tabs. It prints the
address to open.

```sh
mdsh --serve                      # the markdown files under this folder, at 127.0.0.1:8080
mdsh --serve notes.md staging     # notes.md, with $1 set to staging
mdsh --serve=localhost:9000 docs  # the files under docs, on port 9000
```

The address goes after `=`. With a space, as in `--serve host:port`,
mdsh would read `host:port` as the file. A host alone keeps port 8080,
and `:9000` alone listens on every address (see Security below).

## Windows and tabs

Each browser tab is one window, with its own tab bar. Closing the
browser tab leaves the window and its shells running, so a block that
is still going keeps going. Opening the address again picks the window
up. Closing its last tab, with the × on the tab bar, ends it. Ctrl+C in
the terminal stops the server and every shell, and output that wasn't
saved is lost.

The desktop app's menu shortcuts work in the page, with cmd on macOS and
ctrl elsewhere:

| Keys | Does |
| --- | --- |
| `S` / `shift+S` | Save / Save As |
| `R` / `.` | Run all / stop |
| `O` | Open a file |
| `,` | Settings, also the gear on the tab bar |
| `\` / `shift+\` / `shift+O` | Source view / rendered view / outline |
| `shift+]` / `shift+[` | Next tab / previous tab |

The browser keeps ctrl+T and ctrl+W for itself, so use the + and × on
the tab bar. Open, Save As, a report's destination and the start folder
list the folders on the machine mdsh runs on, because that is where the
files are. You can also type a path, starting with `~` or relative to
the folder mdsh started in.

## Security

**Anyone who can use the page can run any command as the user who
started mdsh.** That is what the page is for, so treat its address like
a shell login.

- **This machine only, by default.** mdsh listens on 127.0.0.1, so
  other machines can't reach it. `--serve=0.0.0.0:8080`, `--serve=:8080`
  or another of the machine's addresses lets the network in, and mdsh
  prints a warning when it does.
- **A token on every request.** Every start makes a new random token and
  prints it in the address. The first visit trades it for a cookie, and
  every request without that cookie is refused. Keep the address private
  the way you would a password, and out of shared shell history and chat.
- **Other sites are kept out.** A request from another site's page is
  refused because its `Origin` isn't this server. On 127.0.0.1 the
  `Host` must be a loopback name such as `localhost`, so a site can't
  point its own name at your machine to get in (DNS rebinding).
- **No encryption.** There is no TLS, so on a network the token and
  everything on the page travel in the clear. To use mdsh from another
  machine, keep the default address and tunnel to it over SSH:

  ```sh
  ssh -L 8080:127.0.0.1:8080 server   # then open the address mdsh printed, on this machine
  ```

- **It is you.** The page can read and write any file you can, through
  its file dialogs and through the commands it runs. It also serves the
  images and other files in the folders of open documents.
