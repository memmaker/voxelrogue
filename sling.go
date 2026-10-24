package main

// The sling as Nightfall's (frostnox's mod): hold the right button and
// the hero raises the sling and whirls it, faster and faster, as long as
// the button is held; letting go throws a rock, the harder the longer it
// whirled. The action is Nightfall's ActionsNF.SLING_THROW (a
// LaunchProjectileAction run by its ActionTracker) and the first-person
// motion its SlingThrow.transformModelFP, applied through its
// AnimatedItemModel as the item's display transform: a translation and a
// rotation eased between keyframes by AnimationCalculators. The sling is
// the mod's 3D model (template_sling, template_sling_swinging while the
// action runs), the rock a ThrownRockEntity.

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelrogue/rogue"
)

// slingDur is SLING_THROW's states in ticks: raise, charge (the whirl:
// loops of 10, 9, 8 and 7 ticks, held as long as the button is down),
// release, freeze and return. The charge state is 1, its full charge its
// duration, the least charge half of it (Action.getRequiredCharge).
var slingDur = [5]int{8, 10 + 9 + 8 + 7, 4, 3, 7}

const (
	slingCharge = 1
	rockGravity = 0.04 // ItemProjectileEntity.getGravity
	rockBounce  = 0.3  // speed a stone or blunt thrown weapon keeps rebounding off a wall or the floor: a pebble, not a ball
)

// slingAction is Nightfall's ActionTracker for the sling: the state and
// its frame (1 to the state's duration; 0 when no action runs), and the
// frame the charge was released at.
type slingAction struct {
	state, frame, charge int
	queued               bool // the button came up: throw once charged enough
}

// startSling is the right button going down with the sling in hand
// (ProjectileLauncherItem.use with ammunition, Action.canStart: nothing
// running, or the last state near its end).
func (g *game) startSling() {
	s := &g.sling
	if s.frame != 0 && !(s.state == len(slingDur)-1 && s.frame >= slingDur[s.state]-3) {
		return
	}
	if a := g.findAmmo(); a == nil || !rogue.Launches(g.weapon.Which, a) {
		g.msg("You have nothing to shoot.")
		return
	}
	*s = slingAction{frame: 1}
}

// tickSling is ActionTracker.tick for the sling and
// LaunchProjectileAction.onTick: the frame counts, a released charge of
// at least the least charge moves on to the throw, which launches the
// rock on the release state's second frame.
func (g *game) tickSling() {
	s := &g.sling
	if s.frame == 0 {
		return
	}
	if g.weapon == nil || rogue.Weapons[g.weapon.Which].Action != "sling" {
		*s = slingAction{}
		return
	}
	s.frame++
	if s.state == slingCharge && !g.fireHeld {
		s.queued = true // releaseCharge
	}
	if s.queued && s.state == slingCharge && s.frame >= slingDur[slingCharge]/2 {
		s.charge = s.frame
		s.move()
	} else if s.state != slingCharge && s.frame > slingDur[s.state] {
		s.move()
	}
	if s.state == slingCharge+1 && s.frame == 2 {
		if a := g.findAmmo(); a != nil && rogue.Launches(g.weapon.Which, a) {
			g.shoot(g.takeAmmo(), min(float64(s.charge)/float64(slingDur[slingCharge]), 1)*rogue.Weapons[g.weapon.Which].Speed).Gravity = rockGravity
		}
	}
}

// move is ActionTracker.moveState.
func (s *slingAction) move() {
	if s.state >= len(slingDur)-1 {
		*s = slingAction{}
		return
	}
	s.state++
	s.frame = 1
}

// slingZoom is Nightfall's bow-like zoom for a charging action
// (ClientEventHandler.onFOVModifierEvent), by the charge state's frame,
// as the field of view target of a tick (the frame's partial tick is
// taken as 0, as for the bows).
func slingZoom(frame int) float32 {
	m := float32(slingDur[slingCharge])
	f := float32(frame) + m/3
	p := min(1, (f-1)/m)
	if p < 1 {
		p *= p
	}
	return max(1-p*0.15, 0.15)
}

// --- Nightfall's AnimationCalculator and Easing ---

type easing func(float32) float32

var (
	easeNone      easing = func(p float32) float32 { return p }
	easeInSine    easing = func(p float32) float32 { return 1 - mthCos(p*math.Pi/2) }
	easeInOutSine easing = func(p float32) float32 { return -(mthCos(math.Pi*p) - 1) / 2 }
	easeOutCubic  easing = func(p float32) float32 { return 1 - float32(math.Pow(float64(1-p), 3)) }
)

// calc is an AnimationCalculator: a vector eased from start to end over
// a state's frames (looping over them from offset for a spin).
type calc struct {
	start, end     mgl32.Vec3
	length, offset int
	looping        bool
	ease           easing
}

func (c *calc) setEase(e easing) {
	if e != nil {
		c.ease = e
	}
}

// add moves on by v from the end reached so far; extend moves on to v.
func (c *calc) add(v mgl32.Vec3, e easing)    { c.start, c.end = c.end, c.end.Add(v); c.setEase(e) }
func (c *calc) extend(v mgl32.Vec3, e easing) { c.start, c.end = c.end, v; c.setEase(e) }
func (c *calc) freeze()                       { c.start = c.end }

// reset is resetLength(duration, inOutSine), as IClientAction does
// before each state.
func (c *calc) reset(length int) {
	c.length, c.offset, c.looping, c.ease = length, 0, false, easeInOutSine
}

// wrap is wrapRotation: the angles taken modulo 360 (Java's %).
func (c *calc) wrap() {
	for i := range 3 {
		c.start[i] = float32(math.Mod(float64(c.start[i]), 360))
		c.end[i] = float32(math.Mod(float64(c.end[i]), 360))
	}
}

// at is getTransformations at a frame with its partial tick.
func (c *calc) at(frame int, pt float32) mgl32.Vec3 {
	var p float32 = 1
	if c.looping || frame <= c.length {
		l := float32(c.length - c.offset)
		prev := float32(max(0, frame-1-c.offset)) / l
		p = prev + (float32(frame-c.offset)/l-prev)*pt
		if c.looping && math.Mod(float64(p), 1) != 0 {
			p = float32(math.Mod(float64(p), 1))
		}
	}
	return c.start.Add(c.end.Sub(c.start).Mul(c.ease(p)))
}

// The models' firstperson_righthand displays (template_sling and
// template_sling_swinging): rotation in degrees, translation in blocks.
var (
	slingRestR  = mgl32.Vec3{-50, 90, 0}
	slingRestT  = mgl32.Vec3{-1.0 / 16, 4.5 / 16, -1.0 / 16}
	slingSwingR = mgl32.Vec3{-50, 90, 0}
	slingSwingT = mgl32.Vec3{-1.0 / 16, -5.25 / 16, 10.5 / 16}
)

// animDefault is AnimationData.setDefaults' offsetValue: every nonzero
// default nudged by 1e-5 so an animation can tell it apart.
func animDefault(v mgl32.Vec3) mgl32.Vec3 {
	for i := range 3 {
		if v[i] != 0 {
			v[i] += 0.00001
		}
	}
	return v
}

// slingPose is the sling's display translation (blocks) and rotation
// (degrees) in first person: the resting model's own, or, while the
// action runs, SlingThrow.transformModelFP at the state's frame, built
// through IClientAction's recursion over the states before it (each
// with DEFAULT_DURATION 2) on the swinging model's defaults.
func slingPose(active bool, state, frame int, pt float32) (t, r mgl32.Vec3) {
	if !active {
		return animDefault(slingRestT), animDefault(slingRestR)
	}
	dT, dR := animDefault(slingSwingT), animDefault(slingSwingR)
	tc, rc := calc{start: dT, end: dT}, calc{start: dR, end: dR}
	for s := 0; s <= state; s++ {
		dur, f := 2, 2
		if s == state {
			dur, f = slingDur[s], frame
		}
		tc.reset(dur)
		rc.reset(dur)
		switch s {
		case 0: // raise
			tc.add(mgl32.Vec3{0, 0.35, 0}, nil)
			rc.add(mgl32.Vec3{-240, 0, 0}, easeInSine)
		case 1: // whirl: a turn per loop, the loops shortening
			rc.looping = true
			switch {
			case f <= 10:
				rc.length = 10
			case f <= 10+9:
				rc.offset, rc.length = 10, 9+10
			case f <= 10+9+8:
				rc.offset, rc.length = 19, 8+19
			default:
				rc.offset, rc.length = 27, 7+27
			}
			rc.add(mgl32.Vec3{-360, 0, 0}, easeNone)
			tc.freeze()
		case 2: // release
			tc.extend(mgl32.Vec3{-6.0 / 16, -2.0 / 16, -2.0 / 16}, easeOutCubic)
			rc.add(mgl32.Vec3{-240, 30, 0}, easeOutCubic)
		case 3: // freeze
			tc.freeze()
			rc.freeze()
		case 4: // return
			tc.extend(dT, nil)
			rc.wrap()
			rc.extend(dR, nil)
		}
	}
	return tc.at(frame, pt), rc.at(frame, pt)
}

// slingView is the sling model's view-space transform: the arm, then the
// display (ItemTransform.apply) and the model's centring.
func slingView(h handPose, t, r mgl32.Vec3) mgl32.Mat4 {
	return handArm(h).Mul4(mgl32.Translate3D(t[0], t[1], t[2])).
		Mul4(rotX(r[0])).Mul4(rotY(r[1])).Mul4(rotZ(r[2])).
		Mul4(mgl32.Translate3D(-0.5, -0.5, -0.5))
}

// drawSling draws the held sling: the resting model, or the swinging one
// posed by the action.
func (g *game) drawSling(h handPose, pt float32, sky, blk uint8) {
	s := g.sling
	active := s.frame != 0
	t, r := slingPose(active, s.state, s.frame, pt)
	m := g.kindModels[g.weapon.Which][0] // weapons.rec: its rest and whirl meshes
	if active {
		m = g.kindModels[g.weapon.Which][1]
	}
	g.modelRend.DrawHand(m, float32(g.cam.Aspect), slingView(h, t, r), sky, blk)
}
