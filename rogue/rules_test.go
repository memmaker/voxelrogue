package rogue

import (
	"math"
	"math/rand"
	"testing"
)

// TestHitScale pins the swing roll's average as a damage scale: level 1
// against armour 7 with +1 was 9 in 20, a +100 bolt always lands, and a
// blow the roll could never win does nothing.
func TestHitScale(t *testing.T) {
	for _, c := range []struct{ d, lvl, arm, plus, want int }{{10, 1, 7, 1, 5}, {6, 1, 7, 100, 6}, {6, 0, -5, 0, 0}, {3, 1, 7, 1, 1}} {
		if got := HitScale(c.d, c.lvl, c.arm, c.plus); got != c.want {
			t.Errorf("HitScale(%d, %d, %d, %d) = %d, want %d", c.d, c.lvl, c.arm, c.plus, got, c.want)
		}
	}
}

// TestAttackScale: Scale multiplies a blow's damage (a wand strikes at
// 3/5 of a spear's).
func TestAttackScale(t *testing.T) {
	att, def := Stats{Lvl: 1, Str: Str{Str: 16}}, Stats{HP: 100}
	full := RollEm(rand.New(rand.NewSource(1)), &att, &def, 10, Attack{Dmg: "10d1"})
	scaled := RollEm(rand.New(rand.NewSource(1)), &att, &def, 10, Attack{Dmg: "10d1", Scale: 0.6})
	if full == 0 || scaled != int(math.Round(float64(full)*0.6)) {
		t.Errorf("full %d, scaled %d", full, scaled)
	}
}

// TestScrollLooks: every scroll kind gets a look of its own each game,
// drawn from the whole pool.
func TestScrollLooks(t *testing.T) {
	ScrollLooks = 62
	seen := map[int]bool{}
	for seed := int64(1); seed <= 20; seed++ {
		g := NewGame(rand.New(rand.NewSource(seed)))
		used := map[int]bool{}
		for _, l := range g.SLook {
			if l < 0 || l >= ScrollLooks || used[l] {
				t.Fatalf("seed %d: looks %v", seed, g.SLook)
			}
			used[l], seen[l] = true, true
		}
	}
	if len(seen) < 40 {
		t.Errorf("only %d of %d looks drawn over 20 games", len(seen), ScrollLooks)
	}
}
