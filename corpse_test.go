package main

import (
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxelrogue/rogue"
)

// TestCorpse kills a bat (a flyer) and expects its body out of the
// monsters, fallen flat on the floor where it was, drawn the same with
// the hero elsewhere, and still there 1000 ticks on.
func TestCorpse(t *testing.T) {
	levels := buildLevels(7)
	g := &game{rng: rand.New(rand.NewSource(1)), gen: newLevelGen(voxel.NewRegistry(), levels)}
	g.rg, g.player, g.depth = rogue.NewGame(g.rng), &character.Character{}, 1
	c := g.level().RandomFloor(g.rng)
	m := g.newMonster('B', c)
	g.player.X, g.player.Z = 3, 3
	g.killed(m)
	for range 1000 {
		g.tickMonsters()
	}
	if len(g.monsters) != 0 || len(g.corpses[1]) != 1 || g.corpses[1][0] != m {
		t.Fatalf("%d monsters, corpses %v", len(g.monsters), g.corpses)
	}
	a := g.corpsePose(m, 0.3)
	g.player.X, g.player.Z = 40, 70
	b := g.corpsePose(m, 0.9)
	x, y, z := tileCenter(1, c)
	if a != b || a.X != x || a.Z != z || a.Y != y {
		t.Fatalf("corpse at %v %v %v (%v), then %v %v %v (%v); fell at %v %v %v", a.X, a.Y, a.Z, a.M, b.X, b.Y, b.Z, b.M, x, y, z)
	}
}
