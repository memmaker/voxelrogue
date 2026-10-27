package main

import (
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// TestGenerate checks that a generated chunk has rock and a carved room.
func TestGenerate(t *testing.T) {
	levels := buildLevels(7, false)
	reg := voxel.NewRegistry()
	gen := newLevelGen(reg, levels)
	w := voxel.NewWorld(reg, worldMinY, worldHeight)
	l := levels[0]
	c := l.RandomFloor(rand.New(rand.NewSource(1)))
	x, y, z := tileCenter(1, c)
	ch := w.NewChunk(voxel.ChunkPos{X: int(x) >> 4, Z: int(z) >> 4})
	gen.Generate(w, ch)
	w.AddChunk(ch)
	if b := w.Block(int(x), int(y)-1, int(z)); b == voxel.Air {
		t.Errorf("no floor under %v", c)
	}
	if b := w.Block(int(x), int(y), int(z)); b != voxel.Air {
		t.Errorf("tile %v not carved: %v", c, b)
	}
}

// TestSealStairs: a sealed staircase has no open way through its upper
// half (no air over the steps, the upper floor whole), while its foot in
// the room below stays open; reopened, it is the staircase again.
func TestSealStairs(t *testing.T) {
	levels := buildLevels(7, false)
	gen := newLevelGen(voxel.NewRegistry(), levels)
	l, f := levels[0], floorY(1)
	column := func(i int) map[int]voxel.StateID {
		x := l.Stairs.X*tileW + i
		if l.StairsDir < 0 {
			x = l.Stairs.X*tileW + tileW - 1 - i
		}
		col := map[int]voxel.StateID{}
		gen.fill(levels, x, l.Stairs.Y*tileD+tileD/2, func(y int, id voxel.StateID) { col[y] = id })
		return col
	}
	open := func(i int) bool { return column(i)[f-i+1] == voxel.Air } // the air over step i
	gen.seal(1, true)
	for i := range sealedSteps {
		if open(i) && i > 0 { // step 0 is the upper floor's own tile
			t.Errorf("sealed step %d still open", i)
		}
	}
	if column(1)[f] == voxel.Air {
		t.Error("the upper floor has a hole")
	}
	if !open(sealedSteps) {
		t.Error("the foot of the staircase is shut")
	}
	gen.seal(1, false)
	for i := 1; i < levelPitch; i++ {
		if !open(i) {
			t.Errorf("reopened step %d shut", i)
		}
	}
}
