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
| `Z` / `shift+Z` | Undo / redo |
| `enter` / `shift+enter` | Run the current block / its section |
| `R` / `.` | Run all / stop |
| `alt+K` | Clear all outputs |
| `O` | Open a file |
| `,` | Settings, also the gear on the tab bar |
| `\` / `shift+\` / `shift+O` | Source view / rendered view / outline |
| `shift+H` / `shift+Y` | History pane / Console |
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
- **HTTPS off this machine.** Listening anywhere but loopback, mdsh
  speaks only HTTPS. Give it your certificate with `--serve-cert` and
  `--serve-key`, or let it make a self-signed one. The self-signed
  certificate is kept beside the settings as `serve-self-signed.pem`,
  so a browser told to trust it once keeps trusting it, and mdsh prints
  its SHA-256 fingerprint to check the browser's warning against.
- **Client certificates, if you want them.** `--serve-client-ca ca.pem`
  turns on mutual TLS: the browser must present a certificate one of
  those CAs signed before it gets as far as the token. Chrome, Safari,
  Firefox and Edge all support this; import the certificate and its key
  (usually a `.p12` file) on each device that should get in, and the
  browser asks which one to use.

  ```sh
  mdsh --serve=0.0.0.0:8443 --serve-cert cert.pem --serve-key key.pem --serve-client-ca ca.pem
  ```

- **Or use SSH.** The simplest safe way in from another machine is to
  keep the default address and tunnel to it. Nothing listens on the
  network, and SSH does the encryption and the login:

  ```sh
  ssh -L 8080:127.0.0.1:8080 server   # then open the address mdsh printed, on this machine
  ```

- **It is you.** The page can read and write any file you can, through
  its file dialogs and through the commands it runs. It also serves the
  images and other files in the folders of open documents.
- **Nothing loads from the network.** The pages carry the same
  Content-Security-Policy as the desktop app, so a document's image from
  another site is drawn as a note naming the host instead of being
  fetched, and other sites can't put the pages in a frame.
