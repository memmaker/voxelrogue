package main

import (
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/inventory"
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
