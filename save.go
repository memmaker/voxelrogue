package main

// Saving and loading (the pause menu's S and L, -load): one file, -save,
// by default save.gob in the user's config directory (Library/Application
// Support/VoxelRogue on a Mac), holds the whole game as a gob: the
// dungeon's levels and what was found and sealed in them, Rogue's names
// and knowledge, the hero and the pack, where the hero stands and looks,
// and the monsters, floor objects and corpses of every level visited. The
// dice are seeded afresh at the save, so the saved game and the one that
// goes on roll alike. Meshes, textures, sprites and the terrain are not
// saved: a load rebuilds them (reset), as a new game does.

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

// saveFile is a saved game.
type saveFile struct {
	Levels        []*dungeon.Level
	Found         map[[3]int]bool
	Sealed        map[int]bool
	Seed          int64 // the dice's seed from the save on
	Rogue         rogue.State
	Depth         int
	Hero          *heroState
	Pack          []packSlot
	Weapon, Armor int    // the wielded weapon and worn armour as Pack indices, -1 none
	Rings         [2]int // the rings worn, likewise
	Held          int    // the hotbar slot in hand
	X, Y, Z       float64
	Yaw, Pitch    float64
	Monsters      []*monster
	Floor         []*floorObj
	Corpses       map[int][]*monster
}

// packSlot is an object of the pack and its inventory slot (-1: on the
// cursor).
type packSlot struct {
	Slot int
	Obj  *rogue.Object
}

// configDir is the user's configuration directory, or the working
// directory when there is none.
func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	return d
}

// save writes the game to g.saveFile (-save).
func (g *game) save() {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).Encode(g.snapshot())
	if err == nil {
		err = os.MkdirAll(filepath.Dir(g.saveFile), 0o755)
	}
	if err == nil {
		err = os.WriteFile(g.saveFile, b.Bytes(), 0o644)
	}
	if err != nil {
		log.Print(err)
		g.msg("The save failed: " + err.Error())
		return
	}
	g.msg("Game saved.")
}

// load replaces the running game with the one in g.saveFile; a missing or
// unreadable save is reported.
func (g *game) load() {
	f, err := os.Open(g.saveFile)
	if errors.Is(err, fs.ErrNotExist) {
		g.msg("There is no saved game.")
		return
	}
	var s saveFile
	if err == nil {
		defer f.Close()
		err = gob.NewDecoder(f).Decode(&s)
	}
	if err != nil {
		log.Print(err)
		g.msg("The saved game cannot be read: " + err.Error())
		return
	}
	g.restore(&s)
	g.msg("Game loaded.")
}

// snapshot is the running game as a saveFile. It reseeds the dice.
func (g *game) snapshot() *saveFile {
	seed := time.Now().UnixNano()
	g.rng.Seed(seed)
	s := &saveFile{Levels: g.gen.levels, Found: g.gen.found, Sealed: g.gen.sealed, Seed: seed, Rogue: g.rg.State,
		Depth: g.depth, Hero: &g.heroState, Weapon: -1, Armor: -1, Rings: [2]int{-1, -1}, Held: g.inv.Held,
		X: g.player.X, Y: g.player.Y, Z: g.player.Z, Yaw: g.cam.Yaw, Pitch: g.cam.Pitch,
		Monsters: g.monsters, Floor: g.floor, Corpses: g.corpses}
	for i := range g.inv.Size() {
		if o := obj(g.inv.Get(i)); o != nil {
			s.Pack = append(s.Pack, packSlot{i, o})
		}
	}
	if o := obj(g.inv.Cursor); o != nil {
		s.Pack = append(s.Pack, packSlot{-1, o})
	}
	for i, p := range s.Pack { // the equipment by its place in the pack: one object, not a copy each
		for _, w := range []struct {
			o  *rogue.Object
			at *int
		}{{g.weapon, &s.Weapon}, {g.armor, &s.Armor}, {g.rings[0], &s.Rings[0]}, {g.rings[1], &s.Rings[1]}} {
			if w.o != nil && w.o == p.Obj {
				*w.at = i
			}
		}
	}
	return s
}

// restore makes s the running game.
func (g *game) restore(s *saveFile) {
	g.reset(s.Levels, s.Seed)
	if s.Found == nil {
		s.Found = map[[3]int]bool{}
	}
	g.gen.mu.Lock()
	g.gen.found, g.gen.sealed = s.Found, s.Sealed
	g.gen.mu.Unlock()
	g.rg = &rogue.Game{Rng: g.rng, State: s.Rogue}
	g.heroState = *s.Hero
	g.hasLantern = false // its block went with the terrain
	at := func(i int) *rogue.Object {
		if i < 0 {
			return nil
		}
		return s.Pack[i].Obj
	}
	// the equipment before the slots, which then wield and wear nothing new
	g.weapon, g.armor, g.rings = at(s.Weapon), at(s.Armor), [2]*rogue.Object{at(s.Rings[0]), at(s.Rings[1])}
	g.resetInv()
	for _, p := range s.Pack {
		g.give(p.Obj)
		if p.Slot >= 0 {
			g.place(p.Obj, p.Slot)
		}
	}
	g.inv.Held = s.Held
	g.hold()
	g.depth = s.Depth
	g.monsters, g.floor, g.corpses = s.Monsters, s.Floor, s.Corpses
	if g.player == nil {
		g.player = character.New(s.X, s.Y, s.Z)
	}
	g.player.Teleport(s.X, s.Y, s.Z)
	g.cam.Yaw, g.cam.Pitch = s.Yaw, s.Pitch
}

// The game's own structs have unexported fields, which a gob leaves out.
// They go into the save as they are: gobFields sees a struct as one of the
// same layout whose every field is exported (exported), without copying,
// so a gob carries all of it; ungobFields reads it back the same way. A
// struct type a save holds gets its GobEncode and GobDecode from them.

func (m *monster) GobEncode() ([]byte, error)  { return gobFields(m) }
func (m *monster) GobDecode(b []byte) error    { return ungobFields(m, b) }
func (f *floorObj) GobEncode() ([]byte, error) { return gobFields(f) }
func (f *floorObj) GobDecode(b []byte) error   { return ungobFields(f, b) }
func (p *pin) GobEncode() ([]byte, error)      { return gobFields(p) }
func (p *pin) GobDecode(b []byte) error        { return ungobFields(p, b) }
func (h *heroState) GobDecode(b []byte) error  { return ungobFields(h, b) }

// A gob holds no nil in an array: a bare ring hand goes as an empty
// object (restore sets the rings from the pack anyway).
func (h *heroState) GobEncode() ([]byte, error) {
	c := *h
	for i := range c.rings {
		if c.rings[i] == nil {
			c.rings[i] = &rogue.Object{}
		}
	}
	return gobFields(&c)
}
func (t levelThings) GobEncode() ([]byte, error) { return gobFields(&t) }
func (t *levelThings) GobDecode(b []byte) error  { return ungobFields(t, b) }

// exported is struct type t with its fields renamed F0, F1... so all are
// exported; the layout is the same, which it checks.
func exported(t reflect.Type) reflect.Type {
	fs := make([]reflect.StructField, t.NumField())
	for i := range fs {
		f := t.Field(i)
		f.Name, f.PkgPath, f.Anonymous = "F"+strconv.Itoa(i), "", false
		fs[i] = f
	}
	e := reflect.StructOf(fs)
	if e.Size() != t.Size() {
		panic(fmt.Sprintf("%v: exported layout differs", t))
	}
	for i := range fs {
		if e.Field(i).Offset != t.Field(i).Offset {
			panic(fmt.Sprintf("%v: exported layout differs", t))
		}
	}
	return e
}

// asExported is the struct p points to as a value of its exported type.
func asExported(p any) reflect.Value {
	v := reflect.ValueOf(p)
	return reflect.NewAt(exported(v.Type().Elem()), v.UnsafePointer())
}

func gobFields(p any) ([]byte, error) {
	var b bytes.Buffer
	err := gob.NewEncoder(&b).EncodeValue(asExported(p).Elem())
	return b.Bytes(), err
}

func ungobFields(p any, b []byte) error {
	e := asExported(p)
	e.Elem().SetZero() // a gob leaves zero fields out
	return gob.NewDecoder(bytes.NewReader(b)).DecodeValue(e)
}
