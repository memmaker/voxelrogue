#!/bin/sh
# Opens the armour and weapon tile mapping (assets/tiles/gear.rec, 16px pictures from assets/hobbit) in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin).
# Tab cycles categories, M picks an armour mesh slot and , . cycle its mesh (meshes.rec, from tools/convertarmor.py),
# s saves the icons and meshes back into the rec, F10 quits. The game reads the rec at startup.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/gear.rec
