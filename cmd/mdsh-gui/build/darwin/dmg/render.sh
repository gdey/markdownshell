#!/bin/sh
# Renders background.svg to background.png and background@2x.png with
# headless Chrome or Chromium:
#
#	build/darwin/dmg/render.sh [CHROME [FLAG...]]
#
# The window rising from the bottom shows the newest look: the last
# hero picture in site/data/looks.toml, which lists looks oldest first.
# Run it again when a release brings a new look.
set -eu
cd "$(dirname "$0")"
hero=$(sed -n 's/^ *light = "\(.*\)"$/\1/p' ../../../../../site/data/looks.toml | tail -n 1)
[ -n "$hero" ] || { echo "no hero in site/data/looks.toml" >&2; exit 1; }
sed "s|site/static/images/app/hero-[a-z]*\.png|site/static/images/app/$hero|" background.svg >background.render.svg
trap 'rm -f background.render.svg' EXIT
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
		--screenshot="$PWD/$out" "file://$PWD/background.render.svg" 2>/dev/null
done
