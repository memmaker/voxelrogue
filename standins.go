package main

// Each monster's size and sprite name; objects' sprite names. The
// pictures themselves come from the tile sheets (tiles.go).

import (
	"fmt"
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxelrogue/rogue"
)

// monsterLook is the stand-in shape of a monster.
type monsterLook struct {
	w, h  float32 // hit box in blocks; the sprite is drawn h high
	hover float32 // floats this high above the floor
}

var monsterLooks = map[byte]monsterLook{
	'A': {0.9, 0.6, 0},
	'B': {0.6, 0.4, 1.4},
	'C': {1.0, 2.2, 0},
	'D': {2.0, 2.8, 0},
	'E': {0.7, 0.7, 0.9},
	'F': {0.9, 1.0, 0},
	'G': {0.6, 1.0, 0},
	'H': {0.8, 1.6, 0},
	'I': {0.8, 1.9, 0},
	'J': {0.7, 0.8, 0},
	'K': {0.6, 1.1, 0},
	'L': {0.5, 0.9, 0},
	'M': {1.0, 1.0, 0},
	'N': {0.6, 1.7, 0},
	'O': {0.9, 1.8, 0},
	'P': {1.6, 2.4, 0},
	'Q': {0.6, 1.0, 0.5},
	'R': {1.0, 1.0, 0},
	'S': {0.9, 0.4, 0},
	'T': {1.3, 2.6, 0},
	'U': {1.4, 2.4, 0},
	'V': {0.8, 1.9, 0},
	'W': {0.8, 1.9, 0.2},
	'X': {1.2, 1.2, 0},
	'Y': {1.3, 2.4, 0},
	'Z': {0.8, 1.8, 0},
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
// weaponSprites by kind; the crossbow has no sprite of its own (in the
// hand it is VoxelStealth's model), the missiles use the arrow sprites.
var weaponSprites = [rogue.MaxWeapons]string{"mace_01", "sword_01", "w:bow", "bow_29", "sword_49", "valuable_02",
	"sword_25", "w:sling", "bow_35", "w:crossbow", "bow_31", "spear_02",
	"w:broadsword", "w:glaive", "w:greatsword", "w:halberd", "w:katana", "w:scythe", "w:war_spear",
	"w:great_bow", "w:war_glaive", "w:scimitar", "w:wooden_pole",
	"w:hatchet", "axe_29", "axe_35", "mace_05", "mace_04", "w:wooden_pole", "staff_26", "spear_09", "spear_05", "w:rapier",
	"w:sickle", "sword_44", "sword_18", "sword_30", "sword_57", "mace_06", "gloves_04", "w:chain_crossbow"}

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
