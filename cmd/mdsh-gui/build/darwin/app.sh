#!/bin/sh
# Builds mdsh-gui as a universal (arm64 and amd64) macOS app, mdsh.app,
# in a .dmg and a .zip:
#
#	build/darwin/app.sh VERSION OUTDIR
#
# Run it from cmd/mdsh-gui on macOS with the Xcode command line tools.
#
# Without anything else set, the app is signed ad hoc, not with a
# Developer ID, so the first time it opens macOS asks first (right-click
# › Open, or System Settings › Privacy & Security › Open Anyway). To sign
# and notarize it (docs/signing.md walks through getting these):
#
#	MDSH_SIGN_IDENTITY   "Developer ID Application: Name (TEAMID)", in a
#	                     keychain codesign can use. Signs the app with the
#	                     hardened runtime and a secure timestamp.
#	MDSH_NOTARY_KEY      Path to an App Store Connect API key (.p8), with
#	MDSH_NOTARY_KEY_ID   its key ID and
#	MDSH_NOTARY_ISSUER   issuer ID. With all three and an identity, the
#	                     app is notarized and stapled, and so is the .dmg.
set -eu
version=$1
mkdir -p "$2"
out=$(cd "$2" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

identity=${MDSH_SIGN_IDENTITY:-}
notarize=
if [ -n "$identity" ] && [ -n "${MDSH_NOTARY_KEY:-}" ]; then
	notarize=1
	: "${MDSH_NOTARY_KEY_ID:?set with MDSH_NOTARY_KEY}" "${MDSH_NOTARY_ISSUER:?set with MDSH_NOTARY_KEY}"
fi

# sign PATH signs one piece of code: Developer ID with the hardened
# runtime and a timestamp (both required for notarization) when an
# identity is set, ad hoc otherwise. Nested code is signed inside out,
# so a helper in the bundle is signed before the app.
sign() {
	if [ -n "$identity" ]; then
		codesign --force --options runtime --timestamp --sign "$identity" "$1"
	else
		codesign --force --sign - "$1"
	fi
}

# notarize PATH submits a .zip or .dmg to Apple's notary service and
# waits for the verdict; on failure it prints the log, which says why.
notarize() {
	xcrun notarytool submit "$1" --wait --output-format json \
		--key "$MDSH_NOTARY_KEY" --key-id "$MDSH_NOTARY_KEY_ID" --issuer "$MDSH_NOTARY_ISSUER" \
		>"$work/submit.json" || true
	id=$(sed -n 's/.*"id" *: *"\([^"]*\)".*/\1/p' "$work/submit.json" | head -n 1)
	if ! grep -q '"status" *: *"Accepted"' "$work/submit.json"; then
		echo "notarization of $1 was not accepted:" >&2
		cat "$work/submit.json" >&2
		[ -n "$id" ] && xcrun notarytool log "$id" \
			--key "$MDSH_NOTARY_KEY" --key-id "$MDSH_NOTARY_KEY_ID" --issuer "$MDSH_NOTARY_ISSUER" >&2
		exit 1
	fi
}

export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_ENABLED=1 CGO_CFLAGS=-mmacosx-version-min=13.0 CGO_LDFLAGS=-mmacosx-version-min=13.0
for arch in arm64 amd64; do
	GOOS=darwin GOARCH=$arch go build -tags production -trimpath \
		-ldflags "-s -w -X main.version=$version" -o "$work/mdsh-gui-$arch" .
done

app=$work/mdsh.app
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create -output "$app/Contents/MacOS/mdsh-gui" "$work/mdsh-gui-arm64" "$work/mdsh-gui-amd64"
sed "s/VERSION/$version/g" build/darwin/Info.plist > "$app/Contents/Info.plist"

# The icon, at every size macOS asks for.
icons=$work/icons.iconset
mkdir -p "$icons"
for s in 16 32 128 256 512; do
	sips -z $s $s build/appicon.png --out "$icons/icon_${s}x${s}.png" >/dev/null
	sips -z $((s * 2)) $((s * 2)) build/appicon.png --out "$icons/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns -o "$app/Contents/Resources/icons.icns" "$icons"

sign "$app"
codesign --verify --strict --verbose=2 "$app"

name=mdsh-gui_${version}_macos_universal

# Notarize the app from a zip of it, staple the ticket to the app, and
# only then make the zip and the .dmg that ship, so both carry the
# stapled app. The .dmg is signed, notarized and stapled in turn: one
# ticket covers everything inside it, and Gatekeeper checks the image
# itself when it is opened.
if [ -n "$notarize" ]; then
	ditto -c -k --keepParent "$app" "$work/notarize.zip"
	notarize "$work/notarize.zip"
	xcrun stapler staple "$app"
	spctl --assess --type execute --verbose=2 "$app"
fi

ditto -c -k --keepParent "$app" "$out/$name.zip"

# The .dmg opens on a window that shows dragging mdsh to Applications;
# build/darwin/dmg/settings.py lays it out.
python3 -m venv "$work/venv"
"$work/venv/bin/pip" install --quiet dmgbuild==1.6.7
"$work/venv/bin/dmgbuild" -s build/darwin/dmg/settings.py -D app="$app" mdsh "$out/$name.dmg" >/dev/null

if [ -n "$identity" ]; then
	codesign --force --timestamp --sign "$identity" "$out/$name.dmg"
fi
if [ -n "$notarize" ]; then
	notarize "$out/$name.dmg"
	xcrun stapler staple "$out/$name.dmg"
	spctl --assess --type open --context context:primary-signature --verbose=2 "$out/$name.dmg"
fi
