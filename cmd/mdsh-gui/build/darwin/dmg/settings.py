# dmgbuild settings for mdsh-gui's .dmg; app.sh passes the app's path:
#
#	dmgbuild -s build/darwin/dmg/settings.py -D app=PATH/mdsh.app mdsh OUT.dmg
#
# The icons sit over the ends of background.svg's arrow. dmgbuild finds
# background@2x.png next to background.png for Retina screens.
import os.path

app = defines["app"]

format = "UDZO"
files = [app]
symlinks = {"Applications": "/Applications"}
icon = os.path.join(app, "Contents", "Resources", "icons.icns")

# Relative to cmd/mdsh-gui, where app.sh runs.
background = "build/darwin/dmg/background.png"
window_rect = ((200, 120), (640, 400))
default_view = "icon-view"
icon_size = 128
text_size = 13
icon_locations = {
    os.path.basename(app): (170, 205),
    "Applications": (470, 205),
}
