package main

import (
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

// TestInventorySlots drags the worn mail out of the armour slot and a
// ring into a ring slot, and expects the mail taken off and the ring on.
func TestInventorySlots(t *testing.T) {
	g := &game{rng: rand.New(rand.NewSource(1))}
	g.rg = rogue.NewGame(g.rng)
	mace, mail := rogue.NewWeapon(g.rng, rogue.WeaponID("mace")), &rogue.Object{Type: rogue.Armor, Which: rogue.ArmorID("ring mail"), Count: 1}
	ring := &rogue.Object{Type: rogue.Ring, Which: rogue.RProtect, Count: 1}
	g.weapon, g.armor = mace, mail
	g.resetInv()
	g.give(mace, mail, ring)
	g.place(mail, 36+equipArmor)
	if obj(g.inv.Current()) != mace || obj(g.inv.Equip[equipArmor]) != mail || g.armor != mail {
		t.Fatalf("held %v, armour slot %v", obj(g.inv.Current()), obj(g.inv.Equip[equipArmor]))
	}
	armor := 5 + len(rogueEquip) - 1 - equipArmor        // the column shows the equipment last first
	g.cont.Click(armor, 0, inventory.Pickup)             // the mail onto the mouse
	g.cont.Click(5+len(rogueEquip), 0, inventory.Pickup) // into the main grid
	if g.armor != nil || obj(g.inv.Equip[equipArmor]) != nil {
		t.Fatalf("mail still worn after leaving the armour slot")
	}
	ri := -1
	for i, sl := range g.cont.Slots {
		if obj(sl.Stack()) == ring {
			ri = i
		}
	}
	g.cont.Click(ri, 0, inventory.Pickup)
	g.cont.Click(5+len(rogueEquip)-1-equipRingL, 0, inventory.Pickup)
	if g.rings[0] != ring && g.rings[1] != ring || g.packCount() != 3 {
		t.Fatalf("rings %v, pack %d", g.rings, g.packCount())
	}
}

// TestStackPickup: a pebble shot and picked up again tops up the pebble
// stack instead of taking a slot of its own; the last one empties it.
func TestStackPickup(t *testing.T) {
	g := &game{}
	g.player = &character.Character{}
	g.resetInv()
	pebbles := rogue.NewWeapon(rand.New(rand.NewSource(1)), rogue.WeaponID("pebble"))
	pebbles.Count = 5
	g.give(pebbles)
	shot := &rogue.Object{}
	*shot = *pebbles
	shot.Count = 1
	g.removeFromPack(pebbles, false)
	if n := g.packCount(); n != 1 || pebbles.Count != 4 {
		t.Fatalf("after a shot: %d objects, %d pebbles", n, pebbles.Count)
	}
	if !g.addPack(shot) || g.packCount() != 1 || pebbles.Count != 5 {
		t.Fatalf("picked up: %d objects, %d pebbles", g.packCount(), pebbles.Count)
	}
	for range 5 {
		g.removeFromPack(pebbles, false)
	}
	if g.packCount() != 0 {
		t.Fatalf("emptied: %d objects left", g.packCount())
	}
}

// TestEatHeld: right-clicked held food is eaten VoxelLib's way, over its
// 32 use ticks, and Rogue's meal happens at the end.
func TestEatHeld(t *testing.T) {
	g := &game{rng: rand.New(rand.NewSource(1))}
	g.rg = rogue.NewGame(g.rng)
	g.player = &character.Character{}
	g.resetInv()
	food := &rogue.Object{Type: rogue.Food, Count: 2}
	g.give(food) // the held slot
	if !g.eating.Start(g.inv.Current()) {
		t.Fatal("the food is not eaten")
	}
	for range 31 {
		g.eating.Tick()
	}
	if g.eating.Use.Using == nil || food.Count != 2 {
		t.Fatalf("eaten early: %d left", food.Count)
	}
	g.eating.Tick()
	if g.eating.Use.Using != nil || food.Count != 1 || g.foodLeft == 0 {
		t.Fatalf("after 32 ticks: using %v, %d left, food %d", g.eating.Use.Using, food.Count, g.foodLeft)
	}
}

// TestScrollPickup: a floor scroll of a kind the pack holds joins its
// stack and the floor copy is used up, never doubled; what the inventory
// screen drops (one, or the stack) comes back the same way; a new kind
// leaves the floor too.
func TestScrollPickup(t *testing.T) {
	g := &game{rng: rand.New(rand.NewSource(1))}
	g.rg = rogue.NewGame(g.rng)
	g.player = &character.Character{}
	g.resetInv()
	have := &rogue.Object{Type: rogue.Scroll, Which: 3, Count: 1}
	g.addPack(have)
	for n := 2; n <= 4; n++ {
		f := &rogue.Object{Type: rogue.Scroll, Which: 3, Count: 1}
		g.floor = []*floorObj{{o: f, tile: g.heroTile()}}
		g.pickUp()
		if len(g.floor) != 0 || g.packCount() != 1 || have.Count != n || f.Count != 0 {
			t.Fatalf("pickup %d: %d on the floor (count %d), %d objects, have %d", n, len(g.floor), f.Count, g.packCount(), have.Count)
		}
	}
	g.cont.Click(g.slotOf(have), 0, inventory.Throw) // Q drops one
	if len(g.floor) != 1 || g.floor[0].o.Count != 1 || have.Count != 3 || g.packCount() != 1 {
		t.Fatalf("Q: %d on the floor, have %d, %d objects", len(g.floor), have.Count, g.packCount())
	}
	g.lastTile = dungeon.Coord{X: -1}
	g.pickUp()
	if len(g.floor) != 0 || g.packCount() != 1 || have.Count != 4 {
		t.Fatalf("Q and taken back: %d on the floor, %d objects, have %d", len(g.floor), g.packCount(), have.Count)
	}
	g.cont.Click(g.slotOf(have), 1, inventory.Throw) // shift-Q the stack
	if len(g.floor) != 1 || g.floor[0].o != have || have.Count != 4 || g.packCount() != 0 {
		t.Fatalf("shift-Q: %d on the floor, have %d, %d objects", len(g.floor), have.Count, g.packCount())
	}
	g.lastTile = dungeon.Coord{X: -1}
	g.pickUp()
	if len(g.floor) != 0 || g.packCount() != 1 || have.Count != 4 {
		t.Fatalf("shift-Q and taken back: %d on the floor, %d objects, have %d", len(g.floor), g.packCount(), have.Count)
	}
	mace := rogue.NewWeapon(g.rng, rogue.WeaponID("mace"))
	g.floor = []*floorObj{{o: mace, tile: g.heroTile()}}
	g.pickUp()
	if len(g.floor) != 0 || g.packCount() != 2 || g.weapon != mace {
		t.Fatalf("a mace: %d on the floor, %d objects, wielding %v", len(g.floor), g.packCount(), g.weapon)
	}
}

// slotOf is the inventory screen's slot of an object.
func (g *game) slotOf(o *rogue.Object) int {
	for i, sl := range g.cont.Slots {
		if obj(sl.Stack()) == o {
			return i
		}
	}
	return -1
}
