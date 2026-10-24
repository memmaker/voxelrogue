package main

// Each monster's size and sprite name; objects' sprite names. The
// pictures themselves come from the tile sheets (tiles.go).

import (
	"fmt"
	"image"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxelrogue/rogue"
)

// monsterLook is a monster's size, relative to the others (monsterScale
// makes it blocks); the width comes from its sprite (monsterBox).
type monsterLook struct {
	h     float32 // the sprite's creature is drawn h * monsterScale high
	hover float32 // floats this high above the floor
}

var monsterLooks = map[byte]monsterLook{
	'A': {0.6, 0},
	'B': {0.4, 1.4},
	'C': {2.2, 0},
	'D': {2.8, 0},
	'E': {0.7, 0.9},
	'F': {1.0, 0},
	'G': {1.0, 0},
	'H': {1.6, 0},
	'I': {1.9, 0},
	'J': {0.8, 0},
	'K': {1.1, 0},
	'L': {0.9, 0},
	'M': {1.0, 0},
	'N': {1.7, 0},
	'O': {1.8, 0},
	'P': {2.4, 0},
	'Q': {1.0, 0.5},
	'R': {1.0, 0},
	'S': {1.1, 0}, // coiled, head raised: big enough to fear
	'T': {2.6, 0},
	'U': {2.4, 0},
	'V': {1.9, 0},
	'W': {1.9, 0.2},
	'X': {1.2, 0},
	'Y': {2.4, 0},
	'Z': {1.8, 0},
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
// weaponSprite is the sprite a weapon kind is drawn with: "w:" and its
// id in weapons.rec.
func weaponSprite(which int) string { return "w:" + rogue.Weapons[which].ID }

// weaponKind is the weapon a sprite belongs to ("w:" + id, or that and a
// suffix: the pulling and loaded pictures), nil for other sprites.
func weaponKind(sprite string) *rogue.WeaponKind {
	for k := range rogue.Weapons {
		if w := weaponSprite(k); sprite == w || strings.HasPrefix(sprite, w+"_") {
			return &rogue.Weapons[k]
		}
	}
	return nil
}

// handSprites are the sprites with a separate in-hand picture
// (name+"_hand"), filled by loadSprites.
var handSprites = map[string]bool{}

// spriteFor is the items.rec id an object is drawn with: its name, or
// for potions, rings and scrolls the colour, stone or one of 15 looks
// it has this game; wands are a staff or a wand.
func spriteFor(g *rogue.Game, o *rogue.Object) string {
	switch o.Type {
	case rogue.Weapon:
		return weaponSprite(o.Which)
	case rogue.Armor:
		return rogue.Armors[o.Which].ID
	case rogue.Scroll:
		return fmt.Sprintf("scroll %d", g.SLook[o.Which]%15+1)
	case rogue.Potion:
		return g.PColors[o.Which] + " potion"
	case rogue.Ring:
		return g.RStones[o.Which] + " ring"
	case rogue.Food:
		if o.Which == 1 {
			return "slime-mold"
		}
		return "food ration"
	case rogue.Stick:
		return g.SType[o.Which]
	case rogue.Gold:
		return "gold"
	}
	return "amulet of yendor"
}
