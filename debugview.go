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

// debugAnim plays -anim at its own speed, as Better Combat's previews do,
// ticked to -anim-at, and reports whether there is one.
func (g *game) debugAnim() bool {
	a := g.anims[g.animName]
	if a == nil {
		return false
	}
	g.stack.playRaw(a)
	for i := 0; i < int(g.animAt); i++ {
		g.stack.handle(0, true, !g.tp)
	}
	return true
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

// drawTPBody draws the player model placed by root with its parts posed:
// head, body, right arm, left arm, right leg, left leg.
func (g *game) drawTPBody(root mgl32.Mat4, parts [6]part, aspect float32) {
	box := func(model int, p part, x, y, z, sx, sy, sz float32) {
		g.modelRend.DrawHand(g.tpBody[model], aspect, partBox(root, p, x, y, z, sx, sy, sz), 15, 15)
	}
	box(0, parts[0], -4, -8, -4, 8, 8, 8)
	box(2, parts[0], -3, -5, -4.5, 6, 1, 1) // eyes
	box(1, parts[1], -4, 0, -2, 8, 12, 4)
	box(0, parts[2], -3, -2, -2, 4, 12, 4)
	box(0, parts[3], -1, -2, -2, 4, 12, 4)
	box(2, parts[4], -2, 0, -2, 4, 12, 4)
	box(2, parts[5], -2, 0, -2, 4, 12, 4)
}

// partBox places a model part's cube (ModelPart.render: the part's
// pose, then the cube's origin and size in 1/16 blocks) under root.
func partBox(root mgl32.Mat4, p part, x, y, z, sx, sy, sz float32) mgl32.Mat4 {
	return root.Mul4(partMat(p)).Mul4(mgl32.Translate3D((x+sx/2)/16, y/16, (z+sz/2)/16)).Mul4(mgl32.Scale3D(sx/16, sy/16, sz/16))
}
