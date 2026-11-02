# Signing and notarizing the macOS app

How to get mdsh.app (and, once [the CLI is in the
bundle](design/cli-in-app.md), `mdsh` with it) signed with a Developer
ID and notarized, so that it opens on a Mac without the Open Anyway
dance. The release workflow already knows how to do it: it signs and
notarizes when the repository has the secrets listed below, and signs
ad hoc, as today, until then. What is left is the part only an Apple
account holder can do. In order:

1. [Join the Apple Developer Program](#1-join-the-apple-developer-program).
2. [Create a Developer ID Application certificate](#2-create-the-developer-id-application-certificate).
3. [Export it as a .p12 for CI](#3-export-the-certificate-for-ci).
4. [Create an App Store Connect API key](#4-create-an-app-store-connect-api-key-for-notarytool) for notarytool.
5. [Add the secrets to the repository](#5-add-the-github-secrets).
6. [Check a build](#6-check-a-build), then tag a release.

Then [after the first signed release](#after-the-first-signed-release),
a few pages need their "not signed yet" notes taken out.

Budget about an hour at the keyboard plus Apple's enrollment wait, and
US$99 a year.

## What the pieces are

- **Developer ID Application certificate.** The certificate `codesign`
  signs with. Apple issues it to the Account Holder of a Developer
  Program membership; the private key is made on your Mac, so Apple
  never has it and you have to carry it to CI yourself. It is good for
  five years, and signatures made with a secure timestamp outlive it.
- **Notarization.** Apple's automated scan of a signed build. It isn't
  App Review: it checks that everything is signed with a Developer ID,
  uses the hardened runtime, has a timestamp, and carries no malware,
  and takes a few minutes. The result is a ticket, published online
  and stapled to the app, that Gatekeeper looks for when the app is
  first opened. Since macOS 10.15 a Developer ID app that isn't
  notarized is blocked just like an unsigned one, so the two go
  together.
- **App Store Connect API key.** What `notarytool` authenticates with
  in CI. A team key with the Developer role is enough. The alternative,
  an app-specific password for your Apple ID, works too but ties the
  workflow to a personal login and 2FA; the key doesn't.
- **Stapling.** Attaching the ticket to the app and the `.dmg`, so they
  pass Gatekeeper offline.

Apple's overview: [Notarizing macOS software before distribution][notarize]
and [Signing Mac software with Developer ID][devid]. Wails' guide,
which covers the same steps for its own build tool: [macOS packaging:
code signing][wails].

## 1. Join the Apple Developer Program

Enroll at [developer.apple.com/programs/enroll][enroll] with the Apple
ID the certificate should belong to. Developer ID certificates are
only issued to the **Account Holder**, so an individual membership in
your own name is the simplest; an organization needs a D‑U‑N‑S number
and takes longer. Enrollment is US$99 a year and is usually approved
within 48 hours, sometimes after a phone call. You'll need two-factor
authentication on the Apple ID, and the Apple Developer app or the
website to accept the agreements; a new "Developer ID" or "Apple
Developer Program License Agreement" update has to be accepted before
certificates can be made, so check [the account page][account] for a
banner first.

## 2. Create the Developer ID Application certificate

Apple's steps: [Create Developer ID certificates][create-cert]. You
need a Mac with Xcode or the command line tools.

1. Make a certificate signing request with Keychain Access: Keychain
   Access › Certificate Assistant › Request a Certificate From a
   Certificate Authority. Enter your email and a name (something like
   "mdsh Developer ID"), leave CA Email empty, choose **Saved to
   disk**, and save the `.certSigningRequest`. This also makes the
   private key, in your login keychain. ([Apple's steps][csr].)
2. In [Certificates, Identifiers & Profiles › Certificates][certs],
   click +, choose **Developer ID Application** under Software, and
   continue. Pick the **G2 Sub-CA** profile type if it asks (the
   current one; the older Sub-CA is only for signing kernel
   extensions for old macOS).
3. Upload the `.certSigningRequest`, continue, and download the
   `.cer`.
4. Double-click the `.cer` to add it to Keychain Access. Under My
   Certificates it should show "Developer ID Application: Your Name
   (TEAMID)" with a private key under it. If it has no key, it was
   opened on a different Mac (or user) from the one that made the CSR.

The name in quotes, including the team ID, is the signing identity.
Check it works:

```sh
security find-identity -v -p codesigning
#  1) 0123ABCD... "Developer ID Application: Your Name (TEAMID)"
```

If it isn't listed as valid, install Apple's intermediate certificate
from [Apple PKI][pki] (Developer ID - G2) and try again. The CI
workflow does that for itself.

You can have five of these at a time; a lost key means revoking the
certificate and making a new one, which doesn't break builds already
signed and notarized.

## 3. Export the certificate for CI

The workflow needs the certificate and its private key as a
password-protected `.p12`, base64-encoded in a secret.

1. In Keychain Access › My Certificates, select the Developer ID
   Application certificate (the row with the key under it, not the
   key itself), right-click › **Export**, choose the `.p12` format, and
   save it. Set a strong password when asked; the workflow needs it
   too. Keychain Access asks for your login password to let the key
   out.
2. Encode it for the secret and put it on the clipboard:

   ```sh
   base64 -i DeveloperID.p12 | pbcopy
   ```

3. Keep the `.p12` somewhere safe (a password manager), or delete it:
   the key stays in your keychain and can be exported again. Don't
   commit it.

Exporting from the command line does the same:
`security export -t identities -f pkcs12 -o DeveloperID.p12 -P
'password'` after finding the identity in the keychain, but Keychain
Access is less fiddly.

## 4. Create an App Store Connect API key for notarytool

Apple's steps: [Creating API keys for App Store Connect
API][asc-keys]. It has to be a **team** key; individual keys can't use
`notarytool`.

1. Sign in to [App Store Connect][asc] › Users and Access ›
   Integrations › App Store Connect API › Team Keys. The first time, it
   asks you to request access and accept terms.
2. Click + (Generate API Key), name it (say "mdsh notarization"), give
   it the **Developer** role, and generate.
3. Note the **Issuer ID** at the top of the page and the key's **Key
   ID**, then **Download API Key**. The download is a `.p8` file named
   `AuthKey_KEYID.p8`, and the link works **once**: Apple keeps no
   copy. Keep the file with the `.p12`.

Try it on your Mac before putting it in CI, with a signed zip of
anything (the step after next shows a full local build):

```sh
xcrun notarytool history --key AuthKey_KEYID.p8 --key-id KEYID --issuer ISSUER
```

An empty history with no error means the key, ID and issuer agree.

## 5. Add the GitHub secrets

In the repository's Settings › Secrets and variables › Actions, add
these [repository secrets][gh-secrets]. The release workflow passes
them to the GUI packages workflow with `secrets: inherit`, and the
GUI packages workflow decides from them whether to sign
(`MACOS_CERTIFICATE_P12` set) and whether to notarize (that and
`APP_STORE_CONNECT_KEY` set). Leaving them unset keeps the ad hoc
build, and pull requests from forks never see secrets, so their
builds stay ad hoc too.

| Secret | What |
| --- | --- |
| `MACOS_CERTIFICATE_P12` | The `.p12` from step 3, base64-encoded (what `pbcopy` got). |
| `MACOS_CERTIFICATE_PASSWORD` | The `.p12`'s password. |
| `APP_STORE_CONNECT_KEY` | The contents of the `.p8` from step 4, as is (`pbcopy < AuthKey_KEYID.p8`). |
| `APP_STORE_CONNECT_KEY_ID` | Its Key ID. |
| `APP_STORE_CONNECT_ISSUER_ID` | The Issuer ID. |

The signing identity isn't a secret: the workflow reads it from the
certificate.

## 6. Check a build

Run the **GUI packages** workflow by hand from the Actions tab
(workflow_dispatch). The macOS job prints "Signing as Developer ID
Application: …", notarizes the app and the `.dmg`, and ends with a
notice saying the app is signed and notarized. Download the
`gui-macos` artifact, unzip it, and on a Mac:

```sh
spctl --assess --type execute --verbose=2 mdsh.app
# mdsh.app: accepted
# source=Notarized Developer ID
codesign --verify --strict --verbose=2 mdsh.app
xcrun stapler validate mdsh.app
xcrun stapler validate mdsh-gui_*_macos_universal.dmg
```

Then open the `.dmg` and the app as a user would: a fresh download
should open with no Gatekeeper dialog at all. When that's good, tag a
release as usual; the release's `.dmg` and `.zip` come from the same
job.

## What the workflow does

`.github/workflows/gui-packages.yml`'s macOS job, when the secrets are
set:

1. Makes a temporary keychain with a random password, imports the
   `.p12` and Apple's Developer ID G2 intermediate into it, and lets
   `codesign` use the key without a prompt
   (`security set-key-partition-list`). This is GitHub's own recipe,
   [Installing an Apple certificate on macOS runners][gh-cert]. The
   keychain is deleted at the end of the job, even when a step fails.
2. Writes the `.p8` to a file and runs `build/darwin/app.sh` with
   `MDSH_SIGN_IDENTITY`, `MDSH_NOTARY_KEY`, `MDSH_NOTARY_KEY_ID` and
   `MDSH_NOTARY_ISSUER` set.

`cmd/mdsh-gui/build/darwin/app.sh` then does the signing itself, so
the same thing can be run on a Mac (below). With an identity:

```sh
# The app, with the hardened runtime and a secure timestamp: both are
# required for notarization. Nested code (a helper binary in the
# bundle) is signed first, with the same options; see the CLI-in-app
# design.
codesign --force --options runtime --timestamp \
  --sign "Developer ID Application: Name (TEAMID)" mdsh.app
codesign --verify --strict --verbose=2 mdsh.app

# Notarize the app from a zip, and staple the ticket to the app.
ditto -c -k --keepParent mdsh.app notarize.zip
xcrun notarytool submit notarize.zip --wait \
  --key AuthKey.p8 --key-id KEYID --issuer ISSUER
xcrun stapler staple mdsh.app
spctl --assess --type execute --verbose=2 mdsh.app

# The .zip and the .dmg that ship are made from the stapled app. The
# .dmg is signed, notarized and stapled too, so Gatekeeper is happy
# with the image itself.
ditto -c -k --keepParent mdsh.app mdsh-gui_VERSION_macos_universal.zip
dmgbuild … mdsh-gui_VERSION_macos_universal.dmg
codesign --force --timestamp --sign "Developer ID Application: …" mdsh-gui_VERSION_macos_universal.dmg
xcrun notarytool submit mdsh-gui_VERSION_macos_universal.dmg --wait --key … --key-id … --issuer …
xcrun stapler staple mdsh-gui_VERSION_macos_universal.dmg
```

If notarization isn't accepted, the script prints the submission's
log (`notarytool log`), which lists each file and what was wrong with
it. Apple's [Resolving common notarization issues][issues] explains the
messages.

No entitlements file: a Go binary needs none under the hardened
runtime (no JIT, no unsigned libraries), WebKit's own helper processes
are Apple's, and the app isn't sandboxed because it runs the user's
shells. `--deep` isn't used; Apple deprecates it for signing, and the
bundle's nested code is signed explicitly.

### On a Mac

The same build, signed and notarized, from `cmd/mdsh-gui` with the
certificate in your login keychain:

```sh
MDSH_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" \
MDSH_NOTARY_KEY=~/path/to/AuthKey_KEYID.p8 \
MDSH_NOTARY_KEY_ID=KEYID MDSH_NOTARY_ISSUER=ISSUER \
build/darwin/app.sh 1.4.0 dist
```

Set only `MDSH_SIGN_IDENTITY` to sign without notarizing (the app is
then still blocked by Gatekeeper on download, which is useful for
checking the signature and nothing else). The first `codesign` asks
to use the key; choose Always Allow.

## After the first signed release

- `site/content/install.md` and `site/content/docs/desktop.md` say the
  app isn't signed yet and how to get past Gatekeeper; cut that to a
  line, and the `--no-quarantine` tip goes.
- `cmd/mdsh-gui/README.md` › Install and Packages say "unsigned" and
  "signed ad hoc".
- `CHANGELOG.md`: say the app is now signed and notarized, under
  Changed.
- The [CLI-in-app design](design/cli-in-app.md)'s open question about
  Open Anyway and the helper stops mattering: a notarized bundle's
  helper runs from a terminal without a prompt. Its signing step
  (`sign "$app/Contents/MacOS/mdsh"` before `sign "$app"`) slots into
  `app.sh`'s `sign` function.
- Older releases can be notarized too, without rebuilding, by
  submitting their `.dmg`; Apple recommends it but it isn't needed.

## Troubleshooting

- **"MACOS_CERTIFICATE_P12 has no Developer ID Application
  certificate"** in CI: the `.p12` was exported from the certificate
  without its key (export the row that has the key under it), or the
  secret isn't the base64 of the file.
- **errSecInternalComponent** from `codesign` in CI: the key's
  partition list wasn't set, or the keychain isn't in the search
  list. Both are in the import step; check it ran.
- **"The timestamp service is not available"**: `--timestamp` needs
  to reach Apple; retry.
- **Notarization "Invalid"** with "The binary is not signed with a
  valid Developer ID certificate" or "The signature does not include
  a secure timestamp": a binary in the bundle was missed. The log
  names the path.
- **A notarized build still gets "cannot be opened"** on a Mac: it was
  copied before stapling, or the ticket isn't online yet. `xcrun
  stapler validate` says whether a ticket is attached, and `spctl
  --assess` says what Gatekeeper thinks.
- The notary service's own status is on Apple's [system status
  page][status] under Developer ID Notary Service.

[notarize]: https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution
[devid]: https://developer.apple.com/developer-id/
[wails]: https://v3.wails.io/guides/build/macos/
[enroll]: https://developer.apple.com/programs/enroll/
[account]: https://developer.apple.com/account
[create-cert]: https://developer.apple.com/help/account/certificates/create-developer-id-certificates/
[csr]: https://developer.apple.com/help/account/certificates/create-a-certificate-signing-request
[certs]: https://developer.apple.com/account/resources/certificates/list
[pki]: https://www.apple.com/certificateauthority/
[asc-keys]: https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api
[asc]: https://appstoreconnect.apple.com/
[gh-secrets]: https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions
[gh-cert]: https://docs.github.com/en/actions/deployment/deploying-xcode-applications/installing-an-apple-certificate-on-macos-runners-for-xcode-development
[issues]: https://developer.apple.com/documentation/security/resolving-common-notarization-issues
[status]: https://developer.apple.com/system-status/
