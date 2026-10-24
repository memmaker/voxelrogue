#!/usr/bin/env python3
"""Builds VoxelRogue's tile sheets, one per tile size, and the remapper
rec files that say which cell each game id uses. The sheets hold only
what the game draws; ids are the game's own names (a potion by its
colour, a ring by its stone, a wall by where it stands):

  tiles8.png   items.rec     %rec: item    rings, wands, gold, the Amulet (written whole)
  tiles16.png  gear.rec      %rec: gear    the armour: hand-written records, each naming its picture
               potions.rec   %rec: potion  Rogue's potion colours: hand-written, each an rgb tinting the potion sprite
               food.rec      %rec: food    the food ration (bread) and the slime-mold (assets/food)
               scrolls.rec   %rec: scroll  the scroll looks (assets/scrolls, BigWander's Art of Scrolls), a game's scroll kinds draw theirs from them
               weapons.rec   %rec: weapon  the weapons and their pictures in use (%rec: frame): hand-written, each naming its picture
               textures.rec  %rec: block   the room's and the corridor's floor, wall and ceiling, light, trap, stairs, door frame
               monsters.rec  %rec: monster the 26 monsters (DynaHack, shadows stripped)

gear.rec, weapons.rec and potions.rec are the game's data (every field of an armour, a
weapon or a potion); this script only cuts each record's picture: (relative to the rec, an
animation strip's first 16px frame) or tints the potion sprite with its rgb: (tint_potion)
onto the sheet and fills its icon: in, touching no other line. A rerun keeps
the recs' icons (the remapper's edits); after adding or cutting a look, run it with --fresh,
since the cells behind the kept icons have moved.
The game reads only the recs and sheets (tiles.go); the remapper edits
the recs (remap_*.sh). The first look of everything comes from the art
the game used before: VoxelLib's CC0 item sprites and RPG16 pack, the
mods' weapon textures (assets/hobbit, assets/weapons), assets/monsters/dynahack.txt.

Run: python3 assets/tiles/mktiles.py [VoxelLib checkout]   (default ../VoxelLib)
Re-running rebuilds the sheets and keeps the icon of every id already
in a rec file, so remapped ids stay remapped."""
import os, re, sys
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
        keep = {} if FRESH else existing(path)
        out = [f'# {os.path.basename(path)}: VoxelRogue tile mapping, built by mktiles.py, edited with the remapper',
               f'# (remap_{os.path.basename(path)[:-4]}.sh). icon: cell of {self.name}; -1: unassigned.',
               '', '%rec: Tileset', f'id: {self.name[:-4]}', f'name: VoxelRogue {self.size}px tiles',
               f'file: {self.name}', f'tile_w: {self.size}', f'tile_h: {self.size}',
               'off_x: 0', 'off_y: 0', 'gap_x: 0', 'gap_y: 0']
        out += ['', f'%rec: {cat}', f'%doc: {doc}']
        for r in records:
            rid, name, icon = r[0], r[1], keep.get(r[0], r[2])
            out += ['', f'id: {rid}', f'name: {name}', f'icon: {icon}']
            if len(r) > 3:
                out += [f'w: {r[3]}', f'h: {r[4]}']
        open(path, 'w').write('\n'.join(out) + '\n')
        print(f'{os.path.basename(path)}: {len(records)} {cat} records')

    def patch(self, path):
        """Cuts the picture: of every record of a hand-written rec onto the sheet and fills
        its icon: in (kept if assigned, unless --fresh); every other line stays as it is."""
        lines, out, n = open(path).read().split('\n'), [], 0
        for i, line in enumerate(lines):
            m = re.match(r'icon:\s*(-?\d+)', line)
            if m:  # every picture is cut, so kept icons keep their cells
                rec = lines[max(j for j in range(i, -1, -1) if lines[j].strip() == '' or lines[j].startswith('%')) + 1:i]
                fields = dict(l.split(':', 1) for l in rec if ':' in l)
                if 'rgb' in fields:
                    rgb = tuple(int(v) for v in fields['rgb'].split())
                    cell = self.add(('rgb',) + rgb, tint_potion(rgb))
                else:
                    src = os.path.normpath(os.path.join(os.path.dirname(path), fields['picture'].strip()))
                    cell = self.add(src, rgba(src).crop((0, 0, self.size, self.size)))
                if FRESH or int(m.group(1)) < 0:
                    line, n = f'icon: {cell}', n + 1
            out.append(line)
        open(path, 'w').write('\n'.join(out))
        print(f'{os.path.basename(path)}: {n} icons assigned')


def rgba(path):
    return Image.open(path).convert('RGBA')


POTIONS = os.path.join(ROOT, 'assets', 'potions')
# The potion sheet's own ramp (Bonsaiheldin's rpg_potions_16x16): the black potion's grey marks each
# contents pixel's shade, and a coloured potion is the colour darkened by half (32), the colour (64),
# and the colour 50% (96) and 80% (191) of the way to white.
RAMP = {32: -0.5, 64: 0.0, 96: 0.5, 191: 0.8}


def tint_potion(rgb):
    """The white potion (assets/potions/white.png) with its contents, the pixels that differ from the
    black one, coloured rgb in the sheet's ramp."""
    white, black = rgba(os.path.join(POTIONS, 'white.png')), rgba(os.path.join(POTIONS, 'black.png'))
    out, w, b = white.copy(), white.load(), black.load()
    for y in range(white.size[1]):
        for x in range(white.size[0]):
            if w[x, y] == b[x, y]:
                continue
            t = RAMP[b[x, y][0]]
            out.putpixel((x, y), tuple(round(c * (1 + t)) if t < 0 else round(c + (255 - c) * t) for c in rgb) + (255,))
    return out


def check_tint():
    """tint_potion reproduces the sheet's own red potion (its contents' four shades)."""
    red = {tuple(p[:3]) for p in tint_potion((255, 0, 0)).get_flattened_data() if p[3]}
    for shade in [(128, 0, 0), (255, 0, 0), (255, 128, 128), (255, 204, 204)]:
        assert shade in red, (shade, red)


rules = open(os.path.join(ROOT, 'rogue', 'objects.go')).read()

# ---- tiles8: the items, named as Rogue names them; first looks are VoxelLib's sprites ----
t8 = Sheet('tiles8.png', 8, 16)
def item(rid, sprite, name=None):
    return (rid, name or rid, t8.add(sprite, rgba(os.path.join(ITEMS, sprite + '.png'))))
items = []
for j, s in enumerate(golist(rules, 'stones')):  # a ring looks like its stone
    items.append(item(s + ' ring', f'ring_{j % 18 + 1:02d}'))
items += [item('staff', 'staff_01'), item('wand', 'staff_10'),
          item('gold', 'valuable_09'), item('amulet of yendor', 'amulet_01', 'the Amulet of Yendor')]
t8.save()
t8.rec(os.path.join(HERE, 'items.rec'), 'Items', 'item', items)

# ---- tiles16: the armour and weapons (their recs' pictures), the block textures (world.go's blocks), the monsters ----
t16 = Sheet('tiles16.png', 16, 16)
t16.patch(os.path.join(HERE, 'gear.rec'))
t16.patch(os.path.join(HERE, 'weapons.rec'))
check_tint()
t16.patch(os.path.join(HERE, 'potions.rec'))
# every colour of the rainbow has a potion record, and no two potions' colours are close
potions = open(os.path.join(HERE, 'potions.rec')).read()
colours = {m.group(1): tuple(int(v) for v in m.group(2).split()) for m in re.finditer(r'^id: (\w+) potion\n(?:.*\n)*?(?:rgb: (.*)|picture: .*)$', potions, re.M) if m.group(2)}
missing = [c for c in golist(rules, 'rainbow') if f'id: {c} potion\n' not in potions]
assert not missing, f'potions.rec lacks {missing}'
for a in colours:
    for b in colours:
        d = sum((x - y) ** 2 for x, y in zip(colours[a], colours[b])) ** 0.5
        assert a == b or d >= 55, f'{a} and {b} potions look alike ({d:.0f} apart)'
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
SCROLLS = os.path.join(ROOT, 'assets', 'scrolls')
scrolls = [(f'scroll {i}', f'scroll (look {i})', t16.add('s:' + f, rgba(os.path.join(SCROLLS, f))))
           for i, f in enumerate(sorted(f for f in os.listdir(SCROLLS) if f.endswith('.png')), 1)]
FOOD = os.path.join(ROOT, 'assets', 'food')
food = [(rid, rid, t16.add('f:' + f, rgba(os.path.join(FOOD, f + '.png')))) for rid, f in [('food ration', 'bread'), ('slime-mold', 'slime-mold')]]
t16.save()
t16.rec(os.path.join(HERE, 'food.rec'), 'Food', 'food', food)
t16.rec(os.path.join(HERE, 'scrolls.rec'), 'Scroll looks', 'scroll', scrolls)
t16.rec(os.path.join(HERE, 'textures.rec'), 'Block textures', 'block', blocks)
t16.rec(os.path.join(HERE, 'monsters.rec'), 'Monsters', 'monster', monsters)
