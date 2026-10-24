// Package rogue holds Rogue 3.6's rules and tables: monsters, objects,
// dice, the to-hit and damage rolls (fight.c roll_em) and experience.
// It knows nothing about rendering or real-time movement.
package rogue

import (
	"math/rand"
	"strconv"
	"strings"
)

// Rnd is Rogue's rnd(): 0..n-1, 0 for n < 1.
func Rnd(rng *rand.Rand, n int) int {
	if n < 1 {
		return 0
	}
	return rng.Intn(n)
}

// Roll throws n dice of s sides.
func Roll(rng *rand.Rand, n, s int) int {
	t := 0
	for ; n > 0; n-- {
		t += Rnd(rng, s) + 1
	}
	return t
}

// Str is strength, 3..18 plus the 18/xx extra.
type Str struct{ Str, Add int }

// Stats is struct stats: the fighting numbers of the hero or a monster.
type Stats struct {
	Str   Str
	Exp   int
	Lvl   int
	Arm   int // armour class, lower is better
	HP    int
	Dmg   string // "1d8/1d8/2d6": one attack per term
	MaxHP int
}

// StrPlus is the to-hit bonus for strength.
func StrPlus(s Str) int {
	if s.Str == 18 {
		if s.Add == 100 {
			return 3
		}
		if s.Add > 50 {
			return 2
		}
	}
	if s.Str >= 17 {
		return 1
	}
	if s.Str > 6 {
		return 0
	}
	return s.Str - 7
}

// AddDam is the damage bonus for strength.
func AddDam(s Str) int {
	if s.Str == 18 {
		switch {
		case s.Add == 100:
			return 6
		case s.Add > 90:
			return 5
		case s.Add > 80:
			return 4
		case s.Add > 50:
			return 3
		default:
			return 2
		}
	}
	if s.Str > 15 {
		return 1
	}
	if s.Str > 6 {
		return 0
	}
	return s.Str - 7
}

// swing reports whether an attack of level atLvl hits armour opArm.
// HitScale is what the swing roll (fight.c swing: d20 + bonus against
// 21 - level - armour) used to win on average, in twentieths: with no
// to-hit roll every blow that reaches lands, and (level + armour +
// bonus)/20 of its damage counts, so armour and level keep their worth.
func HitScale(d, atLvl, opArm, wplus int) int {
	return (d*min(20, max(0, atLvl+opArm+wplus)) + 10) / 20
}

// Attack is one attacker's roll_em input beyond its stats.
type Attack struct {
	Dmg          string // damage dice; "" = the attacker's Stats.Dmg
	HPlus, DPlus int
}

// RollEm rolls every attack term of a against def's armour defArm and
// applies the damage to def, scaled by HitScale (reach and position decide
// the hit, not dice). It returns the total damage.
func RollEm(rng *rand.Rand, att *Stats, def *Stats, defArm int, a Attack) (dmg int) {
	cp := a.Dmg
	if cp == "" {
		cp = att.Dmg
	}
	for _, term := range strings.Split(cp, "/") {
		nd, ns, ok := parseDice(term)
		if !ok {
			continue
		}
		d := HitScale(max(0, a.DPlus+Roll(rng, nd, ns)+AddDam(att.Str)), att.Lvl, defArm, a.HPlus+StrPlus(att.Str))
		def.HP -= d
		dmg += d
	}
	return dmg
}

func parseDice(s string) (n, sides int, ok bool) {
	a, b, found := strings.Cut(s, "d")
	if !found {
		return 0, 0, false
	}
	n, err1 := strconv.Atoi(a)
	sides, err2 := strconv.Atoi(b)
	return n, sides, err1 == nil && err2 == nil
}

// Save is a saving throw (fight.c save_throw); which is VS_*.
func Save(rng *rand.Rand, which int, lvl int) bool {
	need := 14 + which - lvl/2
	return Roll(rng, 1, 20) >= need
}

// Saving throw kinds.
const (
	VSPoison = 0
	VSMagic  = 3
)

// ELevels are the experience points needed for each hero level.
var ELevels = []int{10, 20, 40, 80, 160, 320, 640, 1280, 2560, 5120, 10240, 20480,
	40920, 81920, 163840, 327680, 655360, 1310720, 2621440}

// LevelFor is the hero level for exp points (check_level).
func LevelFor(exp int) int {
	i := 0
	for i < len(ELevels) && ELevels[i] <= exp {
		i++
	}
	return i + 1
}

// Monster flags.
const (
	IsMean = 1 << iota
	IsGreed
	IsInvis
	IsBlock
	IsRegen
)

// MonsterKind is an entry of the monster table.
type MonsterKind struct {
	Letter byte
	Name   string
	Carry  int // percent chance to carry an item
	Flags  int
	Exp    int
	Lvl    int
	Arm    int
	Dmg    string
}

// Monsters is init.c's monsters[], indexed by letter-'A'.
var Monsters = [26]MonsterKind{
	{'A', "giant ant", 0, IsMean, 10, 2, 3, "1d6"},
	{'B', "bat", 0, 0, 1, 1, 3, "1d2"},
	{'C', "centaur", 15, 0, 15, 4, 4, "1d6/1d6"},
	{'D', "dragon", 100, IsGreed, 9000, 10, -1, "1d8/1d8/3d10"},
	{'E', "floating eye", 0, 0, 5, 1, 9, "0d0"},
	{'F', "violet fungi", 0, IsMean, 85, 8, 3, "000d0"},
	{'G', "gnome", 10, 0, 8, 1, 5, "1d6"},
	{'H', "hobgoblin", 0, IsMean, 3, 1, 5, "1d8"},
	{'I', "invisible stalker", 0, IsInvis, 120, 8, 3, "4d4"},
	{'J', "jackal", 0, IsMean, 2, 1, 7, "1d2"},
	{'K', "kobold", 0, IsMean, 1, 1, 7, "1d4"},
	{'L', "leprechaun", 0, 0, 10, 3, 8, "1d1"},
	{'M', "mimic", 30, 0, 140, 7, 7, "3d4"},
	{'N', "nymph", 100, 0, 40, 3, 9, "0d0"},
	{'O', "orc", 15, IsBlock, 5, 1, 6, "1d8"},
	{'P', "purple worm", 70, 0, 7000, 15, 6, "2d12/2d4"},
	{'Q', "quasit", 30, IsMean, 35, 3, 2, "1d2/1d2/1d4"},
	{'R', "rust monster", 0, IsMean, 25, 5, 2, "0d0/0d0"},
	{'S', "snake", 0, IsMean, 3, 1, 5, "1d3"},
	{'T', "troll", 50, IsRegen | IsMean, 55, 6, 4, "1d8/1d8/2d6"},
	{'U', "umber hulk", 40, IsMean, 130, 8, 2, "3d4/3d4/2d5"},
	{'V', "vampire", 20, IsRegen | IsMean, 380, 8, 1, "1d10"},
	{'W', "wraith", 0, 0, 55, 5, 4, "1d6"},
	{'X', "xorn", 0, IsMean, 120, 7, -2, "1d3/1d3/1d3/4d6"},
	{'Y', "yeti", 30, 0, 50, 4, 6, "1d6/1d6"},
	{'Z', "zombie", 0, IsMean, 7, 2, 8, "1d8"},
}

var (
	lvlMons  = "KJBSHEAOZGLCRQNYTWFIXUMVDP"
	wandMons = "KJBSH AOZG CRQ Y W IXU V  "
)

// Genocide removes a monster letter from the tables random monsters are
// drawn from (scrolls.c genocide).
func Genocide(letter byte) {
	r := strings.NewReplacer(string(letter), " ")
	lvlMons, wandMons = r.Replace(lvlMons), r.Replace(wandMons)
}

// RandMonster picks a monster letter for a level (monsters.c randmonster).
func RandMonster(rng *rand.Rand, level int, wander bool) byte {
	mons := lvlMons
	if wander {
		mons = wandMons
	}
	for {
		d := level + (Rnd(rng, 10) - 5)
		if d < 1 {
			d = Rnd(rng, 5) + 1
		}
		if d > 26 {
			d = Rnd(rng, 5) + 22
		}
		if mons[d-1] != ' ' {
			return mons[d-1]
		}
	}
}

// NewMonsterStats rolls a new monster's stats (new_monster).
func NewMonsterStats(rng *rand.Rand, letter byte) Stats {
	mk := &Monsters[letter-'A']
	hp := Roll(rng, mk.Lvl, 8)
	return Stats{Str: Str{10, 0}, Exp: mk.Exp, Lvl: mk.Lvl, Arm: mk.Arm, HP: hp, MaxHP: hp, Dmg: mk.Dmg}
}

// KillExp is the experience for killing m (killed()).
func KillExp(m *Stats) int {
	return m.Exp
}
