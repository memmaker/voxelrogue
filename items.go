package main

// Objects: lying on the floor (walk over them to pick them up), in the
// pack (23 slots, a-w) and used from the inventory screen: the effects
// of potions.c, scrolls.c, rings.c, sticks.c and the missiles of
// weapons.c.

import (
	"fmt"
	"image"
	"math"
	"slices"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

const maxPack = 23

type floorObj struct {
	o    *rogue.Object
	tile dungeon.Coord
}

type itemState struct {
	floor      []*floorObj
	itemRend   *render.ItemRenderer
	itemPoses  []render.ItemPose // drawFloorItems and drawArrows reuse it
	dropMats   []mgl32.Mat4
	confuseHit bool // scroll of monster confusion: the next hit confuses
	zapping    *rogue.Object
}

// setupArt loads the item, weapon and monster sprites from the tile
// sheets, the crossbow model and the attack animations.
func (g *game) setupArt(tileDir string) error {
	names, imgs, err := loadSprites(tileDir)
	if err != nil {
		return err
	}
	if g.itemRend, err = render.NewItemRenderer(g.eng.Renderer, names, imgs); err != nil {
		return err
	}
	g.com, g.sprite = map[string][2]float32{}, map[string]image.Image{}
	for i, n := range names {
		g.com[n], g.sprite[n] = massCenter(imgs[i]), imgs[i]
	}
	if g.partRend, err = render.NewParticleRenderer(g.eng.Renderer); err != nil {
		return err
	}
	if g.modelRend, err = render.NewModelRenderer(g.eng.Renderer); err != nil {
		return err
	}
	g.monSprite = map[byte]monSprite{}
	for _, mk := range rogue.Monsters {
		if img := g.sprite["m:"+mk.Name]; img != nil {
			bb := bbox(img)
			g.monSprite[mk.Letter] = monSprite{rows: float32(bb.Dy()), cy: float32(bb.Min.Y+bb.Max.Y) / 2}
		}
	}
	if err := g.loadAnimations("assets/bettercombat"); err != nil {
		return err
	}
	if err := loadWeaponAttributes("assets/weapon_attributes"); err != nil {
		return err
	}
	if err := g.setupSling(imgs[slices.Index(names, "w:sling_model")]); err != nil {
		return err
	}
	return g.setupArrows()
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func isMagic(o *rogue.Object) bool {
	switch o.Type {
	case rogue.Armor:
		return o.AC != rogue.ArmorClass[o.Which]
	case rogue.Weapon:
		return o.HPlus != 0 || o.DPlus != 0
	case rogue.Potion, rogue.Scroll, rogue.Stick, rogue.Ring, rogue.Amulet:
		return true
	}
	return false
}

// pack is what the hero carries: the inventory's slots in order (the
// equipment last), then the stack on the mouse.
func (g *game) pack() []*rogue.Object {
	var os []*rogue.Object
	if g.inv == nil {
		return nil
	}
	for i := range g.inv.Size() {
		if o := obj(g.inv.Get(i)); o != nil {
			os = append(os, o)
		}
	}
	if o := obj(g.inv.Cursor); o != nil {
		os = append(os, o)
	}
	return os
}

// give puts objects into the pack as they are, without a word.
func (g *game) give(os ...*rogue.Object) {
	for _, o := range os {
		g.inv.Add(stackOf(o))
	}
}

// packCount is the slots in use (a stack of a group takes one).
func (g *game) packCount() int { return len(g.pack()) }

// place swaps o into slot i of the player inventory.
func (g *game) place(o *rogue.Object, i int) {
	p := g.inv
	if o == nil || obj(p.Get(i)) == o {
		return
	}
	for j := range p.Size() {
		if obj(p.Get(j)) == o {
			s := p.Get(j)
			p.Set(j, p.Get(i))
			p.Set(i, s)
			return
		}
	}
}

// hold wields what the held hotbar slot has: a weapon in hand, a wand
// held, anything else (or nothing) is bare hands.
func (g *game) hold() {
	o := obj(g.inv.Current())
	g.zapping = nil
	switch {
	case o != nil && o.Type == rogue.Weapon:
		if o != g.weapon {
			g.wield(o)
		}
		return
	case o != nil && o.Type == rogue.Stick:
		g.zapping = o
	}
	g.weapon = nil
}

// heldUse is the held item right click uses: food, potions, scrolls,
// armour and rings; nil for weapons, wands and nothing.
func (g *game) heldUse() *rogue.Object {
	if g.inv == nil {
		return nil
	}
	o := obj(g.inv.Current())
	if o == nil {
		return nil
	}
	switch o.Type {
	case rogue.Food, rogue.Potion, rogue.Scroll, rogue.Armor, rogue.Ring:
		return o
	}
	return nil
}

// dropAll throws a whole object out of the inventory window onto the
// floor.
func (g *game) dropAll(o *rogue.Object) {
	if o == nil {
		return
	}
	g.removeFromPack(o, true)
	h := g.heroTile()
	g.floor = append(g.floor, &floorObj{o: o, tile: h})
	g.lastTile = h
	g.msg("Dropped " + lowerFirst(g.rg.Name(o)) + ".")
}

// pickUp takes whatever lies on the hero's square.
func (g *game) pickUp() {
	h := g.heroTile()
	for i := 0; i < len(g.floor); i++ {
		f := g.floor[i]
		if f.tile != h {
			continue
		}
		if f.o.Type == rogue.Gold {
			g.purse += f.o.GoldVal
			g.msg(fmt.Sprintf("You found %d gold pieces.", f.o.GoldVal))
		} else if f.o.Type == rogue.Scroll && f.o.Which == rogue.SScare && f.o.Flags&rogue.IsFound != 0 {
			g.msg("The scroll turns to dust as you pick it up.")
		} else {
			if f.o.Type == rogue.Scroll && f.o.Which == rogue.SScare && g.packCount() < maxPack {
				f.o.Flags |= rogue.IsFound
			}
			if !g.addPack(f.o) {
				continue
			}
			if f.o.Type == rogue.Amulet {
				g.amulet = true
				for d := 1; d < maxDepth; d++ { // the way up opens again
					g.sealStairs(d, false)
				}
			}
		}
		g.floor = append(g.floor[:i], g.floor[i+1:]...)
		i--
	}
}

// addPack puts an object into the pack, stacking it where it fits. Bare
// hands wield a hand-held weapon at once: a thrown sword pulled back out
// of the wall is ready again.
func (g *game) addPack(o *rogue.Object) bool {
	p := o
	for _, q := range g.pack() {
		if rogue.Stacks(q, o) {
			p = q
			p.Count += o.Count
			break
		}
	}
	if p == o {
		if g.packCount() >= maxPack {
			g.msg("You can't carry anything else.")
			return false
		}
		if !g.inv.Add(stackOf(o)) {
			g.msg("You can't carry anything else.")
			return false
		}
	}
	g.msg("You now have " + lowerFirst(g.rg.Name(p)) + fmt.Sprintf(" (%c).", g.packLetter(p)))
	if g.weapon == nil && p.Type == rogue.Weapon && weaponItem[p.Which] != "" {
		g.place(p, g.inv.Held)
	}
	return true
}

func (g *game) packLetter(o *rogue.Object) byte {
	for i, p := range g.pack() {
		if p == o {
			return byte('a' + i)
		}
	}
	return '?'
}

// removeFromPack takes one of o out of the pack (all if all).
func (g *game) removeFromPack(o *rogue.Object, all bool) {
	if o.Count > 1 && !all {
		o.Count--
		return
	}
	if g.weapon == o {
		g.weapon = nil
	}
	if g.armor == o {
		g.armor = nil
	}
	for i := range g.rings {
		if g.rings[i] == o {
			g.rings[i] = nil
		}
	}
	if g.zapping == o {
		g.zapping = nil
	}
	if o.Type == rogue.Amulet {
		g.amulet = false
	}
	if obj(g.inv.Cursor) == o {
		g.inv.Cursor = nil
	}
	for i := range g.inv.Size() {
		if obj(g.inv.Get(i)) == o {
			g.inv.Set(i, nil)
		}
	}
}

func (g *game) isWorn(o *rogue.Object) bool {
	return o == g.weapon || o == g.armor || o == g.rings[0] || o == g.rings[1]
}

// dropItem puts one of the object (a whole missile stack) on the floor.
func (g *game) dropItem(o *rogue.Object) {
	h := g.heroTile()
	for _, f := range g.floor {
		if f.tile == h {
			g.msg("There is something there already.")
			return
		}
	}
	if g.isWorn(o) && o.Flags&rogue.IsCursed != 0 {
		g.msg("You can't. It appears to be cursed.")
		return
	}
	drop := o
	if o.Count > 1 && o.Type != rogue.Weapon {
		c := *o
		c.Count = 1
		drop = &c
		o.Count--
	} else {
		g.removeFromPack(o, true)
	}
	g.floor = append(g.floor, &floorObj{o: drop, tile: h})
	g.lastTile = h // don't pick it straight back up
	g.msg("Dropped " + lowerFirst(g.rg.Name(drop)) + ".")
}

// useItem is the inventory's default action for an object.
func (g *game) useItem(o *rogue.Object) {
	switch o.Type {
	case rogue.Food:
		g.eat(o)
	case rogue.Potion:
		g.quaff(o)
	case rogue.Scroll:
		g.read(o)
	case rogue.Weapon:
		g.place(o, g.inv.Held)
	case rogue.Armor: // into the armour slot, which wears it
		if g.armor != nil {
			g.wear(o)
		} else {
			g.place(o, 36+equipArmor)
		}
	case rogue.Ring:
		switch {
		case g.inv.Equip[equipRingL] == nil:
			g.place(o, 36+equipRingL)
		case g.inv.Equip[equipRingR] == nil:
			g.place(o, 36+equipRingR)
		default:
			g.ringOnOff(o)
		}
	case rogue.Stick:
		g.zapping = o
		g.msg("You hold " + lowerFirst(g.rg.Name(o)) + ". Right click zaps it; it strikes like a spear.")
	default:
		g.msg("You can't use that.")
	}
}

func (g *game) eat(o *rogue.Object) {
	g.removeFromPack(o, false)
	if o.Which == 1 {
		g.msg("My, that was a yummy slime-mold.")
	} else if rogue.Rnd(g.rng, 100) > 70 {
		g.msg("Yuk, this food tastes awful.")
		g.gainExp(1)
	} else {
		g.msg("Yum, that tasted good.")
	}
	g.foodLeft = min(g.foodLeft+hungerTime-200+rogue.Rnd(g.rng, 400), stomachSize)
	g.hungry = 0
}

func (g *game) wield(o *rogue.Object) {
	if g.weapon != nil && g.weapon.Flags&rogue.IsCursed != 0 {
		g.msg("You can't. It appears to be cursed.")
		return
	}
	if o == g.armor {
		g.msg("You can't wield armor.")
		return
	}
	g.weapon = o
	g.msg("You are now wielding " + lowerFirst(g.rg.Name(o)) + ".")
}

func (g *game) wear(o *rogue.Object) {
	if o == g.armor {
		if o.Flags&rogue.IsCursed != 0 {
			g.msg("You can't. It appears to be cursed.")
			return
		}
		g.armor = nil
		g.msg("You used to be wearing " + lowerFirst(g.rg.Name(o)) + ".")
		return
	}
	if g.armor != nil {
		g.msg("You are already wearing some. You'll have to take it off first.")
		return
	}
	g.armor = o
	o.Flags |= rogue.IsKnow
	g.noCommand = 2 * turnTicks // waste_time while putting it on
	g.msg("You are now wearing " + lowerFirst(g.rg.Name(o)) + ".")
}

// ringOnOff puts a ring on or takes it off (rings.c). Rogue never names a
// ring for being worn: its effect has to give it away.
func (g *game) ringOnOff(o *rogue.Object) {
	for i, r := range g.rings {
		if r == o {
			if o.Flags&rogue.IsCursed != 0 {
				g.msg("You can't. It appears to be cursed.")
				return
			}
			g.rings[i] = nil
			if o.Which == rogue.RAddStr {
				keep := g.maxStr
				g.chgStr(-o.AC)
				g.maxStr = keep
			}
			g.msg("You removed " + lowerFirst(g.rg.Name(o)) + ".")
			return
		}
	}
	for i, r := range g.rings {
		if r == nil {
			g.rings[i] = o
			switch o.Which {
			case rogue.RAddStr:
				keep := g.maxStr
				g.chgStr(o.AC)
				g.maxStr = keep
			case rogue.RSeeInvis:
				g.look()
			case rogue.RAggr:
				for _, m := range g.monsters {
					m.runto()
				}
			}
			g.msg(fmt.Sprintf("You are now wearing %s on your %s hand.", lowerFirst(g.rg.Name(o)), [2]string{"left", "right"}[i]))
			return
		}
	}
	g.msg("You already have a ring on each hand.")
}

// quaff drinks a potion (potions.c).
func (g *game) quaff(o *rogue.Object) {
	g.removeFromPack(o, false)
	know := true
	switch o.Which {
	case rogue.PConfuse:
		if g.confused == 0 {
			g.msg("Wait, what's going on here. Huh? What? Who?")
		}
		g.confused += (rogue.Rnd(g.rng, 8) + 20) * turnTicks
	case rogue.PParalyze:
		g.noCommand = holdTime * turnTicks
		g.msg("You can't move.")
	case rogue.PPoison:
		if g.wearing(rogue.RSustStr) {
			g.msg("You feel momentarily sick.")
		} else {
			g.chgStr(-(rogue.Rnd(g.rng, 3) + 1))
			g.msg("You feel very sick now.")
		}
	case rogue.PStrength:
		g.chgStr(1)
		g.msg("You feel stronger, now. What bulging muscles!")
	case rogue.PSeeInvis:
		know = false
		if g.seeInvis == 0 {
			g.seeInvis = seeDuration * turnTicks
		}
		g.blind = 0
		g.msg("This potion tastes like slime-mold juice.")
	case rogue.PHealing:
		g.heal(rogue.Roll(g.rng, g.stats.Lvl, 4))
		g.blind = 0
		g.msg("You begin to feel better.")
	case rogue.PMFind:
		if len(g.monsters) > 0 {
			g.detect = 'm'
			g.showMap = 1
			g.msg("You begin to sense the presence of monsters.")
		} else {
			know = false
			g.msg("You have a strange feeling for a moment, then it passes.")
		}
	case rogue.PTFind:
		// known as soon as anything lies about or is carried
		magic, any := false, len(g.floor) > 0
		for _, f := range g.floor {
			magic = magic || isMagic(f.o)
		}
		for _, m := range g.monsters {
			if m.pack != nil {
				any, magic = true, magic || isMagic(m.pack)
			}
		}
		know = any
		if magic {
			g.detect = 'o'
			g.showMap = 1
			g.msg("You sense the presence of magic on this level.")
		} else {
			g.msg("You have a strange feeling for a moment, then it passes.")
		}
	case rogue.PRaise:
		g.msg("You suddenly feel much more skillful.")
		g.gainExp(max(0, rogue.ELevels[min(g.stats.Lvl-1, len(rogue.ELevels)-1)]+1-g.stats.Exp))
	case rogue.PXHeal:
		g.heal(rogue.Roll(g.rng, g.stats.Lvl, 8))
		g.blind = 0
		g.msg("You begin to feel much better.")
	case rogue.PHaste:
		if g.hasted > 0 {
			g.msg("You faint from exhaustion.")
			g.noCommand += rogue.Rnd(g.rng, 8) * turnTicks
			g.hasted = 0
		} else {
			g.hasted = (rogue.Rnd(g.rng, 4) + 4) * turnTicks
			g.msg("You feel yourself moving much faster.")
		}
	case rogue.PRestore:
		know = false
		g.stats.Str = g.maxStr
		g.msg("Hey, this tastes great. It make you feel warm all over.")
	case rogue.PBlind:
		if g.blind == 0 {
			g.blind = seeDuration * turnTicks
		}
		g.msg("A cloak of darkness falls around you.")
	case rogue.PNop:
		know = false
		g.msg("This potion tastes extremely dull.")
	}
	if know {
		g.rg.PKnow[o.Which] = true
	}
}

// heal adds hit points; overhealing raises the maximum by one.
func (g *game) heal(n int) {
	if g.stats.HP += n; g.stats.HP > g.stats.MaxHP {
		g.stats.MaxHP++
		g.stats.HP = g.stats.MaxHP
	}
}

// read reads a scroll (scrolls.c).
func (g *game) read(o *rogue.Object) {
	g.removeFromPack(o, false)
	know := false // most scrolls leave the reader guessing
	switch o.Which {
	case rogue.SConfuse:
		g.confuseHit = true
		g.msg("Your hands begin to glow red.")
	case rogue.SMap:
		know = true
		l, s := g.level(), g.slot()
		for y := range l.Grid {
			for x, t := range l.Grid[y] {
				c := dungeon.Coord{X: x, Y: y}
				if t == dungeon.SecretDoor && !g.gen.isFound(s, c) { // the map shows them as doors
					g.gen.markFound(s, c)
					g.redrawTile(c)
				}
				if t != dungeon.Rock && t != dungeon.Floor {
					g.seen[s-1][y][x] = true
				}
			}
		}
		g.showMap = 1
		g.msg("Oh, now this scroll has a map on it.")
	case rogue.SLight:
		know = true
		g.lightRoom()
	case rogue.SHold:
		h := g.heroTile()
		for _, m := range g.monsters {
			if abs(m.tile.X-h.X) <= 2 && abs(m.tile.Y-h.Y) <= 2 {
				m.held, m.running = math.MaxInt32/2, false
			}
		}
	case rogue.SSleep:
		know = true
		g.noCommand += (rogue.Rnd(g.rng, sleepTime) + 4) * turnTicks
		g.msg("You fall asleep.")
	case rogue.SArmor:
		if g.armor != nil {
			g.armor.AC--
			g.armor.Flags &^= rogue.IsCursed
			g.msg("Your armor glows faintly for a moment.")
		}
	case rogue.SIdent:
		know = true
		g.msg("This scroll is an identify scroll. Pick something to identify.")
		g.prompt = 'i'
		g.openInv()
	case rogue.SScare:
		g.msg("You hear maniacal laughter in the distance.")
	case rogue.SGFind:
		gold := false
		for _, f := range g.floor {
			gold = gold || f.o.Type == rogue.Gold
		}
		if gold {
			know = true
			g.detect = 'g'
			g.showMap = 1
			g.msg("You begin to feel greedy and you sense gold.")
		} else {
			g.msg("You begin to feel a pull downward.")
		}
	case rogue.STelep:
		from := g.level().RoomAt(g.heroTile())
		g.teleport()
		know = from == nil || from != g.level().RoomAt(g.heroTile())
	case rogue.SEnch:
		if g.weapon == nil {
			g.msg("You feel a strange sense of loss.")
		} else {
			g.weapon.Flags &^= rogue.IsCursed
			if rogue.Rnd(g.rng, 100) > 50 {
				g.weapon.HPlus++
			} else {
				g.weapon.DPlus++
			}
			g.msg("Your " + rogue.WeaponNames[g.weapon.Which] + " glows blue for a moment.")
		}
	case rogue.SCreate:
		h, l := g.heroTile(), g.level()
		var spot *dungeon.Coord
		n := 0
		for _, d := range dirs8 {
			c := dungeon.Coord{X: h.X + d.X, Y: h.Y + d.Y}
			if l.At(c).Walkable() && g.monsterAt(c) == nil {
				if n++; rogue.Rnd(g.rng, n) == 0 {
					spot = &c
				}
			}
		}
		if spot != nil {
			g.newMonster(rogue.RandMonster(g.rng, g.depth, false), *spot)
		} else {
			g.msg("You hear a faint cry of anguish in the distance.")
		}
	case rogue.SRemove:
		for _, w := range []*rogue.Object{g.weapon, g.armor, g.rings[0], g.rings[1]} {
			if w != nil {
				w.Flags &^= rogue.IsCursed
			}
		}
		g.msg("You feel as if somebody is watching over you.")
	case rogue.SAggr:
		for _, m := range g.monsters {
			m.runto()
		}
		g.msg("You hear a high pitched humming noise.")
	case rogue.SNop:
		g.msg("This scroll seems to be blank.")
	case rogue.SGenocide:
		know = true
		g.msg("You have been granted the boon of genocide. Press a monster's letter.")
		g.prompt = 'g'
	}
	if know {
		g.rg.SKnow[o.Which] = true
	}
}

// lightRoom is the scroll and the wand of light: the hero's room lights
// up for good.
func (g *game) lightRoom() {
	rm := g.level().RoomAt(g.heroTile())
	if rm == nil {
		g.msg("The corridor glows and then fades.")
		return
	}
	g.msg("The room is lit by a shimmering blue light.")
	g.glow, g.glowRoom = 10, rm // rx1: the light swells over the room
	if rm.Dark {
		rm.Dark = false
		for y := rm.Pos.Y; y < rm.Pos.Y+rm.Max.Y; y++ {
			for x := rm.Pos.X; x < rm.Pos.X+rm.Max.X; x++ {
				g.redrawTile(dungeon.Coord{X: x, Y: y})
			}
		}
	}
	g.look()
}

// identify reveals what an object is.
func (g *game) identify(o *rogue.Object) {
	switch o.Type {
	case rogue.Potion:
		g.rg.PKnow[o.Which] = true
	case rogue.Scroll:
		g.rg.SKnow[o.Which] = true
	case rogue.Ring:
		g.rg.RKnow[o.Which] = true
	case rogue.Stick:
		g.rg.WKnow[o.Which] = true
	}
	o.Flags |= rogue.IsKnow
	g.msg(g.rg.Name(o) + ".")
}

// genocide wipes out a monster kind for the rest of the game.
func (g *game) genocide(letter byte) {
	for _, m := range append([]*monster(nil), g.monsters...) {
		if m.letter == letter {
			g.removeMonster(m)
		}
	}
	rogue.Genocide(letter)
	g.msg("You have wiped out the " + rogue.Monsters[letter-'A'].Name + "s.")
}

// findAmmo is the pack's ammunition for the wielded launcher, nil for none.
func (g *game) findAmmo() *rogue.Object {
	for _, o := range g.pack() {
		if o.Type == rogue.Weapon && o != g.weapon && g.weapon != nil && rogue.Launches(g.weapon.Which, o) {
			return o
		}
	}
	return nil
}

// missileLanding is the last open square along the view direction.
func (g *game) missileLanding() dungeon.Coord {
	l := g.level()
	fx, fz := math.Sin(g.cam.Yaw), -math.Cos(g.cam.Yaw)
	last := g.heroTile()
	for d := 0.5; d < 30; d += 0.5 {
		c := tileAt(g.player.X+fx*d, g.player.Z+fz*d)
		if c != last && !stepOK(l, last, c) {
			break
		}
		last = c
	}
	return last
}

// lineOfFire reports whether the squares between hero and monster are
// open.
func (g *game) lineOfFire(m *monster) bool {
	l := g.level()
	mx, _, mz := g.monsterPos(m, 0)
	p := g.player
	d := math.Hypot(mx-p.X, mz-p.Z)
	last := g.heroTile()
	for t := 0.0; t < d; t += 0.5 {
		c := tileAt(p.X+(mx-p.X)*t/d, p.Z+(mz-p.Z)*t/d)
		if c != last && !stepOK(l, last, c) {
			return false
		}
		last = c
	}
	return true
}

// zap uses a wand or staff (sticks.c do_zap). Light and striking act at
// once, the bolts walk their squares (fireBolt), the rest fly to the
// monster in front as a missile and act when it lands (wandHits).
func (g *game) zap(o *rogue.Object) {
	g.attackCD = 12
	if o.Charges <= 0 {
		g.msg("Nothing happens.")
		return
	}
	o.Charges--
	g.noise(g.player.X, g.player.Z, zapNoise)
	switch o.Which {
	case 0: // light
		g.rg.WKnow[0] = true
		g.lightRoom()
	case 1: // striking
		if m := g.monsterInFront(2.4); m != nil {
			if rogue.Rnd(g.rng, 20) == 0 {
				o.Damage, o.DPlus = "3d8", 9
			} else {
				o.Damage, o.DPlus = "1d8", 3
			}
			g.fight(m, o, false)
		}
	case 2, 3, 4: // lightning, fire, cold
		g.rg.WKnow[o.Which] = true
		g.fireBolt([...]string{"bolt", "flame", "ice"}[o.Which-2])
	case 9: // drain life
		g.drainLife()
	case 10: // nothing
		g.msg("What a bizarre schtick!")
		x, y, z := g.wandTip()
		g.parts.Emit(x, y, z, smoke)
	default:
		if o.Which == 6 {
			g.rg.WKnow[6] = true
		}
		m := g.monsterInFront(18)
		if m != nil && !g.lineOfFire(m) {
			m = nil
		}
		g.zapAt(o, m)
	}
}

// zapAt flies a wand's missile to the monster, or to the wall when there is
// none: rx1's '°' in light cyan for the teleports and light green for the
// magic missile, the grey '*' for the rest.
func (g *game) zapAt(o *rogue.Object, m *monster) {
	var x, y, z float64
	if m != nil {
		x, y, z = g.monsterPos(m, 0)
	} else {
		x, y, z = tileCenter(g.slot(), g.missileLanding())
	}
	col := lightGray
	switch o.Which {
	case 6:
		col = lightGreen
	case 11, 12:
		col = lightCyan
	}
	tx, _, tz := g.wandTip()
	g.cast(missileFX(col), max(2, int(math.Hypot(x-tx, z-tz)/1.5)), fxStop{x, y + 1, z, func() { g.wandHits(o, m) }})
}

// wandHits is a wand's effect on the monster its missile reached, with
// rx1's effect animation around it.
func (g *game) wandHits(o *rogue.Object, m *monster) {
	if m == nil || !m.alive() {
		return
	}
	x, y, z := g.monsterPos(m, 0)
	switch o.Which {
	case 5: // polymorph: magenta glow and glitch
		g.burst(x, y, z, magenta, fade(fxWhite), 1.5)
		was := m.letter
		if m.letter == 'F' {
			g.held = false
		}
		g.initMonster(m, byte('A'+rogue.Rnd(g.rng, 26)))
		m.runto()
		if m.letter != was {
			g.rg.WKnow[5] = true
		}
	case 6: // magic missile
		if !rogue.Save(g.rng, rogue.VSMagic, m.st.Lvl) {
			missile := rogue.Object{Type: rogue.Weapon, Damage: "", HurlDmg: "1d4", Launch: -1, HPlus: 100, DPlus: 1}
			g.burst(x, y, z, lightGreen, fade(lightGreen), 1)
			g.fight(m, &missile, true)
		} else {
			g.msg("The missile vanishes with a puff of smoke.")
			g.parts.Emit(x, y+0.8, z, smoke)
		}
	case 7: // haste monster: yellow orbit
		g.ring(x, y+0.5, z, 0.7, 0, 9, fxYellow, fade(darkYellow), 0.8)
		g.ring(x, y+1.1, z, 0.7, 0, 9, fxWhite, fade(fxYellow), 0.8)
		m.speed = min(m.speed+1, 1)
		m.runto()
	case 8: // slow monster: blue wave closing in, turning the other way
		g.ring(x, y+0.8, z, 1.3, -1.2, -3, lightCyan, fade(blue), 1)
		m.speed = max(m.speed-1, -1)
		m.runto()
	case 11: // teleport away
		if m.letter == 'F' {
			g.held = false
		}
		g.vanish(x, y, z)
		l := g.level()
		for tries := 0; tries < 50; tries++ {
			if c := l.RandomFloor(g.rng); g.monsterAt(c) == nil {
				m.tile, m.from, m.moveT = c, c, 0
				break
			}
		}
		g.vanish(g.monsterPos(m, 0))
		m.runto()
	case 12: // teleport to
		if m.letter == 'F' {
			g.held = false
		}
		h := g.heroTile()
		fx, fz := math.Sin(g.cam.Yaw), -math.Cos(g.cam.Yaw)
		c := tileAt(g.player.X+fx*tileD, g.player.Z+fz*tileD)
		if c != h && stepOK(g.level(), h, c) && (g.monsterAt(c) == nil || g.monsterAt(c) == m) {
			g.vanish(x, y, z)
			m.tile, m.from, m.moveT = c, c, 0
			g.vanish(g.monsterPos(m, 0))
		}
		m.runto()
	case 13: // cancellation: grey wave closing in
		g.ring(x, y+0.8, z, 1.3, -1.5, 0, lightGray, fade(darkGray), 0.9)
		if m.letter == 'F' {
			g.held = false
		}
		m.cancelled = true
		m.flags &^= rogue.IsInvis
	}
}

// drainLife is the wand of drain life (sticks.c drain): half the hero's
// hit points, shared out among the monsters of the room, or around the
// hero in a passage.
func (g *game) drainLife() {
	if g.stats.HP < 2 {
		g.msg("You are too weak to use it.")
		return
	}
	h := g.heroTile()
	in := func(c dungeon.Coord) bool { return abs(c.X-h.X) <= 1 && abs(c.Y-h.Y) <= 1 }
	if rm := g.level().RoomAt(h); rm != nil {
		in = func(c dungeon.Coord) bool {
			return c.X >= rm.Pos.X-1 && c.X <= rm.Pos.X+rm.Max.X && c.Y >= rm.Pos.Y-1 && c.Y <= rm.Pos.Y+rm.Max.Y
		}
	}
	var hit []*monster
	for _, mm := range g.monsters {
		if mm.alive() && in(mm.tile) {
			hit = append(hit, mm)
		}
	}
	if len(hit) == 0 {
		g.msg("You have a tingling feeling.")
		return
	}
	each := g.stats.HP / len(hit)
	g.stats.HP /= 2
	g.hurt = 6
	tx, _, tz := g.wandTip()
	for _, mm := range hit { // rx1: light red '☼' to each of them
		x, y, z := g.monsterPos(mm, 0)
		g.cast(missileFX(lightRed), max(2, int(math.Hypot(x-tx, z-tz)/1.5)), fxStop{x, y + 1, z, func() {
			g.burst(x, y, z, lightRed, fade(fxRed), 1)
			if mm.st.HP -= each; mm.st.HP < 1 {
				g.killed(mm)
			}
		}})
	}
}

// drawFloorItems draws the objects lying around, slowly turning.
func (g *game) drawFloorItems(alpha float64) {
	g.itemPoses = g.itemPoses[:0]
	for i, f := range g.floor {
		x, y, z := tileCenter(g.slot(), f.tile)
		g.dropPoses(f.o, x, y, z, g.tick+i*7, alpha)
	}
	for i, m := range g.monsters { // a mimic looks like what it pretends to be
		if m.disguise == nil {
			continue
		}
		x, y, z := tileCenter(g.slot(), m.tile)
		g.dropPoses(m.disguise, x, y, z, g.tick+i*7, alpha)
	}
	g.itemRend.DrawPosed(&g.cam, g.itemPoses)
}

// dropPoses adds an object lying on the floor at x, y (the floor's top),
// z to itemPoses as the reference's dropped item: hovering, spinning, a
// copy more per stack size step. age ticks spread the spins.
func (g *game) dropPoses(o *rogue.Object, x, y, z float64, age int, alpha float64) {
	k := spriteFor(g.rg, o)
	ms, bob := render.FlatDropPoses(age, alpha, o.Count, g.itemRend.In2D(k), g.dropMats[:0])
	g.dropMats = ms
	y += render.DropYOffset + bob
	sky, blk := g.light(x, y, z)
	for _, m := range ms {
		g.itemPoses = append(g.itemPoses, render.ItemPose{Item: k, X: x, Y: y, Z: z, M: m, Sky: sky, Block: blk})
	}
}

// drawHand draws the wielded weapon (or held wand) in first person.
func (g *game) drawHand(alpha float64) {
	held := g.handItem
	if g.zapping != nil {
		held = g.zapping
	}
	if held == nil || g.dead {
		return
	}
	sky, blk := g.light(g.cam.X, g.cam.Y, g.cam.Z)
	pt := float32(alpha)
	t := (g.prevWalk + (g.walkDist-g.prevWalk)*alpha) * 1.6
	h := handPose{
		bobX: float32(math.Sin(t)) * 0.02, bobY: -float32(math.Abs(math.Cos(t))) * 0.03,
		armHeight: 1 - (g.prevHandHeight + (g.handHeight-g.prevHandHeight)*pt),
		swayX:     (g.xRotMC() - (g.prevXBob + (g.xBob-g.prevXBob)*pt)) * 0.1,
		swayY:     wrapDegrees(g.yRotMC()-(g.prevYBob+(g.yBob-g.prevYBob)*pt)) * 0.1,
	}
	if g.swing > 0 {
		h.swing = 1 - (float32(g.swing)-pt)/swingTicks
	}
	if held == g.weapon {
		h.use, h.useT = g.handUse(pt)
		h.charge = chargeTicks(held.Which)
	}
	name := spriteFor(g.rg, held)
	if name == "w:sling" { // Nightfall's model and action
		g.drawSling(h, pt, sky, blk)
		return
	}
	switch {
	case name == weaponSprite(rogue.Bow) && h.use == useBow: // items/bow: use_duration scaled 0.05
		name += fmt.Sprintf("_pulling_%d", pullStage(float32(g.drawTicks)*0.05, 0.65, 0.9))
	case name == weaponSprite(rogue.GreatBow) && h.use == useBow: // the longbow's pulling models, use_duration scaled 1/32
		name += fmt.Sprintf("_pulling_%d", pullStage(float32(g.drawTicks)/fullDraw(rogue.GreatBow), 0.65, 0.9))
	case crossbowSprite(name) && h.use == useCrossbowCharge: // crossbow/pull
		name += fmt.Sprintf("_pulling_%d", pullStage((h.useT+1-pt)/h.charge, 0.58, 1)) // the ticks held so far over the charge
	case crossbowSprite(name) && h.use == useCrossbowHold: // charge_type arrow
		name += "_arrow"
	}
	if handSprites[name] {
		name += "_hand"
	}
	if g.drawAttack(name, pt, sky, blk) {
		return // Better Combat's arms and weapon
	}
	g.itemRend.DrawHandModel(name, float32(g.cam.Aspect), handModel(name, h), sky, blk)
}

// pullStage is a bow's or crossbow's pulling model for its pull: 0, or 1
// and 2 from the model's range_dispatch thresholds.
// crossbowSprite reports whether a sprite is a crossbow with pulling and
// loaded pictures (items/crossbow, weapons_expanded's chain crossbow).
func crossbowSprite(name string) bool {
	return name == weaponSprite(rogue.Crossbow) || name == weaponSprite(rogue.ChainCrossbow)
}

func pullStage(pull, one, two float32) int {
	switch {
	case pull >= two:
		return 2
	case pull >= one:
		return 1
	}
	return 0
}
