package main

import (
	"image"
	"math"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxelrogue/dungeon"
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

// TestSlabHit: an arrow flying at a kobold from the hero's side meets the
// turned slab, not the air a sweep radius in front of it, and one
// passing beside the opaque columns misses.
func TestSlabHit(t *testing.T) {
	names, imgs, err := loadSprites("assets/tiles")
	if err != nil {
		t.Skip(err)
	}
	g := &game{}
	g.sprite = map[string]image.Image{}
	for i, n := range names {
		g.sprite[n] = imgs[i]
	}
	g.player = character.New(0, 0, 0)
	m := &monster{letter: 'K'}
	tgt := arrowTarget{g, m}
	fx, fy, fz, _ := tgt.Frame()
	g.player.X, g.player.Z = fx+3, fz+4 // off the axes, so the slab is turned
	b := g.box('K')
	ux, uz := 3.0/5, 4.0/5
	y := fy + float64(b.h)/2
	x, _, z, _, ok := tgt.Intersect(fx+ux*5, y, fz+uz*5, fx-ux*5, y, fz-uz*5)
	if d := math.Hypot(x-fx, z-fz); !ok || d > float64(b.d)+1e-6 {
		t.Fatalf("hit %v at %.3f from the slab's axis, want within %.3f (r %.3f)", ok, d, b.d, b.r)
	}
	// past the slab's widest side, sideways
	off := float64(max(-b.x0, b.x1)) + 0.05
	sx, sz := -uz*off, ux*off
	if _, _, _, _, ok := tgt.Intersect(fx+sx+ux*5, y, fz+sz+uz*5, fx+sx-ux*5, y, fz+sz-uz*5); ok {
		t.Fatal("an arrow beside the slab hit it")
	}
}

// TestSlabReach: a blow reaches a monster by the distance to its box's
// face, not to a radius around it, and the face is where it lands.
func TestSlabReach(t *testing.T) {
	names, imgs, err := loadSprites("assets/tiles")
	if err != nil {
		t.Skip(err)
	}
	g := &game{}
	g.sprite = map[string]image.Image{}
	for i, n := range names {
		g.sprite[n] = imgs[i]
	}
	m := &monster{letter: 'K'}
	g.monsters = []*monster{m}
	g.player = character.New(0, 0, 0)
	x, _, z := g.monsterPos(m, 0)
	b := g.box('K')
	for _, c := range []struct {
		gap  float64 // from the face
		want bool
	}{{0.4, true}, {0.6, false}} {
		g.player.X, g.player.Z = x, z+float64(b.d)+c.gap                                 // south of it
		g.cam.X, g.cam.Y, g.cam.Z, g.cam.Yaw = g.player.X, float64(b.h)/2, g.player.Z, 0 // looking north at it
		if got := g.monsterInFront(0.5) != nil; got != c.want {
			t.Fatalf("%.1f from the face, reach 0.5: hit %v (r %.2f)", c.gap, got, b.r)
		}
	}
	g.player.Z = z + float64(b.d) + 0.4
	g.cam.Z = g.player.Z
	if _, _, iz := g.impact(m); math.Abs(iz-(z+float64(b.d))) > 1e-6 {
		t.Fatalf("the blow lands at z %.3f, the face is at %.3f", iz, z+float64(b.d))
	}
}

// TestSweep: an axe's sideways swing takes every monster in front within
// reach, the nearest first; its overhead strike and other weapons the
// nearest only.
func TestSweep(t *testing.T) {
	names, imgs, err := loadSprites("assets/tiles")
	if err != nil {
		t.Skip(err)
	}
	g := &game{}
	g.sprite = map[string]image.Image{}
	for i, n := range names {
		g.sprite[n] = imgs[i]
	}
	near, far := &monster{letter: 'K'}, &monster{letter: 'K', tile: dungeon.Coord{Y: -1}}
	g.monsters = []*monster{far, near}
	g.player = character.New(0, 0, 0)
	x, _, z := g.monsterPos(near, 0)
	g.player.X, g.player.Z = x, z+1 // south of them, looking north
	g.cam.X, g.cam.Y, g.cam.Z, g.cam.Yaw = g.player.X, 0.5, g.player.Z, 0
	if ms := g.monstersInFront(tileD + 1); len(ms) != 2 || ms[0] != near {
		t.Fatalf("in front: %d monsters, the nearest first %v", len(ms), len(ms) > 0 && ms[0] == near)
	}
	axe, mace := &rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("axe")}, &rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("mace")}
	if !sweeps(axe, 0) || sweeps(axe, 1) || sweeps(mace, 0) || sweeps(nil, 0) {
		t.Fatal("only an axe's sideways swing sweeps")
	}
}

// TestSleepers: a blow on a sleeper counts double, a dagger's five times;
// a noise wakes the sleepers within its reach and no others.
func TestSleepers(t *testing.T) {
	dagger, mace := &rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("dagger")}, &rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("mace")}
	if sleepScale(dagger) != 5 || sleepScale(mace) != 2 || sleepScale(nil) != 2 {
		t.Fatal("sleeping blows: dagger 5x, the rest 2x")
	}
	g := &game{}
	near, far := &monster{letter: 'K', st: rogue.Stats{HP: 1}}, &monster{letter: 'K', st: rogue.Stats{HP: 1}, tile: dungeon.Coord{X: 10}}
	g.monsters = []*monster{near, far}
	x, _, z := g.monsterPos(near, 0)
	g.noise(x+1, z, 3)
	if !near.running || far.running {
		t.Fatalf("after a noise 3 blocks around: near awake %v, far awake %v", near.running, far.running)
	}
}
