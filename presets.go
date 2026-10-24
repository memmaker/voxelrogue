package main

// Better Combat's weapon → attributes mapping, as the mod does it: the
// weapon_attributes data files (assets/weapon_attributes/<namespace>/
// <item>.json: the mod's own presets, its registrations for Minecraft's
// weapons and the weapon mods' for theirs) resolved through their parent
// chains (WeaponRegistry.resolveAttributes, WeaponAttributesHelper.override),
// and for items without a registration the fallback regex lists
// (WeaponAttributesFallback with FallbackConfig's defaults). Attacks whose
// conditions the hero never meets (dual wielding, the off hand, riding)
// are dropped as PlayerAttackHelper.selectAttack drops them. Only the
// attacks matter here: poses never show in first person.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/memmaker/voxelrogue/rogue"
)

type attrAttack struct {
	Animation  string
	Upswing    float32
	Conditions []string
}

type attrContainer struct {
	Parent     string
	Attributes *struct{ Attacks []attrAttack }
}

// weaponItem is the item id each weapon kind stands for: a Minecraft or
// mod item where there is one, else an id of our own for the fallback
// regexes. Empty: no Better Combat attributes (the vanilla swing).
var weaponItem = [rogue.MaxWeapons]string{
	rogue.Mace: "rogue:mace", rogue.LongSword: "rogue:long_sword", rogue.Bow: "minecraft:bow", rogue.Dagger: "rogue:dagger",
	rogue.TwoSword: "rogue:two_handed_sword", rogue.Crossbow: "minecraft:crossbow", rogue.Spear: "rogue:spear",
	rogue.Broadsword: "weaponsexpanded:iron_broadsword", rogue.Glaive: "weaponsexpanded:iron_glaive",
	rogue.Greatsword: "weaponsexpanded:iron_greatsword", rogue.Halberd: "weaponsexpanded:iron_halberd",
	rogue.Katana: "additionalweapon:iron_katana", rogue.Scythe: "weaponsexpanded:iron_scythe",
	rogue.WarSpear: "additionalweapon:iron_spear", rogue.GreatBow: "weaponsexpanded:longbow",
	rogue.WarGlaive: "additionalweapon:iron_glaive", rogue.Scimitar: "additionalweapon:iron_scimitar",
	rogue.WoodenPole: "additionalweapon:wooden_pole", rogue.Hatchet: "weaponsexpanded:iron_hatchet",
	rogue.DoubleAxe: "rogue:double_axe", rogue.GreatAxe: "additionalweapon:iron_greataxe", rogue.Maul: "rogue:maul",
	rogue.HeavyMace: "minecraft:mace", rogue.Battlestaff: "additionalweapon:iron_battlestaff", rogue.Sceptre: "rogue:sceptre",
	rogue.Lance: "rogue:lance", rogue.Trident: "minecraft:trident", rogue.Rapier: "weaponsexpanded:iron_rapier",
	rogue.Sickle: "weaponsexpanded:iron_sickle", rogue.SoulKnife: "rogue:soul_knife", rogue.CoralBlade: "rogue:coral_blade",
	rogue.TwinBlade: "rogue:twin_blade", rogue.Claw: "rogue:claw", rogue.Anchor: "rogue:anchor", rogue.Gauntlet: "rogue:gauntlet",
	rogue.ChainCrossbow: "weaponsexpanded:chain_crossbow",
}

// FallbackConfig.createDefault: item id regex → preset, first match
// wins; for melee weapons and for ranged ones.
var fallbackMelee = [][2]string{
	{"claymore|great_sword|greatsword", "bettercombat:claymore"},
	{"great_hammer|greathammer|war_hammer|warhammer|maul", "bettercombat:hammer"},
	{"double_axe|doubleaxe|war_axe|waraxe|great_axe|greataxe", "bettercombat:double_axe"},
	{"scythe", "bettercombat:scythe"},
	{"halberd", "bettercombat:halberd"},
	{"glaive", "bettercombat:glaive"},
	{"spear", "bettercombat:spear"},
	{"lance", "bettercombat:lance"},
	{"anchor", "bettercombat:anchor"},
	{"battlestaff|battle_staff", "bettercombat:battlestaff"},
	{"claw", "bettercombat:claw"},
	{"fist|gauntlet", "bettercombat:fist"},
	{"trident|javelin|impaled", "bettercombat:trident"},
	{"katana", "bettercombat:katana"},
	{"rapier", "bettercombat:rapier"},
	{"sickle", "bettercombat:sickle"},
	{"soul_knife", "bettercombat:soul_knife"},
	{"dagger|knife", "bettercombat:dagger"},
	{"staff|wand|sceptre|stave|rod", "bettercombat:wand"},
	{"mace|hammer|flail", "bettercombat:mace"},
	{"axe", "bettercombat:axe"},
	{"coral_blade", "bettercombat:coral_blade"},
	{"twin_blade|twinblade", "bettercombat:twin_blade"},
	{"cutlass|scimitar|machete", "bettercombat:cutlass"},
	{"sword|blade", "bettercombat:sword"},
}
var fallbackRanged = [][2]string{
	{"two_handed_crossbow", "bettercombat:crossbow_two_handed_heavy"},
	{"two_handed_bow", "bettercombat:bow_two_handed_heavy"},
}

const fallbackBlacklist = "pickaxe"

// unmet are the attack conditions the hero never fulfils: no off-hand
// item, no mount. (OFF_HAND_SHIELD is fulfilled: the mod's check is
// true for any off hand, empty or not.)
var unmet = map[string]bool{"DUAL_WIELDING_ANY": true, "DUAL_WIELDING_SAME": true, "DUAL_WIELDING_SAME_CATEGORY": true, "OFF_HAND_ONLY": true, "MOUNTED": true}

var (
	attrContainers = map[string]attrContainer{}
	weaponPresets  [rogue.MaxWeapons][]bcAttack // resolved per kind by loadWeaponAttributes
)

// loadWeaponAttributes reads the attribute files under dir and resolves
// every weapon kind's attacks.
func loadWeaponAttributes(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*", "*.json"))
	if err != nil {
		return err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var c attrContainer
		if err := json.Unmarshal(b, &c); err != nil {
			return err
		}
		attrContainers[filepath.Base(filepath.Dir(f))+":"+strings.TrimSuffix(filepath.Base(f), ".json")] = c
	}
	for k := range weaponPresets {
		weaponPresets[k] = attacksOf(weaponItem[k], launchSpeed[k] > 0)
	}
	return nil
}

// attacksOf is an item's attacks after WeaponRegistry's registration or
// the fallback, with the unmet conditions filtered out; nil for none.
func attacksOf(id string, ranged bool) []bcAttack {
	var attacks []attrAttack
	if _, ok := attrContainers[id]; ok {
		attacks = resolveAttacks(id)
	} else if id != "" && !regexp.MustCompile("(?i)"+fallbackBlacklist).MatchString(id) {
		list := fallbackMelee
		if ranged {
			list = fallbackRanged
		}
		for _, spec := range list {
			if _, known := attrContainers[spec[1]]; known && regexp.MustCompile("(?i)"+spec[0]).MatchString(id) {
				attacks = resolveAttacks(spec[1])
				break
			}
		}
	}
	var out []bcAttack
	for _, a := range attacks {
		ok := true
		for _, c := range a.Conditions {
			ok = ok && !unmet[c]
		}
		if ok { // the animation id's namespace is the registry's; the emotes go by name
			out = append(out, bcAttack{a.Animation[strings.IndexByte(a.Animation, ':')+1:], a.Upswing})
		}
	}
	return out
}

// resolveAttacks is WeaponRegistry.resolveAttributes for the attacks:
// the parent chain from the root down, each level's attacks (if it has
// any) overriding the one above's index by index, a field at a time.
func resolveAttacks(id string) []attrAttack {
	var chain []attrContainer
	for c, ok := attrContainers[id]; ok; c, ok = attrContainers[c.Parent] {
		chain = append(chain, c)
	}
	var attacks []attrAttack
	for i := len(chain) - 1; i >= 0; i-- {
		b := chain[i].Attributes
		if b == nil || len(b.Attacks) == 0 {
			continue
		}
		merged := make([]attrAttack, len(b.Attacks))
		for j, o := range b.Attacks {
			var base attrAttack
			if j < len(attacks) {
				base = attacks[j]
			}
			if o.Animation == "" {
				o.Animation = base.Animation
			}
			if o.Upswing == 0 {
				o.Upswing = base.Upswing
			}
			if o.Conditions == nil {
				o.Conditions = base.Conditions
			}
			merged[j] = o
		}
		attacks = merged
	}
	return attacks
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

// patterns splits a preset's attacks into the weapon's attack patterns,
// each a combo of its own, in the order they are bound to the left
// button, the right button and F: sideways, overhead, stab; a long
// reaching weapon leads with its stab.
func patterns(p []bcAttack, long bool) [][]bcAttack {
	var by [3][]bcAttack
	for _, a := range p {
		k := patternOf(a.anim)
		by[k] = append(by[k], a)
	}
	order := []int{0, 1, 2}
	if long {
		order = []int{2, 0, 1}
	}
	var out [][]bcAttack
	for _, k := range order {
		if len(by[k]) > 0 {
			out = append(out, by[k])
		}
	}
	return out
}

// presetFor is the wielded weapon's attacks, nil for a weapon the mod
// leaves to the vanilla swing.
func presetFor(o *rogue.Object) []bcAttack {
	if o == nil || o.Type != rogue.Weapon && o.Type != rogue.Stick {
		return nil
	}
	return weaponPresets[meleeKind(o)]
}
