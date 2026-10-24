package main

import (
	"testing"

	"github.com/memmaker/voxelrogue/dungeon"
)

// TestBoltPath pins fire_bolt's walk: six counted squares from the hero's
// own, the bounce free, so a wall three squares off sends the bolt back to
// the hero and one four squares off does not.
func TestBoltPath(t *testing.T) {
	at := func(c dungeon.Coord) dungeon.Tile {
		if c.Y == 5 && c.X >= 0 && c.X < 10 {
			return dungeon.Floor
		}
		return dungeon.Rock
	}
	hero := dungeon.Coord{X: 7, Y: 5}
	p := boltPath(at, hero, dungeon.Coord{X: 1})
	want := []int{7, 8, 9, 10, 9, 8, 7}
	if len(p) != len(want) {
		t.Fatalf("%d squares, want %d: %v", len(p), len(want), p)
	}
	for i, x := range want {
		if p[i].c.X != x || p[i].wall != (i == 3) || p[i].bounced != (i >= 3) {
			t.Errorf("square %d: %+v", i, p[i])
		}
	}
	if p = boltPath(at, dungeon.Coord{X: 6, Y: 5}, dungeon.Coord{X: 1}); len(p) != 7 || p[6].c.X != 8 {
		t.Errorf("from 6: %v", p)
	}
}
