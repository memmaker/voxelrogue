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

// TestScrollLooks: every scroll, ring and staff kind gets a look of its
// own each game, drawn from the whole pool.
func TestScrollLooks(t *testing.T) {
	ScrollLooks, RingLooks, StaffLooks = 62, 49, 40
	for _, pool := range []struct {
		name  string
		n     int
		looks func(*Game) []int
	}{{"scroll", ScrollLooks, func(g *Game) []int { return g.SLook[:] }}, {"ring", RingLooks, func(g *Game) []int { return g.RLook[:] }}, {"staff", StaffLooks, func(g *Game) []int { return g.StLook[:] }}} {
		seen := map[int]bool{}
		for seed := int64(1); seed <= 20; seed++ {
			g := NewGame(rand.New(rand.NewSource(seed)))
			used := map[int]bool{}
			for _, l := range pool.looks(g) {
				if l < 0 || l >= pool.n || used[l] {
					t.Fatalf("%s seed %d: looks %v", pool.name, seed, pool.looks(g))
				}
				used[l], seen[l] = true, true
			}
		}
		if len(seen) < 2*pool.n/3 {
			t.Errorf("only %d of %d %s looks drawn over 20 games", len(seen), pool.n, pool.name)
		}
	}
}

// TestPickMons: a vorpalized weapon's enemy is always a monster of the
// level list, mostly from its mean end.
func TestPickMons(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	n := map[byte]int{}
	for range 1000 {
		n[PickMons(rng)]++
	}
	for c := range n {
		if c < 'A' || c > 'Z' {
			t.Fatalf("picked %q", c)
		}
	}
	if n['P'] < 50 || n['P'] > 150 || n['K'] > 10 {
		t.Fatalf("1000 picks: %d purple worms, %d kobolds", n['P'], n['K'])
	}
}
