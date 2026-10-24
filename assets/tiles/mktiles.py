#!/usr/bin/env python3
"""Builds VoxelRogue's tile sheets, one per tile size, and the remapper
rec files that say which cell each game id uses (the game reads only
these; ~/Projects/remapper edits the recs, remap_*.sh opens them):

  tiles8.png   every CC0 RPG item sprite shipped with VoxelLib (8x8)
               items.rec     %rec: item   (potions, scrolls, rings, armour, vanilla weapons)
  tiles16.png  RPG16's block textures, the DynaHack monsters, the mod weapons (16x16)
               textures.rec  %rec: block
               monsters.rec  %rec: monster
               weapons.rec   %rec: weapon
  tiles32.png  the mod weapons larger than 16px (in 32x32 cells; w/h keep the sprite's own size)
               weapons32.rec %rec: weapon

Run: python3 assets/tiles/mktiles.py [VoxelLib checkout]   (default ../VoxelLib)
Re-running rebuilds the sheets in the same order and keeps the icon of
every id already in a rec file, so remapped ids stay remapped."""
import glob, os, re, sys
from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
LIB = sys.argv[1] if len(sys.argv) > 1 else os.path.join(ROOT, '..', 'VoxelLib')


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
        self.name, self.size, self.cols, self.cells = name, size, cols, []

    def add(self, img):
        """Returns the cell index of img (its top-left in the cell)."""
        self.cells.append(img)
        return len(self.cells) - 1

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


# ---- tiles8: VoxelLib's item sprites, records for the names the game uses (standins.go spriteNames) ----
t8 = Sheet('tiles8.png', 8, 32)
cell8 = {}
for p in sorted(glob.glob(os.path.join(LIB, 'assets', 'items', '*.png'))):
    im = rgba(p)
    if im.size == (8, 8):
        cell8[os.path.basename(p)[:-4]] = t8.add(im)
names = ['misc_01', 'misc_21', 'misc_10', 'valuable_02', 'valuable_09', 'amulet_01', 'staff_01', 'staff_10',
         'sword_01', 'sword_25', 'sword_49', 'mace_01', 'spear_02', 'bow_29', 'bow_31', 'bow_35',
         'axe_29', 'axe_35', 'mace_05', 'mace_04', 'staff_26', 'spear_09', 'spear_05', 'sword_44', 'sword_18', 'sword_30',
         'sword_57', 'mace_06', 'gloves_04']
names += [f'armor_{i * 3:02d}' for i in range(1, 9)] + [f'scroll_{i:02d}' for i in range(1, 16)]
names += [f'potion_{i * 3 + 1:02d}' for i in range(24)] + [f'ring_{i:02d}' for i in range(1, 19)]
t8.save()
t8.rec(os.path.join(HERE, 'items.rec'), 'Items', 'item', [(n, n.replace('_', ' '), cell8[n]) for n in names])

# ---- tiles16: block textures, monsters, 16px weapons ----
t16 = Sheet('tiles16.png', 16, 16)
blocks = []
for p in sorted(glob.glob(os.path.join(LIB, 'assets', 'rpg16pack', 'assets', 'minecraft', 'textures', 'block', '*.png'))):
    im = rgba(p)
    if im.size == (16, 16):  # animated strips and odd sizes stay in the pack
        n = os.path.basename(p)[:-4]
        blocks.append(('block/' + n, n.replace('_', ' '), t16.add(im)))

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
    monsters.append((m.group(1), m.group(1), t16.add(im)))

t32 = Sheet('tiles32.png', 32, 16)
weapons, weapons32 = [], []
for p in sorted(glob.glob(os.path.join(ROOT, 'assets', 'weapons', '*.png'))):
    im, n = rgba(p), os.path.basename(p)[:-4]
    if im.size == (16, 16):
        weapons.append((n, n.replace('_', ' '), t16.add(im)))
    else:
        cell = Image.new('RGBA', (32, 32), (0, 0, 0, 0))
        cell.paste(im, (0, 0))
        weapons32.append((n, n.replace('_', ' '), t32.add(cell), im.size[0], im.size[1]))
t16.save()
t16.rec(os.path.join(HERE, 'textures.rec'), 'Block textures', 'block', blocks)
t16.rec(os.path.join(HERE, 'monsters.rec'), 'Monsters', 'monster', monsters)
t16.rec(os.path.join(HERE, 'weapons.rec'), 'Weapons (mod sprites, 16px)', 'weapon', weapons)
t32.save()
t32.rec(os.path.join(HERE, 'weapons32.rec'), 'Weapons (mod sprites, larger than 16px)', 'weapon', weapons32)
