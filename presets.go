package main

// The weapons' Better Combat attacks: a melee weapon's attack lines in
// weapons.rec (the mod's animation and upswing) in the order of its
// patterns: a sideways swing (its horizontal slashes alternate as a
// combo), an overhead strike and a stab. A weapon without attacks (the
// launchers and projectiles) gets the vanilla swing.

import (
	"strings"

	"github.com/memmaker/voxelrogue/rogue"
)

// patternOf sorts an attack animation into its pattern: 0 sideways
// (horizontal slashes, swipes, spins), 1 overhead (vertical slashes,
// slams, uppercuts, punches), 2 stabs.
func patternOf(anim string) int {
	switch {
	case strings.Contains(anim, "stab"):
		return 2
	case strings.Contains(anim, "vertical") || strings.Contains(anim, "slam") || strings.Contains(anim, "uppercut") || strings.Contains(anim, "punch"):
		return 1
	}
	return 0
}

// patterns splits a weapon's attacks into its attack patterns, each a
// combo of its own, in the order the record lists them (bound to the
// left button, the right button and F).
func patterns(p []rogue.Move) [][]rogue.Move {
	var out [][]rogue.Move
	at := map[int]int{}
	for _, a := range p {
		k := patternOf(a.Anim)
		if i, ok := at[k]; ok {
			out[i] = append(out[i], a)
			continue
		}
		at[k] = len(out)
		out = append(out, []rogue.Move{a})
	}
	return out
}

// presetFor is the wielded weapon's attacks, nil for a weapon with the
// vanilla swing.
func presetFor(o *rogue.Object) []rogue.Move {
	if o == nil || o.Type != rogue.Weapon && o.Type != rogue.Stick {
		return nil
	}
	return rogue.Weapons[meleeKind(o)].Attacks
}
