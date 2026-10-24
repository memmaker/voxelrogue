#!/bin/sh
# Opens the textures tile mapping (assets/tiles/textures.rec) in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin).
# Tab cycles categories, s saves the icons back into the rec, F10 quits. The game reads the rec at startup.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/textures.rec
