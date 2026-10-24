#!/bin/sh
# Opens the armour (assets/tiles/gear.rec: the game's single source of armour data, 16px pictures from assets/hobbit)
# in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin).
# M picks an armour mesh slot and , . cycle its mesh (meshes.rec, from tools/convertarmor.py),
# s saves the icons and meshes back into the rec and leaves every other field as it was, F10 quits.
# The game reads the rec at startup; mktiles.py cuts each record's picture onto the sheet and fills unassigned icons.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/gear.rec
