#!/bin/sh
# Builds mdsh-gui as an AppImage for this machine's architecture,
# carrying the mdsh command line tool:
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

# The mdsh command line tool, which the image carries at usr/bin/mdsh
# for Settings › Shell to copy to ~/.local/bin, and which build/linux/
# AppRun runs when the image is run as mdsh or with --cli
# (docs/design/cli-in-app.md). wails3 makes the AppDir afresh and packs
# it in one step, so the tool and the AppRun go in after, and the image
# is packed again with the linuxdeploy it left in the build folder.
(cd ../.. && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/mdsh" ./cmd/mdsh)
appdir=$(echo "$work"/appdir/*.AppDir)
install -m 755 "$work/mdsh" "$appdir/usr/bin/mdsh"
mv "$appdir/AppRun" "$appdir/AppRun.wails"
install -m 755 build/linux/AppRun "$appdir/AppRun"
rm "$work"/dist/*.AppImage
(cd "$work/appdir" && NO_STRIP=1 OUTPUT=mdsh-gui.AppImage ./linuxdeploy-*.AppImage \
	--appimage-extract-and-run --appdir "$appdir" --output appimage >/dev/null)

arch=$(uname -m)
image=$out/mdsh-gui_${version}_linux_${arch}.AppImage
mv "$work/appdir/mdsh-gui.AppImage" "$image"

# The image answers as mdsh both ways, at this version.
mkdir "$work/bin"
ln -s "$image" "$work/bin/mdsh"
for run in "$work/bin/mdsh --version" "$image --cli --version"; do
	case $(APPIMAGE_EXTRACT_AND_RUN=1 $run) in
	*"$version"*) ;;
	*)
		echo "$run doesn't say it is mdsh $version" >&2
		exit 1
		;;
	esac
done
