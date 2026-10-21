package rogue

import (
	"fmt"
	"math/rand"
	"strings"
)

// ObjType is what kind of object something is (Rogue's display char).
type ObjType byte

const (
	Potion ObjType = '!'
	Scroll ObjType = '?'
	Food   ObjType = ':'
	Weapon ObjType = ')'
	Armor  ObjType = ']'
	Ring   ObjType = '='
	Stick  ObjType = '/'
	Gold   ObjType = '*'
	Amulet ObjType = ','
)

// Object flags.
const (
	IsCursed = 1 << iota
	IsKnow
	IsMissile
	IsMany
	IsFound // a scare monster scroll that was picked up once
)

// Object is struct object.
type Object struct {
	Type         ObjType
	Which        int
	Count        int
	HPlus, DPlus int
	AC           int    // armour class of armour, bonus of rings
	Damage       string // wielded damage
	HurlDmg      string // thrown damage
	Launch       int    // weapon that fires it, -1 none
	Flags        int
	Charges      int
	Group        int
	GoldVal      int
}

// Weapons (weapons.c), in table order; after Spear come the weapons
// added from the weapons_expanded and additionalweapon mods.
const (
	Mace = iota
	LongSword
	Bow
	Arrow
	Dagger
	RockW
	TwoSword
	Sling
	Dart
	Crossbow
	Bolt
	Spear
	Broadsword
	Glaive
	Greatsword
	Halberd
	Katana
	Scythe
	WarSpear
	GreatBow
	WarGlaive
	Scimitar
	WoodenPole
	MaxWeapons
)

var WeaponNames = [MaxWeapons]string{"mace", "long sword", "long bow", "arrow", "dagger", "rock",
	"two handed sword", "sling", "dart", "crossbow", "crossbow bolt", "spear",
	"broadsword", "glaive", "greatsword", "halberd", "katana", "scythe", "war spear", "great bow",
	"war glaive", "scimitar", "wooden pole"}

var weaponInit = [MaxWeapons]struct {
	dam, hrl string
	launch   int
	flags    int
}{
	{"2d4", "1d3", -1, 0},
	{"1d10", "1d2", -1, 0},
	{"1d1", "1d1", -1, 0},
	{"1d1", "1d6", Bow, IsMany | IsMissile},
	{"1d6", "1d4", -1, IsMissile},
	{"1d2", "1d4", Sling, IsMany | IsMissile},
	{"3d6", "1d2", -1, 0},
	{"0d0", "0d0", -1, 0},
	{"1d1", "1d3", -1, IsMany | IsMissile},
	{"1d1", "1d1", -1, 0},
	{"1d2", "1d10", Crossbow, IsMany | IsMissile},
	{"1d8", "1d6", -1, IsMissile},
	{"2d5", "1d2", -1, 0},
	{"1d10", "1d3", -1, 0},
	{"3d5", "1d2", -1, 0},
	{"2d6", "1d3", -1, 0},
	{"1d8", "1d2", -1, 0},
	{"2d4", "1d2", -1, 0},
	{"1d10", "1d8", -1, IsMissile},
	{"1d1", "1d1", -1, 0},
	{"2d5", "1d3", -1, 0},
	{"1d8", "1d2", -1, 0},
	{"1d6", "1d2", -1, 0},
}

// Launches reports whether launcher fires ammo; arrows fit both bows.
func Launches(launcher int, ammo *Object) bool {
	return ammo.Launch >= 0 && (ammo.Launch == launcher || ammo.Launch == Bow && launcher == GreatBow)
}

// LaunchDamage is ammo's damage fired from launcher: the great bow
// drives arrows harder.
func LaunchDamage(launcher int, ammo *Object) string {
	if launcher == GreatBow && ammo.Which == Arrow {
		return "1d8"
	}
	return ammo.HurlDmg
}

// Armour (init.c), in table order.
const (
	Leather = iota
	RingMail
	StuddedLeather
	ScaleMail
	ChainMail
	SplintMail
	BandedMail
	PlateMail
	MaxArmors
)

var ArmorNames = [MaxArmors]string{"leather armor", "ring mail", "studded leather armor", "scale mail",
	"chain mail", "splint mail", "banded mail", "plate mail"}
var ArmorClass = [MaxArmors]int{8, 7, 7, 6, 5, 4, 4, 3}
var armorChances = [MaxArmors]int{20, 35, 50, 63, 75, 85, 95, 100}

// MagicItem is struct magic_item: a name and a pick probability.
type MagicItem struct {
	Name  string
	Prob  int
	Worth int
}

// Potions.
const (
	PConfuse = iota
	PParalyze
	PPoison
	PStrength
	PSeeInvis
	PHealing
	PMFind
	PTFind
	PRaise
	PXHeal
	PHaste
	PRestore
	PBlind
	PNop
	MaxPotions
)

// Scrolls.
const (
	SConfuse = iota
	SMap
	SLight
	SHold
	SSleep
	SArmor
	SIdent
	SScare
	SGFind
	STelep
	SEnch
	SCreate
	SRemove
	SAggr
	SNop
	SGenocide
	MaxScrolls
)

// Rings.
const (
	RProtect = iota
	RAddStr
	RSustStr
	RSearch
	RSeeInvis
	RNop
	RAggr
	RAddHit
	RAddDam
	RRegen
	RDigest
	RTeleport
	RStealth
	MaxRings
)

const MaxSticks = 14

var (
	things = []MagicItem{{"", 27, 0}, {"", 27, 0}, {"", 18, 0}, {"", 9, 0}, {"", 9, 0}, {"", 5, 0}, {"", 5, 0}}
	// PotionMagic etc. are the magic tables of init.c.
	PotionMagic = []MagicItem{{"confusion", 8, 50}, {"paralysis", 10, 50}, {"poison", 8, 50},
		{"gain strength", 15, 150}, {"see invisible", 2, 170}, {"healing", 15, 130},
		{"monster detection", 6, 120}, {"magic detection", 6, 105}, {"raise level", 2, 220},
		{"extra healing", 5, 180}, {"haste self", 4, 200}, {"restore strength", 14, 120},
		{"blindness", 4, 50}, {"thirst quenching", 1, 50}}
	ScrollMagic = []MagicItem{{"monster confusion", 8, 170}, {"magic mapping", 5, 180}, {"light", 10, 100},
		{"hold monster", 2, 200}, {"sleep", 5, 50}, {"enchant armor", 8, 130}, {"identify", 21, 100},
		{"scare monster", 4, 180}, {"gold detection", 4, 110}, {"teleportation", 7, 175},
		{"enchant weapon", 10, 150}, {"create monster", 5, 75}, {"remove curse", 8, 105},
		{"aggravate monsters", 1, 60}, {"blank paper", 1, 50}, {"genocide", 1, 200}}
	RingMagic = []MagicItem{{"protection", 9, 200}, {"add strength", 9, 200}, {"sustain strength", 5, 180},
		{"searching", 10, 200}, {"see invisible", 10, 175}, {"adornment", 1, 100},
		{"aggravate monster", 11, 100}, {"dexterity", 8, 220}, {"increase damage", 8, 220},
		{"regeneration", 4, 260}, {"slow digestion", 9, 240}, {"teleportation", 9, 100}, {"stealth", 7, 100}}
	StickMagic = []MagicItem{{"light", 12, 120}, {"striking", 9, 115}, {"lightning", 3, 200}, {"fire", 3, 200},
		{"cold", 3, 200}, {"polymorph", 15, 210}, {"magic missile", 10, 170}, {"haste monster", 9, 50},
		{"slow monster", 11, 220}, {"drain life", 9, 210}, {"nothing", 1, 70}, {"teleport away", 5, 140},
		{"teleport to", 5, 60}, {"cancellation", 5, 130}}
)

func pickOne(rng *rand.Rand, magic []MagicItem) int {
	i := Rnd(rng, 100)
	sum := 0
	for k, m := range magic {
		if sum += m.Prob; i < sum {
			return k
		}
	}
	return 0
}

// NewWeapon makes a weapon with its initial damage (init_weapon).
func NewWeapon(rng *rand.Rand, which int) *Object {
	w := weaponInit[which]
	o := &Object{Type: Weapon, Which: which, Count: 1, Damage: w.dam, HurlDmg: w.hrl, Launch: w.launch, Flags: w.flags, AC: 11}
	if o.Flags&IsMany != 0 {
		o.Count = Rnd(rng, 8) + 8
	}
	return o
}

// NewArmor makes armour of a kind with its base class.
func NewArmor(which int) *Object {
	return &Object{Type: Armor, Which: which, Count: 1, AC: ArmorClass[which], Launch: -1}
}

// Game is the per-game state objects need: the random names and what the
// hero has identified.
type Game struct {
	Rng     *rand.Rand
	PColors [MaxPotions]string
	SNames  [MaxScrolls]string
	RStones [MaxRings]string
	SMade   [MaxSticks]string
	SType   [MaxSticks]string // wand or staff
	PKnow   [MaxPotions]bool
	SKnow   [MaxScrolls]bool
	RKnow   [MaxRings]bool
	WKnow   [MaxSticks]bool
	// PLook, SLook, RLook index this game's colour, title and stone of
	// each kind, to pick a picture for it.
	PLook       [MaxPotions]int
	SLook       [MaxScrolls]int
	RLook       [MaxRings]int
	NoFood      int
	groupSerial int
}

var (
	rainbow = []string{"red", "blue", "green", "yellow", "black", "brown", "orange", "pink", "purple", "grey",
		"white", "silver", "gold", "violet", "clear", "vermilion", "ecru", "turquoise", "magenta", "amber",
		"topaz", "plaid", "tan", "tangerine"}
	sylls = strings.Fields("a ab ag aks ala an ankh app arg arze ash ban bar bat bek bie bin bit bjor blu bot bu " +
		"byt comp con cos cre dalf dan den do e eep el eng er ere erk esh evs fa fid for fri fu gan gar glen gop " +
		"gre ha he hyd i ing ion ip ish it ite iv jo kho kli klis la lech man mar me mi mic mik mon mung mur nej " +
		"nelg nep ner nes nes nih nin o od ood org orn ox oxy pay pet ple plu po pot prok re rea rhov ri ro rog " +
		"rok rol sa san sat see sef seh shu ski sna sne snik sno so sol sri sta sun ta tab tem ther ti tox trol " +
		"tue turs u ulk um un uni ur val viv vly vom wah wed werg wex whon wun xo y yot yu zant zap zeb zim zok zon zum")
	stones = []string{"agate", "alexandrite", "amethyst", "carnelian", "diamond", "emerald", "granite", "jade",
		"kryptonite", "lapus lazuli", "moonstone", "obsidian", "onyx", "opal", "pearl", "ruby", "saphire",
		"tiger eye", "topaz", "turquoise"}
	woods = []string{"avocado wood", "balsa", "banyan", "birch", "cedar", "cherry", "cinnibar", "driftwood",
		"ebony", "eucalyptus", "hemlock", "ironwood", "mahogany", "manzanita", "maple", "oak", "persimmon wood",
		"redwood", "rosewood", "teak", "walnut", "zebra wood"}
	metals = []string{"aluminium", "bone", "brass", "bronze", "copper", "iron", "lead", "pewter", "steel", "tin", "zinc"}
)

// NewGame rolls the random names for a game (init_colors, init_names,
// init_stones, init_materials).
func NewGame(rng *rand.Rand) *Game {
	g := &Game{Rng: rng}
	used := map[int]bool{}
	for i := range g.PColors {
		j := Rnd(rng, len(rainbow))
		for used[j] {
			j = Rnd(rng, len(rainbow))
		}
		used[j] = true
		g.PColors[i], g.PLook[i] = rainbow[j], j
	}
	for i := range g.SNames {
		var words []string
		for n := Rnd(rng, 4) + 2; n > 0; n-- {
			w := ""
			for k := Rnd(rng, 3) + 1; k > 0; k-- {
				w += sylls[Rnd(rng, len(sylls))]
			}
			words = append(words, w)
		}
		g.SNames[i], g.SLook[i] = strings.Join(words, " "), Rnd(rng, 1000)
	}
	clear(used)
	for i := range g.RStones {
		j := Rnd(rng, len(stones))
		for used[j] {
			j = Rnd(rng, len(stones))
		}
		used[j] = true
		g.RStones[i], g.RLook[i] = stones[j], j
	}
	usedW, usedM := map[int]bool{}, map[int]bool{}
	for i := range g.SMade {
		if Rnd(rng, 2) == 0 {
			j := Rnd(rng, len(metals))
			for usedM[j] && len(usedM) < len(metals) {
				j = Rnd(rng, len(metals))
			}
			usedM[j] = true
			g.SMade[i], g.SType[i] = metals[j], "wand"
		} else {
			j := Rnd(rng, len(woods))
			for usedW[j] && len(usedW) < len(woods) {
				j = Rnd(rng, len(woods))
			}
			usedW[j] = true
			g.SMade[i], g.SType[i] = woods[j], "staff"
		}
	}
	return g
}

// NewThing makes a random object (things.c new_thing).
func (g *Game) NewThing() *Object {
	rng := g.Rng
	o := &Object{Count: 1, AC: 11, Launch: -1, Damage: "0d0", HurlDmg: "0d0"}
	kind := pickOne(rng, things)
	if g.NoFood > 3 {
		kind = 2
	}
	switch kind {
	case 0:
		o.Type, o.Which = Potion, pickOne(rng, PotionMagic)
	case 1:
		o.Type, o.Which = Scroll, pickOne(rng, ScrollMagic)
	case 2:
		g.NoFood = 0
		o.Type = Food
		if Rnd(rng, 100) > 10 {
			o.Which = 0
		} else {
			o.Which = 1
		}
	case 3:
		o = NewWeapon(rng, Rnd(rng, MaxWeapons))
		if k := Rnd(rng, 100); k < 10 {
			o.Flags |= IsCursed
			o.HPlus -= Rnd(rng, 3) + 1
		} else if k < 15 {
			o.HPlus += Rnd(rng, 3) + 1
		}
	case 4:
		k, j := Rnd(rng, 100), 0
		for j < MaxArmors-1 && k >= armorChances[j] {
			j++
		}
		o = NewArmor(j)
		if k := Rnd(rng, 100); k < 20 {
			o.Flags |= IsCursed
			o.AC += Rnd(rng, 3) + 1
		} else if k < 28 {
			o.AC -= Rnd(rng, 3) + 1
		}
	case 5:
		o.Type, o.Which = Ring, pickOne(rng, RingMagic)
		switch o.Which {
		case RAddStr, RProtect, RAddHit, RAddDam:
			if o.AC = Rnd(rng, 3); o.AC == 0 {
				o.AC = -1
				o.Flags |= IsCursed
			}
		case RAggr, RTeleport:
			o.Flags |= IsCursed
		}
	case 6:
		o.Type, o.Which = Stick, pickOne(rng, StickMagic)
		g.fixStick(o)
	}
	if o.Flags&IsMany != 0 {
		g.groupSerial++
		o.Group = g.groupSerial
	}
	return o
}

// fixStick sets a new wand's or staff's damage and charges (fix_stick).
func (g *Game) fixStick(o *Object) {
	o.Damage, o.HurlDmg = "1d1", "1d1"
	if g.SType[o.Which] == "staff" {
		o.Damage = "2d3"
	}
	o.Charges = 3 + Rnd(g.Rng, 5)
	switch o.Which {
	case 1: // striking
		o.HPlus, o.DPlus, o.Damage = 3, 3, "1d8"
	case 0: // light
		o.Charges = 10 + Rnd(g.Rng, 10)
	}
}

// weaponWorth is what total_winner pays for each weapon; the mods'
// weapons are worth nothing to Rogue's fighters guild.
var weaponWorth = [MaxWeapons]int{8, 15, 75, 1, 2, 1, 30, 1, 1, 15, 1, 2}
var armorWorth = [MaxArmors]int{5, 30, 15, 3, 75, 80, 90, 400}

// Worth is what an object sells for when the hero escapes
// (total_winner); it also identifies the object, as there.
func (g *Game) Worth(o *Object) int {
	w := 0
	switch o.Type {
	case Food:
		w = 2 * o.Count
	case Weapon:
		w = weaponWorth[o.Which] * (1 + 10*o.HPlus + 10*o.DPlus) * o.Count
		o.Flags |= IsKnow
	case Armor:
		w = armorWorth[o.Which] * (1 + 10*(ArmorClass[o.Which]-o.AC))
		o.Flags |= IsKnow
	case Scroll:
		g.SKnow[o.Which] = true
		w = ScrollMagic[o.Which].Worth * o.Count
	case Potion:
		g.PKnow[o.Which] = true
		w = PotionMagic[o.Which].Worth * o.Count
	case Ring:
		o.Flags |= IsKnow
		g.RKnow[o.Which] = true
		w = RingMagic[o.Which].Worth
		switch o.Which {
		case RAddStr, RAddDam, RProtect, RAddHit:
			if o.AC > 0 {
				w += o.AC * 20
			} else {
				w = 50
			}
		}
	case Stick:
		o.Flags |= IsKnow
		g.WKnow[o.Which] = true
		w = StickMagic[o.Which].Worth + 20*o.Charges
	case Amulet:
		w = 1000
	}
	return w
}

func vowelstr(s string) string {
	if s != "" && strings.ContainsRune("aeiou", rune(s[0])) {
		return "n"
	}
	return ""
}

func signed(n int) string {
	if n < 0 {
		return fmt.Sprint(n)
	}
	return fmt.Sprintf("+%d", n)
}

// Name is inv_name(): what the hero calls an object.
func (g *Game) Name(o *Object) string {
	switch o.Type {
	case Scroll:
		s := "A scroll "
		if o.Count > 1 {
			s = fmt.Sprintf("%d scrolls ", o.Count)
		}
		if g.SKnow[o.Which] {
			return s + "of " + ScrollMagic[o.Which].Name
		}
		return s + "titled '" + g.SNames[o.Which] + "'"
	case Potion:
		c := g.PColors[o.Which]
		if g.PKnow[o.Which] {
			s := "A potion "
			if o.Count > 1 {
				s = fmt.Sprintf("%d potions ", o.Count)
			}
			return s + "of " + PotionMagic[o.Which].Name + "(" + c + ")"
		}
		if o.Count > 1 {
			return fmt.Sprintf("%d %s potions", o.Count, c)
		}
		return "A" + vowelstr(c) + " " + c + " potion"
	case Food:
		name := "food"
		if o.Which == 1 {
			name = "slime-mold"
			if o.Count > 1 {
				return fmt.Sprintf("%d %ss", o.Count, name)
			}
			return "A " + name
		}
		if o.Count > 1 {
			return fmt.Sprintf("%d rations of food", o.Count)
		}
		return "Some " + name
	case Weapon:
		s := "A "
		if o.Count > 1 {
			s = fmt.Sprintf("%d ", o.Count)
		}
		if o.Flags&IsKnow != 0 {
			s += signed(o.HPlus) + "," + signed(o.DPlus) + " "
		}
		s += WeaponNames[o.Which]
		if o.Count > 1 {
			s += "s"
		}
		return s
	case Armor:
		if o.Flags&IsKnow != 0 {
			return signed(ArmorClass[o.Which]-o.AC) + " " + ArmorNames[o.Which]
		}
		return ArmorNames[o.Which]
	case Amulet:
		return "The Amulet of Yendor"
	case Stick:
		if g.WKnow[o.Which] {
			return fmt.Sprintf("A %s of %s [%d charges](%s)", g.SType[o.Which], StickMagic[o.Which].Name, o.Charges, g.SMade[o.Which])
		}
		return "A" + vowelstr(g.SMade[o.Which]) + " " + g.SMade[o.Which] + " " + g.SType[o.Which]
	case Ring:
		st := g.RStones[o.Which]
		if g.RKnow[o.Which] {
			n := ""
			switch o.Which {
			case RProtect, RAddStr, RAddHit, RAddDam:
				n = " " + signed(o.AC)
			}
			return "A" + n + " ring of " + RingMagic[o.Which].Name + "(" + st + ")"
		}
		return "A" + vowelstr(st) + " " + st + " ring"
	case Gold:
		return fmt.Sprintf("%d gold pieces", o.GoldVal)
	}
	return "Something bizarre"
}

// Stacks reports whether o merges into p in the pack.
func Stacks(p, o *Object) bool {
	if p.Type != o.Type || p.Which != o.Which {
		return false
	}
	switch o.Type {
	case Potion, Scroll, Food:
		return true
	case Weapon:
		return o.Group != 0 && p.Group == o.Group
	}
	return false
}
