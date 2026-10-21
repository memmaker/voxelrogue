package main

// Stand-in art until the real monster models arrive: each monster is a
// box with its Rogue letter on every side, sized roughly like the
// creature. Objects use the CC0 RPG item sprites shipped with voxellib;
// the mod weapons use their mods' MIT textures from assets/weapons.

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxelrogue/rogue"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// monsterLook is the stand-in shape of a monster.
type monsterLook struct {
	w, h  float32 // box size in blocks
	hover float32 // floats this high above the floor
	col   color.NRGBA
}

var monsterLooks = map[byte]monsterLook{
	'A': {0.9, 0.6, 0, color.NRGBA{120, 30, 20, 255}},
	'B': {0.6, 0.4, 1.4, color.NRGBA{60, 50, 60, 255}},
	'C': {1.0, 2.2, 0, color.NRGBA{150, 110, 60, 255}},
	'D': {2.0, 2.8, 0, color.NRGBA{170, 30, 30, 255}},
	'E': {0.7, 0.7, 0.9, color.NRGBA{230, 230, 240, 255}},
	'F': {0.9, 1.0, 0, color.NRGBA{140, 60, 170, 255}},
	'G': {0.6, 1.0, 0, color.NRGBA{90, 130, 60, 255}},
	'H': {0.8, 1.6, 0, color.NRGBA{170, 90, 40, 255}},
	'I': {0.8, 1.9, 0, color.NRGBA{200, 220, 255, 255}},
	'J': {0.7, 0.8, 0, color.NRGBA{170, 140, 90, 255}},
	'K': {0.6, 1.1, 0, color.NRGBA{110, 90, 70, 255}},
	'L': {0.5, 0.9, 0, color.NRGBA{40, 150, 50, 255}},
	'M': {1.0, 1.0, 0, color.NRGBA{140, 100, 50, 255}},
	'N': {0.6, 1.7, 0, color.NRGBA{230, 160, 190, 255}},
	'O': {0.9, 1.8, 0, color.NRGBA{80, 120, 50, 255}},
	'P': {1.6, 2.4, 0, color.NRGBA{110, 40, 140, 255}},
	'Q': {0.6, 1.0, 0.5, color.NRGBA{170, 50, 50, 255}},
	'R': {1.0, 1.0, 0, color.NRGBA{170, 90, 30, 255}},
	'S': {0.9, 0.4, 0, color.NRGBA{50, 140, 60, 255}},
	'T': {1.3, 2.6, 0, color.NRGBA{70, 100, 60, 255}},
	'U': {1.4, 2.4, 0, color.NRGBA{120, 80, 40, 255}},
	'V': {0.8, 1.9, 0, color.NRGBA{60, 20, 40, 255}},
	'W': {0.8, 1.9, 0.2, color.NRGBA{90, 90, 110, 255}},
	'X': {1.2, 1.2, 0, color.NRGBA{100, 100, 100, 255}},
	'Y': {1.3, 2.4, 0, color.NRGBA{235, 235, 245, 255}},
	'Z': {0.8, 1.8, 0, color.NRGBA{100, 120, 90, 255}},
}

// letterTexture is a 32x32 face: the monster's colour with its letter.
func letterTexture(letter byte, bg color.NRGBA) image.Image {
	small := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	d := &font.Drawer{Dst: small, Src: image.NewUniform(color.NRGBA{255, 255, 255, 255}), Face: basicfont.Face7x13, Dot: fixed.P(4, 13)}
	d.DrawString(string(letter))
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	edge := color.NRGBA{bg.R / 2, bg.G / 2, bg.B / 2, 255}
	for i := 0; i < 32; i++ {
		img.Set(i, 0, edge)
		img.Set(i, 31, edge)
		img.Set(0, i, edge)
		img.Set(31, i, edge)
	}
	ink := color.NRGBA{20, 20, 20, 255}
	if int(bg.R)+int(bg.G)+int(bg.B) < 380 {
		ink = color.NRGBA{250, 240, 200, 255}
	}
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if _, _, _, a := small.At(x, y).RGBA(); a > 0 {
				img.Set(x*2, y*2, ink)
				img.Set(x*2+1, y*2, ink)
				img.Set(x*2, y*2+1, ink)
				img.Set(x*2+1, y*2+1, ink)
			}
		}
	}
	return img
}

// boxModel is an axis-aligned box w wide, h high, standing on the origin,
// every face showing texture 0.
func boxModel(name string, w, h float32, tex image.Image) *model.Model {
	m := &model.Model{Name: name, Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{}}
	m.Bones = []model.Bone{{Name: "root", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}
	r := w / 2
	type face struct{ n, u, v, o mgl32.Vec3 }
	faces := []face{
		{mgl32.Vec3{0, 0, 1}, mgl32.Vec3{w, 0, 0}, mgl32.Vec3{0, h, 0}, mgl32.Vec3{-r, 0, r}},
		{mgl32.Vec3{0, 0, -1}, mgl32.Vec3{-w, 0, 0}, mgl32.Vec3{0, h, 0}, mgl32.Vec3{r, 0, -r}},
		{mgl32.Vec3{1, 0, 0}, mgl32.Vec3{0, 0, -w}, mgl32.Vec3{0, h, 0}, mgl32.Vec3{r, 0, r}},
		{mgl32.Vec3{-1, 0, 0}, mgl32.Vec3{0, 0, w}, mgl32.Vec3{0, h, 0}, mgl32.Vec3{-r, 0, -r}},
		{mgl32.Vec3{0, 1, 0}, mgl32.Vec3{w, 0, 0}, mgl32.Vec3{0, 0, -w}, mgl32.Vec3{-r, h, r}},
		{mgl32.Vec3{0, -1, 0}, mgl32.Vec3{w, 0, 0}, mgl32.Vec3{0, 0, w}, mgl32.Vec3{-r, 0, -r}},
	}
	var me model.Mesh
	for _, f := range faces {
		base := uint32(len(me.Verts))
		for _, c := range [4][2]float32{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
			p := f.o.Add(f.u.Mul(c[0])).Add(f.v.Mul(c[1]))
			me.Verts = append(me.Verts, model.Vertex{Pos: p, Normal: f.n, UV: mgl32.Vec2{c[0], 1 - c[1]}, Weights: [4]float32{1}})
		}
		me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
	}
	m.Meshes = []model.Mesh{me}
	return m
}

// spriteNames lists every sprite the game uses, loaded from itemDir.
func spriteNames() []string {
	names := []string{"misc_01", "misc_21", "misc_10", "valuable_02", "valuable_09", "amulet_01", "staff_01", "staff_10",
		"sword_01", "sword_25", "sword_49", "mace_01", "spear_02", "bow_01", "bow_10", "bow_29", "bow_31", "bow_35"}
	for i := 1; i <= rogue.MaxArmors; i++ {
		names = append(names, fmt.Sprintf("armor_%02d", i*3))
	}
	for i := 1; i <= 15; i++ {
		names = append(names, fmt.Sprintf("scroll_%02d", i))
	}
	for i := 0; i < 24; i++ {
		names = append(names, fmt.Sprintf("potion_%02d", i*3+1))
	}
	for i := 1; i <= 18; i++ {
		names = append(names, fmt.Sprintf("ring_%02d", i))
	}
	return names
}

// weaponSprites by kind; the crossbow has no sprite of its own (in the
// hand it is VoxelStealth's model), the missiles use the arrow sprites.
var weaponSprites = [rogue.MaxWeapons]string{"mace_01", "sword_01", "bow_01", "bow_29", "sword_49", "valuable_02",
	"sword_25", "misc_10", "bow_35", "bow_10", "bow_31", "spear_02",
	"w:broadsword", "w:glaive", "w:greatsword", "w:halberd", "w:katana", "w:scythe", "w:war_spear",
	"w:great_bow", "w:war_glaive", "w:scimitar", "w:wooden_pole"}

// handSprites are the sprites with a separate in-hand picture
// (name+"_hand"), filled by loadSprites.
var handSprites = map[string]bool{}

// spriteFor is the sprite an object is drawn with; unidentified potions,
// scrolls and rings look like their colour, title or stone this game.
func spriteFor(g *rogue.Game, o *rogue.Object) string {
	switch o.Type {
	case rogue.Weapon:
		return weaponSprites[o.Which]
	case rogue.Armor:
		return fmt.Sprintf("armor_%02d", (o.Which+1)*3)
	case rogue.Scroll:
		return fmt.Sprintf("scroll_%02d", g.SLook[o.Which]%15+1)
	case rogue.Potion:
		return fmt.Sprintf("potion_%02d", g.PLook[o.Which]%24*3+1)
	case rogue.Ring:
		return fmt.Sprintf("ring_%02d", g.RLook[o.Which]%18+1)
	case rogue.Food:
		if o.Which == 1 {
			return "misc_21"
		}
		return "misc_01"
	case rogue.Stick:
		if o.Which%2 == 0 {
			return "staff_01"
		}
		return "staff_10"
	case rogue.Gold:
		return "valuable_09"
	}
	return "amulet_01"
}

// loadSprites loads spriteNames from dir and every PNG of modDir as
// "w:"+its base name.
func loadSprites(dir, modDir string) ([]string, []image.Image, error) {
	names := spriteNames()
	paths := make([]string, len(names))
	for i, n := range names {
		paths[i] = filepath.Join(dir, n+".png")
	}
	mods, err := filepath.Glob(filepath.Join(modDir, "*.png"))
	if err != nil || len(mods) == 0 {
		return nil, nil, fmt.Errorf("no weapon sprites in %s", modDir)
	}
	for _, p := range mods {
		n := "w:" + strings.TrimSuffix(filepath.Base(p), ".png")
		names, paths = append(names, n), append(paths, p)
		if base, ok := strings.CutSuffix(n, "_hand"); ok {
			handSprites[base] = true
		}
	}
	imgs := make([]image.Image, len(names))
	for i, n := range names {
		f, err := os.Open(paths[i])
		if err != nil {
			return nil, nil, err
		}
		imgs[i], err = png.Decode(f)
		f.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", n, err)
		}
	}
	return names, imgs, nil
}
