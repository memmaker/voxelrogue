#!/bin/sh
# Opens the Amulet (assets/tiles/amulet.rec: one record, assets/amulet/amulet.png, mktiles.py writes it)
# in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin). s saves the icons back into the rec and leaves
# every other field as it was, F10 quits. The game reads the rec at startup; mktiles.py cuts the pictures.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/amulet.rec
