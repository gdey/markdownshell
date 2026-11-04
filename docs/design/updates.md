# Updates for the desktop app

Status: proposed, Nov 4, 2026. Asked for by j after the v1.4.0 release:
"For the desktop apps, we should look into add auto update capabilities",
then "update capabilities, not necessarily auto."

Today nobody who installed the `.dmg` or the AppImage hears about the
next release. The app's About tab says which version it is and links to
the releases page, and that is all. People who installed with Homebrew
get `brew upgrade`; everyone else finds out by chance. The v1.3 release
plan listed an update check (P8) and deferred install-in-app (G10)
until the app was signed. It is signed and notarized as of 1.4.0, so
both are on the table.

This note designs the update path for `mdsh-gui` (Wails, macOS and
Linux), leaves room for the SwiftUI app that is in progress, and keeps
the Mac App Store build, which is being assessed separately, out of it.
The core is user-driven: the app checks, says a version is out, and
installs it when asked. Fully automatic updating is an optional mode,
off by default, in the last phase.

## Goals

- Someone running 1.4.0 learns that 1.4.1 exists within a day of its
  release, without a dialog in their face, and can read what changed.
- One click gets the new version installed and running, on macOS and
  from the Linux AppImage, with the same verification the first install
  had: a notarized app from a signed feed, nothing weaker.
- People who installed with Homebrew keep updating with Homebrew. The
  app tells them how; it doesn't go around their package manager.
- The feed is static files on GitHub releases. No server, no account
  for the app to talk to, no telemetry: the app fetches one small file
  a day and GitHub sees an IP address.
- The same feed and key serve the Swift app through Sparkle, so the two
  apps don't need two release pipelines.
- Checking can be turned off in one switch. Dev builds never check.

Not in scope: Windows (there is no GUI build), updating the `mdsh` CLI
on its own (the formula, `.deb` and `.rpm` have their package managers;
the copy the app installs already follows the app), and an apt or rpm
repository for the GUI, which has no `.deb` today.

## Options

### Sparkle

[Sparkle](https://sparkle-project.org/) is the macOS convention: an
Objective-C framework that reads an RSS "appcast", downloads the
archive, verifies an EdDSA (Ed25519) signature and the Developer ID
signature, swaps the bundle and relaunches. It is what most non-App
Store Mac apps use, including Wails apps that went to the trouble.

For `mdsh-gui` the trouble is real. The app is Go; Sparkle would come
in through cgo with an Objective-C shim (there are Go wrappers for
Sparkle on GitHub, none widely used), as `Contents/Frameworks/Sparkle.framework`
in the bundle, which `app.sh` would have to sign as nested code with the
hardened runtime (Sparkle 2's installer helpers are XPC services and an
agent, each signed in turn) before notarization. It would be the first
non-Go dependency in the bundle and the first thing `go mod vendor`
can't carry. And it does nothing for Linux, which needs its own path
regardless.

For the Swift app it is the right choice: a Swift Package dependency,
two `Info.plist` keys and a menu item. The design below keeps the feed
in Sparkle's format so that is all it takes.

### A Go updater, fed from GitHub releases

The app fetches a feed over HTTPS, compares versions, downloads the
release asset it already ships (`mdsh-gui_VERSION_macos_universal.zip`,
`mdsh-gui_VERSION_linux_x86_64.AppImage`), verifies it, swaps it into
place and relaunches. Everything is Go standard library (`net/http`,
`encoding/xml`, `crypto/ed25519`), one package shared by both
platforms, and tests run against `httptest` and a temp folder. The verification is the same two
checks Sparkle makes. This is the recommended path for `mdsh-gui`; the
rest of this note is mostly its design.

### Homebrew and apt

`brew upgrade --cask mdsh` already works, driven by the tap that
`.github/scripts/tap.sh` writes when a release is published. It stays
the update path for anyone who installed that way, and the app detects
that install and defers to it (see [Homebrew](#homebrew-installs)).
There is no `.deb` of the GUI and no apt repository; adding one is a
packaging project of its own and would still leave the AppImage, which
is the only Linux build today, without updates. Not pursued here.

### Linux: AppImage and the rest

An AppImage is one file, which makes replacing it simple, and the
AppImage ecosystem has its own update convention: a line of update
information embedded in the image (`gh-releases-zsync|gdey|markdownshell|latest|mdsh-gui_*_linux_x86_64.AppImage.zsync`)
plus a `.zsync` file on the release, which AppImageUpdate, AppImageLauncher
and Gear Lever use to fetch only the blocks that changed. The app's own
updater downloads the whole image (98 MB for 1.4.0); embedding the
update information costs one environment variable in `appimage.sh` and
gives the delta route to the tools that have it. Both are in the plan.

Flatpak and Snap would bring their own updaters, and their own sandbox
rules, which a program that runs the user's shells sits badly in. Not
pursued.

## The feed

One file per app, attached to every release and reachable at a fixed
address:

```
https://github.com/gdey/markdownshell/releases/latest/download/appcast.xml
```

GitHub redirects `releases/latest/download/NAME` to that asset of the
newest published, non-prerelease release, so the address never changes,
there is no API call and no rate limit to think about, and a draft
release (which is what goreleaser makes) isn't visible until it is
published, the same moment the tap workflow runs. Pre-releases are
reachable by a second address, `releases/download/vX.Y.Z-rc.1/appcast.xml`;
the app only looks there when the setting asks for them.

The file is a Sparkle appcast, so the Swift app can read it unchanged:

```xml
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>mdsh</title>
    <item>
      <title>1.4.1</title>
      <pubDate>Wed, 12 Nov 2026 17:55:26 +0000</pubDate>
      <sparkle:version>1.4.1</sparkle:version>
      <sparkle:shortVersionString>1.4.1</sparkle:shortVersionString>
      <sparkle:minimumSystemVersion>13.0</sparkle:minimumSystemVersion>
      <sparkle:releaseNotesLink>https://github.com/gdey/markdownshell/releases/tag/v1.4.1</sparkle:releaseNotesLink>
      <description><![CDATA[ …the release notes as HTML… ]]></description>
      <enclosure url="https://github.com/gdey/markdownshell/releases/download/v1.4.1/mdsh-gui_1.4.1_macos_universal.zip"
                 length="45304495" type="application/octet-stream"
                 sparkle:edSignature="…base64 Ed25519 signature of the file…"/>
    </item>
  </channel>
</rss>
```

A second `<enclosure>`-bearing item with `sparkle:os="linux"` isn't
Sparkle's (it knows macOS and Windows), so the Linux image goes in its
own element the Go side reads and Sparkle ignores:

```xml
      <mdsh:asset os="linux" arch="x86_64"
                  url="…/mdsh-gui_1.4.1_linux_x86_64.AppImage"
                  length="98073080" edSignature="…"/>
```

with `xmlns:mdsh` on the root. The feed lists only the release it
belongs to; the app wants the newest, and that is what `latest` is.
Sparkle is content with one item.

The release notes come from the `CHANGELOG.md` section the release
workflow already extracts into `notes.md`, rendered to HTML with
goldmark (a root-module dependency already), so the app can show
"what's new in 1.4.1" before installing it, from a build that only
knows its own changelog. The link points at the release page for the
same text.

### Signing the feed

Two signatures, each covering what the other can't:

- **Ed25519 over each asset**, in the appcast as `sparkle:edSignature`,
  which Sparkle verifies and the Go updater verifies with
  `crypto/ed25519`. Sparkle's EdDSA is plain Ed25519 over the file's
  bytes, base64-encoded, and its public key is the 32-byte key
  base64-encoded in `SUPublicEDKey`; a signature made with Go's
  `ed25519.Sign` verifies in Sparkle, and the reverse. (To be confirmed
  against Sparkle's `sign_update` on a test file when implementing,
  before the key is minted.) The public key is compiled into `mdsh-gui`
  and written into the Swift app's `Info.plist`; the private key is a
  repository secret, `APPCAST_ED25519_KEY`, and lives in j's password
  manager. It never touches a Mac keychain or a laptop checkout.
- **Developer ID, on macOS**, checked after the download is unpacked:
  the new bundle must verify with `codesign --verify --strict`, must be
  signed by the same Team ID as the running app, and must pass
  `spctl --assess --type execute` (notarized). An attacker who had the
  Ed25519 key and a way to serve a feed would still need Apple's
  signature. On Linux there is no second signature; Ed25519 is the
  check.

The appcast itself isn't signed: the signatures are on the assets, the
feed is fetched over HTTPS from github.com, and a forged feed can only
offer assets the key signed. The version comparison refuses anything
not newer than the running build, so a feed can't downgrade an app
either.

The key is minted once, on j's Mac, by the same Go tool that signs
(`go run ./.github/scripts/appcast -keygen`), so the format is the
tool's own and there is nothing to convert. If it ever has to be
rotated, a release signed with the old key ships the new public key,
and the one after signs with the new one; the public key in the app is
a list, newest first, so that release is an ordinary one.

### Making it in the release workflow

The release job has the GUI packages in `gui-dist/` before goreleaser
runs, and goreleaser already attaches everything in that folder. One
step between the two, after Release notes from CHANGELOG.md:

```yaml
      - name: Appcast
        env:
          APPCAST_ED25519_KEY: ${{ secrets.APPCAST_ED25519_KEY }}
        run: go run ./.github/scripts/appcast -version "${GITHUB_REF_NAME#v}" \
               -notes "$RUNNER_TEMP/notes.md" -out gui-dist gui-dist/*
```

`.github/scripts/appcast` is a small `main` package: it reads the
assets, signs each, renders the notes, writes `gui-dist/appcast.xml`
(and, when the Swift app ships, `appcast-swift.xml` for its own
archive). Standard library plus goldmark, so nothing to vendor. Without
the secret it writes a feed with no signatures and the job fails, so a
release can't go out unsigned by accident. `checksums.txt` stays as it
is; the tap and the docs use it.

The AppImage's update information and `.zsync` come from `appimage.sh`:
linuxdeploy's AppImage plugin embeds the string and writes the
`.zsync` beside the image when its update-information variable is set
(`LDAI_UPDATE_INFORMATION`; the plugin's README has the exact name to
use, to be checked when implementing). The `.zsync` goes into
`gui-dist/` with the image, and the release gets it like the rest.

## How the app checks

A `cmd/mdsh-gui/update` package, platform files for the install step.

**When.** At launch, after the first window is up and ten seconds have
passed, if the last check was more than 24 hours ago, and then once a
day while the app stays open. Never on a `dev` build (a `go run` or a
checkout build, whose version is `dev` or a module pseudo-version) and
never on the first launch after install: the first check waits for the
second run, so someone who opens Settings first sees the switch before
the app has talked to anyone. Check for Updates… in the menu checks
right away, whatever the clock says.

**What it sends.** One `GET` of the appcast with a `User-Agent` of
`mdsh-gui/1.4.0 (darwin; arm64)`, following redirects only to
`github.com` and `objects.githubusercontent.com`, with a 20 second
timeout. The setting's text says this much: "Checks github.com once a
day for a new version. Nothing about you is sent."

**What it keeps.** `update.json` in the app's data folder (beside the
settings, which stay the user's own file): the time of the last check,
the newest version seen with its notes, a version the user chose to
skip, and the path of a download that is ready to install. The feed's
strings are data: version strings are parsed as semver and rejected
otherwise, URLs must be `https` on the two hosts above, lengths cap
the download, and nothing from the feed becomes a file name (the
download is named by the app from the version it parsed).

**Comparison.** `sparkle:version` against the running build's version,
as semver with pre-release ordering, so `1.5.0-rc.1` is newer than
`1.4.1` and older than `1.5.0`. Equal or older is "up to date". The
`minimumSystemVersion` is compared against the running macOS, and a
too-old Mac sees "1.5.0 needs macOS 14; this Mac has 13" instead of an
Update button.

## Installing

### macOS

The asset is the `.zip`, not the `.dmg`: a disk image has to be
mounted, and the zip already exists on every release, made by `ditto`
from the stapled app. In order:

1. Download to `~/Library/Application Support/mdsh/updates/1.4.1.zip`,
   with progress, resumable if the window closes. Check the length and
   the Ed25519 signature over the whole file.
2. Unpack with `ditto -x -k` into a folder beside it. `ditto` is on
   every Mac, keeps the bundle exactly as it was zipped, and is what
   made the zip; Go's `archive/zip` would do but would be a second
   opinion on symlinks and attributes for no gain.
3. Verify the bundle: `codesign --verify --strict --verbose=2`, the Team
   ID from `codesign -dv` equal to the running app's, and
   `spctl --assess --type execute`. Any failure deletes the download,
   says "the download didn't verify" in the About hero and the Console,
   and leaves the installed app alone.
4. Check where the running app is. If its path starts with `/Volumes/`
   it is running from the `.dmg` and the hero says "Move mdsh to
   Applications, then update". If the parent folder isn't writable
   (an app a different user put in `/Applications`), the swap runs
   through `osascript … with administrator privileges` the way
   Install Command Line Tool does, with the same quoting and the same
   test.
5. Swap: move `/Applications/mdsh.app` to `updates/old/mdsh.app`, move
   the new bundle to `/Applications/mdsh.app`. A running app's files
   stay open across the rename, so the old process keeps working
   until it quits. Both moves are on one volume, so each is a single
   `rename`.
6. Relaunch: `open -n /Applications/mdsh.app --args --after-update`,
   then quit through the normal path, which asks about unsaved files
   and running blocks exactly as Quit does. The new process waits for
   the old one to exit, reopens the windows the old one listed in
   `update.json` before quitting (files and their places, which the
   settings already remember), deletes `updates/old` and the zip, and
   the About hero says "Updated to 1.4.1".

Things this keeps working, each checked in the test pass:

- **Gatekeeper.** Go's `net/http` doesn't set `com.apple.quarantine`
  (only apps that opt in with `LSFileQuarantineEnabled`, or that
  download through the system's own session API, do), so the new
  bundle isn't quarantined; and even quarantined it would pass, since
  it is notarized and the ticket is stapled inside the zip. App
  translocation only applies to quarantined bundles, so a swap at the
  same path doesn't trigger it either. The first launch of the new
  version has no dialog, the same as a fresh install from 1.4.0 on.
- **The command line tool.** `/usr/local/bin/mdsh` points into
  `/Applications/mdsh.app`, which is the same path after the swap. The
  launch-time repair from the CLI design covers an app that was
  somewhere else. A `~/.local/bin` copy doesn't exist on macOS.
- **Permissions.** macOS ties privacy grants (Full Disk Access, if
  someone gave it) to the bundle's code signing identity, not its
  inode, so a Developer ID build at the same path keeps them. Sparkle
  relies on the same thing.
- **The cask.** See [Homebrew installs](#homebrew-installs): a cask
  install never reaches this step.

### Linux

The AppImage runtime sets `APPIMAGE` to the image's path and `APPDIR`
to its mount, which the CLI install already reads. In order:

1. Download to `~/.local/share/mdsh/updates/mdsh-gui_1.4.1_linux_x86_64.AppImage`,
   check the length and the Ed25519 signature, `chmod +x`.
2. If the folder holding `$APPIMAGE` isn't writable (an image in
   `/opt`, put there by root), the hero says so and offers to open the
   download's folder instead; the user moves it.
3. Rename the download over `$APPIMAGE`, keeping the user's file name.
   The name is what their launcher, desktop file or AppImageLauncher
   entry knows, and a version in the old name is a small wrong that
   fixes itself the next time they rename it; the alternative, a new
   file beside the old, leaves two images and a launcher pointing at
   the stale one. The old image stays mounted for the running process
   because the rename replaces the directory entry, not the inode.
4. Relaunch: exec `$APPIMAGE --after-update` with the runtime's
   variables (`APPIMAGE`, `APPDIR`, `OWD`, `ARGV0`) cleared so the new
   runtime sets its own, then quit the same way as on macOS.
5. The copy of `mdsh` in `~/.local/bin`, when the app made it, is
   refreshed on the next launch by the existing version check. The
   `.zsync` delta route is for the outside tools; the app downloads
   the whole image, which is honest about what it costs (the hero
   shows the size before the click).

### Homebrew installs

A cask install puts the app in `/Applications` like a drag install,
and a `brew upgrade` later would quietly swap it again. Two updaters on
one bundle is one too many, so the app finds out which it is: a
`mdsh` folder under the Caskroom (`/opt/homebrew/Caskroom/mdsh` or
`/usr/local/Caskroom/mdsh`) whose newest version folder names the
running version. Then the update check still runs and still says
"1.4.1 is available", but the button is **Copy brew command** and the
line under it is `brew upgrade --cask mdsh`, which is also what the
Shell tab's Command line tool row says about the formula. The cask
keeps its `auto_updates` unset, so `brew upgrade` keeps upgrading it.

The same applies to the Mac App Store build, if that assessment says
yes: a store app that replaces itself is rejected in review, so the
`appstore` build tag compiles the updater out and About says "Updates
come from the App Store".

## What it looks like

Nothing new is a dialog. The HIG's rule is that only the person's own
action gets to interrupt, and a new version is news, not a problem.

**Settings › General** gets an Updates group, under the existing
rows, in the tab's row style:

```
Updates
  [x] Check for updates once a day
      Checks github.com for a new version. Nothing about you is sent.
  [ ] Include pre-releases
      Last checked today at 9:12. You have the latest version, 1.4.0.      [Check Now]
```

The last line is the status, and changes with it: "1.4.1 is available",
"Couldn't reach github.com; last checked yesterday", "Checks are off for
dev builds", "Updates come from Homebrew: brew upgrade --cask mdsh".
Phase 3 adds a third switch, off by default:

```
  [ ] Download and install updates automatically
      The new version is installed the next time the app quits.
```

**Settings › About**: the hero (icon, name and version, build line)
gets a Check for Updates button beside Copy, as the v1.3 wireframe drew
it, and a line under the build line that is the update's state:

| State | Line | Button |
| --- | --- | --- |
| Up to date | You have the latest version. Checked today at 9:12. | Check for Updates |
| Checking | Checking… | (disabled) |
| Available | **1.4.1 is available.** What's new ↓ | Update to 1.4.1 · Skip this version |
| Downloading | Downloading 1.4.1… 23 of 45 MB | Cancel |
| Verifying | Verifying… | |
| Ready | 1.4.1 is ready. Relaunch to finish. | Relaunch Now · Later |
| Failed | The download didn't verify. Nothing was changed. | Try Again |
| Homebrew | 1.4.1 is available. Update with Homebrew: `brew upgrade --cask mdsh` | Copy |
| Too old | 1.5.0 needs macOS 14; this Mac has 13. | Open release page |
| Dev build | Checks are off for dev builds. | |

"What's new" opens the release notes from the feed under the hero, in
the same read-only style as the built-in changelog, so the person reads
what 1.4.1 changes before updating. Skip this version keeps 1.4.1 out
of the status lines until something newer appears; Check for Updates
asks again regardless. "Later" leaves the download ready; the app
installs it on its next quit, and says so on the start screen line.

**The start screen** gets one quiet line at the bottom when a version
is available, in the muted style the pane uses for its folder line:
"1.4.1 is available · Update…", which opens About. It goes away on
Skip, on update, and on nothing else; it never animates in.

**Menus.** macOS: Check for Updates… in the app menu, between About
mdsh and Settings…, where every Mac app has it. Linux: the same item
in the Help menu, under Changelog. Both open About and start a check.

**Relaunching** goes through Quit, so an unsaved file gets the
Save / Don't Save / Cancel dialog it gets today, and a running block
gets today's question. Cancel there cancels the relaunch; the download
stays ready.

**Accessibility.** The hero line is the About tab's polite live region,
so a screen reader hears "1.4.1 is ready. Relaunch to finish" when it
changes; the buttons have the same labels as their text; progress is a
`<progress>` element with the bytes in its label.

## The Swift app

When mdsh-swift reaches a release, it adds Sparkle as a package
dependency, sets `SUFeedURL` to `…/releases/latest/download/appcast-swift.xml`
and `SUPublicEDKey` to the same public key, and gets Check for Updates…
from Sparkle's standard updater controller. The appcast tool writes
`appcast-swift.xml` from that app's zip with the same signature, so
the release workflow changes by one input. Sparkle's own UI is fine
there; a SwiftUI app is where it looks native. Its Mac App Store
variant, if there is one, leaves Sparkle out the same way.

## Phases

| Phase | What | Where | Effort |
| --- | --- | --- | --- |
| 0 | **The feed.** The appcast tool, the Ed25519 key and secret, the workflow step; `appimage.sh` embeds update information and emits the `.zsync`. Ships with the next release so there is something for phase 1 to read. No app change. | `.github/scripts/appcast/`, `release.yml`, `appimage.sh`, `docs/signing.md` (the key, beside the Apple ones) | S |
| 1 | **Check and tell.** The `update` package's fetch, parse, compare and state; Settings › General's Updates group; the About hero's line, What's new and Skip; the start screen line; Check for Updates… in the menus; Homebrew detection. Update to 1.4.1 opens the release page in the browser. | `cmd/mdsh-gui/update/`, `settings/`, `frontend/settings.html`, `settings.ts`, `app.ts`, `menu.go`, `about.go` | M |
| 2 | **Download and install.** The macOS zip path and the Linux AppImage path above, the swap, the relaunch with reopened windows, the Team ID check, the admin fallback. The button becomes Update to 1.4.1. | `update/download.go`, `update/install_darwin.go`, `update/install_linux.go`, `main.go` (`--after-update`) | M |
| 3 | **Automatic mode and the Swift app.** The Download and install automatically switch with install-on-quit; Sparkle in mdsh-swift reading `appcast-swift.xml`. | `settings/`, `update/`, the Swift app | S each |

Phase 1 is worth shipping on its own; it is the v1.3 plan's P8 and most
of the value. Phase 2 is where the design above earns its length.

Dependencies: none new. Everything is the standard library, goldmark
(already in the root module) for the notes, and `ditto`, `codesign`,
`spctl` and `open` on macOS, which are always there.

## Testing

- `update` package: version comparison (semver, pre-releases, garbage),
  appcast parsing (both elements, missing fields, wrong hosts, an
  `http` URL, a 1 GB length), Ed25519 verification with a test key,
  the once-a-day clock, Skip, and the state file, all against
  `httptest` and temp folders.
- The appcast tool: a golden `appcast.xml` from fixed inputs; a
  round-trip (`-keygen`, sign, verify in the app's package); and, once,
  by hand on a Mac, Sparkle's `sign_update` and the tool agreeing on a
  file.
- The swap, on each platform, in a temp folder standing in for
  `/Applications` or the image's folder: the moves, the rollback when
  the second move fails, the read-only folder, the `/Volumes/` case.
- End to end on j's Mac and on a Linux box with the AppImage: build a
  `1.4.0` from the tag, point the app at a local feed
  (`MDSH_UPDATE_FEED=file:///…`, which only a non-release build or
  Settings' hidden developer row honours) advertising a locally signed
  `1.4.1`, and go through the hero's states, the relaunch with two
  files open, the cask case (install the cask, run the check), and the
  dmg-mounted case.
- The site's desktop page and `cmd/mdsh-gui/README.md` say how updates
  work and how to turn them off; `CHANGELOG.md` under Added, with a
  screenshot of the About hero and the Settings group.

## Open questions

- **On by default.** The design checks once a day without asking, from
  the second launch, with the switch and its sentence in Settings.
  Sparkle asks on the second launch instead ("Check automatically?").
  Recommendation: on by default, no question; the request carries no
  identity and the switch is where someone expecting one looks. j's
  call.
- **Who holds the key.** The Ed25519 private key is as important as the
  Developer ID certificate: with it, and write access to the releases,
  someone can ship an update. The design puts it in the same password
  manager and the same secrets page, and the Team ID check means the
  key alone isn't enough on macOS. Whether to go further (a hardware
  key, signing on j's Mac rather than in CI) is a question of how much
  the release workflow should be able to do on its own.
- **Where the Linux image lives.** Replacing the image in place keeps
  the user's launcher working and is what AppImageUpdate does too;
  the cost is a file name with the old version in it until they
  rename it. The alternative (new file beside the old, old deleted
  on next launch) was rejected above for leaving launchers stale.
  Worth a second opinion from someone who runs AppImages daily.
- **Release notes in the feed.** HTML rendered from `CHANGELOG.md` is
  a second rendering of the same text (the site has one). If the two
  drift in look, the feed could carry only the link and the hero open
  the release page; the design keeps the in-app notes because reading
  before updating is the point.
