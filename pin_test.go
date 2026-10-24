package main

import (
	"image"
	"math"
	"testing"

	"github.com/memmaker/voxellib/projectile"
)

// TestPin: a kobold killed by an arrow flying west with a wall 2 blocks
// behind is carried to it, its back against the wall and the tip in it;
// with the wall farther than pinReach it is not pinned.
func TestPin(t *testing.T) {
	names, imgs, err := loadSprites("assets/tiles")
	if err != nil {
		t.Skip(err)
	}
	g := &game{}
	g.sprite = map[string]image.Image{}
	for i, n := range names {
		g.sprite[n] = imgs[i]
	}
	d := float64(g.box('K').d)
	for _, c := range []struct {
		wall float64 // x of the wall's east face
		pin  bool
	}{{18, true}, {14, false}} {
		m := &monster{letter: 'K', dying: 1, dead: [4]float64{20, 425, 70, math.Pi / 2}}
		a := &projectile.Arrow{X: 20 + d - 0.1, Y: 425.9, Z: 70, Yaw: -math.Pi / 2, Stuck: true} // tip just inside its face
		g.arrows = projectile.World{Arrows: []*projectile.Arrow{a}}
		g.pinTo(m, a, func(x, _, _ int) bool { return float64(x) < c.wall })
		if (m.pin != nil) != c.pin {
			t.Fatalf("wall at %v: pinned %v", c.wall, m.pin != nil)
		}
		if !c.pin {
			continue
		}
		g.corpses = map[int][]*monster{0: {m}}
		m.dying = pinTicks + 1
		g.tickPins()
		if math.Abs(a.X-(c.wall-pinDepth)) > 0.06 {
			t.Errorf("tip at x %.2f, want %.2f in the wall", a.X, c.wall-pinDepth)
		}
		if back := g.pinnedPose(m, 0).X - d; math.Abs(back-c.wall) > 0.1 {
			t.Errorf("the body's back at x %.2f, the wall at %.2f", back, c.wall)
		}
	}
}
