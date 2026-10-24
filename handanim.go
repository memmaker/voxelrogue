package main

// The held weapon in first person, posed exactly like Minecraft's
// (ItemInHandRenderer): the arm offset and attack swing, or the bow use
// pose, then the item model's firstperson_righthand display transform.
// The modded weapons use the display transforms of their mods' models;
// the vanilla crossbow is VoxelStealth's model with a display of its own.

import (
	"math"
	"strings"

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

// crossbowDisplay is minecraft:item/crossbow's (its pulling and loaded
// pictures and weapons_expanded's chain crossbow inherit it).
var crossbowDisplay = display{mgl32.Vec3{-90, 0, -55}, mgl32.Vec3{1.13, 3.2, 1.13}, mgl32.Vec3{0.68, 0.68, 0.68}}

// displayFor is a sprite's first-person display transform.
func displayFor(sprite string) display {
	if d, ok := modDisplay[sprite]; ok {
		return d
	}
	if strings.HasPrefix(sprite, "w:crossbow") || strings.HasPrefix(sprite, "w:chain_crossbow") {
		return crossbowDisplay
	}
	return handheld
}

// upright are voxellib weapon sprites drawn standing up instead of on the
// diagonal of Minecraft's handheld textures (grip bottom left, head top
// right), which the display transforms are made for.
var upright = map[string]bool{"mace_01": true, "mace_04": true, "mace_05": true, "mace_06": true, "axe_29": true, "axe_35": true}

// spriteTurn turns a sprite's slab like the Minecraft texture its display
// transform expects.
func spriteTurn(sprite string) mgl32.Mat4 {
	if upright[sprite] {
		return rotZ(-45) // about the middle, which puts the grip near a sword's
	}
	return mgl32.Ident4()
}

// Hand uses: what the held item is doing besides swinging.
const (
	useNone           = iota
	useBow            // drawing a bow
	useCrossbowCharge // spanning a crossbow
	useCrossbowHold   // carrying a spanned crossbow
)

// handPose is what ItemInHandRenderer draws the main hand from.
type handPose struct {
	swing        float32 // the attack swing's progress, 0..1
	bobX, bobY   float32 // the view bob's offset
	armHeight    float32 // inverseArmHeight: how far the hand is lowered, 0..1 (swapping items)
	swayX, swayY float32 // how far the view has turned ahead of the hand, in degrees
	use          int
	charge       float32 // a crossbow's CrossbowItem.getChargeDuration, in ticks
	useT         float32 // timeHeld: ticks the use has gone on, with the partial tick
}

// drawPower is BowItem.getPowerForTime: a bow's strength after t ticks
// of drawing, 0..1, for a bow fully drawn in full ticks (vanilla 20,
// weapons_expanded's longbow 32).
func drawPower(t, full float32) float32 {
	f := t / full
	return min((f*f+f*2)/3, 1)
}

// fullDraw is how many ticks a bow takes to draw fully.
func fullDraw(which int) float32 {
	if which == rogue.GreatBow {
		return 32
	}
	return 20
}

// bowZoom is the field of view factor after t ticks of drawing
// (AbstractClientPlayer.getFieldOfViewModifier; the longbow's mixin zooms
// the same over its longer draw).
func bowZoom(t, full float32) float32 {
	f := min(t/full, 1)
	return 1 - f*f*0.15
}

func rotX(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DX(mgl32.DegToRad(deg)) }
func rotY(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DY(mgl32.DegToRad(deg)) }
func rotZ(deg float32) mgl32.Mat4 { return mgl32.HomogRotate3DZ(mgl32.DegToRad(deg)) }

// handArm is the view-space transform of the main hand's item before
// its model's display transform: ItemInHandRenderer.submitHandsWithItems'
// sway and renderArmWithItem's arm offset, use pose or swing.
func handArm(h handPose) mgl32.Mat4 {
	pi := float32(math.Pi)
	m := rotX(h.swayX).Mul4(rotY(h.swayY))
	// applyItemArmTransform
	m = m.Mul4(mgl32.Translate3D(0.56+h.bobX, -0.52+h.bobY+h.armHeight*-0.6, -0.72))
	switch h.use {
	case useBow, useCrossbowCharge:
		var f float32
		if h.use == useBow {
			m = m.Mul4(mgl32.Translate3D(-0.2785682, 0.18344387, 0.15731531)).Mul4(rotX(-13.935)).Mul4(rotY(35.3)).Mul4(rotZ(-9.785))
			f = drawPower(h.useT, 20) // always the vanilla bow's
		} else {
			m = m.Mul4(mgl32.Translate3D(-0.4785682, -0.094387, 0.05731531)).Mul4(rotX(-11.935)).Mul4(rotY(65.3)).Mul4(rotZ(-9.785))
			f = min(h.useT/h.charge, 1)
		}
		if f > 0.1 { // the strain shakes it
			m = m.Mul4(mgl32.Translate3D(0, mthSin((h.useT-0.1)*1.3)*(f-0.1)*0.004, 0))
		}
		m = m.Mul4(mgl32.Translate3D(0, 0, f*0.04)).Mul4(mgl32.Scale3D(1, 1, 1+f*0.2)).Mul4(rotY(-45))
	default: // swingArm
		p := h.swing
		sq := float32(math.Sqrt(float64(p)))
		m = m.Mul4(mgl32.Translate3D(-0.4*mthSin(sq*pi), 0.2*mthSin(sq*2*pi), -0.2*mthSin(p*pi)))
		// applyItemArmAttackTransform
		f, f1 := mthSin(p*p*pi), mthSin(sq*pi)
		m = m.Mul4(rotY(45 - 20*f)).Mul4(rotZ(-20 * f1)).Mul4(rotX(-80 * f1)).Mul4(rotY(-45))
		if h.use == useCrossbowHold && p < 0.001 {
			m = m.Mul4(mgl32.Translate3D(-0.641864, 0, 0)).Mul4(rotY(10))
		}
	}
	return m
}

// displayMat is a display transform as ItemTransform.apply applies it.
func displayMat(d display) mgl32.Mat4 {
	t := d.trans.Mul(1.0 / 16)
	return mgl32.Translate3D(t[0], t[1], t[2]).
		Mul4(rotX(d.rot[0])).Mul4(rotY(d.rot[1])).Mul4(rotZ(d.rot[2])).
		Mul4(mgl32.Scale3D(d.scale[0], d.scale[1], d.scale[2]))
}

// handModel is the view-space transform of a held item's unit slab.
func handModel(sprite string, h handPose) mgl32.Mat4 {
	return handArm(h).Mul4(displayMat(displayFor(sprite))).Mul4(spriteTurn(sprite))
}
