#!/bin/sh
# Opens the food (assets/tiles/food.rec: the food ration and the slime-mold, their pictures in assets/food, mktiles.py writes it)
# in the remapper (~/Projects/remapper; deploy.sh puts it in ~/bin). s saves the icons back into the rec and leaves
# every other field as it was, F10 quits. The game reads the rec at startup; mktiles.py cuts the pictures.
cd "$(dirname "$0")" || exit 1
exec "${REMAPPER:-$HOME/bin/remapper}" assets/tiles/food.rec
