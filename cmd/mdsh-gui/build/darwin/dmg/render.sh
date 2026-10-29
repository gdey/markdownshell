#!/bin/sh
# Renders background.svg to background.png and background@2x.png with
# headless Chrome or Chromium:
#
#	build/darwin/dmg/render.sh [CHROME [FLAG...]]
set -eu
cd "$(dirname "$0")"
chrome="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
if [ $# -gt 0 ]; then
	chrome=$1
	shift
fi
for scale in 1 2; do
	out=background.png
	[ $scale = 1 ] || out=background@${scale}x.png
	"$chrome" "$@" --headless --disable-gpu --hide-scrollbars --allow-file-access-from-files \
		--force-device-scale-factor=$scale --window-size=640,400 \
		--screenshot="$PWD/$out" "file://$PWD/background.svg" 2>/dev/null
done
