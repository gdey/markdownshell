#!/bin/sh
# Builds mdsh-gui as an AppImage for this machine's architecture:
#
#	build/linux/appimage.sh VERSION OUTDIR
#
# Run it from cmd/mdsh-gui on Linux, with GTK 4 and WebKitGTK 6.0
# installed (libgtk-4-dev, libwebkitgtk-6.0-dev). wails3 generate
# appimage bundles WebKit's helper processes and downloads linuxdeploy.
set -eu
version=$1
out=$(realpath -m "$2")
mkdir -p "$out"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

go build -tags production -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/mdsh-gui" .
cp build/appicon.png "$work/mdsh-gui.png"
mkdir -p "$work/dist"
go run github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0 generate appimage \
	-binary "$work/mdsh-gui" \
	-icon "$work/mdsh-gui.png" \
	-desktopfile build/linux/mdsh-gui.desktop \
	-outputdir "$work/dist" \
	-builddir "$work/appdir"
arch=$(uname -m)
mv "$work"/dist/*.AppImage "$out/mdsh-gui_${version}_linux_${arch}.AppImage"
