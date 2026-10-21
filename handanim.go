package main

// The held weapon in first person, posed exactly like Minecraft's
// (ItemInHandRenderer): the arm offset and attack swing, or the bow use
// pose, then the item model's firstperson_righthand display transform.
// The modded weapons use the display transforms of their mods' models;
// the crossbow is VoxelStealth's (arrows.go).

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxelrogue/rogue"
)

// swingTicks is Minecraft's arm swing duration.
const swingTicks = 6

// display is a model's display transform: rotation in degrees (applied
// X, then Y, then Z), translation in 1/16 blocks, scale.
type display struct{ rot, trans, scale mgl32.Vec3 }

// handheld is minecraft:item/handheld (and item/generated, item/bow) in
// first person.
var handheld = display{mgl32.Vec3{0, -90, 25}, mgl32.Vec3{1.13, 3.2, 1.13}, mgl32.Vec3{0.68, 0.68, 0.68}}

// modDisplay holds the mods' own firstperson_righthand transforms, by
// sprite; the rest of their weapons inherit item/handheld.
var modDisplay = map[string]display{
	// weapons_expanded *_in_hand models
	"w:glaive_hand":     {mgl32.Vec3{-20, -90, -75}, mgl32.Vec3{3.13, 0, -1.13}, mgl32.Vec3{1.36, 1.36, 0.68}},
	"w:halberd_hand":    {mgl32.Vec3{-20, -90, -75}, mgl32.Vec3{3.13, 0, -1.13}, mgl32.Vec3{1.36, 1.36, 0.68}},
	"w:scythe_hand":     {mgl32.Vec3{-20, -90, -75}, mgl32.Vec3{1.13, 0, -1.13}, mgl32.Vec3{1.36, 1.36, 0.68}},
	"w:greatsword_hand": {mgl32.Vec3{0, -90, 25}, mgl32.Vec3{1.13, 4, 1.13}, mgl32.Vec3{0.85, 0.85, 0.85}},
	// additionalweapon
	"w:war_glaive": {mgl32.Vec3{0, -90, 35}, mgl32.Vec3{1.13, 0, 1}, mgl32.Vec3{1.36, 1.36, 0.68}},
	"w:scimitar":   {mgl32.Vec3{0, -90, 35}, mgl32.Vec3{1.13, 4.25, 1.13}, mgl32.Vec3{1.02, 1.02, 0.68}},
}

// flipped are voxellib bow sprites drawn the other way round from
// Minecraft's bow texture (limbs bottom right instead of top left).
var flipped = map[string]bool{"bow_01": true, "bow_10": true}

// upright are voxellib weapon sprites drawn standing up instead of on the
// diagonal of Minecraft's handheld textures (grip bottom left, head top
// right), which the display transforms are made for.
var upright = map[string]bool{"mace_01": true}

// spriteTurn turns a sprite's slab like the Minecraft texture its display
// transform expects.
func spriteTurn(sprite string) mgl32.Mat4 {
	switch {
	case flipped[sprite]:
		return rotZ(180)
	case upright[sprite]:
		return rotZ(-45) // about the middle, which puts the grip near a sword's
	}
	return mgl32.Ident4()
}

// Hand uses: what the held item is doing besides swinging.
const (
	useNone = iota
	useBow  // drawing a bow
)

// drawPower is BowItem.getPowerForTime: a bow's strength after t ticks
// of drawing, 0..1, for a bow fully drawn in full ticks (vanilla 20,
// weapons_expanded's longbow 32).
func drawPower(t, full float32) float32 {
	f := t / full
	return min((f*f+f*2)/3, 1)
}

// fullDraw is how many ticks a launcher takes to draw fully.
func fullDraw(which int) float32 {
	if which == rogue.GreatBow {
		return 32
	}
	return 20
}

// bowZoom is the FOV factor while drawing (AbstractClientPlayer's
// getFieldOfViewModifier; the longbow zooms the same over its longer draw).
func bowZoom(t, full float32) float32 {
	f := min(t/full, 1)
	if f < 1 {
		f *= f
	}
	return 1 - f*0.15
}

func rotX(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DX(mgl32.DegToRad(deg)) }
func rotY(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DY(mgl32.DegToRad(deg)) }
func rotZ(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DZ(mgl32.DegToRad(deg)) }

// handModel is the view-space transform of a held item's unit slab for
// swing progress p (0..1), a view bob offset and a use: its kind and the
// ticks (with the partial tick) it has gone on.
func handModel(sprite string, p, bobX, bobY float32, use int, useT float32) mgl32.Mat4 {
	d, ok := modDisplay[sprite]
	if !ok {
		d = handheld
	}
	sin := func(x float32) float32 { return float32(math.Sin(float64(x))) }
	// applyItemArmTransform (no equip animation)
	arm := mgl32.Translate3D(0.56+bobX, -0.52+bobY, -0.72)
	var m mgl32.Mat4
	switch use {
	case useBow:
		m = arm.Mul4(mgl32.Translate3D(-0.2785682, 0.18344387, 0.15731531)).
			Mul4(rotX(-13.935)).Mul4(rotY(35.3)).Mul4(rotZ(-9.785))
		f := drawPower(useT, 20) // ItemInHandRenderer always uses the vanilla bow's
		if f > 0.1 {             // the strain shakes it
			m = m.Mul4(mgl32.Translate3D(0, sin((useT-0.1)*1.3)*(f-0.1)*0.004, 0))
		}
		m = m.Mul4(mgl32.Translate3D(0, 0, f*0.04)).Mul4(mgl32.Scale3D(1, 1, 1+f*0.2)).Mul4(rotY(-45))
	default:
		sq := float32(math.Sqrt(float64(p)))
		pi := float32(math.Pi)
		// swing offset, then the arm
		m = mgl32.Translate3D(-0.4*sin(sq*pi), 0.2*sin(sq*2*pi), -0.2*sin(p*pi)).Mul4(arm)
		// applyItemArmAttackTransform
		f, f1 := sin(p*p*pi), sin(sq*pi)
		m = m.Mul4(rotY(45 - 20*f)).Mul4(rotZ(-20 * f1)).Mul4(rotX(-80 * f1)).Mul4(rotY(-45))
	}
	// the model's display transform
	t := d.trans.Mul(1.0 / 16)
	return m.Mul4(mgl32.Translate3D(t[0], t[1], t[2])).
		Mul4(rotX(d.rot[0])).Mul4(rotY(d.rot[1])).Mul4(rotZ(d.rot[2])).
		Mul4(mgl32.Scale3D(d.scale[0], d.scale[1], d.scale[2])).Mul4(spriteTurn(sprite))
}
