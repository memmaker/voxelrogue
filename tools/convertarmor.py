#!/usr/bin/env python3
"""convertarmor.py: the Epic Knights armour models as cuboid geometry.

Epic Knights (github.com/magistu/epic-knights, MIT) draws its armour with
Blockbench-exported model classes (LayerDefinition: parts of addBox cubes
with a PartPose). This reads those classes and the mod's armour item
table, and writes
  assets/knight/meshes/<model>.json   VoxelLib cuboid.Geometry, one per model and slot
  assets/knight/armor/<tex>.png       the layer textures (the 32x pack's, else the mod's)
  assets/tiles/meshes.rec             one record per armour piece: id, name, piece, geo, tex
for the remapper (gear.rec's helmet/chest/leggings/boots fields) and the
game's inventory hero.

Run: python3 tools/convertarmor.py <epic-knights checkout> [<unpacked 32x pack>]
"""
import glob, json, os, re, shutil, sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
SRC = sys.argv[1]
PACK = sys.argv[2] if len(sys.argv) > 2 else None
MOD = os.path.join(SRC, 'common', 'src', 'main')
JAVA = os.path.join(MOD, 'java', 'com', 'magistuarmory')
MODELS = os.path.join(JAVA, 'client', 'render', 'model')
OUT_MESH, OUT_TEX = os.path.join(ROOT, 'assets', 'knight', 'meshes'), os.path.join(ROOT, 'assets', 'knight', 'armor')
REC = os.path.join(ROOT, 'assets', 'tiles', 'meshes.rec')

# the parts each slot draws (modern HumanoidModel names; the hat is the head's child)
SLOT_PARTS = {'helmet': {'head', 'hat'}, 'chest': {'body', 'right_arm', 'left_arm'},
              'leggings': {'body', 'right_leg', 'left_leg'}, 'boots': {'right_leg', 'left_leg'}}
SLOT_OF = {'HELMET': 'helmet', 'CHESTPLATE': 'chest', 'LEGGINGS': 'leggings', 'BOOTS': 'boots'}
CHEST_MODELS = {'wingedhussarchestplate'}  # the only special model that is not a helmet


def num(s):
    return float(s.strip().rstrip('F').rstrip('f'))


def parse_model(path):
    """A model class as a cuboid geometry: parts (name, parent, pivot, rot, boxes) and the texture size."""
    src = open(path).read()
    m = re.search(r'LayerDefinition\.create\(\w+,\s*(\d+),\s*(\d+)\)', src)
    tex = [int(m.group(1)), int(m.group(2))]
    parts, byvar, calls = [], {}, 0  # calls: the texOffs/mirror/addBox calls parsed, against the source's count
    # one statement per part: [PartDefinition var =] parent.addOrReplaceChild("name", CubeListBuilder.create()..., PartPose...);
    stmt = re.compile(r'^\s*(?:PartDefinition\s+(\w+)\s*=\s*)?(\w+(?:\.getChild\("\w+"\))?)\.addOrReplaceChild\("(\w+)",\s*(CubeListBuilder\.create\(\).*?),\s*(PartPose\.\w+(?:\([^)]*\))?)\);', re.S | re.M)
    for var, parent_expr, name, builder, pose in stmt.findall(src):
        parent = -1
        g = re.search(r'getChild\("(\w+)"\)', parent_expr)
        if g:
            parent = next(i for i, p in enumerate(parts) if p['name'] == g.group(1))
        elif parent_expr.strip() in byvar:
            parent = byvar[parent_expr.strip()]
        pivot, rot = [0.0, 0.0, 0.0], [0.0, 0.0, 0.0]
        pm = re.match(r'PartPose\.(\w+)(?:\((.*)\))?', pose)
        if pm.group(2):
            vals = [num(v) for v in pm.group(2).split(',')]
            if pm.group(1) == 'offset':
                pivot = vals
            elif pm.group(1) == 'offsetAndRotation':
                pivot, rot = vals[:3], vals[3:]
            elif pm.group(1) == 'rotation':
                rot = vals
        boxes, uv, mirror = [], [0, 0], False
        for call in re.finditer(r'\.(texOffs|mirror|addBox)\((.*?)\)(?=\s*\.|\s*$)', builder, re.S):
            kind, args = call.group(1), call.group(2)
            calls += 1
            if kind == 'texOffs':
                uv = [int(v) for v in args.split(',')]
            elif kind == 'mirror':
                mirror = args.strip() != 'false'
            else:
                am = re.match(r'([-\d.Ff, ]+),\s*new CubeDeformation\(([-\d.Ff]+)\)', args)
                v = [num(x) for x in am.group(1).split(',')]
                boxes.append({'pos': v[:3], 'size': [int(x) for x in v[3:6]], 'inflate': num(am.group(2)),
                              'uv': uv, 'mirror': mirror, 'tex': [float(tex[0]), float(tex[1])]})
        if var:
            byvar[var] = len(parts)
        parts.append({'name': name, 'parent': parent, 'pivot': pivot, 'rot': rot, 'boxes': boxes})
    want = sum(src.count('.' + k + '(') for k in ('texOffs', 'mirror', 'addBox'))
    assert calls == want, f'{path}: parsed {calls} of {want} builder calls (a chained call the regex missed?)'
    return parts, tex


def models():
    """model name -> class path, from ModModels' addArmorModel registrations."""
    src = open(os.path.join(MODELS, 'ModModels.java')).read()
    consts = dict(re.findall(r'LayerDefinition\s+(\w+)\s*=\s*(\w+)\.createLayer\(\)', src))
    classes = {os.path.basename(p)[:-5]: p for p in glob.glob(os.path.join(MODELS, '*', '*.java'))}
    out = {}
    for name, cls, const in re.findall(r'addArmorModel\("(\w+)",\s*(?:(\w+)::createLayer|\(\)\s*->\s*(\w+))\)', src):
        out[name] = classes[cls or consts[const]]
    return out


def items():
    """(item id, texture base, model name, slot) per armour item."""
    types = {}
    for line in open(os.path.join(JAVA, 'item', 'armor', 'ArmorTypes.java')):
        m = re.search(r'(\w+)\s*=\s*create\(Identifier\.fromNamespaceAndPath\("magistuarmory",\s*"(\w+)"\),\s*Identifier\.(?:parse\("magistuarmory:(\w+)"\)|withDefaultNamespace\("(\w+)"\))', line)
        if m:
            types[m.group(1)] = (m.group(2), m.group(3) or m.group(4))
    out = []
    for line in open(os.path.join(JAVA, 'item', 'ModItems.java')):
        m = re.search(r'Item\("(\w+)",\s*ArmorTypes\.(\w+),\s*[\w.]*ArmorType\.(\w+)', line)
        if m and m.group(2) in types:
            tex, model = types[m.group(2)]
            slot = SLOT_OF[m.group(3)]
            special = (slot == 'helmet' and model not in CHEST_MODELS) or (slot == 'chest' and model in CHEST_MODELS)
            out.append((m.group(1), tex, model if special and model != 'default' else 'default', slot))
    return out


def main():
    os.makedirs(OUT_MESH, exist_ok=True)
    os.makedirs(OUT_TEX, exist_ok=True)
    mods, geos, recs, missing = models(), {}, [], []
    for item, tex, model, slot in items():
        geo = model if model != 'default' else 'default_' + slot
        if geo not in geos:
            cls = mods['default_leggings' if geo == 'default_leggings' else model]
            parts, texsize = parse_model(cls)
            keep = SLOT_PARTS[slot]
            render = [i for i, p in enumerate(parts) if p['parent'] < 0 and p['name'] in keep]
            g = {'name': geo, 'tex': texsize, 'parts': parts, 'render': render}
            json.dump(g, open(os.path.join(OUT_MESH, geo + '.json'), 'w'), indent=1)
            geos[geo] = sum(len(p['boxes']) for i, p in enumerate(parts) if i in render or p['parent'] in render)
        if geos[geo] == 0: # a decoration's crown: code-driven, nothing to draw
            continue
        layer = f'{tex}_layer_{2 if slot == "leggings" else 1}.png'
        src = None
        for d in ([os.path.join(PACK, 'assets', 'magistuarmory', 'textures', 'models', 'armor')] if PACK else []) + [os.path.join(MOD, 'resources', 'assets', 'magistuarmory', 'textures', 'models', 'armor')]:
            if os.path.exists(os.path.join(d, layer)):
                src = os.path.join(d, layer)
                break
        if src is None:
            missing.append(layer)
            continue
        over = src[:-4] + '_overlay.png'  # a dyeable piece: the plain part is the overlay, the layer is the dyed part (undyed here)
        if os.path.exists(over):
            from PIL import Image
            img = Image.open(src).convert('RGBA')
            img.alpha_composite(Image.open(over).convert('RGBA').resize(img.size, Image.NEAREST))
            img.save(os.path.join(OUT_TEX, layer))
        else:
            shutil.copyfile(src, os.path.join(OUT_TEX, layer))
        recs.append((item, item.replace('_', ' '), slot, f'../knight/meshes/{geo}.json', f'../knight/armor/{layer}'))
    out = ['# meshes.rec: the armour pieces the inventory hero can wear as cube meshes, built by tools/convertarmor.py',
           '# from the Epic Knights models (MIT). gear.rec maps an armour to them (helmet/chest/leggings/boots: an id here).',
           '', '%rec: mesh', '%doc: Armour meshes']
    for r in recs:
        out += ['', f'id: {r[0]}', f'name: {r[1]}', f'piece: {r[2]}', f'geo: {r[3]}', f'tex: {r[4]}']
    open(REC, 'w').write('\n'.join(out) + '\n')
    print(f'{len(geos)} geometries ({sum(geos.values())} drawn boxes), {len(recs)} pieces; missing textures: {missing}')
    for g, n in sorted(geos.items()):
        print(f'  {g}: {n} boxes')


if __name__ == '__main__':
    main()
