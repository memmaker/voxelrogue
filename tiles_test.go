package main

import (
	"fmt"
	"testing"

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
	for _, w := range weaponSprites {
		if w != "" {
			want = append(want, w)
		}
	}
	for _, m := range rogue.Monsters {
		want = append(want, "m:"+m.Name)
	}
	want = append(want, "misc_01", "misc_21", "staff_01", "staff_10", "valuable_09", "amulet_01", "w:sling_model")
	for i := 1; i <= rogue.MaxArmors; i++ {
		want = append(want, fmt.Sprintf("armor_%02d", i*3))
	}
	for i := 1; i <= 15; i++ {
		want = append(want, fmt.Sprintf("scroll_%02d", i))
	}
	for i := 0; i < 24; i++ {
		want = append(want, fmt.Sprintf("potion_%02d", i*3+1))
	}
	for i := 1; i <= 18; i++ {
		want = append(want, fmt.Sprintf("ring_%02d", i))
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("no tile for sprite %q", n)
		}
	}
	blocks, err := loadTiles("assets/tiles/textures.rec")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"stone", "stone_bricks", "oak_planks", "dark_oak_planks", "cobblestone", "glowstone", "redstone_ore", "oak_log", "oak_log_top"} {
		if blocks["block/"+n] == nil {
			t.Errorf("no tile for block texture %q", n)
		}
	}
}
