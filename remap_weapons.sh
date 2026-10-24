#!/bin/sh
# Opens the weapons (assets/tiles/weapons.rec: the weapon records, the game's single source of weapon data, and the
# frame records, their pictures in use) in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin).
# Tab cycles the categories, s saves the icons back into the rec and leaves every other field as it was, F10 quits.
# The game reads the rec at startup; mktiles.py cuts each record's picture onto the sheet and fills unassigned icons.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/weapons.rec
