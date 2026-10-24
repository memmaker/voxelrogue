package main

// Bows and crossbows as in Minecraft: hold the right button to draw a bow
// and let go to shoot, the arrow the faster the longer it was drawn
// (BowItem). The crossbows fire as VoxelStealth's: on the click, then
// span themselves again (over Minecraft's charge) and repeat while the
// button is held. The hand draws them as ItemInHandRenderer does, with
// its item swap and view sway (handanim.go). Arrows fly as
// real projectiles (voxellib's projectile:
// Minecraft's arc and drag). An arrow that strikes a monster rolls
// fight.c's dice for a thrown missile; one that misses its roll, or
// sticks in a wall, drops where it came down.

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

// launchSpeed is a launcher's arrow speed in blocks per tick, fully
// drawn: Minecraft's bow and crossbow, and weapons_expanded's longbow
// for the great bow.
var launchSpeed = map[int]float64{
	rogue.Bow:           3.0,
	rogue.GreatBow:      4.0,
	rogue.Crossbow:      projectile.Speed,
	rogue.ChainCrossbow: projectile.Speed,
	rogue.Sling:         slingVelocity, // Nightfall's, at full charge (sling.go)
}

// chargeTicks is a crossbow's CrossbowItem.getChargeDuration: 1.25 s,
// or weapons_expanded's chain crossbow's load time (its config's default).
func chargeTicks(which int) float32 {
	if which == rogue.ChainCrossbow {
		return 38
	}
	return 25
}

// reloadTicks is how long a crossbow spans: its charge and the three ticks
// of use beyond it (CrossbowItem.getUseDuration).
func reloadTicks(which int) int { return int(chargeTicks(which)) + 3 }

// isCrossbow reports whether a weapon kind is a crossbow.
func isCrossbow(which int) bool { return which == rogue.Crossbow || which == rogue.ChainCrossbow }

// weapons_expanded's chain crossbow (its config's defaults): the magazine's
// chambers, and the cooldown between shots. Each chamber is loaded by
// holding the right button over the crossbow's charge and letting go; the
// left button shoots instead of attacking while one is loaded, and the
// next chamber slides in at once.
const chainMagazine, chainCooldown = 3, 8

// crossbowGroup is the bone holding the crossbow in cleannrooster's artmob.
const crossbowGroup = "repeater_crossbow_arrow"

type arrowState struct {
	arrows      projectile.World
	arrowObjs   map[*projectile.Arrow]*rogue.Object // what each arrow is
	arrowModel  int
	arrowInsts  []render.ModelInstance
	drawing     bool            // the right button is drawing a bow
	drawTicks   int             // how long
	fireHeld    bool            // the right button is down on a crossbow or sling
	reload      int             // ticks until the crossbow is spanned again
	chambers    []*rogue.Object // the chain crossbow's loaded bolts, the next shot first
	cooldown    int             // ticks until the chain crossbow shoots again
	sling       slingAction
	slingModels [2]int // the sling's resting and swinging models

	// the first-person hand (ItemInHandRenderer): the item it shows and its
	// height, which dips to swap items; the sway (LocalPlayer's xBob,
	// yBob) and the smoothed field of view (Camera's fovModifier)
	handItem                       *rogue.Object
	handHeight, prevHandHeight     float32
	xBob, yBob, prevXBob, prevYBob float32
	fovMod, prevFovMod             float32
}

// arrowTarget makes a monster something arrows can hit.
type arrowTarget struct {
	g *game
	m *monster
}

func (t arrowTarget) Frame() (x, y, z, yaw float64) {
	x, y, z = t.g.monsterPos(t.m, 0)
	return x, y + float64(monsterLooks[t.m.letter].hover), z, 0
}

func (t arrowTarget) Box() collide.AABB {
	x, y, z, _ := t.Frame()
	lk := monsterLooks[t.m.letter]
	r := float64(lk.w) / 2
	return collide.AABB{MinX: x - r, MinY: y, MinZ: z - r, MaxX: x + r, MaxY: y + float64(lk.h), MaxZ: z + r}
}

func (g *game) setupArrows() error {
	g.arrowObjs = map[*projectile.Arrow]*rogue.Object{}
	var err error
	g.arrowModel, err = g.modelRend.Add(arrowModel())
	if err != nil {
		return err
	}
	g.arrows.Hit = func(a *projectile.Arrow, t projectile.Target) {
		at, ok := t.(arrowTarget)
		o := g.arrowObjs[a]
		if !ok || o == nil {
			return
		}
		delete(g.arrowObjs, a)
		a.Discard()
		if !g.fight(at.m, o, true) {
			g.dropMissile(o, at.m.tile)
		} // a hit breaks it in the monster
	}
	return err
}

// launcher reports whether a bow or crossbow is wielded.
func (g *game) launcher() bool { return g.weapon != nil && launchSpeed[g.weapon.Which] > 0 }

// startDraw is the right button going down with a launcher in hand.
func (g *game) startDraw() {
	switch g.weapon.Which {
	case rogue.Crossbow:
		g.fireHeld = true
		return
	case rogue.Sling:
		g.fireHeld = true
		g.startSling()
		return
	case rogue.ChainCrossbow:
		if len(g.chambers) >= chainMagazine { // ChainCrossbowItem.use: FAIL with its "full" sound
			g.msg("The chain crossbow is full.")
			return
		}
	}
	if a := g.findAmmo(); a == nil || !rogue.Launches(g.weapon.Which, a) {
		g.msg("You have nothing to shoot.")
		return
	}
	g.drawing, g.drawTicks = true, 0
	g.handHeight = 0 // the use starts (startUseItem: itemUsed)
}

// releaseDraw is the right button coming up: a drawn bow shoots (if
// drawn a tenth or more), a crossbow stops repeating, and the chain
// crossbow keeps the chamber if it was held over the whole charge
// (releaseUsing: the bolt loaded at the charge's end joins the queue).
func (g *game) releaseDraw() {
	g.fireHeld = false
	if !g.drawing {
		return
	}
	g.drawing = false
	if g.weapon.Which == rogue.ChainCrossbow {
		if g.drawTicks >= int(chargeTicks(rogue.ChainCrossbow)) {
			g.chambers = append(g.chambers, g.takeAmmo())
			g.msg(fmt.Sprintf("Chain crossbow: %d/%d shots.", len(g.chambers), chainMagazine))
		}
		return
	}
	if pw := float64(drawPower(float32(g.drawTicks), fullDraw(g.weapon.Which))); pw >= 0.1 {
		g.shoot(g.takeAmmo(), pw*launchSpeed[g.weapon.Which])
	}
}

// tickDraw draws the bow on, and spans and fires the crossbow.
func (g *game) tickDraw() {
	defer g.tickFov()
	if g.reload > 0 {
		g.reload--
	}
	if g.cooldown > 0 {
		g.cooldown--
	}
	if !g.launcher() || g.dead || g.showInv || g.noCommand > 0 {
		g.drawing, g.fireHeld, g.sling = false, false, slingAction{}
		return
	}
	if g.fireHeld && isCrossbow(g.weapon.Which) && g.reload == 0 {
		g.fireCrossbow()
	}
	g.tickSling()
	if g.drawing {
		g.drawTicks++
	}
}

// fireCrossbow shoots a bolt (VoxelStealth's shoot) and starts spanning;
// the hand dips as on any item use (ItemInHandRenderer.itemUsed).
func (g *game) fireCrossbow() {
	g.reload = reloadTicks(g.weapon.Which)
	g.handHeight = 0
	if a := g.findAmmo(); a == nil || !rogue.Launches(g.weapon.Which, a) {
		g.msg("You have nothing to shoot.")
		return
	}
	g.shoot(g.takeAmmo(), launchSpeed[g.weapon.Which])
}

// chainLoaded reports whether a chain crossbow with a chamber loaded is
// wielded (CrossbowItem.isCharged; while a chamber loads, the loaded
// ones are put aside and the crossbow counts as empty).
func (g *game) chainLoaded() bool {
	return g.weapon != nil && g.weapon.Which == rogue.ChainCrossbow && len(g.chambers) > 0 && !g.drawing
}

// fireChain is the left button with a loaded chain crossbow (the mod's
// startAttack mixin: the click shoots instead of attacking, and does
// nothing on the cooldown); the next chamber is loaded at once.
func (g *game) fireChain() {
	if !g.chainLoaded() || g.cooldown > 0 {
		return
	}
	g.shoot(g.chambers[0], launchSpeed[rogue.ChainCrossbow])
	g.chambers = g.chambers[1:]
	g.cooldown = chainCooldown
}

// tickHand is ItemInHandRenderer.tick with Player Animation Lib's mixin
// (the hand sinks while a Better Combat attack plays, 0.4 + 0.6 * (1 -
// the first-person transition's progress), instant for the mod), and
// LocalPlayer's sway, xBob and yBob following the view.
func (g *game) tickHand() {
	g.prevHandHeight = g.handHeight
	if g.handItem != nil && g.weapon != nil && g.handItem.Type == g.weapon.Type && g.handItem.Which == g.weapon.Which {
		g.handItem = g.weapon // shouldInstantlyReplaceVisibleItem: the same item
	}
	target := float32(1) // getItemSwapScale is 1 without a swap animation
	if g.handItem != g.weapon {
		target = 0
	}
	g.handHeight += max(-0.4, min(target-g.handHeight, 0.4))
	if g.handHeight < 0.1 {
		g.handItem = g.weapon
	}
	if g.stack.c.isActive() {
		g.handHeight *= 0.4
	}
	g.prevXBob, g.prevYBob = g.xBob, g.yBob
	g.xBob += (g.xRotMC() - g.xBob) * 0.5
	g.yBob += wrapDegrees(g.yRotMC()-g.yBob) * 0.5 // the camera's yaw wraps
}

// tickFov is Camera.tick: the field of view modifier follows the player's
// (a drawn bow zooms in) by half each tick.
func (g *game) tickFov() {
	if g.fovMod == 0 {
		g.fovMod = 1
	}
	g.prevFovMod = g.fovMod
	target := float32(1)
	if g.drawing && g.weapon.Which == rogue.Bow { // only Items.BOW zooms, not the longbow
		target = bowZoom(float32(g.drawTicks), fullDraw(g.weapon.Which))
	}
	if g.sling.frame != 0 && g.sling.state == slingCharge {
		target = slingZoom(g.sling.frame)
	}
	g.fovMod = max(0.1, min(g.fovMod+(target-g.fovMod)*0.5, 1.5))
}

// takeAmmo takes one of the launcher's ammunition out of the pack.
func (g *game) takeAmmo() *rogue.Object {
	a := g.findAmmo()
	o := *a
	o.Count = 1
	g.removeFromPack(a, false)
	return &o
}

// handUse is what the hand is doing with the wielded launcher for
// drawHand, and for how long: Minecraft's timeHeld, which lags the use's
// ticks by one (getUseDuration - (getUseItemRemainingTicks - pt + 1)).
func (g *game) handUse(pt float32) (int, float32) {
	switch {
	case g.weapon == nil:
	case g.weapon.Which == rogue.ChainCrossbow && g.drawing: // loading a chamber
		return useCrossbowCharge, float32(g.drawTicks) - 1 + pt
	case g.weapon.Which == rogue.ChainCrossbow:
		if len(g.chambers) > 0 {
			return useCrossbowHold, 0
		}
	case g.drawing:
		return useBow, float32(g.drawTicks) - 1 + pt
	case isCrossbow(g.weapon.Which) && g.reload > 0:
		return useCrossbowCharge, float32(reloadTicks(g.weapon.Which)-g.reload) - 1 + pt
	case isCrossbow(g.weapon.Which):
		return useCrossbowHold, 0
	}
	return useNone, 0
}

// fovZoom is the view's zoom factor, the smoothed field of view modifier.
func (g *game) fovZoom(alpha float64) float64 {
	if g.fovMod == 0 {
		return 1
	}
	return float64(g.prevFovMod + (g.fovMod-g.prevFovMod)*float32(alpha))
}

// shoot looses an arrow, bolt or rock along the view at speed.
func (g *game) shoot(ammo *rogue.Object, speed float64) *projectile.Arrow {
	ex, ey, ez := g.player.Eye()
	a := g.arrows.Shoot(ex, ey-0.1, ez, g.cam.Yaw, g.cam.Pitch, speed)
	g.arrowObjs[a] = ammo
	return a
}

// stickTicks is how long an arrow stays stuck in a wall or the floor
// before it drops as a pickup (Minecraft's stay a minute; Rogue's land at
// once). ponytail: a second, so the shot can be seen landing.
const stickTicks = 20

// tickArrows flies the arrows; those stuck in the walls or floor drop as
// pickups on the square they came down on.
func (g *game) tickArrows() {
	if len(g.arrows.Arrows) == 0 {
		return
	}
	var targets []projectile.Target
	for _, m := range g.monsters {
		if m.alive() {
			targets = append(targets, arrowTarget{g, m})
		}
	}
	g.arrows.Tick(g.terrain, targets)
	l := g.level()
	for _, a := range g.arrows.Arrows {
		o := g.arrowObjs[a]
		if !a.Stuck || a.In != nil || o == nil || a.Age < stickTicks {
			continue
		}
		delete(g.arrowObjs, a)
		a.Discard()
		c := tileAt(a.PrevX, a.PrevZ)
		if !l.At(c).Walkable() {
			c = g.heroTile()
		}
		g.dropMissile(o, c)
	}
}

// dropMissile puts a missile on the floor, stacking it with its kind.
func (g *game) dropMissile(o *rogue.Object, c dungeon.Coord) {
	for _, p := range g.floor {
		if p.tile == c && rogue.Stacks(p.o, o) {
			p.o.Count++
			return
		}
	}
	g.floor = append(g.floor, &floorObj{o: o, tile: c})
}

// clearArrows forgets every arrow, e.g. on a new level.
func (g *game) clearArrows() {
	g.arrows.Arrows = nil
	clear(g.arrowObjs)
}

func (g *game) drawArrows(alpha float64) {
	g.arrowInsts = g.arrowInsts[:0]
	for _, a := range g.arrows.Arrows {
		if g.arrowObjs[a] == nil {
			continue // landed this tick
		}
		x, y, z, yaw, pitch := a.Lerp(alpha)
		// a fresh shot leaving the eye would fill the screen (vanilla hides
		// such projectiles for two ticks too)
		if dx, dy, dz := x-g.cam.X, y-g.cam.Y, z-g.cam.Z; a.Flown < 2 && dx*dx+dy*dy+dz*dz < 3.5*3.5 {
			continue
		}
		if o := g.arrowObjs[a]; o.Which == rogue.RockW { // ThrownItemRenderer: the rock's picture turned to the camera, as on the ground
			sky, blk := g.light(x, y, z)
			m := mgl32.Translate3D(float32(x-g.cam.X), float32(y-g.cam.Y), float32(z-g.cam.Z)).
				Mul4(rotY(-g.yRotMC())).Mul4(rotX(g.xRotMC())).Mul4(rotY(180)).
				Mul4(mgl32.Translate3D(0, 2.0/16, 0)).Mul4(mgl32.Scale3D(0.5, 0.5, 0.5))
			g.itemRend.DrawInWorld(spriteFor(g.rg, o), &g.cam, m, sky, blk)
			continue
		}
		back := 0.4 * math.Cos(pitch) // light from the middle of the shaft
		sky, blk := g.light(x-math.Sin(yaw)*back, y-0.4*math.Sin(pitch), z+math.Cos(yaw)*back)
		g.arrowInsts = append(g.arrowInsts, render.ModelInstance{Model: g.arrowModel, X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk})
	}
	g.modelRend.Draw(&g.cam, g.arrowInsts)
}

// arrowModel is voxellib's explorer arrow: pointing along +Z with its tip
// at the origin, a shaft, a head and fletching from a 4-pixel texture.
func arrowModel() *model.Model {
	tex := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	tex.Set(0, 0, color.NRGBA{120, 86, 52, 255})   // shaft
	tex.Set(1, 0, color.NRGBA{170, 170, 175, 255}) // head
	tex.Set(2, 0, color.NRGBA{235, 235, 235, 255}) // fletching
	tex.Set(3, 0, color.NRGBA{70, 70, 75, 255})    // nock
	m := &model.Model{Name: "arrow", Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: "arrow", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	var me model.Mesh
	box := func(lo, hi mgl32.Vec3, px int) {
		u := (float32(px) + 0.5) / 4
		for _, f := range [6]struct {
			n mgl32.Vec3
			c [4]mgl32.Vec3
		}{
			{mgl32.Vec3{1, 0, 0}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {hi[0], hi[1], lo[2]}, {hi[0], hi[1], hi[2]}, {hi[0], lo[1], hi[2]}}},
			{mgl32.Vec3{-1, 0, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], hi[1], hi[2]}, {lo[0], hi[1], lo[2]}, {lo[0], lo[1], lo[2]}}},
			{mgl32.Vec3{0, 1, 0}, [4]mgl32.Vec3{{lo[0], hi[1], lo[2]}, {lo[0], hi[1], hi[2]}, {hi[0], hi[1], hi[2]}, {hi[0], hi[1], lo[2]}}},
			{mgl32.Vec3{0, -1, 0}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {lo[0], lo[1], lo[2]}, {hi[0], lo[1], lo[2]}, {hi[0], lo[1], hi[2]}}},
			{mgl32.Vec3{0, 0, 1}, [4]mgl32.Vec3{{lo[0], lo[1], hi[2]}, {hi[0], lo[1], hi[2]}, {hi[0], hi[1], hi[2]}, {lo[0], hi[1], hi[2]}}},
			{mgl32.Vec3{0, 0, -1}, [4]mgl32.Vec3{{hi[0], lo[1], lo[2]}, {lo[0], lo[1], lo[2]}, {lo[0], hi[1], lo[2]}, {hi[0], hi[1], lo[2]}}},
		} {
			base := uint32(len(me.Verts))
			for _, c := range f.c {
				me.Verts = append(me.Verts, model.Vertex{Pos: c, Normal: f.n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
			}
			me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
		}
	}
	const s = 1.0 / 32 // half the shaft's thickness
	box(mgl32.Vec3{-s, -s, -0.75}, mgl32.Vec3{s, s, -0.28}, 0)
	// head: a four-sided pyramid, its point at the origin
	u := (1 + 0.5) / float32(4)
	apex := mgl32.Vec3{0, 0, 0}
	c := [4]mgl32.Vec3{{-2 * s, -2 * s, -0.3}, {2 * s, -2 * s, -0.3}, {2 * s, 2 * s, -0.3}, {-2 * s, 2 * s, -0.3}}
	tri := func(a, b, c mgl32.Vec3) {
		n := b.Sub(a).Cross(c.Sub(a)).Normalize()
		i := uint32(len(me.Verts))
		for _, p := range []mgl32.Vec3{a, b, c} {
			me.Verts = append(me.Verts, model.Vertex{Pos: p, Normal: n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
		}
		me.Indices = append(me.Indices, i, i+1, i+2)
	}
	for k := 0; k < 4; k++ {
		tri(c[k], c[(k+1)%4], apex)
	}
	tri(c[0], c[2], c[1])
	tri(c[0], c[3], c[2])
	// fletching: two crossed vanes turned 45° about the shaft
	first := len(me.Verts)
	box(mgl32.Vec3{-0.002, -4 * s, -0.72}, mgl32.Vec3{0.002, 4 * s, -0.55}, 2)
	box(mgl32.Vec3{-4 * s, -0.002, -0.72}, mgl32.Vec3{4 * s, 0.002, -0.55}, 2)
	turn := mgl32.HomogRotate3DZ(math.Pi / 4)
	for i := first; i < len(me.Verts); i++ {
		v := &me.Verts[i]
		v.Pos = mgl32.TransformCoordinate(v.Pos, turn)
		v.Normal = mgl32.TransformNormal(v.Normal, turn)
	}
	box(mgl32.Vec3{-1.2 * s, -1.2 * s, -0.78}, mgl32.Vec3{1.2 * s, 1.2 * s, -0.74}, 3) // nock
	m.Meshes = []model.Mesh{me}
	return m
}
