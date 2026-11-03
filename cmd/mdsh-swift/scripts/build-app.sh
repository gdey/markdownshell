#!/bin/sh
# build-app.sh builds mdsh-swift.app in build/: the SwiftUI app with the
# Go bridge in its Resources. It needs macOS, Xcode's command line tools
# and Go.
#
#   scripts/build-app.sh [debug|release]
set -eu
cd "$(dirname "$0")/.."
config=${1:-release}
version=${MDSH_SWIFT_VERSION:-0.0.0-dev}

echo "building the bridge"
(cd bridge && go build -trimpath -o ../build/mdsh-swift-bridge .)

echo "building the app ($config)"
swift build -c "$config"
bin=$(swift build -c "$config" --show-bin-path)

app=build/mdsh-swift.app
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
cp "$bin/mdsh-swift" "$app/Contents/MacOS/mdsh-swift"
cp build/mdsh-swift-bridge "$app/Contents/Resources/mdsh-swift-bridge"
# mdsh-gui's icon.
if [ -f ../mdsh-gui/build/darwin/icons.icns ]; then
	cp ../mdsh-gui/build/darwin/icons.icns "$app/Contents/Resources/AppIcon.icns"
fi
sed "s/@VERSION@/$version/g" scripts/Info.plist > "$app/Contents/Info.plist"
# An ad hoc signature, so macOS runs it on this Mac.
codesign --force --deep --sign - "$app" >/dev/null 2>&1 || true
echo "built $app"
