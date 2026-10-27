package main

import (
	"math/rand"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

// TestSaveLoad saves a game a few ticks in, with a corpse, a found trap, a
// sealed staircase and a potion in the pack, loads it into another game
// and expects the same Rogue state, levels, hero, pack, monsters and
// place, the equipment the pack's own objects, and dice that roll alike.
func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.gob")
	newG := func(seed int64) *game {
		g := &game{rng: rand.New(rand.NewSource(seed)), gen: newLevelGen(voxel.NewRegistry(), buildLevels(seed, false)), saveFile: path}
		g.rg = rogue.NewGame(g.rng)
		g.initHero()
		g.enterLevel(1)
		return g
	}
	g := newG(7)
	for range 30 {
		g.tickMonsters()
	}
	g.killed(g.newMonster('B', g.level().RandomFloor(g.rng)))
	g.give(&rogue.Object{Type: rogue.Potion, Which: 1, Count: 2})
	g.gen.markFound(1, dungeon.Coord{X: 3, Y: 4})
	g.gen.seal(1, true)
	g.cam.Yaw, g.cam.Pitch = 1.5, -0.2
	g.purse = 42
	g.save()
	names := func(g *game) (ns []string) {
		for i := range g.inv.Size() {
			if o := obj(g.inv.Get(i)); o != nil {
				ns = append(ns, g.rg.Name(o))
			} else {
				ns = append(ns, "")
			}
		}
		return ns
	}
	gPack := names(g) // before the load: the pack's item maps are the package's

	h := newG(8)
	h.load()
	if !slices.Contains(h.msgs, "Game loaded.") {
		t.Fatalf("messages %q", h.msgs)
	}
	same := func(what string, a, b any) {
		if !reflect.DeepEqual(a, b) {
			t.Errorf("%s differs:\n%+v\n%+v", what, a, b)
		}
	}
	hv, gv := asExported(&h.heroState).Elem(), asExported(&g.heroState).Elem()
	for i := range hv.NumField() {
		if !reflect.DeepEqual(hv.Field(i).Interface(), gv.Field(i).Interface()) {
			t.Errorf("hero's %s differs", reflect.TypeOf(h.heroState).Field(i).Name)
		}
	}
	if h.rg.State != g.rg.State {
		t.Error("the Rogue state differs")
	}
	same("levels", h.gen.levels, g.gen.levels)
	same("found", h.gen.found, g.gen.found)
	same("sealed", h.gen.sealed, g.gen.sealed)
	same("monsters", h.monsters, g.monsters)
	same("floor", h.floor, g.floor)
	same("corpses", h.corpses, g.corpses)
	same("depth", h.depth, g.depth)
	same("place", [5]float64{h.player.X, h.player.Y, h.player.Z, h.cam.Yaw, h.cam.Pitch}, [5]float64{g.player.X, g.player.Y, g.player.Z, g.cam.Yaw, g.cam.Pitch})
	same("pack", names(h), gPack)
	same("held", h.inv.Held, g.inv.Held)
	if h.weapon == nil || !slices.Contains(h.pack(), h.weapon) || h.armor == nil || !slices.Contains(h.pack(), h.armor) {
		t.Errorf("the equipment is not the pack's: weapon %p, armour %p", h.weapon, h.armor)
	}
	if h.rng.Int63() != g.rng.Int63() {
		t.Error("the dice roll apart")
	}
}

// TestLoadMissing: loading with no save is a message, not a crash.
func TestLoadMissing(t *testing.T) {
	g := &game{saveFile: filepath.Join(t.TempDir(), "none.gob")}
	g.load()
	if !slices.Contains(g.msgs, "There is no saved game.") {
		t.Fatalf("messages %q", g.msgs)
	}
}
