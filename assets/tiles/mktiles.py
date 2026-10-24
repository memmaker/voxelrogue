#!/usr/bin/env python3
"""Builds VoxelRogue's tile sheets, one per tile size, and the remapper
rec files that say which cell each game id uses. The sheets hold only
what the game draws; ids are the game's own names (a potion by its
colour, a ring by its stone, a wall by where it stands):

  tiles8.png   items.rec     %rec: item    weapons, armour, potions, scrolls, rings, food, wands, gold, the Amulet
  tiles16.png  textures.rec  %rec: block   the room's and the corridor's floor, wall and ceiling, light, trap, stairs, door frame
               monsters.rec  %rec: monster the 26 monsters (DynaHack, shadows stripped)
               weapons.rec   %rec: weapon  the mod weapons the game draws (16px): held, in hand, pulling, loaded
  tiles32.png  weapons32.rec %rec: weapon  the mod weapons larger than 16px (w/h keep their own size in a 32px cell)

The game reads only the recs and sheets (tiles.go); the remapper edits
the recs (remap_*.sh). The first look of everything comes from the art
the game used before: VoxelLib's CC0 item sprites and RPG16 pack, the
mods' weapon textures (assets/weapons), assets/monsters/dynahack.txt.

Run: python3 assets/tiles/mktiles.py [VoxelLib checkout]   (default ../VoxelLib)
Re-running rebuilds the sheets and keeps the icon of every id already
in a rec file, so remapped ids stay remapped."""
import glob, os, re, sys
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
LIB = sys.argv[1] if len(sys.argv) > 1 else os.path.join(ROOT, '..', 'VoxelLib')
ITEMS, BLOCKS = os.path.join(LIB, 'assets', 'items'), os.path.join(LIB, 'assets', 'rpg16pack', 'assets', 'minecraft', 'textures', 'block')


def golist(src, name):
    """The strings of a Go string-slice literal `name = ...{...}` in src."""
    s = src[src.index(name + ' = '):]
    return re.findall(r'"([^"]*)"', s[:s.index('}')])


def existing(rec):
    """id -> icon of a rec file already there (the remapper's edits)."""
    icons, rid = {}, None
    if not os.path.exists(rec):
        return icons
    for line in open(rec):
        m = re.match(r'(\w+):\s*(.*)', line)
        if m and m.group(1) == 'id':
            rid = m.group(2).strip()
        elif m and m.group(1) == 'icon' and rid:
            icons[rid] = int(m.group(2))
    return icons


class Sheet:
    def __init__(self, name, size, cols):
        self.name, self.size, self.cols, self.cells, self.index = name, size, cols, [], {}

    def add(self, key, img):
        """The cell index of img; one cell per key (ids sharing a look share it)."""
        if key not in self.index:
            self.cells.append(img)
            self.index[key] = len(self.cells) - 1
        return self.index[key]

    def save(self):
        s, rows = self.size, (len(self.cells) + self.cols - 1) // self.cols
        im = Image.new('RGBA', (self.cols * s, max(rows, 1) * s), (0, 0, 0, 0))
        for i, c in enumerate(self.cells):
            im.paste(c, ((i % self.cols) * s, (i // self.cols) * s))
        im.save(os.path.join(HERE, self.name))
        print(f'{self.name}: {len(self.cells)} cells of {s}px, {self.cols} per row')

    def rec(self, path, doc, cat, records):
        """records: (id, name, icon[, w, h]); a remapped id keeps its icon."""
        keep = existing(path)
        out = [f'# {os.path.basename(path)}: VoxelRogue tile mapping, built by mktiles.py, edited with the remapper',
               f'# (remap_{os.path.basename(path)[:-4]}.sh). icon: cell of {self.name}; -1: unassigned.',
               '', '%rec: Tileset', f'id: {self.name[:-4]}', f'name: VoxelRogue {self.size}px tiles',
               f'file: {self.name}', f'tile_w: {self.size}', f'tile_h: {self.size}',
               'off_x: 0', 'off_y: 0', 'gap_x: 0', 'gap_y: 0', '', f'%rec: {cat}', f'%doc: {doc}']
        for r in records:
            rid, name, icon = r[0], r[1], keep.get(r[0], r[2])
            out += ['', f'id: {rid}', f'name: {name}', f'icon: {icon}']
            if len(r) > 3:
                out += [f'w: {r[3]}', f'h: {r[4]}']
        open(path, 'w').write('\n'.join(out) + '\n')
        print(f'{os.path.basename(path)}: {len(records)} {cat} records')


def rgba(path):
    return Image.open(path).convert('RGBA')


rules = open(os.path.join(ROOT, 'rogue', 'objects.go')).read()
standins = open(os.path.join(ROOT, 'standins.go')).read()

# ---- tiles8: the items, named as Rogue names them; first looks are VoxelLib's sprites ----
t8 = Sheet('tiles8.png', 8, 16)
def item(rid, sprite, name=None):
    return (rid, name or rid, t8.add(sprite, rgba(os.path.join(ITEMS, sprite + '.png'))))
items = []
# weapons drawn with an 8px sprite (the rest are mod weapons: weapons.rec); standins.go weaponSprites
for rid, sprite in [('mace', 'mace_01'), ('long sword', 'sword_01'), ('arrow', 'bow_29'), ('dagger', 'sword_49'),
                    ('rock', 'valuable_02'), ('two handed sword', 'sword_25'), ('dart', 'bow_35'),
                    ('crossbow bolt', 'bow_31'), ('spear', 'spear_02'), ('double axe', 'axe_29'), ('great axe', 'axe_35'),
                    ('maul', 'mace_05'), ('heavy mace', 'mace_04'), ('sceptre', 'staff_26'), ('lance', 'spear_09'),
                    ('trident', 'spear_05'), ('soul knife', 'sword_44'), ('coral blade', 'sword_18'),
                    ('twin blade', 'sword_30'), ('claw', 'sword_57'), ('anchor', 'mace_06'), ('gauntlet', 'gloves_04')]:
    items.append(item(rid, sprite))
for i, n in enumerate(golist(rules, 'ArmorNames')):
    items.append(item(n, f'armor_{(i + 1) * 3:02d}'))
for j, c in enumerate(golist(rules, 'rainbow')):  # a potion looks like its colour
    items.append(item(c + ' potion', f'potion_{j % 24 * 3 + 1:02d}'))
for i in range(1, 16):  # a scroll's title is random each game; it has one of 15 looks
    items.append(item(f'scroll {i}', f'scroll_{i:02d}', f'scroll (look {i})'))
for j, s in enumerate(golist(rules, 'stones')):  # a ring looks like its stone
    items.append(item(s + ' ring', f'ring_{j % 18 + 1:02d}'))
items += [item('food ration', 'misc_01'), item('slime-mold', 'misc_21'), item('staff', 'staff_01'), item('wand', 'staff_10'),
          item('gold', 'valuable_09'), item('amulet of yendor', 'amulet_01', 'the Amulet of Yendor')]
t8.save()
t8.rec(os.path.join(HERE, 'items.rec'), 'Items', 'item', items)

# ---- tiles16: the block textures (world.go's blocks), the monsters, the 16px mod weapons ----
t16 = Sheet('tiles16.png', 16, 16)
def block(rid, tex):
    return (rid, rid, t16.add('b:' + tex, rgba(os.path.join(BLOCKS, tex + '.png'))))
blocks = [block('room floor', 'oak_planks'), block('room wall', 'stone_bricks'), block('room ceiling', 'stone'),
          block('dark room floor', 'dark_oak_planks'), block('room light', 'glowstone'),
          block('corridor floor', 'cobblestone'), block('corridor wall', 'stone'), block('corridor ceiling', 'stone'),
          block('stairs', 'cobblestone'), block('trap', 'redstone_ore'),
          block('door frame', 'oak_log'), block('door frame top', 'oak_log_top')]

# DynaHack's monsters from the NetHack text tile format (assets/monsters/dynahack.txt, shadows stripped)
txt = open(os.path.join(ROOT, 'assets', 'monsters', 'dynahack.txt')).read()
pal = {m.group(1): tuple(map(int, m.groups()[1:])) for m in re.finditer(r'^(\S) = \((\d+), (\d+), (\d+)\)', txt, re.M)}
monsters = []
for m in re.finditer(r'^# tile \d+ \((.*?)\).*\n\{\n((?:  .{16}\n){16})\}', txt, re.M):
    im = Image.new('RGBA', (16, 16), (0, 0, 0, 0))
    for y, row in enumerate(m.group(2).split('\n')[:16]):
        for x, c in enumerate(row[2:]):
            if c != 'M':
                im.putpixel((x, y), pal[c] + (255,))
    monsters.append((m.group(1), m.group(1), t16.add('m:' + m.group(1), im)))

# the mod weapons standins.go names ("w:" + id), with the pictures items.go derives from them
refs = set(re.findall(r'"w:(\w+)"', standins)) | {'sling_model'}
t32 = Sheet('tiles32.png', 32, 8)
weapons, weapons32 = [], []
for p in sorted(glob.glob(os.path.join(ROOT, 'assets', 'weapons', '*.png'))):
    n = os.path.basename(p)[:-4]
    base, kind = re.fullmatch(r'(.*?)(?:_(hand|arrow|pulling_\d))?', n).groups()
    if base not in refs:
        continue
    name = base.replace('_', ' ') + {None: '', 'hand': ' (in hand)', 'arrow': ' (loaded)'}.get(kind, f' ({kind.replace("_", " ")})' if kind else '')
    if n == 'sling_model':
        name = 'sling (model texture)'
    im = rgba(p)
    if im.size == (16, 16):
        weapons.append((n, name, t16.add('w:' + n, im)))
    else:
        cell = Image.new('RGBA', (32, 32), (0, 0, 0, 0))
        cell.paste(im, (0, 0))
        weapons32.append((n, name, t32.add('w:' + n, cell), im.size[0], im.size[1]))
t16.save()
t16.rec(os.path.join(HERE, 'textures.rec'), 'Block textures', 'block', blocks)
t16.rec(os.path.join(HERE, 'monsters.rec'), 'Monsters', 'monster', monsters)
t16.rec(os.path.join(HERE, 'weapons.rec'), 'Weapons (mod pictures, 16px)', 'weapon', weapons)
t32.save()
t32.rec(os.path.join(HERE, 'weapons32.rec'), 'Weapons (mod pictures larger than 16px)', 'weapon', weapons32)
