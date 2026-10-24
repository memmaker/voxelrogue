package main

import (
	"math"

	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

// Spell and wand effects. Rogue 3.6 draws only the bolt's '*' path, so the
// look follows rx1 (console/anim_fx.go, controller.go: a bright lead with
// a coloured trail per ray, '°' and '*' missiles, glows and orbits at the
// target) translated to VoxelLib particles, with Minecraft's particle
// manners where they fit (SpellParticle's slow rise and 0.96 friction,
// the lightning's jittered segments redrawn every frame). Plain glowing
// squares, no atlas.

// rx1's theme colours (data_rx1/themes/fancy.rec).
func rgb(r, g, b uint8) [4]float32 {
	return [4]float32{float32(r) / 255, float32(g) / 255, float32(b) / 255, 1}
}

func fade(c [4]float32) [4]float32 { c[3] = 0; return c }

var (
	fxWhite    = rgb(255, 255, 255)
	fxYellow   = rgb(255, 255, 85)
	darkYellow = rgb(191, 188, 2)
	lightRed   = rgb(255, 85, 85)
	fxRed      = rgb(255, 20, 20)
	lightCyan  = rgb(85, 255, 255)
	blue       = rgb(60, 183, 250)
	lightGreen = rgb(85, 255, 85)
	lightGray  = rgb(170, 170, 170)
	darkGray   = rgb(60, 60, 60)
	magenta    = rgb(230, 60, 255)  // rx1's polymorph glow
	warm       = rgb(255, 220, 140) // rx1's light swell
)

// rayFX is each bolt's wake, rx1's trail colours: ice White→LightCyan→Blue
// falling like snow, flame White→Yellow→Red rising, the lightning bolt
// White/Yellow flickering off its line.
var rayFX = map[string]particles.Props{
	"ice":   {Count: 4, Spread: 0.12, VelVar: 0.5, Gravity: 1, Drag: 0.8, Life: 0.4, LifeVar: 0.1, Size0: 0.22, Size1: 0.05, Col0: lightCyan, Col1: fade(blue), Additive: true, Emit: 15},
	"flame": {Count: 4, Spread: 0.12, Vel: [3]float64{0, 1, 0}, VelVar: 0.5, Drag: 0.8, Life: 0.35, LifeVar: 0.1, Size0: 0.25, Size1: 0.05, Col0: fxYellow, Col1: fade(fxRed), Additive: true, Emit: 15},
	"bolt":  {Count: 3, Spread: 0.3, Life: 0.08, Size0: 0.18, Size1: 0.18, Col0: fxWhite, Col1: fxYellow, Additive: true, Emit: 15},
}

// missileFX is a wand's missile: a fxWhite head with a short wake in c.
func missileFX(c [4]float32) particles.Props {
	return particles.Props{Count: 3, Spread: 0.08, VelVar: 0.3, Drag: 2, Life: 0.25, LifeVar: 0.1, Size0: 0.2, Size1: 0.03, Col0: fxWhite, Col1: fade(c), Additive: true, Emit: 15}
}

// smoke is a puff, Minecraft's smoke particle: slow rise, grows, fades.
var smoke = particles.Props{Count: 12, Spread: 0.1, Vel: [3]float64{0, 0.6, 0}, VelVar: 0.3, Gravity: -0.4, Drag: 0.5, Life: 1.2, LifeVar: 0.4, Size0: 0.15, Size1: 0.35, Col0: [4]float32{0.5, 0.5, 0.5, 0.8}, Col1: [4]float32{0.3, 0.3, 0.3, 0}}

// glowFX is one spark of the wand of light's swell over the room.
var glowFX = particles.Props{Count: 1, Spread: 1, Vel: [3]float64{0, 0.7, 0}, VelVar: 0.1, Life: 1.4, LifeVar: 0.3, Size0: 0.2, Size1: 0, Col0: warm, Col1: fade(warm), Additive: true, Emit: 15}

// fxStop is a square a shot passes, and what happens when it gets there.
type fxStop struct {
	x, y, z float64
	on      func()
}

// shot is something flying square to square: the fire, cold or lightning
// bolt along its path or a wand's missile to its target, leaving a trail
// and calling on at every stop.
type shot struct {
	path  []fxStop
	i, t  int // segment and ticks into it
	ticks int // per segment
	trail particles.Props
}

type fxState struct {
	parts    particles.System
	partRend *render.ParticleRenderer
	shots    []*shot
	glow     int // ticks left of the wand of light's swell
	glowRoom *dungeon.Room
}

// wandTip is where a wand's magic leaves the hand: before the eye, a bit
// right and down.
func (g *game) wandTip() (x, y, z float64) {
	p := g.player
	fx, fz := math.Sin(g.cam.Yaw), -math.Cos(g.cam.Yaw)
	return p.X + fx*0.6 - fz*0.25, p.Y + p.EyeHeight - 0.3, p.Z + fz*0.6 + fx*0.25
}

// cast launches a shot from the wand tip through stops, ticks per square.
func (g *game) cast(tr particles.Props, ticks int, stops ...fxStop) {
	x, y, z := g.wandTip()
	g.shots = append(g.shots, &shot{path: append([]fxStop{{x: x, y: y, z: z}}, stops...), ticks: ticks, trail: tr})
}

// tickEffects moves the shots and the particles one tick.
func (g *game) tickEffects() {
	live := g.shots[:0]
	for _, s := range g.shots {
		a, b := s.path[s.i], s.path[s.i+1]
		s.t++
		f := float64(s.t) / float64(s.ticks)
		g.parts.Emit(a.x+(b.x-a.x)*f, a.y+(b.y-a.y)*f, a.z+(b.z-a.z)*f, s.trail)
		if s.t == s.ticks {
			s.i, s.t = s.i+1, 0
			if b.on != nil {
				b.on() // ponytail: must not shoot (it would alias live); the wands only emit
			}
			if s.i+1 >= len(s.path) {
				continue
			}
		}
		live = append(live, s)
	}
	g.shots = live
	if g.glow > 0 {
		g.glow--
		rm := g.glowRoom
		for i := 0; i < 15; i++ {
			c := dungeon.Coord{X: rm.Pos.X + 1 + rogue.Rnd(g.rng, rm.Max.X-2), Y: rm.Pos.Y + 1 + rogue.Rnd(g.rng, rm.Max.Y-2)}
			x, y, z := tileCenter(g.slot(), c)
			g.parts.Emit(x, y+0.3, z, glowFX)
		}
	}
	g.parts.Tick(tickDur.Seconds(), g.terrain.Solid) // blood lies on the floor
}

// drawEffects draws the particles after the world and the models.
func (g *game) drawEffects(alpha float64) {
	if g.partRend != nil {
		g.partRend.Draw(&g.cam, g.parts.P, alpha, g.light) // Emit 15 keeps magic bright in dark rooms; smoke is lit like terrain
	}
}

// burst is a puff around a monster's body, outward (radial > 0) or
// gathering (radial < 0).
func (g *game) burst(x, y, z float64, c0, c1 [4]float32, radial float64) {
	g.parts.Emit(x, y+0.8, z, particles.Props{Count: 40, Spread: 0.3, Radial: radial, Drag: 1.5, Life: 0.6, LifeVar: 0.2, Size0: 0.18, Size1: 0.02, Col0: c0, Col1: c1, Additive: true, Emit: 15})
}

// ring puts sparks on a circle of radius r around a monster, each moving
// radially (blocks/s, inward when negative) and around it (radians/s):
// rx1's orbits and waves.
func (g *game) ring(x, y, z, r, radial, around float64, c0, c1 [4]float32, life float64) {
	particles.Ring(r, radial, 15, func(dx, dz, vx, vz float64) {
		q := particles.Particle{X: x + dx, Y: y, Z: z + dz, VX: vx - dz*around, VZ: vz + dx*around,
			Life: life, Size0: 0.16, Size1: 0.04, Col0: c0, Col1: c1, Additive: true, Emit: 15}
		q.PrevX, q.PrevY, q.PrevZ = q.X, q.Y, q.Z
		g.parts.Add(q)
	})
}

// vanish is rx1's teleport animation: a fxWhite column fading to light cyan
// where something was, and again where it appears.
func (g *game) vanish(x, y, z float64) {
	g.parts.Emit(x, y+0.9, z, particles.Props{Count: 40, Spread: 0.35, Vel: [3]float64{0, 1.5, 0}, VelVar: 0.4, Drag: 1, Life: 0.6, LifeVar: 0.2, Size0: 0.15, Size1: 0.03, Col0: fxWhite, Col1: fade(lightCyan), Additive: true, Emit: 15})
}

// boltSquare is one square of a bolt's walk.
type boltSquare struct {
	c       dungeon.Coord
	wall    bool // the bolt bounces here (the square is not counted)
	bounced bool // it had bounced: it hits the hero here, not monsters
}

// boltPath is fire_bolt's walk (sticks.c): BOLT_LENGTH squares counted from
// the hero's own, bouncing back off doors, secret doors, walls and rock
// (the bounce costs no square). Only an unbounced bolt hits monsters and
// only a bounced one the hero; passing the hero unbounces it.
func boltPath(at func(dungeon.Coord) dungeon.Tile, hero, delta dungeon.Coord) (path []boltSquare) {
	pos, bounced := hero, false
	for y, n := 0, 0; y < 6 && n < 20; y, n = y+1, n+1 { // ponytail: n caps a walled-in caller
		switch at(pos) {
		case dungeon.Door, dungeon.SecretDoor, dungeon.VWall, dungeon.HWall, dungeon.Rock:
			bounced = true
			delta.X, delta.Y = -delta.X, -delta.Y
			y--
			path = append(path, boltSquare{c: pos, wall: true, bounced: true})
		default:
			path = append(path, boltSquare{c: pos, bounced: bounced})
			if bounced && pos == hero {
				bounced = false
			}
		}
		pos.X, pos.Y = pos.X+delta.X, pos.Y+delta.Y
	}
	return path
}

// fireBolt zaps a wand of lightning, fire or cold (sticks.c WS_ELECT, WS_FIRE,
// WS_COLD): the bolt walks boltPath at three ticks a square, 6d6 to the first
// monster that fails its save, or on into the wall and back at the hero,
// who takes 6d6 too unless he saves. The dice are rolled now, the hits and
// messages land as the bolt arrives.
func (g *game) fireBolt(name string) {
	h, l := g.heroTile(), g.level()
	delta := dungeon.Coord{X: int(math.Round(math.Sin(g.cam.Yaw))), Y: int(math.Round(-math.Cos(g.cam.Yaw)))}
	bolt := rogue.Object{Type: rogue.Weapon, HurlDmg: "6d6", HPlus: 100, Launch: -1}
	trail := rayFX[name]
	var stops []fxStop
	used := false
	px, _, pz := tileCenter(g.slot(), h)
	for _, sq := range boltPath(l.At, h, delta)[1:] { // the hero's own square first, nothing happens there
		x, y, z := tileCenter(g.slot(), sq.c)
		y++
		var on func()
		switch {
		case sq.wall:
			x, z = (x+px)/2, (z+pz)/2 // the wall's face
			on = func() { g.msg("The bolt bounces"); g.burst(x, y-0.8, z, fxWhite, fade(trail.Col1), 2) }
		case !sq.bounced && g.monsterAt(sq.c) != nil:
			m := g.monsterAt(sq.c)
			if !rogue.Save(g.rng, rogue.VSMagic, m.st.Lvl) {
				on = func() { g.fight(m, &bolt, true) }
				used = true
			} else {
				on = func() { g.msg("The " + name + " whizzes past the " + m.name() + "."); m.runto() }
			}
		case sq.bounced && sq.c == h:
			if rogue.Save(g.rng, rogue.VSMagic, g.stats.Lvl) {
				on = func() { g.msg("The " + name + " whizzes by you.") }
			} else {
				on = func() {
					g.msg("You are hit by the " + name + ".")
					g.hurt = 6
					if g.stats.HP -= rogue.Roll(g.rng, 6, 6); g.stats.HP <= 0 {
						g.die("a bolt")
					}
				}
				used = true
			}
		}
		stops = append(stops, fxStop{x, y, z, on})
		if used {
			break
		}
		px, pz = x, z
	}
	g.cast(trail, 3, stops...)
}
