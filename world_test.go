package main

import (
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// TestGenerate checks that a generated chunk has rock and a carved room.
func TestGenerate(t *testing.T) {
	levels := buildLevels(7)
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
