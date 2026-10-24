package main

import (
	"fmt"
	"math/rand"
	"testing"

	geo "github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/ui/guientity"
	"github.com/memmaker/voxelrogue/rogue"
)

// TestTilesCoverEveryName: every sprite the game asks for by name (items,
// weapons, monsters) and every block texture the dungeon uses is assigned
// a cell in assets/tiles.
func TestTilesCoverEveryName(t *testing.T) {
	names, _, err := loadSprites("assets/tiles")
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	var want []string
	for _, m := range rogue.Monsters {
		want = append(want, "m:"+m.Name)
	}
	rg := rogue.NewGame(rand.New(rand.NewSource(3)))
	for t, n := range map[rogue.ObjType]int{rogue.Weapon: rogue.MaxWeapons, rogue.Armor: rogue.MaxArmors, rogue.Scroll: rogue.MaxScrolls,
		rogue.Potion: rogue.MaxPotions, rogue.Ring: rogue.MaxRings, rogue.Stick: rogue.MaxSticks, rogue.Food: 2, rogue.Gold: 1, rogue.Amulet: 1} {
		for which := 0; which < n; which++ {
			want = append(want, spriteFor(rg, &rogue.Object{Type: t, Which: which}))
		}
	}
	for _, w := range []int{rogue.Bow, rogue.GreatBow, rogue.Crossbow, rogue.ChainCrossbow} {
		for i := range 3 {
			want = append(want, fmt.Sprintf("%s_pulling_%d", weaponSprite(w), i))
		}
	}
	want = append(want, weaponSprite(rogue.Crossbow)+"_arrow", weaponSprite(rogue.ChainCrossbow)+"_arrow", "w:sling_model", "w:greatsword_hand", "w:scythe_hand")
	for _, n := range want {
		if !have[n] {
			t.Errorf("no tile for sprite %q", n)
		}
	}
	blocks, err := loadTiles("assets/tiles/textures.rec")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"room floor", "room wall", "room ceiling", "dark room floor", "room light", "corridor floor", "corridor wall", "corridor ceiling", "stairs", "trap", "door frame", "door frame top"} {
		if blocks[n] == nil {
			t.Errorf("no tile for block texture %q", n)
		}
	}
}

// TestMeshes expects meshes.rec's armet to build and the plate mail to
// wear it.
func TestMeshes(t *testing.T) {
	meshes, wear, err := loadMeshes("assets/tiles")
	if err != nil {
		t.Fatal(err)
	}
	a, ok := meshes["armet"]
	if !ok || a.Piece != guientity.Helmet || a.Tex == nil {
		t.Fatalf("armet %v %v", ok, a.Piece)
	}
	if m, err := geo.Build(a.Geo, nil); err != nil || len(m.Meshes) == 0 {
		t.Fatalf("armet build %v %v", err, m)
	}
	if wear["plate mail"][0] != "armet" {
		t.Fatalf("plate mail wears %v", wear["plate mail"])
	}
}
