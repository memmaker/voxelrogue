package main

import (
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
