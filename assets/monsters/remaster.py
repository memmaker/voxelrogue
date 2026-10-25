#!/usr/bin/env python3
"""Remasters the 16px monster tiles of tiles16.png (monsters.rec's cells) into 256x256
sprites with Gemini, through BlobberRPG's new-monster.py (its generate and chroma cutout):
  python3 assets/monsters/remaster.py [name ...]     all 26 when no names are given
Each monster is sent as its tile blown up 16x and comes back head-on, looking at the
viewer, Wizardry style, on a magenta key; the cutout is fitted into assets/monsters/remastered/<name>.png
(RGBA, transparent). The raw Gemini image goes to ~/Pictures/voxelrogue-monsters/<name>.png.
A monster whose output exists is skipped: delete it to redo it."""
import importlib.util, io, re, sys
from pathlib import Path
from PIL import Image

HERE = Path(__file__).resolve().parent
TILES = HERE.parent / "tiles"
OUT = HERE / "remastered"
RAW = Path.home() / "Pictures/voxelrogue-monsters"
SIZE = 256
spec = importlib.util.spec_from_file_location("new_monster", Path.home() / "Projects/BlobberRPG/tools/new-monster.py")
nm = importlib.util.module_from_spec(spec)
spec.loader.exec_module(nm)
nm.CONFIG["imageConfig"]["aspectRatio"] = "1:1"

PROMPT = ("This pixel-art sprite is {name}, a monster from the roguelike game Rogue. Remaster it as a "
          "detailed, painted monster portrait for a classic Wizardry style first-person dungeon crawler: "
          "the same creature, keeping its colours, proportions and silhouette recognisable, but drawn with "
          "full detail and shading. It faces the viewer head-on, symmetrically, and looks straight into "
          "the camera, menacing. The whole monster fits inside the frame with a clear margin of empty "
          "background on all four sides: head, feet, claws, tail and wing tips all visible, nothing "
          "touching or cut off by the edge. Only the monster, no text. "
          + nm.BG.format(name="magenta", hex=nm.KEYS["magenta"][0]))


HINTS = {  # where the tile misleads the model
    "vampire": " It is a pale, gaunt vampire with two eyes, fangs and a high-collared black cape lined red.",
    "jackal": " It is a scrawny wild dog with pointed ears and a bushy tail, no horns or antlers.",
    "yeti": " It is a white-furred ape-like yeti standing upright, no tail.",
}


def monsters():
    """name -> tile cell of monsters.rec."""
    cells, rid = {}, None
    for line in (TILES / "monsters.rec").read_text().splitlines():
        if line.startswith("id: "):
            rid = line[4:].strip()
        elif line.startswith("icon: ") and rid:
            cells[rid] = int(line[6:])
    return cells


def tile(cell):
    """The cell on magenta, blown up 16x with hard pixels (what the model sees)."""
    sheet = Image.open(TILES / "tiles16.png").convert("RGBA")
    x, y = cell % 16 * 16, cell // 16 * 16
    t = Image.new("RGBA", (16, 16), nm.KEYS["magenta"][1] + (255,))
    t.alpha_composite(sheet.crop((x, y, x + 16, y + 16)))
    buf = io.BytesIO()
    t.resize((SIZE, SIZE), Image.NEAREST).convert("RGB").save(buf, "PNG")
    return buf.getvalue()


def fit(img):
    """The cutout centred in a SIZE square, longest side 15/16 of it."""
    img = img.crop(img.getbbox())
    s = SIZE * 15 // 16 / max(img.size)
    img = img.resize((max(1, round(img.width * s)), max(1, round(img.height * s))), Image.LANCZOS)
    out = Image.new("RGBA", (SIZE, SIZE), (0, 0, 0, 0))
    out.paste(img, ((SIZE - img.width) // 2, (SIZE - img.height) // 2))
    return out


def main():
    key, cells = nm.api_key(), monsters()
    OUT.mkdir(exist_ok=True)
    RAW.mkdir(parents=True, exist_ok=True)
    for name in sys.argv[1:] or cells:
        file = re.sub(r"\W+", "_", name)
        if (OUT / f"{file}.png").exists():
            print(f"{name}: done, skip", flush=True)
            continue
        print(f"{name}…", flush=True)
        for attempt in range(3):  # the model sometimes crops the monster at the frame edge
            png, _ = nm.generate(key, PROMPT.format(name=name) + HINTS.get(name, ""), image=tile(cells[name]))
            cut = nm.cutout(png, nm.KEYS["magenta"][1])
            l, t, r, b = cut.getbbox()
            if l > 0 and t > 0 and r < cut.width and b < cut.height:
                break
            print(f"  cut off at the edge, retrying ({attempt + 1}/3)", flush=True)
        (RAW / f"{file}.png").write_bytes(png)
        fit(cut).save(OUT / f"{file}.png")
        print(f"  -> {OUT / f'{file}.png'}", flush=True)


if __name__ == "__main__":
    main()
