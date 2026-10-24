package main

import (
	"image"
	"math"
	"testing"

	"github.com/memmaker/voxelrogue/rogue"
)

// TestMonsterBoxes pins two sprites' boxes to their opaque bounds: the
// kobold's columns 2-9 (6 from the centre column at most) over 10 rows,
// the dragon's whole cell; the box is as high as the drawn rows and as
// wide as the farthest opaque column sweeps.
func TestMonsterBoxes(t *testing.T) {
	names, imgs, err := loadSprites("assets/tiles")
	if err != nil {
		t.Skip(err)
	}
	g := &game{}
	g.sprite = map[string]image.Image{}
	for i, n := range names {
		g.sprite[n] = imgs[i]
	}
	for _, c := range []struct {
		letter byte
		bb     image.Rectangle
		r, h   float64
	}{
		{'K', image.Rect(2, 4, 10, 14), 6 * 1.1 * monsterScale / 10, 1.1 * monsterScale},
		{'D', image.Rect(0, 0, 15, 16), 8 * 2.8 * monsterScale / 16, 2.8 * monsterScale},
	} {
		bb := bbox(g.sprite["m:"+rogue.Monsters[c.letter-'A'].Name])
		b := g.box(c.letter)
		if bb != c.bb || math.Abs(float64(b.r)-c.r) > 1e-5 || math.Abs(float64(b.h)-c.h) > 1e-5 {
			t.Fatalf("%c: opaque %v, box %+v; want %v, r %.3f h %.3f", c.letter, bb, b, c.bb, c.r, c.h)
		}
	}
	if b := g.box('D'); b.h > roomHeight {
		t.Fatalf("the dragon is %v high, rooms %d", b.h, roomHeight)
	}
}
