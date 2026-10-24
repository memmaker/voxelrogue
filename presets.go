package main

// The weapons' Better Combat attacks: assets/weapon_attributes/<weapon
// name with underscores>.json lists a melee weapon's attacks (the mod's
// animation and upswing) in the order of its patterns: a sideways swing
// (its horizontal slashes alternate as a combo), an overhead strike and a
// stab, as the weapon list has them. A weapon without a file (the
// launchers and missiles) gets the vanilla swing.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/memmaker/voxelrogue/rogue"
)

// weaponPresets are each weapon kind's attacks, resolved by loadWeaponAttributes.
var weaponPresets [rogue.MaxWeapons][]bcAttack

// loadWeaponAttributes reads every weapon kind's attack file under dir.
func loadWeaponAttributes(dir string) error {
	for k, name := range rogue.WeaponNames {
		b, err := os.ReadFile(filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".json"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		var f struct {
			Attacks []struct {
				Animation string
				Upswing   float32
			}
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return err
		}
		weaponPresets[k] = nil
		for _, a := range f.Attacks { // the animation id's namespace is the registry's; the emotes go by name
			weaponPresets[k] = append(weaponPresets[k], bcAttack{a.Animation[strings.IndexByte(a.Animation, ':')+1:], a.Upswing})
		}
	}
	return nil
}

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
// combo of its own, in the order the file lists them (bound to the left
// button, the right button and F).
func patterns(p []bcAttack) [][]bcAttack {
	var out [][]bcAttack
	at := map[int]int{}
	for _, a := range p {
		k := patternOf(a.anim)
		if i, ok := at[k]; ok {
			out[i] = append(out[i], a)
			continue
		}
		at[k] = len(out)
		out = append(out, []bcAttack{a})
	}
	return out
}

// presetFor is the wielded weapon's attacks, nil for a weapon with the
// vanilla swing.
func presetFor(o *rogue.Object) []bcAttack {
	if o == nil || o.Type != rogue.Weapon && o.Type != rogue.Stick {
		return nil
	}
	return weaponPresets[meleeKind(o)]
}
