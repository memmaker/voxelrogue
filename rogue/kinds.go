package rogue

// The weapon and armour kinds are data: the weapon records of
// assets/tiles/weapons.rec and the gear records of gear.rec (the files
// the remapper edits), read by LoadKinds before anything else. A new
// record there is a new kind; the game has no table of its own. The
// fields are documented in the files' headers.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Move is one attack of a melee weapon's pattern: a Better Combat
// animation and the point of it where the blow lands.
type Move struct {
	Anim    string
	Upswing float32
}

// Display is a Minecraft item display transform: rotation (degrees),
// translation and scale.
type Display struct{ Rot, Pos, Scale [3]float32 }

// WeaponKind is a weapons.rec record.
type WeaponKind struct {
	ID, Name  string
	Kind      string // melee, launcher or projectile
	Hands     int
	Blunt     bool    // a thrown or shot blunt thing bounces; a sharp one sticks
	Reach     float64 // melee: blocks, plus the target's size
	Swing     int     // melee: ticks between blows
	Damage    string  // dice wielded
	Thrown    string  // dice thrown, or shot from its launcher
	Missile   bool    // flies well thrown (Rogue's ISMISSL)
	Launchers []string
	Action    string // a launcher's: bow (held to draw), crossbow (spanned with R), sling (whirled)
	Speed     float64
	Draw      int      // a bow's ticks to a full draw
	Charge    int      // a crossbow's ticks to span one chamber
	Magazine  int      // a crossbow's chambers
	Cooldown  int      // a crossbow's ticks between shots
	Fly       string   // a thrown or shot one's flight: tumble (end over end), spear (tip first, rolling), tip (a model at its tip), cube (a model, resting flat)
	Meshes    []string // glb files: a projectile's model, the sling's rest and whirl
	Sweeps    bool     // the sideways swing strikes every monster in its arc
	Backstab  float64  // the damage multiplier on a sleeping monster (2 without)
	Worth     int      // what the fighters guild pays
	Sound     string   // the sound event of a blow or a shot
	Attacks   []Move
	Hand      *Display // first-person display (item/handheld without)
	Hand3     *Display // Better Combat's third-person display
}

// ArmorKind is a gear.rec record (its mesh fields are read by the game).
type ArmorKind struct {
	ID, Name        string
	AC, Prob, Worth int
	Skin            string // the hobbit armour layer set
}

var (
	Weapons []WeaponKind
	Armors  []ArmorKind
)

// LoadKinds reads dir's weapons.rec and gear.rec.
func LoadKinds(dir string) error {
	recs, err := ReadRecs(filepath.Join(dir, "weapons.rec"))
	if err != nil {
		return err
	}
	Weapons = nil
	for _, r := range recs {
		if r["%rec"] != "weapon" || r["id"] == "" {
			continue
		}
		w := WeaponKind{ID: r["id"], Name: r["name"], Kind: r["kind"], Hands: atoi(r["hands"]), Blunt: r["edge"] == "blunt",
			Reach: atof(r["reach"]), Swing: atoi(r["swing"]), Damage: r["damage"], Thrown: r["thrown"], Missile: r["missile"] == "yes",
			Action: r["action"], Speed: atof(r["speed"]), Draw: atoi(r["draw"]), Charge: atoi(r["charge"]), Magazine: atoi(r["magazine"]),
			Cooldown: atoi(r["cooldown"]), Fly: r["fly"], Sweeps: r["sweeps"] == "yes", Backstab: atof(r["backstab"]), Worth: atoi(r["worth"]),
			Sound: r["sound"], Hand: display(r["hand"]), Hand3: display(r["hand3"])}
		if r["launcher"] != "" {
			w.Launchers = strings.Split(r["launcher"], "\n")
		}
		for _, m := range lines(r["mesh"]) {
			w.Meshes = append(w.Meshes, filepath.Join(dir, m))
		}
		for _, a := range lines(r["attack"]) {
			anim, up, _ := strings.Cut(a, " ")
			w.Attacks = append(w.Attacks, Move{anim, float32(atof(up))})
		}
		if w.Kind == "" || w.Name == "" || w.Damage == "" || w.Thrown == "" {
			return fmt.Errorf("weapons.rec: %s: kind, name, damage and thrown are required", w.ID)
		}
		Weapons = append(Weapons, w)
	}
	recs, err = ReadRecs(filepath.Join(dir, "gear.rec"))
	if err != nil {
		return err
	}
	Armors = nil
	for _, r := range recs {
		if r["%rec"] != "gear" || r["id"] == "" {
			continue
		}
		if r["ac"] == "" {
			return fmt.Errorf("gear.rec: %s: ac is required", r["id"])
		}
		Armors = append(Armors, ArmorKind{r["id"], r["name"], atoi(r["ac"]), atoi(r["prob"]), atoi(r["worth"]), r["skin"]})
	}
	return nil
}

func atoi(s string) int     { n, _ := strconv.Atoi(s); return n }
func atof(s string) float64 { f, _ := strconv.ParseFloat(s, 64); return f }

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// display parses "rx ry rz tx ty tz sx sy sz".
func display(s string) *Display {
	f := strings.Fields(s)
	if len(f) != 9 {
		return nil
	}
	var d Display
	for i := range 9 {
		[]*[3]float32{&d.Rot, &d.Pos, &d.Scale}[i/3][i%3] = float32(atof(f[i]))
	}
	return &d
}

// WeaponID is the index of a weapon kind by its id; a kind the game
// counts on must exist.
func WeaponID(id string) int {
	i := slices.IndexFunc(Weapons, func(w WeaponKind) bool { return w.ID == id })
	if i < 0 {
		panic("weapons.rec: no weapon " + id)
	}
	return i
}

// ArmorID is the index of an armour kind by its id.
func ArmorID(id string) int {
	i := slices.IndexFunc(Armors, func(a ArmorKind) bool { return a.ID == id })
	if i < 0 {
		panic("gear.rec: no armour " + id)
	}
	return i
}

// IsAmmo reports whether a weapon kind is a projectile (comes in bundles,
// not something to wield).
func IsAmmo(which int) bool { return Weapons[which].Kind == "projectile" }

// Launches reports whether launcher fires ammo.
func Launches(launcher int, ammo *Object) bool {
	return ammo.Type == Weapon && slices.Contains(Weapons[ammo.Which].Launchers, Weapons[launcher].ID)
}

// ReadRecs reads a rec file's records (field: value lines, blank lines
// between records, "%rec" the record's type); a repeated field's values
// are joined by newlines.
func ReadRecs(path string) ([]map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var recs []map[string]string
	rec, typ := map[string]string{}, ""
	flush := func() {
		if len(rec) > 0 {
			rec["%rec"] = typ
			recs, rec = append(recs, rec), map[string]string{}
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "%rec:"):
			flush()
			typ = strings.TrimSpace(line[5:])
		case strings.TrimSpace(line) == "":
			flush()
		case line[0] == '%' || line[0] == '#':
		default:
			if k, v, ok := strings.Cut(line, ":"); ok {
				if old, dup := rec[k]; dup {
					v = old + "\n" + strings.TrimSpace(v)
				}
				rec[k] = strings.TrimSpace(v)
			}
		}
	}
	flush()
	return recs, nil
}
