#!/bin/sh
# Opens the monsters tile mapping (assets/tiles/monsters.rec) in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin).
# Tab cycles categories, s saves the icons back into the rec, F10 quits. The game reads the rec at startup.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/monsters.rec
