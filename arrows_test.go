package main

import (
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxelrogue/rogue"
)

// TestLaunchers spans (crossbows), holds and lets go of every launcher,
// given its ammunition as the debug keys do, and expects a shot.
func TestLaunchers(t *testing.T) {
	for _, which := range []int{rogue.Bow, rogue.GreatBow, rogue.Crossbow, rogue.ChainCrossbow, rogue.Sling} {
		g := &game{rng: rand.New(rand.NewSource(1))}
		g.arrowObjs, g.player, g.rg = map[*projectile.Arrow]*rogue.Object{}, &character.Character{}, rogue.NewGame(g.rng)
		g.weapon = rogue.NewWeapon(g.rng, which)
		g.pack = []*rogue.Object{g.weapon}
		g.giveAmmo()
		if isCrossbow(which) {
			g.startReload()
			for range reloadTicks(which) {
				g.tickDraw()
			}
		}
		g.startDraw()
		for range 40 {
			g.tickDraw()
		}
		g.releaseDraw()
		for range 40 {
			g.tickDraw()
		}
		if len(g.arrowObjs) == 0 {
			t.Errorf("weapon %d: nothing shot (pack %d)", which, len(g.pack))
		}
	}
}

// TestThrowWeapon throws the wielded weapon: one dart of a bundle leaves
// the rest in hand, a lone dagger leaves the hands bare, and bare hands
// throw nothing.
func TestThrowWeapon(t *testing.T) {
	g := &game{rng: rand.New(rand.NewSource(1))}
	g.arrowObjs, g.player, g.rg = map[*projectile.Arrow]*rogue.Object{}, &character.Character{}, rogue.NewGame(g.rng)
	darts := rogue.NewWeapon(g.rng, rogue.Dart)
	darts.Count = 3
	dagger := rogue.NewWeapon(g.rng, rogue.Dagger)
	mace := rogue.NewWeapon(g.rng, rogue.Mace)
	g.pack = []*rogue.Object{darts, dagger, mace}
	g.weapon = darts
	g.throwItem()
	if darts.Count != 2 || g.weapon != darts || len(g.arrowObjs) != 1 {
		t.Fatalf("darts: %d left, wielding %v, %d flying", darts.Count, g.weapon, len(g.arrowObjs))
	}
	g.weapon = dagger
	g.throwItem()
	g.throwItem() // bare hands
	if g.weapon != nil || len(g.pack) != 2 || len(g.arrowObjs) != 2 {
		t.Fatalf("dagger: wielding %v, pack %d, %d flying", g.weapon, len(g.pack), len(g.arrowObjs))
	}
	g.weapon = mace
	g.throwItem()
	for a, o := range g.arrowObjs {
		if (a.Bounce > 0) != blunt[o.Which] {
			t.Errorf("%s: bounce %.1f; sharp weapons stick, blunt ones drop", rogue.WeaponNames[o.Which], a.Bounce)
		}
	}
	if len(g.arrowObjs) != 3 {
		t.Fatalf("mace: %d flying", len(g.arrowObjs))
	}
	g.nextWeapon() // 3 with bare hands used to crash
	if g.weapon == nil {
		t.Fatal("nextWeapon wielded nothing")
	}
}

// TestMassCenter: a sprite with only its top-right pixel set has its
// centre of mass there.
func TestMassCenter(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.Set(3, 0, color.NRGBA{255, 255, 255, 255})
	if c := massCenter(img); c != [2]float32{0.375, 0.375} {
		t.Errorf("centre of mass %v, want 0.375, 0.375", c)
	}
}
