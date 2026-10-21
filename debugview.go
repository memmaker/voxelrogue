package main

// -tp, a debug view for checking attacks against Better Combat's own
// third-person previews: the weapon is posed exactly as in first person,
// but seen from in front of the hero, with the whole player model.

import (
	"image"
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
)

type debugView struct {
	tp       bool   // -tp
	tpBody   [3]int // skin, shirt and trousers boxes
	animName string // -anim
	animAt   float32
}

// debugAttack is the -anim animation to show in place of the attack, and
// its tick.
func (g *game) debugAttack() (a *bcAnim, t float32) {
	return g.bcAnims[g.animName], g.animAt
}

func (g *game) setupDebugView() error {
	for i, c := range []color.NRGBA{{214, 166, 128, 255}, {40, 170, 170, 255}, {70, 60, 160, 255}} {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.SetNRGBA(0, 0, c)
		h, err := g.modelRend.Add(boxModel("tp_part", 1, 1, img))
		if err != nil {
			return err
		}
		g.tpBody[i] = h
	}
	return nil
}

// tpCamera maps the eye's view space to a camera in front of the hero and
// to its left, looking back at it, as in Better Combat's previews.
func tpCamera() mgl32.Mat4 {
	return mgl32.LookAtV(mgl32.Vec3{-1.6, -0.4, -2.8}, mgl32.Vec3{0, -0.75, 0}, mgl32.Vec3{0, 1, 0})
}

// drawTPBody draws the player model placed by root, its head, body and
// legs at rest and its arms placed by right and left.
func (g *game) drawTPBody(root, right, left mgl32.Mat4, aspect float32) {
	box := func(model int, part mgl32.Mat4, x, y, z, sx, sy, sz float32) {
		m := part.Mul4(mgl32.Translate3D((x+sx/2)/16, y/16, (z+sz/2)/16)).Mul4(mgl32.Scale3D(sx/16, sy/16, sz/16))
		g.modelRend.DrawHand(g.tpBody[model], aspect, m, 15, 15)
	}
	box(0, root, -4, -8, -4, 8, 8, 8)
	box(2, root, -3, -5, -4.5, 6, 1, 1) // eyes
	box(1, root, -4, 0, -2, 8, 12, 4)
	box(2, root.Mul4(mgl32.Translate3D(-1.9/16, 12.0/16, 0)), -2, 0, -2, 4, 12, 4)
	box(2, root.Mul4(mgl32.Translate3D(1.9/16, 12.0/16, 0)), -2, 0, -2, 4, 12, 4)
	box(0, right, -3, -2, -2, 4, 12, 4)
	box(0, left, -1, -2, -2, 4, 12, 4)
}
