package main

// Bows and crossbows as in Minecraft: hold the left button to draw a bow
// and let go to shoot, the arrow the faster the longer it was drawn
// (BowItem). A crossbow is spanned with R (over Minecraft's charge, at a
// crawl) and shoots a spanned bolt on the click; the chain crossbow holds
// three. T throws the wielded weapon itself. The hand draws them as
// ItemInHandRenderer does, with its item swap and view sway
// (handanim.go). Everything flies as a real projectile (voxellib's
// projectile: Minecraft's arc and drag) and sticks in what it hits, a
// wall or a monster, which it rolls fight.c's dice for a thrown missile
// against; sharp ones stick with their tip at the face, sling stones and
// blunt weapons bounce off and come to rest on the floor instead.
// Nothing fired ever despawns or turns back into a floor item: stuck or
// resting ones are picked up by touch (Arrow.playerTouch), and a dead
// monster's fall out of it.

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
// chambers, and the cooldown between shots. R loads one chamber over the
// crossbow's charge; a shot chambers the next at once.
const chainMagazine, chainCooldown = 3, 8

// magazine is how many bolts a crossbow holds spanned.
func magazine(which int) int {
	if which == rogue.ChainCrossbow {
		return chainMagazine
	}
	return 1
}

// throwSpeed and throwGravity are a thrown weapon's, blocks per tick and
// per tick²: Minecraft's ThrowableProjectile (a snowball's), slow enough
// to watch cross a room.
const throwSpeed, throwGravity = 1.5, 0.03

// tumble is how fast a thrown weapon turns end over end, degrees per tick.
const tumble = 40

// crossbowGroup is the bone holding the crossbow in cleannrooster's artmob.
const crossbowGroup = "repeater_crossbow_arrow"

type arrowState struct {
	arrows      projectile.World
	arrowObjs   map[*projectile.Arrow]*rogue.Object // what each arrow is
	arrowModel  int
	stoneModel  int
	com         map[string][2]float32 // each sprite's centre of mass (massCenter)
	arrowInsts  []render.ModelInstance
	drawing     bool            // the right button is drawing a bow
	drawTicks   int             // how long
	fireHeld    bool            // the right button is down on a sling
	spanning    bool            // R: the crossbow is being spanned
	reload      int             // ticks until it is
	chambers    []*rogue.Object // a crossbow's spanned bolts, the next shot first
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
	if g.arrowModel, err = g.modelRend.Add(arrowModel()); err != nil {
		return err
	}
	if g.stoneModel, err = g.modelRend.Add(stoneModel()); err != nil {
		return err
	}
	g.arrows.Keep = true
	g.arrows.Hit = func(a *projectile.Arrow, t projectile.Target) {
		at, ok := t.(arrowTarget)
		o := g.arrowObjs[a]
		if !ok || o == nil {
			return // stuck in another arrow, Robin Hood style
		}
		g.fight(at.m, o, true)
	}
	return err
}

// loseMonster lets the missiles stuck in a monster whose body is gone
// fall out: they land on whatever is below.
func (g *game) loseMonster(m *monster) { g.arrows.Release(arrowTarget{g, m}) }

// throwItem (T) hurls the wielded weapon along the view: one of a bundle,
// or the weapon itself, leaving the hands bare (a spanned crossbow takes
// its bolts with it).
func (g *game) throwItem() {
	w := g.weapon
	if w == nil {
		g.msg("You have nothing to throw.")
		return
	}
	g.attackCD, g.swing = 12, swingTicks
	one := *w
	one.Count = 1
	g.removeFromPack(w, false)
	if g.weapon == nil {
		g.chambers, g.spanning, g.drawing = nil, false, false
	}
	g.shoot(&one, throwSpeed).Gravity = throwGravity
}

// startReload (R) spans a crossbow over its charge; the bolt goes in at
// the end (tickDraw).
func (g *game) startReload() {
	if !isCrossbow(g.weapon.Which) || g.spanning {
		return
	}
	if len(g.chambers) >= magazine(g.weapon.Which) {
		g.msg("The crossbow is loaded.")
		return
	}
	if a := g.findAmmo(); a == nil || !rogue.Launches(g.weapon.Which, a) {
		g.msg("You have nothing to load.")
		return
	}
	g.spanning, g.reload, g.handHeight = true, reloadTicks(g.weapon.Which), 0
}

// launcher reports whether a bow or crossbow is wielded.
func (g *game) launcher() bool { return g.weapon != nil && launchSpeed[g.weapon.Which] > 0 }

// startDraw is the left button going down with a launcher in hand.
func (g *game) startDraw() {
	switch g.weapon.Which {
	case rogue.Crossbow, rogue.ChainCrossbow:
		g.fireCrossbow()
		return
	case rogue.Sling:
		g.fireHeld = true
		g.startSling()
		return
	}
	if a := g.findAmmo(); a == nil || !rogue.Launches(g.weapon.Which, a) {
		g.msg("You have nothing to shoot.")
		return
	}
	g.drawing, g.drawTicks = true, 0
	g.handHeight = 0 // the use starts (startUseItem: itemUsed)
}

// releaseDraw is the left button coming up: a drawn bow shoots (if
// drawn a tenth or more), a sling lets fly.
func (g *game) releaseDraw() {
	g.fireHeld = false
	if !g.drawing {
		return
	}
	g.drawing = false
	if pw := float64(drawPower(float32(g.drawTicks), fullDraw(g.weapon.Which))); pw >= 0.1 {
		g.shoot(g.takeAmmo(), pw*launchSpeed[g.weapon.Which])
	}
}

// tickDraw draws the bow on, and spans the crossbow.
func (g *game) tickDraw() {
	defer g.tickFov()
	if g.reload > 0 {
		g.reload--
	}
	if g.cooldown > 0 {
		g.cooldown--
	}
	if !g.launcher() || g.dead || g.showInv || g.noCommand > 0 {
		g.drawing, g.fireHeld, g.spanning, g.reload, g.sling = false, false, false, 0, slingAction{}
		return
	}
	if g.spanning && g.reload == 0 { // spanned: the bolt goes in
		g.spanning = false
		if a := g.findAmmo(); a != nil && rogue.Launches(g.weapon.Which, a) {
			g.chambers = append(g.chambers, g.takeAmmo())
			g.msg(fmt.Sprintf("Loaded: %d/%d.", len(g.chambers), magazine(g.weapon.Which)))
		}
	}
	g.tickSling()
	if g.drawing {
		g.drawTicks++
	}
}

// fireCrossbow is the left button with a crossbow: a spanned bolt
// flies (nothing on the chain crossbow's cooldown, or while spanning), the
// next chamber slides in at once, and the hand dips as on any item use
// (ItemInHandRenderer.itemUsed).
func (g *game) fireCrossbow() {
	if g.cooldown > 0 || g.spanning {
		return
	}
	if len(g.chambers) == 0 {
		g.msg("The crossbow is not spanned.")
		return
	}
	g.shoot(g.chambers[0], launchSpeed[g.weapon.Which])
	g.chambers = g.chambers[1:]
	g.cooldown, g.handHeight = chainCooldown, 0
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
	case g.drawing:
		return useBow, float32(g.drawTicks) - 1 + pt
	case isCrossbow(g.weapon.Which) && g.spanning:
		return useCrossbowCharge, float32(reloadTicks(g.weapon.Which)-g.reload) - 1 + pt
	case isCrossbow(g.weapon.Which) && len(g.chambers) > 0:
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
	if blunt[ammo.Which] {
		a.Bounce = rockBounce
	}
	g.arrowObjs[a] = ammo
	return a
}

// tickArrows flies the arrows; one stuck in a wall or the floor is picked
// up when the hero touches it (Arrow.playerTouch: the hero's box meets
// its tip).
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
	p := g.player
	for _, a := range g.arrows.Arrows {
		o := g.arrowObjs[a]
		if !a.Stuck || a.In != nil || o == nil || o.Count == 0 {
			continue
		}
		if math.Abs(a.X-p.X) > 0.8 || math.Abs(a.Z-p.Z) > 0.8 || a.Y < p.Y-0.3 || a.Y > p.Y+2 {
			continue
		}
		delete(g.arrowObjs, a)
		a.Discard()
		if !g.addPack(o) {
			g.dropMissile(o, g.heroTile())
		}
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
	g.arrowInsts, g.itemPoses = g.arrowInsts[:0], g.itemPoses[:0]
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
		switch o := g.arrowObjs[a]; o.Which {
		case rogue.RockW: // a solid little cube, sitting flat once at rest
			if a.Stuck {
				y, pitch = y+stoneSize/2, 0
			}
			sky, blk := g.light(x, y, z)
			g.arrowInsts = append(g.arrowInsts, render.ModelInstance{Model: g.stoneModel, X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk})
			continue
		case rogue.Arrow, rogue.Bolt, rogue.Dart:
		default: // a thrown weapon in its extruded form, its flat sideways
			const s = 0.6
			sprite := spriteFor(g.rg, o)
			t := mgl32.DegToRad((float32(a.Flown) + float32(alpha)) * tumble)
			lx, ly, lz := x, y, z // where its light is sampled: outside the block a stuck one's tip is in
			if a.Stuck && a.Bounce == 0 {
				y += s / 2
				back := projectile.Penetration + s/2
				lx, ly, lz = x-math.Sin(yaw)*math.Cos(pitch)*back, y-math.Sin(pitch)*back, z+math.Cos(yaw)*math.Cos(pitch)*back
			}
			m := mgl32.Translate3D(float32(x-g.cam.X), float32(y-g.cam.Y), float32(z-g.cam.Z))
			along := mgl32.HomogRotate3DY(float32(math.Pi/2 - yaw)).Mul4(mgl32.HomogRotate3DZ(float32(pitch))) // local +X along the flight
			tipFirst := mgl32.Translate3D(-s*math.Sqrt2/2, 0, 0).Mul4(mgl32.HomogRotate3DZ(-math.Pi / 4))      // the sprites' blades point to the top right: turned onto +X, the tip at the origin
			switch {
			case a.Stuck && a.Bounce > 0: // blunt, at rest on the floor: a dropped item
				g.dropPoses(o, x, y, z, g.tick, alpha)
				continue
			case a.Stuck: // sharp, stuck: the tip at the face it struck
				m = m.Mul4(along).Mul4(mgl32.Translate3D(-projectile.Penetration, 0, 0)).Mul4(tipFirst)
			case weaponFeel[o.Which].reach >= longReach: // long: thrown like a spear, tip first and rolling about its length
				m = m.Mul4(along).Mul4(mgl32.HomogRotate3DX(t / 4)).Mul4(tipFirst)
			default: // short: a throwing knife, end over end in the vertical plane of its flight about its centre of mass, the tip leading at first
				c := g.com[sprite]
				m = m.Mul4(along).Mul4(mgl32.HomogRotate3DZ(-math.Pi/4 - t)).Mul4(mgl32.Translate3D(-s*c[0], -s*c[1], 0))
			}
			sky, blk := g.light(lx, ly, lz)
			g.itemRend.DrawInWorld(sprite, &g.cam, m.Mul4(mgl32.Scale3D(s, s, s)), sky, blk)
			continue
		}
		back := 0.4 * math.Cos(pitch) // light from the middle of the shaft
		sky, blk := g.light(x-math.Sin(yaw)*back, y-0.4*math.Sin(pitch), z+math.Cos(yaw)*back)
		g.arrowInsts = append(g.arrowInsts, render.ModelInstance{Model: g.arrowModel, X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk})
	}
	g.modelRend.Draw(&g.cam, g.arrowInsts)
	g.itemRend.DrawPosed(&g.cam, g.itemPoses)
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
	box := func(lo, hi mgl32.Vec3, px int) { addBox(&me, lo, hi, (float32(px)+0.5)/4) }
	// head: a four-sided pyramid, its point at the origin
	u := (1 + 0.5) / float32(4)
	apex := mgl32.Vec3{0, 0, 0}
	tri := func(a, b, c mgl32.Vec3) {
		n := b.Sub(a).Cross(c.Sub(a)).Normalize()
		i := uint32(len(me.Verts))
		for _, p := range []mgl32.Vec3{a, b, c} {
			me.Verts = append(me.Verts, model.Vertex{Pos: p, Normal: n, UV: mgl32.Vec2{u, 0.5}, Weights: [4]float32{1}})
		}
		me.Indices = append(me.Indices, i, i+1, i+2)
	}
	const s = 1.0 / 32 // half the shaft's thickness
	box(mgl32.Vec3{-s, -s, -0.75}, mgl32.Vec3{s, s, -0.28}, 0)
	c := [4]mgl32.Vec3{{-2 * s, -2 * s, -0.3}, {2 * s, -2 * s, -0.3}, {2 * s, 2 * s, -0.3}, {-2 * s, 2 * s, -0.3}}
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

// blunt weapons bounce off what they hit; the rest stick like arrows.
var blunt = map[int]bool{rogue.Mace: true, rogue.Bow: true, rogue.RockW: true, rogue.Sling: true, rogue.Crossbow: true,
	rogue.GreatBow: true, rogue.WoodenPole: true, rogue.Maul: true, rogue.HeavyMace: true, rogue.Battlestaff: true,
	rogue.Sceptre: true, rogue.Anchor: true, rogue.Gauntlet: true, rogue.ChainCrossbow: true}

// massCenter is the centroid of a sprite's opaque pixels in the item
// slab's units (X, Y in [-0.5, 0.5], the picture's left-bottom at
// -0.5, -0.5): what a thrown weapon spins about.
func massCenter(img image.Image) [2]float32 {
	b := img.Bounds()
	var sx, sy, n float64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
				sx, sy, n = sx+float64(x-b.Min.X)+0.5, sy+float64(y-b.Min.Y)+0.5, n+1
			}
		}
	}
	if n == 0 {
		return [2]float32{}
	}
	return [2]float32{float32(sx/n/float64(b.Dx()) - 0.5), float32(0.5 - sy/n/float64(b.Dy()))}
}

// stoneSize is a sling stone's cube, in blocks.
const stoneSize = 0.25

// stoneModel is a sling stone: a solid little cube in the rock sprite's
// average colour.
func stoneModel() *model.Model {
	tex := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	tex.Set(0, 0, color.NRGBA{141, 134, 152, 255})
	m := &model.Model{Name: "stone", Textures: []image.Image{tex}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: "stone", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	var me model.Mesh
	const r = stoneSize / 2
	addBox(&me, mgl32.Vec3{-r, -r, -r}, mgl32.Vec3{r, r, r}, 0.5)
	m.Meshes = []model.Mesh{me}
	return m
}

// addBox appends a box's six faces to the mesh, all with the texture
// column u.
func addBox(me *model.Mesh, lo, hi mgl32.Vec3, u float32) {
	{
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
}
