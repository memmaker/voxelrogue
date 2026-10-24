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
	a             *projectile.Arrow
	ux, uz        float64    // the flight, level
	tip           [3]float64 // where it struck
	tipGo, bodyGo float64    // how far the tip and the body travel
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
// with the body (tickPins). solid is the terrain's.
func (g *game) pinTo(m *monster, a *projectile.Arrow, solid func(x, y, z int) bool) {
	ux, uz := math.Sin(a.Yaw), -math.Cos(a.Yaw)
	d := float64(g.box(m.letter).d)
	w, ok := wallBehind(solid, a.X, a.Y, a.Z, ux, uz, pinReach+2*d)
	if !ok {
		return
	}
	g.arrows.Release(arrowTarget{g, m})
	a.Stuck = true
	foot := (m.dead[0]-a.X)*ux + (m.dead[2]-a.Z)*uz // the body's foot past the tip
	m.pin = &pin{a: a, ux: ux, uz: uz, tip: [3]float64{a.X, a.Y, a.Z}, tipGo: w + pinDepth, bodyGo: math.Max(0, w-d-0.02-foot)}
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

// pinnedPose is a pinned body: carried upright along the flight to the
// wall, then hanging there at the height it was struck.
func (g *game) pinnedPose(m *monster, alpha float64) render.ItemPose {
	p := m.pin
	f := pinAt(float64(m.dying)-1+alpha) * p.bodyGo
	x, z := m.dead[0]+p.ux*f, m.dead[2]+p.uz*f
	y := m.dead[1] + float64(g.box(m.letter).hover)
	in := g.monsterPose(m.letter, x, y, z, m.dead[3], 0)
	if g.eng != nil {
		in.Sky, in.Block = g.light(x, y+0.5, z)
	}
	return in
}
