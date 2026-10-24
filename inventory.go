package main

// The pack is VoxelLib's player inventory, run by ui.PlayerGUI: the
// hotbar (1-9, the wheel) holds what is wielded, E/Tab/I opens the
// inventory screen (no crafting) with a ring each side of the armour, and right click uses what is held. The slots are the
// pack; Rogue's rules come in through the events and Deny.

import (
	"fmt"
	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxellib/survival"
	"image"
	"math/rand"

	"github.com/memmaker/voxellib/ui/guientity"
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
	eating survival.Eating // VoxelLib's eating of the held food (setupEating)
	icons  map[string]*ui.Sprite
	sprite map[string]image.Image     // item sprites by name, for the icons
	layers map[string]*image.NRGBA    // armour textures by set and layer, for the hero preview
	meshes map[string]guientity.Armor // the armour meshes by id (meshes.rec)
	worn   map[string][4]string       // armour name -> mesh id per slot (gear.rec)
}

// resetInv makes the hero's empty inventory: Rogue has no half stacks,
// and the slot events wear, take off and wield.
func (g *game) resetInv() {
	objOf, itemOf = map[*inventory.Item]*rogue.Object{}, map[string]*inventory.Item{}
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
	g.setupEating(p)
	if g.pg != nil {
		g.pg.Player, g.pg.Container = p, c
	}
}

// obj is the Rogue object a stack stands for.
// The pack is VoxelLib's player inventory: an object is a stack of its
// count under the item of its stacking identity (stackKey), one item per
// identity, so the inventory stacks alike objects itself (Player.Add tops
// up a matching stack, the container merges them) and the stack's count
// is the count. objOf is the object behind each item, itemOf the item of
// each identity; obj mirrors the stack's count into the object.
var (
	objOf  = map[*inventory.Item]*rogue.Object{}
	itemOf = map[string]*inventory.Item{}
)

func obj(s *inventory.Stack) *rogue.Object {
	if s == nil {
		return nil
	}
	o := objOf[s.Item]
	if o != nil {
		o.Count = s.Count
	}
	return o
}

// stackKey is what an object stacks with (rogue.Stacks): potions, scrolls
// and food of a kind, missiles of a group; anything else only itself.
func stackKey(o *rogue.Object) string {
	if !rogue.Stacks(o, o) {
		return fmt.Sprintf("%p", o)
	}
	return fmt.Sprint(o.Type, o.Which, o.Group)
}

// stackOf is an object as an inventory stack: its count under the item
// of its identity. The item's object is o unless the pack holds one.
func (g *game) stackOf(o *rogue.Object) *inventory.Stack {
	o.Count = max(o.Count, 1) // a bare object is one
	k := stackKey(o)
	it := itemOf[k]
	if it == nil {
		it = &inventory.Item{Key: k, MaxStack: 1}
		if rogue.Stacks(o, o) {
			it.MaxStack = 64
		}
		itemOf[k] = it
	}
	if o.Type == rogue.Food {
		it.Food = &inventory.Food{AlwaysEdible: true} // Rogue has no hunger bar to fill
	}
	if g.stackIn(it) == nil {
		objOf[it] = o
	}
	return &inventory.Stack{Item: it, Count: o.Count}
}

// setupEating wires VoxelLib's eating (survival.Eating) to the pack: the
// held food is eaten over its use ticks with the hand swung to the mouth
// (drawFlat), crumbs of the sprite's colour and the eat sounds, and
// Rogue's meal (eat) is the finish.
func (g *game) setupEating(p *inventory.Player) {
	g.eating = survival.Eating{
		Use: &actors.ItemUse{}, Actor: &actors.Actor{}, Body: &survival.Body{Food: survival.NewFood()}, Inv: p, Rand: rngV2{g.rng},
		Sound: func(event string, _, _ float64) { g.play(event) },
		Crumb: func(st *inventory.Stack, x, y, z, vx, vy, vz float64) {
			c := avgColor(g.sprite[spriteFor(g.rg, obj(st))])
			g.parts.Emit(x, y, z, particles.Props{Count: 1, Vel: [3]float64{vx * 20, vy * 20, vz * 20}, Gravity: 8, Life: 0.5, LifeVar: 0.2, Size0: 0.07, Size1: 0.07, Col0: c, Col1: c})
		},
		Mouth:   func() (x, y, z float64, yaw, pitch float32) { return g.cam.X, g.cam.Y, g.cam.Z, g.yRotMC(), g.xRotMC() },
		OnEaten: func(e *survival.Eating, st *inventory.Stack) { e.Feedback(st, 16); e.Burp(); g.eat(obj(st)) },
	}
}

// rngV2 is the game's rand as survival.Rand (math/rand/v2's names).
type rngV2 struct{ *rand.Rand }

func (r rngV2) IntN(n int) int { return r.Intn(n) }

// stackIn is the pack's stack of an item (a slot's, else the cursor's); nil for none.
func (g *game) stackIn(it *inventory.Item) *inventory.Stack {
	for i := range g.inv.Size() {
		if s := g.inv.Get(i); s != nil && s.Item == it {
			return s
		}
	}
	if s := g.inv.Cursor; s != nil && s.Item == it {
		return s
	}
	return nil
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
	tip := []string{g.rg.Name(o)}
	switch o.Type { // the lore lines: a weapon's damage dice, an armour's class (lower is better)
	case rogue.Weapon:
		d := "Damage " + o.Damage
		if o.HurlDmg != o.Damage {
			if o.Launch >= 0 {
				d += ", fired " + o.HurlDmg
			} else {
				d += ", thrown " + o.HurlDmg
			}
		}
		tip = append(tip, d)
	case rogue.Armor:
		ac := rogue.Armors[o.Which].AC // the plain kind's class until the enchantment is known
		if o.Flags&rogue.IsKnow != 0 {
			ac = o.AC
		}
		tip = append(tip, fmt.Sprintf("Armor class %d", ac))
	}
	return ui.ItemStack{Icon: g.icon(spriteFor(g.rg, o)), Count: o.Count, Tooltip: tip}
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
