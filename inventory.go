package main

// The pack is VoxelLib's player inventory, run by ui.PlayerGUI: the
// hotbar (1-9, the wheel) holds what is wielded, E/Tab/I opens the
// inventory screen (no crafting) with a ring each side of the armour, and right click uses what is held. The slots are the
// pack; Rogue's rules come in through the events and Deny.

import (
	"fmt"
	"image"
	"image/color"

	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxelrogue/rogue"
)

// Equipment slots (Player.Equip indices; the column shows them last first).
const (
	equipRingR = iota
	equipRingL
	equipArmor
)

func takes(t rogue.ObjType) func(*inventory.Stack) bool {
	return func(s *inventory.Stack) bool { o := obj(s); return o != nil && o.Type == t }
}

var rogueEquip = []inventory.EquipSlot{
	{Name: "ring", Icon: "rogue:ring_slot", Accept: takes(rogue.Ring), Max: 1},
	{Name: "ring", Icon: "rogue:ring_slot", Accept: takes(rogue.Ring), Max: 1},
	{Name: "armor", Icon: "textures/items/empty_armor_slot_chestplate.png", Accept: takes(rogue.Armor), Max: 1},
}

// rogueEquipAt puts a ring on each side of the armour (the right hand's
// on the left, as the player faces out of the screen).
var rogueEquipAt = [][2]int{equipRingR: {0, 0}, equipRingL: {2, 0}, equipArmor: {1, 0}}

type invState struct {
	inv    *inventory.Player
	cont   *inventory.Container
	pg     *ui.PlayerGUI
	icons  map[string]*ui.Sprite
	sprite map[string]image.Image // item sprites by name, for the icons
}

// resetInv makes the hero's empty inventory: Rogue has no half stacks,
// and the slot events wear, take off and wield.
func (g *game) resetInv() {
	p := inventory.NewPlayer(inventory.PlayerSpec{Equip: rogueEquip})
	c, _ := inventory.NewPlayerContainer(p, nil)
	c.NoSplit, c.Deny, c.Drop = true, g.denyClick, func(s *inventory.Stack) { g.dropAll(obj(s)) }
	p.OnEquip, p.OnHeld = g.onEquip, g.onHeld
	p.OnChange = func(i int, _, _ *inventory.Stack) {
		if i == p.Held {
			g.hold()
		}
	}
	g.inv, g.cont = p, c
	if g.pg != nil {
		g.pg.Player, g.pg.Container = p, c
	}
}

// obj is the Rogue object a stack stands for.
func obj(s *inventory.Stack) *rogue.Object {
	if s == nil {
		return nil
	}
	o, _ := s.Tag["o"].(*rogue.Object)
	return o
}

// stackOf wraps an object as a stack of one: the object keeps its count.
func stackOf(o *rogue.Object) *inventory.Stack {
	it := &inventory.Item{Key: fmt.Sprint(o.Type, o.Which), MaxStack: 1}
	return &inventory.Stack{Item: it, Count: 1, Tag: map[string]any{"o": o}}
}

// onEquip puts on what came into the armour and ring slots and takes
// off what left them.
func (g *game) onEquip(slot int, old, new *inventory.Stack) {
	o, n := obj(old), obj(new)
	switch slot {
	case equipArmor:
		if o != nil && o == g.armor {
			g.wear(o) // takes it off
		}
		if n != nil && n != g.armor {
			g.wear(n)
		}
	case equipRingL, equipRingR:
		if o != nil && (o == g.rings[0] || o == g.rings[1]) {
			g.ringOnOff(o)
		}
		if n != nil && n != g.rings[0] && n != g.rings[1] {
			g.ringOnOff(n)
		}
	}
}

// onHeld wields the new hotbar slot; a cursed weapon won't leave the hand.
func (g *game) onHeld(old, _ int) {
	if g.weapon != nil && g.weapon.Flags&rogue.IsCursed != 0 {
		g.inv.Held = old
		g.msg("You can't. It appears to be cursed.")
		return
	}
	g.hold()
}

// denyClick keeps Rogue's rules in the container: an identify scroll's
// click names the object instead of moving it, cursed equipment stays put.
func (g *game) denyClick(slot, button, mode int) bool {
	var o *rogue.Object
	if slot >= 0 && slot < len(g.cont.Slots) {
		o = obj(g.cont.Slots[slot].Stack())
	}
	switch {
	case g.prompt == 'i':
		if o != nil {
			g.prompt = 0
			g.identify(o)
		}
		return true
	case o != nil && g.isWorn(o) && o.Flags&rogue.IsCursed != 0:
		g.msg("You can't. It appears to be cursed.")
		return true
	}
	return false
}

func (g *game) itemStack(s *inventory.Stack) ui.ItemStack {
	o := obj(s)
	if o == nil {
		return ui.ItemStack{}
	}
	return ui.ItemStack{Icon: g.icon(spriteFor(g.rg, o)), Count: o.Count, Tooltip: []string{g.rg.Name(o)}}
}

// slotIcon is an empty slot's icon: the sprite's shape in the colour of
// the jar's empty armour slots.
func slotIcon(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		for x := range b.Dx() {
			if _, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA(); a > 0x8000 {
				out.SetNRGBA(x, y, color.NRGBA{25, 32, 34, 255})
			}
		}
	}
	return out
}

// icon is an item sprite as a GUI texture, uploaded once.
func (g *game) icon(name string) *ui.Sprite {
	if sp := g.icons[name]; sp != nil {
		return sp
	}
	img := g.sprite[name]
	if img == nil {
		img = image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	b := img.Bounds()
	sp := &ui.Sprite{Tex: g.pg.Ctx.Canvas.NewTexture(img), W: float32(b.Dx()), H: float32(b.Dy())}
	g.icons[name] = sp
	return sp
}
