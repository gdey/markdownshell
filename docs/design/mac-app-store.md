# mdsh.app and the Mac App Store

Status: decided, not implementing. Nov 4, 2026.

Asked for by j after v1.4.0: "Do you know what it takes to get into the
Mac App Store, and any features that would be blocked?" The assessment
below was the answer, and j decided the same day: mdsh.app will not be
built for the Mac App Store. Developer ID plus notarization, which
v1.4.0 already ships with, stays the channel. This page is the record of
why.

## Decision

Not pursued, for these reasons:

1. The App Store requires the App Sandbox, and a sandboxed shell can't
   see the user's machine: no `~/.zshrc`, Homebrew, `~/.ssh` or
   `~/.aws`, only files picked in an open panel. Shell blocks, the
   core feature, become useless.
2. Settings › Shell can't exist: writing a symlink into
   `/usr/local/bin` and the admin password prompt are both blocked by
   the sandbox and forbidden by guideline 2.4.5(iii) and (v).
3. The bundled `mdsh` can't double as the terminal CLI, because every
   executable in the bundle has to be sandboxed and an `inherit` helper
   only runs under a sandboxed parent.
4. Features in design die with it: sudo password prompts (2.4.5(v)),
   1Password secrets (`op` and the agent socket are outside the
   container), and ssh with keys.
5. The embedded Neovim can't run: `nvim` lives in Homebrew and reads
   `~/.config/nvim`, both unreadable.
6. The temporary-exception entitlements that would reopen `$HOME` are
   rejected by App Review for exactly this use, and none grant root.
7. It would be a second signing and packaging pipeline (Apple
   Distribution and Mac Installer Distribution certificates, a
   provisioning profile, a `.pkg`, App Review that takes days) with no
   shared updater, for a build with fewer features than the `.dmg`.
8. Review risk beyond the sandbox: 2.4.5(iii)–(v) on installing code
   and escalation, and the "executes code" clause of 2.5.2, which a
   reviewer can apply to an app whose description says it runs shell
   scripts.
9. The direct channel already has what the store would add:
   Gatekeeper-clean install, the cask for discovery, the release page
   for updates. iTerm2, Ghostty, Zed and VS Code stay off the store for
   the same reason.

If store presence ever matters for discovery alone, the realistic shape
is a separate reduced product (render, edit, outline, no execution)
with its own bundle ID, not a sandboxed build of this app.

## The assessment

Against the code as it was at v1.4.0.

### The sandbox is not optional

Every app on the Mac App Store must have `com.apple.security.app-sandbox`
(guideline 2.4.5(i)). A sandboxed process sees its own container
(`~/Library/Containers/io.github.gdey.mdsh/Data`), the system folders
(`/usr`, `/bin`, `/System`, `/Library`, `/Applications`), and whatever
the user picks in an open or save panel, kept across launches as a
security-scoped bookmark. Everything else in `$HOME` is invisible, and so
is `/opt/homebrew`. Child processes are allowed, but they inherit the
same sandbox (`com.apple.security.inherit`); there is no entitlement
that lets a child out.

What that does to mdsh:

| Feature | In the sandbox | Why |
|---|---|---|
| Running shell blocks (`env/shell.go`, `env/tty_unix.go`) | Runs, but useless | `/bin/zsh` starts under the inherited sandbox: no `~/.zshrc`, no Homebrew, no `~/.ssh`, no `~/.aws`, only files the app was granted. A runbook that touches the user's machine fails on the first block. |
| Open folder, outline, `/doc/` assets (`folders.go`, `docfiles.go`) | Keepable | Open panel plus security-scoped bookmarks per folder. Needs code to resolve and renew bookmarks; `os.Open` on a remembered path stops working. |
| Settings, history (`os.UserConfigDir`) | Keepable | Redirected into the container. Separate from the CLI's `~/.config/mdsh`, so the two stop sharing state. |
| Settings › Shell: symlink into `/usr/local/bin` (`cli.go`, `cli_darwin.go`) | Impossible | Writing outside the container is blocked, the `osascript` admin prompt is blocked, and 2.4.5(iii) and (v) forbid installing outside the bundle and asking for root even if it weren't. |
| Bundled `mdsh` as a terminal CLI | Impossible | Every executable in the bundle must be sandboxed. An `inherit` helper only runs under a sandboxed parent, so the bundled binary can't double as the CLI someone runs from Terminal. |
| `xattr -dr com.apple.quarantine` (`cli_darwin.go`) | Impossible and moot | No App Store install is quarantined; the code goes. |
| Sudo password prompts (in design) | Impossible | 2.4.5(v): no escalation to root, and no `sudo` can read the sandboxed process's tty anyway. |
| `ssh` from a block | Password auth only | `/usr/bin/ssh` runs, but `~/.ssh/id_*`, `known_hosts` and the `SSH_AUTH_SOCK` socket are outside the container. |
| 1Password secrets (in design) | Impossible as designed | `op` lives in Homebrew, and the desktop app's agent socket is under `~/.1password`. The only sandbox-safe route is 1Password's URL scheme or an in-app browser flow. |
| Embedded Neovim (`neovim.go`, `nvim/`) | Impossible | `nvim` comes from Homebrew and reads `~/.config/nvim`; both unreadable. |
| Control socket (`ctl/`) | Partly | The socket in the container works for the app itself; an outside `mdsh` can't find or dial it. |
| `mdsh --serve`, remote images, links | Fine | `com.apple.security.network.client` and `.server` entitlements. |
| Webview, clipboard, spelling, open with | Fine | Wails v3 on WKWebView is sandbox-clean. |

Temporary-exception entitlements (`com.apple.security.temporary-exception.files.home-relative-path.read-write` and friends) exist and would reopen `$HOME`, but App Review rejects them without a justification Apple accepts, and "it's a shell" is the justification they reject. None of them grant root.

### Pipeline changes if it were done anyway

Everything in `build/darwin/app.sh` and `gui-packages.yml` stays for the
direct build; the App Store build is a second pipeline beside it:

- Two more certificates: Apple Distribution (app) and Mac Installer
  Distribution (the `.pkg`). Developer ID is not accepted.
- A provisioning profile for the bundle ID, embedded as
  `Contents/embedded.provisionprofile`, and an entitlements plist with
  `app-sandbox`, the network keys, and `inherit` for the nested `mdsh`.
  The hardened runtime and timestamp are allowed but not required.
- `productbuild --component mdsh.app /Applications --sign "Mac Installer
  Distribution: …"` makes the `.pkg`; upload with Transporter or the App
  Store Connect API. No notarization, no stapling, no `.dmg`.
- App Review instead of notarization: days, not minutes, with a human
  reading the sandbox entitlements and trying the Settings › Shell
  button.
- App Store Connect: listing, screenshots, privacy policy URL, age
  rating, a build number that never repeats.
- Receipt validation is optional for a free app; skip it.
- Updates come from the store, so the direct build and the store build
  can't share a version check or an auto-updater.

### Review-guideline risk, beyond the sandbox

- 2.4.5(iii) and (iv): no code or symlinks outside the bundle, no
  installing other software. The whole of Settings › Shell.
- 2.4.5(v): no privilege escalation. The admin prompt today, sudo
  prompts in design.
- 2.5.1: public APIs only. Go, Wails and `pty.Open` are fine.
- 2.5.2: apps "may not download, install, or execute code which
  introduces or changes features". Running the user's own blocks is the
  same ground BBEdit, Script Editor and Automator stand on and has been
  accepted, but a reviewer who sees "run arbitrary shell" in the
  description can flag it, and the appeal costs weeks.
- 5.1.1: a privacy label, and a data-collection answer if 1Password or
  any telemetry ever lands.

### Recommendation

Don't build for the Mac App Store. A sandboxed mdsh would be a markdown
viewer that can run `echo`, with the CLI install, Neovim, ssh keys, sudo
and 1Password all stripped, and the store would still only reach people
who then need the CLI from Homebrew. The direct channel already has what
the store would add: Gatekeeper-clean install (Developer ID +
notarization, shipped in v1.4.0), the cask for discovery, and the
release page for updates. If an auto-updater is ever wanted, Sparkle
works with the current signing and is the usual answer for apps in this
position (iTerm2, Ghostty, Zed and VS Code all stay off the store for
the same sandbox reason).

If a store listing ever matters for discovery alone, the realistic
shape is a separate, reduced product ("mdsh Viewer": render, edit,
outline, no execution) with its own bundle ID, not a sandboxed build of
this app.
