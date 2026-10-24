package main

// Better Combat 3.2.2's attack animations, played as the mod plays them
// through Player Animation Lib (palib.go: speed gears, mirror, the
// first-person pitch adjustment) over Minecraft 26.2's vanilla player pose
// (HumanoidModel, WalkAnimationState, LivingEntity's body yaw) and drawn
// through its first-person render chain (LivingEntityRenderer with the
// lib's mixins, ItemInHandLayer, ItemTransform). bettercombat_test.go
// checks each of those against Minecraft's own classes
// (testdata/vanilla.json).

import (
	"math"
	"path/filepath"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelrogue/rogue"
)

// bcAttack is a preset's attack: an animation and its upswing.
type bcAttack struct {
	anim    string
	upswing float32
}

// thirdPerson holds the thirdperson_righthand display transforms, by the
// drawn sprite; the rest are item/handheld's.
var thirdPerson = map[string]display{
	"w:scythe_hand":     {mgl32.Vec3{5, 270, -35}, mgl32.Vec3{0, 8, 2}, mgl32.Vec3{1.7, 1.7, 0.85}},
	"w:halberd_hand":    {mgl32.Vec3{5, 270, -40}, mgl32.Vec3{0, 2, 2}, mgl32.Vec3{1.7, 1.7, 0.85}},
	"w:glaive_hand":     {mgl32.Vec3{5, 270, -40}, mgl32.Vec3{0, 2, 2}, mgl32.Vec3{1.7, 1.7, 0.85}},
	"w:greatsword_hand": {mgl32.Vec3{0, -90, 55}, mgl32.Vec3{0, 6, 0.5}, mgl32.Vec3{1.25, 1.25, 1.25}},
	"w:katana":          {mgl32.Vec3{0, -90, 55}, mgl32.Vec3{0, 8.5, 0}, mgl32.Vec3{1.275, 1.275, 0.85}},
	"w:scimitar":        {mgl32.Vec3{0, -90, 55}, mgl32.Vec3{0, 8.5, 0}, mgl32.Vec3{1.275, 1.275, 0.85}},
	"w:war_spear":       {mgl32.Vec3{0, -90, 55}, mgl32.Vec3{0, 4, 0.5}, mgl32.Vec3{1.7, 1.7, 0.85}},
	"w:war_glaive":      {mgl32.Vec3{0, -90, 55}, mgl32.Vec3{0, 4, 0.5}, mgl32.Vec3{1.7, 1.7, 0.85}},
}

var handheld3 = display{mgl32.Vec3{0, -90, 55}, mgl32.Vec3{0, 4, 0.5}, mgl32.Vec3{0.85, 0.85, 0.85}}

type combatState struct {
	anims map[string]*animation
	stack *attackStack

	attack      *animation // playing, or nil
	attackLen   float32    // its length, the weapon's cooldown in ticks
	combo       int
	comboOf     *rogue.Object
	comboPat    int // the pattern the combo is in
	sinceAttack int // ticks since the last attack started (Better Combat's lastAttacked)
	hitIn       int // ticks until the pending hit lands (0 = none)
	hitReach    float64

	// the vanilla player state the animation plays over
	bodyYaw, prevBodyYaw float32 // LivingEntity.yBodyRot and yBodyRotO, Minecraft degrees
	walk                 walkState
}

// loadAnimations reads the emotes in dir.
func (g *game) loadAnimations(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	g.anims = map[string]*animation{}
	for _, f := range files {
		a, err := loadPlayerAnim(f)
		if err != nil {
			return err
		}
		g.anims[a.name] = a
	}
	g.stack = newAttackStack()
	return nil
}

// --- Better Combat's attack animation stack ---

// attackStack is AttackAnimationSubStack: the controller behind the speed,
// mirror and adjustment modifiers.
type attackStack struct {
	c      *controller
	speed  *speedMod
	mirror *mirrorMod
	pitch  float32 // the player's xRot in degrees, down positive
}

func newAttackStack() *attackStack {
	s := &attackStack{speed: &speedMod{speed: 1}, mirror: &mirrorMod{}}
	s.c = newController(s.speed, s.mirror, &adjustMod{source: s.adjustment})
	s.c.postSetup = func(b map[string]*advBone) {
		b["torso"].setEnabled(true)
		b["head"].enabled[axRX] = false
	}
	return s
}

// adjustment is AttackAnimationStack.createAttackAdjustment: the body
// follows the look pitch (and lifts a little looking up) in first person;
// body, arms and legs share it in third.
func (s *attackStack) adjustment(bone string, data *animData) (partMod, bool) {
	pitch := d2r(s.pitch)
	var m partMod
	if data.firstPerson {
		if bone != "body" {
			return m, false
		}
		m.rot[0] += pitch
		if pitch < 0 {
			off := math.Abs(math.Sin(float64(pitch)))
			m.off[1] = float32(off * 0.5)
			m.off[2] = -float32(off)
		}
		return m, true
	}
	switch bone {
	case "body":
		m.rot[0] += pitch * 0.75
	case "right_arm", "left_arm":
		m.rot[0] += pitch * 0.25
	case "right_leg", "left_leg":
		m.rot[0] = -float32(float64(pitch) * 0.75)
	default:
		return m, false
	}
	return m, true
}

// play is AbstractClientPlayerEntityMixin.playAttackAnimation: the
// animation's endTick spread over length ticks, the first upswing share
// faster (upswing multiplier 0.5) and the rest slower, then the end at
// the plain speed.
func (s *attackStack) play(a *animation, offHand bool, length, upswing float32) {
	endTick := a.length
	if a.end != nil {
		endTick = *a.end
	}
	speed := endTick / length
	upswingSpeed := speed / (upswing / 0.5)
	downwind := float32(float64(speed) * float64(1-upswing)) // lerp(0, 1-upswing, ...)
	s.speed.speed = speed
	s.mirror.enabled = offHand
	s.speed.set(upswingSpeed, []gear{{float32(length * upswing), downwind}, {length, speed}})
	s.c.trigger(a)
}

// playRaw plays a at its own speed, as Better Combat's previews do.
func (s *attackStack) playRaw(a *animation) {
	s.speed.speed = 1
	s.mirror.enabled = false
	s.speed.set(1, nil)
	s.c.trigger(a)
}

// handle is AvatarAnimManager.handleAnimations: the stack's tick at the end
// of the player's tick and its partial-tick setup before each frame.
func (s *attackStack) handle(pt float32, fullTick, firstPerson bool) {
	if fullTick {
		d := animData{pt: pt, firstPerson: firstPerson}
		s.c.tickLayers(&d)
	}
	if s.c.isActive() {
		d := animData{pt: pt, firstPerson: firstPerson}
		s.c.setupAnim(&d)
	}
}

// transform writes the animation over the vanilla bone b.
func (s *attackStack) transform(b *animBone) { s.c.transform(b) }

// --- The attack itself ---

// longReach is the reach from which a weapon leads with its stab.
const longReach = 3.0

// startAttack plays the next attack of the combo of the weapon's attack
// pattern (patterns) over length ticks, landing its hit at the upswing
// on a monster within reach; false if the weapon has no such pattern.
func (g *game) startAttack(length int, reach float64, pattern int) bool {
	p := presetFor(g.weapon)
	if p == nil { // no Better Combat attacks: Minecraft's swing, hitting at once
		if pattern > 0 {
			return false
		}
		g.swing, g.hitIn, g.hitReach = swingTicks, 1, reach
		return true
	}
	pats := patterns(p, reach >= longReach)
	if pattern >= len(pats) {
		return false
	}
	pat := pats[pattern]
	comboReset := int(math.Round(float64(length) * 3)) // combo_reset_rate
	if g.comboOf != g.weapon || g.comboPat != pattern || g.sinceAttack > comboReset {
		g.combo = 0
	}
	g.comboOf, g.comboPat = g.weapon, pattern
	at := pat[g.combo%len(pat)]
	g.combo++
	g.attack, g.attackLen, g.sinceAttack = g.anims[at.anim], float32(length), 0
	g.hitIn, g.hitReach = max(1, int(math.Round(float64(g.attackLen*at.upswing)))), reach
	g.stack.play(g.attack, false, g.attackLen, at.upswing)
	return true
}

// tickCombat is the end of the hero's tick: the walk animation and body
// yaw of LivingEntity.tick, the animation stack's tick (its TAIL), Better
// Combat's extra turn of the body toward the head while attacking
// (Player.tick TAIL), the pending hit and the combo clock.
func (g *game) tickCombat() {
	p := g.player
	dx, dz := p.X-p.PrevX, p.Z-p.PrevZ
	g.walk.update(min(float32(math.Sqrt(float64(dx*dx)+float64(dz*dz)))*4, 1), 0.4, 1)
	g.stack.handle(0, true, true)
	g.tickBodyYaw(g.yRotMC(), dx, dz, g.stack.c.isActive())
	if !g.stack.c.isActive() {
		g.attack = nil
	}
	if g.hitIn > 0 {
		if g.hitIn--; g.hitIn == 0 && !g.dead {
			if m := g.monsterInFront(g.hitReach); m != nil {
				g.fight(m, g.weapon, false)
			}
		}
	}
	g.sinceAttack++
}

// yRotMC is the hero's yaw as Minecraft's yRot (degrees, 0 = south, +Z).
func (g *game) yRotMC() float32 { return float32(g.cam.Yaw*(180/math.Pi)) + 180 }

// xRotMC is the hero's pitch as Minecraft's xRot (degrees, down positive).
func (g *game) xRotMC() float32 { return float32(-g.cam.Pitch * (180 / math.Pi)) }

// --- Vanilla: Mth, WalkAnimationState, HumanoidModel, LivingEntity ---

var mthSinTab = func() (t [65536]float32) {
	for i := range t {
		t[i] = float32(math.Sin(float64(i) / 10430.378350470453))
	}
	return
}()

func mthSin(v float32) float32 { return mthSinTab[int64(float64(v)*10430.378350470453)&65535] }
func mthCos(v float32) float32 {
	return mthSinTab[int64(float64(v)*10430.378350470453+16384.0)&65535]
}

// wrapDegrees is Mth.wrapDegrees for a float.
func wrapDegrees(v float32) float32 {
	v = float32(math.Mod(float64(v), 360))
	if v >= 180 {
		v -= 360
	}
	if v < -180 {
		v += 360
	}
	return v
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }

var asinTab, cosTab = func() (asin, cos [257]float64) {
	for i := range asin {
		asin[i] = math.Asin(float64(i) / 256)
		cos[i] = math.Cos(asin[i])
	}
	return
}()

// mthAtan2 is Mth.atan2, Minecraft's table approximation.
func mthAtan2(y, x float64) float64 {
	fracBias := math.Float64frombits(4805340802404319232)
	d2 := float64(x*x) + float64(y*y)
	if math.IsNaN(d2) {
		return math.NaN()
	}
	negY := y < 0
	if negY {
		y = -y
	}
	negX := x < 0
	if negX {
		x = -x
	}
	steep := y > x
	if steep {
		x, y = y, x
	}
	rinv := fastInvSqrt(d2)
	x *= rinv
	y *= rinv
	yp := fracBias + y
	idx := int(int32(math.Float64bits(yp)))
	phi, cPhi := asinTab[idx], cosTab[idx]
	sPhi := yp - fracBias
	sd := float64(y*cPhi) - float64(x*sPhi)
	d := (6 + float64(sd*sd)) * sd * 0.16666666666666666
	theta := phi + d
	if steep {
		theta = math.Pi/2 - theta
	}
	if negX {
		theta = math.Pi - theta
	}
	if negY {
		theta = -theta
	}
	return theta
}

// fastInvSqrt is Mth.fastInvSqrt.
func fastInvSqrt(x float64) float64 {
	xhalf := 0.5 * x
	i := int64(math.Float64bits(x))
	i = 6910469410427058090 - (i >> 1)
	x = math.Float64frombits(uint64(i))
	return x * (1.5 - float64(xhalf*x*x))
}

// walkState is WalkAnimationState.
type walkState struct{ speedOld, speed, position float32 }

func (w *walkState) update(target, factor, scale float32) {
	w.speedOld = w.speed
	w.speed += float32((target - w.speed) * factor)
	w.position += float32(w.speed * scale)
}

func (w *walkState) positionAt(pt float32) float32 { return w.position - float32(w.speed*(1-pt)) }
func (w *walkState) speedAt(pt float32) float32 {
	return min(w.speedOld+float32(pt*(w.speed-w.speedOld)), 1)
}

// part is a ModelPart's pose: x, y, z, xRot, yRot, zRot.
type part [6]float32

// The humanoid parts' initial poses.
var (
	initHead     = part{}
	initBody     = part{}
	initRightArm = part{-5, 2, 0}
	initLeftArm  = part{5, 2, 0}
	initRightLeg = part{-1.9, 12, 0}
	initLeftLeg  = part{1.9, 12, 0}
)

// vanillaPose is HumanoidModel.setupAnim for the player standing with an
// item in the right hand and nothing in the left, not attacking (Better
// Combat zeroes the vanilla attack time). xRot, yRot are the head's in
// degrees.
func vanillaPose(walkPos, walkSpeed, age, xRot, yRot float32) (head, body, rightArm, leftArm, rightLeg, leftLeg part) {
	head[3] = xRot * float32(math.Pi/180)
	head[4] = yRot * float32(math.Pi/180)
	rightArm, leftArm, rightLeg, leftLeg = initRightArm, initLeftArm, initRightLeg, initLeftLeg
	wp := float32(walkPos * 0.6662)
	rightArm[3] = mthCos(wp+piF) * 2 * walkSpeed * 0.5 / 1
	leftArm[3] = mthCos(wp) * 2 * walkSpeed * 0.5 / 1
	rightLeg[3] = mthCos(wp) * 1.4 * walkSpeed / 1
	leftLeg[3] = mthCos(wp+piF) * 1.4 * walkSpeed / 1
	rightLeg[4], leftLeg[4], rightLeg[5], leftLeg[5] = 0.005, -0.005, 0.005, -0.005
	// poseRightArm ITEM (poseLeftArm EMPTY does nothing)
	rightArm[3] = float32(rightArm[3]*0.5) - float32(math.Pi/10)
	rightArm[4] = 0
	// AnimationUtils.bobModelPart
	rightArm[5] += float32(mthCos(age*0.09)*0.05) + 0.05
	rightArm[3] += float32(mthSin(age*0.067) * 0.05)
	leftArm[5] += -(float32(mthCos(age*0.09)*0.05) + 0.05)
	leftArm[3] += -float32(mthSin(age*0.067) * 0.05)
	return
}

// vanillaBone is RenderUtil.copyVanillaPart: a part's pose as the bone the
// animation writes over (its position relative to the initial pose, y up).
func vanillaBone(name string, p, init part) *animBone {
	b := newAnimBone(name)
	b.v[axPX], b.v[axPY], b.v[axPZ] = p[0]-init[0], -(p[1] - init[1]), p[2]-init[2]
	b.v[axRX], b.v[axRY], b.v[axRZ] = p[3], p[4], p[5]
	return b
}

// bonePart is RenderUtil.translatePartToBone: the bone back as a part's pose.
func bonePart(b *animBone, init part) part {
	return part{b.v[axPX] + init[0], -b.v[axPY] + init[1], b.v[axPZ] + init[2], b.v[axRX], b.v[axRY] + init[4], b.v[axRZ]}
}

// tickBodyYaw is LivingEntity.tick's turning of the body after the head
// (yRot) and the tick's movement (xd, zd), then Better Combat's extra
// turn toward the head while an attack plays.
func (g *game) tickBodyYaw(yRot float32, xd, zd float64, attacking bool) {
	g.prevBodyYaw = g.bodyYaw
	sideDist := float32(float64(xd*xd) + float64(zd*zd))
	t := g.bodyYaw
	if sideDist > 0.0025000002 {
		walkDir := float32(float32(mthAtan2(zd, xd))*(180/piF)) - 90
		diff := abs32(wrapDegrees(yRot) - walkDir)
		if 95 < diff && diff < 265 {
			t = walkDir - 180
		} else {
			t = walkDir
		}
	}
	g.bodyYaw = tickHeadTurn(g.bodyYaw, yRot, t)
	for g.bodyYaw-g.prevBodyYaw < -180 {
		g.prevBodyYaw -= 360
	}
	for g.bodyYaw-g.prevBodyYaw >= 180 {
		g.prevBodyYaw += 360
	}
	if attacking {
		g.bodyYaw = tickHeadTurn(g.bodyYaw, yRot, yRot)
	}
}

// tickHeadTurn is LivingEntity.tickHeadTurn: the body turns 30% toward t
// and never more than 50° from the head.
func tickHeadTurn(bodyYaw, yRot, t float32) float32 {
	d := wrapDegrees(t - bodyYaw)
	bodyYaw += float32(d * 0.3)
	headDiff := wrapDegrees(yRot - bodyYaw)
	if abs32(headDiff) > 50 {
		sign := float32(0)
		if headDiff != 0 {
			sign = float32(math.Copysign(1, float64(headDiff)))
		}
		bodyYaw += headDiff - float32(sign*50)
	}
	return bodyYaw
}

// bodyRotAt is the body yaw drawn at partial tick pt (Mth.rotLerp).
func (g *game) bodyRotAt(pt float32) float32 {
	return g.prevBodyYaw + float32(pt*wrapDegrees(g.bodyYaw-g.prevBodyYaw))
}

// --- The render chain ---

const eyeHeight = 1.62 // the player's, standing

// rotateZYX is JOML's Matrix4f.rotateZYX: m * Rz * Ry * Rx.
func rotateZYX(m mgl32.Mat4, z, y, x float32) mgl32.Mat4 {
	return m.Mul4(mgl32.HomogRotate3DZ(z)).Mul4(mgl32.HomogRotate3DY(y)).Mul4(mgl32.HomogRotate3DX(x))
}

// partMat is ModelPart.translateAndRotate (unscaled).
func partMat(p part) mgl32.Mat4 {
	return rotateZYX(mgl32.Translate3D(p[0]/16, p[1]/16, p[2]/16), p[5], p[4], p[3])
}

// bcChain is the first-person render chain from the camera (at the eye,
// world orientation) to the model's root, the right arm and the weapon's
// model space (before the item model's own -0.5 centring):
// EntityRenderDispatcher, LivingEntityRenderer.submit with Player
// Animation Lib's doTranslations mixin, AvatarRenderer.scale,
// translateToHand with the lib's PlayerModelMixin, ItemInHandLayer with
// its ItemInHandLayerMixin and the model's thirdperson_righthand
// ItemTransform. body, arm and item are the animated bones, bodyRot the
// body yaw in degrees, disp the display's rotation (degrees), translation
// (sixteenths) and scale. headYaw (the head's yaw from the body) and
// xRot (the pitch), in degrees, turn the body about the eye with the
// camera: the lib's firstPersonFollowsCamera, which Better Combat leaves
// off, so that the weapon stays on screen looking up or down.
func bcChain(body, arm, item [axes]float32, bodyRot, headYaw, xRot float32, disp display) (root, armM, itemM mgl32.Mat4) {
	m := mgl32.Translate3D(0, -eyeHeight, 0)
	m = m.Mul4(mgl32.HomogRotate3DY((180 - bodyRot) * (piF / 180)))
	m = m.Mul4(mgl32.Scale3D(-1, -1, 1))
	// doTranslations (first person pass)
	m = m.Mul4(mgl32.Scale3D(-1, -1, 1)).Mul4(mgl32.Translate3D(0, eyeHeight, 0))
	m = m.Mul4(mgl32.HomogRotate3DY(-headYaw * (piF / 180))).Mul4(mgl32.HomogRotate3DX(-xRot * (piF / 180)))
	m = m.Mul4(mgl32.Translate3D(0, -eyeHeight, 0))
	m = m.Mul4(mgl32.Translate3D(-body[axPX]/16, body[axPY]/16+0.75, body[axPZ]/16))
	m = rotateZYX(m, body[axRZ], -body[axRY], -body[axRX])
	m = m.Mul4(mgl32.Scale3D(body[axSX], body[axSY], body[axSZ])).Mul4(mgl32.Translate3D(0, -0.75, 0)).Mul4(mgl32.Scale3D(-1, -1, 1))
	m = m.Mul4(mgl32.Scale3D(0.9375, 0.9375, 0.9375)).Mul4(mgl32.Translate3D(0, -1.501, 0))
	root = m
	// translateToHand: the right arm part posed by its bone
	p := bonePart(&animBone{v: arm}, initRightArm)
	m = m.Mul4(mgl32.Translate3D(p[0]/16, p[1]/16, p[2]/16))
	if p[3] != 0 || p[4] != 0 || p[5] != 0 {
		m = rotateZYX(m, p[5], p[4], p[3])
	}
	m = m.Mul4(mgl32.Translate3D(0, (arm[axSY]-1)*0.609375, (arm[axSZ]-1)*0.0625))
	armM = m
	// ItemInHandLayer with the item bone's location, rotation and scale
	m = m.Mul4(mgl32.Translate3D(item[axPX]/16, -item[axPY]/16, item[axPZ]/16))
	m = m.Mul4(mgl32.HomogRotate3DX(-90 * (piF / 180))).Mul4(mgl32.HomogRotate3DY(180 * (piF / 180)))
	m = m.Mul4(mgl32.Translate3D(1.0/16, 2.0/16, -10.0/16))
	if item[axRZ] != 0 {
		m = m.Mul4(mgl32.HomogRotate3DZ(-item[axRY]))
	}
	if item[axRY] != 0 {
		m = m.Mul4(mgl32.HomogRotate3DY(-item[axRZ]))
	}
	if item[axRX] != 0 {
		m = m.Mul4(mgl32.HomogRotate3DX(-item[axRX]))
	}
	m = m.Mul4(mgl32.Scale3D(item[axSX], item[axSY], item[axSZ]))
	// ItemTransform.apply, right hand: translation in sixteenths clamped to ±5
	tr := disp.trans.Mul(0.0625)
	for i := range tr {
		tr[i] = min(max(tr[i], -5), 5)
	}
	m = m.Mul4(mgl32.Translate3D(tr[0], tr[1], tr[2]))
	m = m.Mul4(mgl32.HomogRotate3DX(disp.rot[0] * (piF / 180))).Mul4(mgl32.HomogRotate3DY(disp.rot[1] * (piF / 180))).Mul4(mgl32.HomogRotate3DZ(disp.rot[2] * (piF / 180)))
	itemM = m.Mul4(mgl32.Scale3D(disp.scale[0], disp.scale[1], disp.scale[2]))
	return
}

// drawAttack draws the weapon (sprite) posed by the attack animation at
// partial tick pt and reports whether it did. With -tp the whole player
// model is drawn, seen from in front.
func (g *game) drawAttack(sprite string, pt float32, sky, blk uint8) bool {
	switch {
	case g.dead:
		return false
	case g.animName != "":
		if !g.debugAnim() {
			return false
		}
		pt = g.animAt - float32(math.Floor(float64(g.animAt)))
	case !g.stack.c.isActive():
		return false
	}
	fp := !g.tp
	// ponytail: first person follows the camera instead of Better Combat's
	// tilt of the body at the hips, which would add to it
	var follow float32
	g.stack.pitch, follow = 0, g.xRotMC()
	if g.tp {
		g.stack.pitch, follow = follow, 0
	}
	g.stack.handle(pt, false, fp)
	bodyRot := g.bodyRotAt(pt)
	head, torso, rArm, lArm, rLeg, lLeg := vanillaPose(g.walk.positionAt(pt), g.walk.speedAt(pt), float32(g.tick)+pt, g.stack.pitch, wrapDegrees(g.yRotMC()-bodyRot))
	posed := func(name string, p, init part) *animBone {
		b := vanillaBone(name, p, init)
		g.stack.transform(b)
		return b
	}
	body, item := newAnimBone("body"), newAnimBone("right_item")
	g.stack.transform(body)
	g.stack.transform(item)
	arm := posed("right_arm", rArm, initRightArm)
	d, ok := thirdPerson[sprite]
	if !ok {
		d = handheld3
	}
	root, _, itemM := bcChain(body.v, arm.v, item.v, bodyRot, wrapDegrees(g.yRotMC()-bodyRot), follow, d)
	model := itemM.Mul4(spriteTurn(sprite))
	if !g.tp {
		// the lib's first-person pass: the player model in the world with
		// only the item layer drawn (Better Combat's FirstPersonConfiguration
		// shows the items but not the arms: isShowingArmsInFirstPerson
		// defaults to false)
		g.itemRend.DrawInWorld(sprite, &g.cam, model, sky, blk)
		return true
	}
	view := tpCamera().Mul4(mgl32.HomogRotate3DY(float32(g.cam.Yaw)))
	render.BeginHands()
	defer render.EndHands()
	parts := [6]part{
		bonePart(posed("head", head, initHead), initHead),
		bonePart(posed("torso", torso, initBody), initBody),
		bonePart(arm, initRightArm),
		bonePart(posed("left_arm", lArm, initLeftArm), initLeftArm),
		bonePart(posed("right_leg", rLeg, initRightLeg), initRightLeg),
		bonePart(posed("left_leg", lLeg, initLeftLeg), initLeftLeg),
	}
	g.drawTPBody(view.Mul4(root), parts, float32(g.cam.Aspect))
	g.itemRend.DrawHandModel(sprite, float32(g.cam.Aspect), view.Mul4(model), 15, 15)
	return true
}
