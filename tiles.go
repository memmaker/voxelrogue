package main

// Every 2D picture the game shows comes from the tile sheets in
// assets/tiles, one per tile size, mapped by the remapper's rec files
// (assets/tiles/mktiles.py builds them, remap_*.sh edits them): block
// textures, item and weapon sprites, monster sprites.

import (
	"encoding/json"
	"slices"

	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	geo "github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/ui/guientity"
	"github.com/memmaker/voxelrogue/rogue"

	"github.com/memmaker/voxellib/voxel/respack"
)

// loadTiles reads a typed rec file (a Tileset record, then id/icon
// records) and cuts every assigned id's tile out of the sheet; a record's
// w and h crop a sprite smaller than the cell.
func loadTiles(recPath string) (map[string]image.Image, error) {
	recs, err := rogue.ReadRecs(recPath)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 || recs[0]["%rec"] != "Tileset" {
		return nil, fmt.Errorf("%s: no %%rec: Tileset record first", recPath)
	}
	ts := recs[0]
	num := func(k string, def int) int {
		if n, err := strconv.Atoi(ts[k]); err == nil {
			return n
		}
		return def
	}
	f, err := os.Open(filepath.Join(filepath.Dir(recPath), ts["file"]))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheet, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ts["file"], err)
	}
	tw, th, ox, oy, gx, gy := num("tile_w", 16), num("tile_h", 16), num("off_x", 0), num("off_y", 0), num("gap_x", 0), num("gap_y", 0)
	cols := (sheet.Bounds().Dx() - ox + gx) / (tw + gx)
	tiles := map[string]image.Image{}
	for _, r := range recs[1:] {
		icon, err := strconv.Atoi(r["icon"])
		if r["id"] == "" || err != nil || icon < 0 {
			continue
		}
		w, h := tw, th
		if n, err := strconv.Atoi(r["w"]); err == nil {
			w = n
		}
		if n, err := strconv.Atoi(r["h"]); err == nil {
			h = n
		}
		x, y := ox+icon%cols*(tw+gx), oy+icon/cols*(th+gy)
		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(img, img.Bounds(), sheet, sheet.Bounds().Min.Add(image.Pt(x, y)), draw.Src)
		tiles[r["id"]] = img
	}
	return tiles, nil
}

// meshSlots are gear.rec's armour mesh fields, in guientity piece order.
var meshSlots = [4]string{"helmet", "chest", "leggings", "boots"}

// loadMeshes reads the tile directory's gear.rec armour records' mesh
// fields and the meshes.rec pieces they name (tools/convertarmor.py's
// Epic Knights models): the armour meshes by id, and each armour's
// mesh ids by slot.
func loadMeshes(dir string) (map[string]guientity.Armor, map[string][4]string, error) {
	gear, err := rogue.ReadRecs(filepath.Join(dir, "gear.rec"))
	if err != nil || len(gear) == 0 || gear[0]["meshes"] == "" {
		return nil, nil, err
	}
	recs, err := rogue.ReadRecs(filepath.Join(dir, gear[0]["meshes"]))
	if err != nil {
		return nil, nil, err
	}
	meshes, wear := map[string]guientity.Armor{}, map[string][4]string{}
	for _, r := range recs {
		if r["id"] == "" || r["piece"] == "" {
			continue
		}
		a := guientity.Armor{Piece: slices.Index(meshSlots[:], r["piece"])}
		data, err := os.ReadFile(filepath.Join(dir, r["geo"]))
		if err != nil {
			return nil, nil, err
		}
		a.Geo = &geo.Geometry{}
		if err := json.Unmarshal(data, a.Geo); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", r["geo"], err)
		}
		if a.Tex, err = loadNRGBA(filepath.Join(dir, r["tex"])); err != nil {
			return nil, nil, err
		}
		meshes[r["id"]] = a
	}
	for _, r := range gear[1:] {
		var ids [4]string
		for i, s := range meshSlots {
			ids[i] = r[s]
		}
		if ids != [4]string{} {
			wear[r["id"]] = ids
		}
	}
	return meshes, wear, nil
}

// sheetTextures serves the block textures of textures.rec (the game's
// cubes name them directly; the pack's stairs and log models name their
// own, aliased here) and leaves anything else to the resource pack.
type sheetTextures struct {
	pack  *respack.Pack
	tiles map[string]image.Image
}

var texAlias = map[string]string{"block/cobblestone": "stairs", "block/oak_log": "door frame", "block/oak_log_top": "door frame top"}

func (s *sheetTextures) tile(name string) image.Image {
	if _, rel, ok := strings.Cut(name, ":"); ok { // minecraft:block/stone
		name = rel
	}
	if a := texAlias[name]; a != "" {
		name = a
	}
	return s.tiles[name]
}

func (s *sheetTextures) Texture(name string) (image.Image, error) {
	if img := s.tile(name); img != nil {
		return img, nil
	}
	return s.pack.Texture(name)
}

func (s *sheetTextures) TextureFrames(name string) ([]image.Image, int, error) {
	if img := s.tile(name); img != nil {
		return []image.Image{img}, 0, nil
	}
	return s.pack.TextureFrames(name)
}

// loadSprites loads the item, gear, weapon ("w:" + id) and monster ("m:" + id)
// sprites of the tile directory's rec files.
func loadSprites(dir string) ([]string, []image.Image, error) {
	var names []string
	var imgs []image.Image
	for _, f := range []struct{ rec, prefix string }{{"items.rec", ""}, {"gear.rec", ""}, {"potions.rec", ""}, {"scrolls.rec", ""}, {"weapons.rec", "w:"}, {"monsters.rec", "m:"}} {
		tiles, err := loadTiles(filepath.Join(dir, f.rec))
		if err != nil {
			return nil, nil, err
		}
		for id, img := range tiles {
			n := f.prefix + id
			names, imgs = append(names, n), append(imgs, img)
			if base, ok := strings.CutSuffix(n, "_hand"); ok {
				handSprites[base] = true
			}
		}
	}
	return names, imgs, nil
}

// bbox is the rows and columns an image's opaque pixels span.
func bbox(img image.Image) image.Rectangle {
	b, r := img.Bounds(), image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				r = r.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return r
}
