#!/bin/sh
# Builds mdsh-gui as a universal (arm64 and amd64) macOS app, mdsh.app,
# in a .dmg and a .zip:
#
#	build/darwin/app.sh VERSION OUTDIR
#
# Run it from cmd/mdsh-gui on macOS with the Xcode command line tools.
# The app is signed ad hoc, not with a Developer ID, so the first time it
# opens macOS asks first (right-click › Open, or System Settings › Privacy
# & Security › Open Anyway).
set -eu
version=$1
mkdir -p "$2"
out=$(cd "$2" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

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

codesign --force --deep --sign - "$app"

name=mdsh-gui_${version}_macos_universal
ditto -c -k --keepParent "$app" "$out/$name.zip"
mkdir "$work/dmg"
cp -R "$app" "$work/dmg/"
ln -s /Applications "$work/dmg/Applications"
hdiutil create -volname mdsh -srcfolder "$work/dmg" -ov -format UDZO "$out/$name.dmg" >/dev/null
