#!/bin/sh
# Opens the staff looks (assets/tiles/staffs.rec: one record per picture in assets/staffs, mktiles.py writes it)
# in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin). s saves the icons back into the rec and leaves
# every other field as it was, F10 quits. The game reads the rec at startup; mktiles.py cuts the pictures.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/staffs.rec
