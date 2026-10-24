#!/bin/sh
# Opens the potions (assets/tiles/potions.rec: one record per Rogue potion colour, its rgb tinting the potion sprite)
# in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin). s saves the icons back into the rec and leaves
# every other field as it was, F10 quits. The game reads the rec at startup; mktiles.py tints and cuts the pictures.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/potions.rec
