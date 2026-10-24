#!/usr/bin/env python3
"""Builds VoxelRogue's tile sheets, one per tile size, and the remapper
rec files that say which cell each game id uses. The sheets hold only
what the game draws; ids are the game's own names (a potion by its
colour, a ring by its stone, a wall by where it stands):

  tiles8.png   items.rec     %rec: item    weapons, armour, potions, scrolls, rings, food, wands, gold, the Amulet
  tiles16.png  gear.rec      %rec: gear    the armour and the weapons with a 16px picture (assets/hobbit: LOTR mod and vanilla sprites)
               textures.rec  %rec: block   the room's and the corridor's floor, wall and ceiling, light, trap, stairs, door frame
               monsters.rec  %rec: monster the 26 monsters (DynaHack, shadows stripped)
               weapons.rec   %rec: weapon  the sling's pictures (Nightfall's), held and its model texture

A rerun keeps the recs' icons (the remapper's edits); after adding or cutting a look,
run it with --fresh, since the cells behind the kept icons have moved.
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
FRESH = '--fresh' in sys.argv  # ignore the recs' kept icons: the cells moved (a look added or cut)
ARGS = [a for a in sys.argv[1:] if a != '--fresh']
LIB = ARGS[0] if ARGS else os.path.join(ROOT, '..', 'VoxelLib')
ITEMS, BLOCKS = os.path.join(LIB, 'assets', 'items'), os.path.join(LIB, 'assets', 'rpg16pack', 'assets', 'minecraft', 'textures', 'block')


def golist(src, name):
    """The strings of a Go string-slice literal `name = ...{...}` in src."""
    s = src[src.index(name + ' = '):]
    return re.findall(r'"([^"]*)"', s[:s.index('}')])


MESH_FIELDS = ('helmet', 'chest', 'leggings', 'boots')


def existing(rec):
    """id -> icon of a rec file already there (the remapper's edits), and
    id -> its mesh field lines (gear.rec's armour meshes, meshes.rec)."""
    icons, meshes, rid = {}, {}, None
    if not os.path.exists(rec):
        return icons, meshes
    for line in open(rec):
        m = re.match(r'(\w+):\s*(.*)', line)
        if m and m.group(1) == 'id':
            rid = m.group(2).strip()
        elif m and m.group(1) == 'icon' and rid:
            icons[rid] = int(m.group(2))
        elif m and m.group(1) in MESH_FIELDS and rid:
            meshes.setdefault(rid, []).append(line.rstrip())
    return icons, meshes


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
        keep, meshes = existing(path)
        if FRESH:
            keep = {}
        out = [f'# {os.path.basename(path)}: VoxelRogue tile mapping, built by mktiles.py, edited with the remapper',
               f'# (remap_{os.path.basename(path)[:-4]}.sh). icon: cell of {self.name}; -1: unassigned.',
               '', '%rec: Tileset', f'id: {self.name[:-4]}', f'name: VoxelRogue {self.size}px tiles',
               f'file: {self.name}', f'tile_w: {self.size}', f'tile_h: {self.size}',
               'off_x: 0', 'off_y: 0', 'gap_x: 0', 'gap_y: 0']
        if cat == 'gear':
            out[1] += ' helmet/chest/leggings/boots: the\n# meshes.rec pieces the inventory hero wears (the Epic Knights models).'
            out.append('meshes: meshes.rec')  # tools/convertarmor.py writes it
        out += ['', f'%rec: {cat}', f'%doc: {doc}']
        for r in records:
            rid, name, icon = r[0], r[1], keep.get(r[0], r[2])
            out += ['', f'id: {rid}', f'name: {name}', f'icon: {icon}'] + meshes.get(rid, [])
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
# the weapons with no 16px picture in assets/hobbit (the Better Combat exotics) keep VoxelLib's 8px sprites
items.append(item('two handed sword', 'sword_25'))
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

# ---- tiles16: the armour and weapons (assets/hobbit), the block textures (world.go's blocks), the monsters, the kept mod weapons ----
t16 = Sheet('tiles16.png', 16, 16)
HOB = os.path.join(ROOT, 'assets', 'hobbit')
def gear(rid, src, name=None):
    im = rgba(os.path.join(HOB, src + '.png')).crop((0, 0, 16, 16))  # an animation strip's first frame; a stray 17th row
    return (rid, name or rid, t16.add('h:' + src, im))
gears = [gear(n, src) for n, src in zip(golist(rules, 'ArmorNames'),
         ['bodyDunlending', 'bodyRanger', 'bodyCorsair', 'bodyNearHarad', 'chainmail_chestplate', 'bodyDale', 'bodyRohan', 'iron_chestplate'])]
for rid, src in [('mace', 'maceBlackNumenorean'), ('long sword', 'swordGondor'), ('long bow', 'bow_standby'), ('arrow', 'arrow'),
                 ('dagger', 'daggerGondor'), ('rock', 'flint'), ('dart', 'tauredainDart'), ('crossbow', 'ironCrossbow'),
                 ('crossbow bolt', 'crossbowBolt'), ('spear', 'spearGondor'), ('axe', 'axeDwarven'), ('halberd', 'halberdMithril'),
                 ('maul', 'hammerDwarven'), ('battle axe', 'battleaxeDwarven'), ('chain crossbow', 'urukCrossbow')]:
    gears.append(gear(rid, src))
for i in range(3):  # the frames items.go shows while a bow is drawn or a crossbow spanned
    gears.append(gear(f'long bow_pulling_{i}', f'bow_pulling_{i}', f'long bow (pulling {i})'))
    gears.append(gear(f'crossbow_pulling_{i}', f'ironCrossbow_pull_{i}', f'crossbow (pulling {i})'))
    gears.append(gear(f'chain crossbow_pulling_{i}', f'urukCrossbow_pull_{i}', f'chain crossbow (pulling {i})'))
gears.append(gear('crossbow_arrow', 'ironCrossbow_pull_2', 'crossbow (loaded)'))
gears.append(gear('chain crossbow_arrow', 'urukCrossbow_pull_2', 'chain crossbow (loaded)'))
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

# the weapons standins.go draws with a picture of their own ("w:" + id: the sling and its model texture)
refs = set(re.findall(r'"w:(\w+)"', standins)) | {'sling_model'}
weapons = []
for p in sorted(glob.glob(os.path.join(ROOT, 'assets', 'weapons', '*.png'))):
    n = os.path.basename(p)[:-4]
    base, kind = re.fullmatch(r'(.*?)(?:_(hand|arrow|pulling_\d))?', n).groups()
    if base not in refs:
        continue
    name = base.replace('_', ' ') + {None: '', 'hand': ' (in hand)', 'arrow': ' (loaded)'}.get(kind, f' ({kind.replace("_", " ")})' if kind else '')
    if n == 'sling_model':
        name = 'sling (model texture)'
    weapons.append((n, name, t16.add('w:' + n, rgba(p))))
t16.save()
t16.rec(os.path.join(HERE, 'gear.rec'), 'Armour and weapons (16px)', 'gear', gears)
t16.rec(os.path.join(HERE, 'textures.rec'), 'Block textures', 'block', blocks)
t16.rec(os.path.join(HERE, 'monsters.rec'), 'Monsters', 'monster', monsters)
t16.rec(os.path.join(HERE, 'weapons.rec'), 'Weapons (mod pictures, 16px)', 'weapon', weapons)
