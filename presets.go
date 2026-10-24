package main

// The weapons' Better Combat attacks: a melee weapon's attack_1, attack_2
// and attack_3 lines in weapons.rec (the mod's animations; their upswings
// are the animation records') bound to the left button, the right button
// and F, each a combo played in turn. A weapon without attacks (the
// launchers and projectiles) gets the vanilla swing.

import "github.com/memmaker/voxelrogue/rogue"

// presetFor is the wielded weapon's attack patterns, nil for a weapon
// with the vanilla swing.
func presetFor(o *rogue.Object) [][]rogue.Move {
	if o == nil || o.Type != rogue.Weapon && o.Type != rogue.Stick {
		return nil
	}
	if o.Type == rogue.Stick {
		return rogue.StaffAttacks
	}
	return rogue.Weapons[o.Which].Attacks
}
