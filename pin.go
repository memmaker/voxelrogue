package main

// A sharp missile that kills a monster with a wall close behind it
// carries the body back and pins it there, the shaft through it and the
// tip in the wall (Painkiller's stake gun).

import (
	"math"

	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/render"
)

const (
	pinReach = 4   // blocks from the body's back to the wall at most
	pinTicks = 4   // the flight to the wall
	pinDepth = 0.3 // how deep the tip goes into the wall
)

type pin struct {
	a      *projectile.Arrow
	ux, uz float64    // the flight, level
	tip    [3]float64 // where it struck
	tipGo  float64    // how far the tip travels
	end    [3]float64 // where the body hangs: x, z with its back flat on the wall, and its yaw, facing out of it
}

// wallBehind is how far along ux, uz from x, y, z the first solid block
// is, within limit.
func wallBehind(solid func(x, y, z int) bool, x, y, z, ux, uz, limit float64) (float64, bool) {
	for s := 0.0; s <= limit; s += 0.05 {
		if solid(int(math.Floor(x+ux*s)), int(math.Floor(y)), int(math.Floor(z+uz*s))) {
			return s, true
		}
	}
	return 0, false
}

// pinTo pins m, just killed by the sharp missile a, if a wall is close
// enough behind it: its other missiles fall out, a leaves it to fly on
// with the body (tickPins), which flies on along the shot until its back
// lies flat on the wall's face, turned to face out of it like a decal.
// solid is the terrain's.
func (g *game) pinTo(m *monster, a *projectile.Arrow, solid func(x, y, z int) bool) {
	ux, uz := math.Sin(a.Yaw), -math.Cos(a.Yaw)
	d := float64(g.box(m.letter).d)
	w, ok := wallBehind(solid, a.X, a.Y, a.Z, ux, uz, pinReach+2*d)
	if !ok {
		return
	}
	g.arrows.Release(arrowTarget{g, m})
	a.Stuck = true
	p := &pin{a: a, ux: ux, uz: uz, tip: [3]float64{a.X, a.Y, a.Z}, tipGo: w + pinDepth}
	// the face the flight enters the wall block by: across x, else across z
	hx, px := math.Floor(a.X+ux*w), math.Floor(a.X+ux*(w-0.05))
	if hx != px {
		n := -math.Copysign(1, ux) // out of the wall
		face := hx
		if ux < 0 {
			face++
		}
		p.end[0] = face + n*d
		p.end[1] = m.dead[2] + uz*math.Max(0, (p.end[0]-m.dead[0])/ux)
		p.end[2] = math.Atan2(n, 0)
	} else {
		n := -math.Copysign(1, uz)
		face := math.Floor(a.Z + uz*w)
		if uz < 0 {
			face++
		}
		p.end[1] = face + n*d
		p.end[0] = m.dead[0] + ux*math.Max(0, (p.end[1]-m.dead[2])/uz)
		p.end[2] = math.Atan2(0, -n)
	}
	m.pin = p
}

// pinAt is how far along its flight a pin is k ticks after the blow (0..1).
func pinAt(k float64) float64 { return math.Min(math.Max(k, 0)/pinTicks, 1) }

// tickPins moves the pinning missiles with their bodies, after the
// arrows' tick (so they draw between this tick and the last).
func (g *game) tickPins() {
	for _, m := range g.corpses[g.depth] {
		p := m.pin
		if p == nil || p.a.In != nil || !p.a.Stuck {
			continue // picked up or knocked loose
		}
		f := pinAt(float64(m.dying)) * p.tipGo
		p.a.X, p.a.Y, p.a.Z = p.tip[0]+p.ux*f, p.tip[1], p.tip[2]+p.uz*f
		if m.dying == pinTicks {
			g.bleed(p.a.X-p.ux*p.tipGo*0.2, p.a.Y, p.a.Z-p.uz*p.tipGo*0.2) // against the wall
		}
	}
}

// pinnedPose is a pinned body: carried upright to the wall, turning to
// face out of it, then hanging there flat on it at the height it was
// struck.
func (g *game) pinnedPose(m *monster, alpha float64) render.ItemPose {
	p := m.pin
	f := pinAt(float64(m.dying) - 1 + alpha)
	x, z := m.dead[0]+(p.end[0]-m.dead[0])*f, m.dead[2]+(p.end[1]-m.dead[2])*f
	yaw := m.dead[3] + math.Remainder(p.end[2]-m.dead[3], 2*math.Pi)*f
	y := m.dead[1] + float64(g.box(m.letter).hover)
	in := g.monsterPose(m.letter, x, y, z, yaw, 0)
	if g.eng != nil {
		in.Sky, in.Block = g.light(x, y+0.5, z)
	}
	return in
}
