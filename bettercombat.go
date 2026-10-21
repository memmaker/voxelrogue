package main

// Melee attacks played like Better Combat: its player animations
// (assets/bettercombat, playerAnimator emotes), each weapon's attack
// combo from its weapon_attributes preset, and the timing of
// playAttackAnimation. While an attack plays, the weapon is posed as on
// the third-person player model, held by its thirdperson_righthand
// display, and only it shows: Better Combat's first person hides the arms
// by default (isShowingArmsInFirstPerson), which from inside the head are
// boxes cut off at the shoulder. As in PlayerAnimationLib's first person
// (which Better Combat uses), the camera stays at the eye and the torso
// bone, which these version 1 emotes mean as "body", moves the whole
// model; Better Combat adds the camera's pitch to it, so the weapon tracks
// looking up and down.
// The hit lands at the upswing point, as in Better Combat.

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelrogue/rogue"
)

// bcAxes are a bone's animated values, in this order.
var bcAxes = [6]string{"x", "y", "z", "pitch", "yaw", "roll"}

// bcBones are the bones drawn.
var bcBones = [4]string{"torso", "rightArm", "leftArm", "rightItem"}

type bcKey struct {
	tick float32
	v    float32
	ease string
}

// bcAnim is one emote: keyframes per bone axis.
type bcAnim struct {
	begin, end, stop float32
	keys             [len(bcBones)][len(bcAxes)][]bcKey
}

// bcAttack is a preset's attack: an animation and its upswing.
type bcAttack struct {
	anim    string
	upswing float32
}

// bcCombo is a preset's attack combo, all with the same upswing.
func bcCombo(upswing float32, anims ...string) []bcAttack {
	a := make([]bcAttack, len(anims))
	for i, n := range anims {
		a[i] = bcAttack{n, upswing}
	}
	return a
}

var (
	bcSword    = bcCombo(0.5, "one_handed_slash_horizontal_right", "one_handed_slash_horizontal_left", "one_handed_stab")
	bcCutlass  = bcCombo(0.5, "one_handed_slash_horizontal_right", "one_handed_slash_horizontal_left")
	bcClaymore = bcCombo(0.5, "two_handed_slash_horizontal_right", "two_handed_stab_left", "two_handed_slam")
	bcGlaive   = bcCombo(0.5, "two_handed_slash_vertical_right", "two_handed_slash_vertical_left", "two_handed_slash_horizontal_right")
)

// bcPresets maps Rogue weapon kinds to their Better Combat presets' combos
// (the mods' own weapon_attributes where they have them); the rest use
// sword.
var bcPresets = map[int][]bcAttack{
	rogue.Mace:       bcCombo(0.5, "one_handed_slash_horizontal_right", "one_handed_slash_horizontal_left", "one_handed_slam"),
	rogue.Dagger:     bcCombo(0.5, "one_handed_slash_horizontal_right", "one_handed_slash_horizontal_left", "dual_handed_stab"),
	rogue.TwoSword:   bcClaymore,
	rogue.Spear:      bcCombo(0.5, "two_handed_stab_right"),
	rogue.Glaive:     bcGlaive,
	rogue.Greatsword: bcClaymore,
	rogue.Halberd:    bcCombo(0.5, "two_handed_slash_vertical_left", "two_handed_stab_right"),
	rogue.Katana: bcCombo(0.5, "two_handed_slash_horizontal_right", "two_handed_slash_vertical_left",
		"two_handed_slash_vertical_right", "two_handed_slash_horizontal_left"),
	rogue.Scythe:    bcCombo(0.5, "two_handed_slash_horizontal_right", "two_handed_slash_horizontal_left"),
	rogue.WarSpear:  bcCombo(0.5, "one_handed_stab"), // additionalweapon's spear
	rogue.WarGlaive: bcGlaive,
	rogue.Scimitar:  bcCutlass,
	rogue.WoodenPole: bcCombo(0.5, "two_handed_slash_horizontal_right", "two_handed_slash_horizontal_left",
		"two_handed_spin", "two_handed_slam", "two_handed_stab_left", "two_handed_stab_right"), // battlestaff
}

func presetFor(o *rogue.Object) []bcAttack {
	if o != nil && o.Type == rogue.Weapon {
		if p, ok := bcPresets[o.Which]; ok {
			return p
		}
	}
	return bcSword
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
	bcAnims map[string]*bcAnim

	attack     *bcAnim // playing, or nil
	attackT    int     // game ticks since it started
	attackLen  float32 // the attack's length (cooldown) in ticks
	upswing    float32
	combo      int
	comboOf    *rogue.Object
	sinceSwing int // ticks since the last attack ended
	hitIn      int // ticks until the pending hit lands (0 = none)
	hitReach   float64
}

// loadAnimations reads the emotes in dir.
func (g *game) loadAnimations(dir string) error {
	g.bcAnims = map[string]*bcAnim{}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	for _, f := range files {
		a, err := loadEmote(f)
		if err != nil {
			return err
		}
		g.bcAnims[filepath.Base(f[:len(f)-len(".json")])] = a
	}
	return nil
}

func loadEmote(path string) (*bcAnim, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f struct {
		Emote struct {
			BeginTick, EndTick, StopTick float32
			Moves                        []map[string]json.RawMessage
		}
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	e := f.Emote
	a := &bcAnim{begin: e.BeginTick, end: e.EndTick, stop: e.StopTick}
	for _, mv := range e.Moves {
		var tick float32
		var ease string
		json.Unmarshal(mv["tick"], &tick)
		json.Unmarshal(mv["easing"], &ease)
		for bi, bone := range bcBones {
			raw, ok := mv[bone]
			if !ok {
				continue
			}
			var vals map[string]float32
			if err := json.Unmarshal(raw, &vals); err != nil {
				continue
			}
			for ai, axis := range bcAxes {
				if v, ok := vals[axis]; ok {
					a.keys[bi][ai] = append(a.keys[bi][ai], bcKey{tick, v, ease})
				}
			}
		}
	}
	for bi := range a.keys {
		for ai := range a.keys[bi] {
			k := a.keys[bi][ai]
			sort.SliceStable(k, func(i, j int) bool { return k[i].tick < k[j].tick })
		}
	}
	return a, nil
}

func ease(name string, x float32) float32 {
	switch name {
	case "EASEINQUAD":
		return x * x
	case "EASEOUTQUAD":
		return 1 - (1-x)*(1-x)
	case "EASEINOUTQUAD":
		if x < 0.5 {
			return 2 * x * x
		}
		return 1 - (-2*x+2)*(-2*x+2)/2
	case "EASEINOUTSINE":
		return -(float32(math.Cos(math.Pi*float64(x))) - 1) / 2
	}
	return x
}

func lerp(a, b, f float32) float32 { return a + (b-a)*f }

// bcDefaults are the bones' default values (KeyframeAnimation's
// StateCollections): an axis eases from them once the emote has begun.
var bcDefaults = [len(bcBones)][6]float32{{}, {-5, 2, 0, 0, 0, 0}, {5, 2, 0, 0, 0, 0}, {}}

// value is a bone axis at animation tick t, as PlayerAnimationLib plays a
// legacy emote (PlayerAnimatorLoader; the same curve as playerAnimator's
// KeyframeAnimationPlayer once begun): from the default at tick 0,
// keyframe to keyframe eased by the earlier one's easing, held to endTick
// and back to the model's own value cur by stopTick.
func (a *bcAnim) value(bone, axis int, t, cur float32) float32 {
	k := a.keys[bone][axis]
	if len(k) == 0 {
		return cur
	}
	def := bcDefaults[bone][axis]
	ct := float32(math.Floor(float64(t)))
	pos := -1
	for i := range k {
		if k[i].tick <= ct {
			pos = i
		}
	}
	const sine = "EASEINOUTSINE"
	var before, after bcKey
	switch {
	case pos == -1 && ct < a.end:
		before = bcKey{0, def, sine}
	case pos == -1:
		before = bcKey{a.end, def, sine}
	case ct >= a.end && pos == len(k)-1 && k[pos].tick < a.end:
		before = bcKey{a.end, k[pos].v, k[pos].ease}
	default:
		before = k[pos]
	}
	switch last := k[len(k)-1]; {
	case pos+1 < len(k):
		after = k[pos+1]
	case ct < a.end:
		after = bcKey{a.end, last.v, last.ease}
	default:
		after = bcKey{a.stop, cur, sine}
	}
	if before.tick >= after.tick {
		return before.v
	}
	return lerp(before.v, after.v, ease(before.ease, (t-before.tick)/(after.tick-before.tick)))
}

// bone is a bone's position and rotation at tick t; cur is the model's
// own pose of it.
func (a *bcAnim) bone(bone int, t float32, cur [6]float32) (v [6]float32) {
	for i := range v {
		v[i] = a.value(bone, i, t, cur[i])
	}
	return v
}

// startAttack plays the next attack of the wielded weapon's combo over
// length ticks and schedules its hit.
func (g *game) startAttack(length int, reach float64) {
	p := presetFor(g.weapon)
	if g.comboOf != g.weapon || g.attack == nil && g.sinceSwing > 20 {
		g.combo = 0
	}
	g.comboOf = g.weapon
	at := p[g.combo%len(p)]
	g.combo++
	g.attack, g.attackT = g.bcAnims[at.anim], 0
	g.attackLen, g.upswing = float32(length), at.upswing
	g.hitIn, g.hitReach = max(1, int(math.Round(float64(g.attackLen*g.upswing)))), reach
}

// tickAttack advances the attack animation and lands its hit.
func (g *game) tickAttack() {
	if g.hitIn > 0 {
		if g.hitIn--; g.hitIn == 0 && !g.dead {
			if m := g.monsterInFront(g.hitReach); m != nil {
				g.fight(m, g.weapon, false)
			}
		}
	}
	if g.attack == nil {
		g.sinceSwing++
		return
	}
	if g.attackT++; g.animTick(float32(g.attackT)) >= g.attack.stop {
		g.attack, g.sinceSwing = nil, 0
	}
}

// animTick is the animation's tick after t game ticks:
// playAttackAnimation's speed gears (upswing multiplier 0.5).
func (g *game) animTick(t float32) float32 {
	speed := g.attack.end / g.attackLen
	u, length := g.upswing, g.attackLen
	up := speed / (u / 0.5)
	down := speed * (1 - u)
	switch {
	case t < length*u:
		return t * up
	case t < length:
		return length*u*up + (t-length*u)*down
	}
	return length*u*up + length*(1-u)*down + (t-length)*speed
}

func partMat(v [6]float32) mgl32.Mat4 {
	return mgl32.Translate3D(v[0]/16, v[1]/16, v[2]/16).
		Mul4(mgl32.HomogRotate3DZ(v[5])).Mul4(mgl32.HomogRotate3DY(v[4])).Mul4(mgl32.HomogRotate3DX(v[3]))
}

// drawAttack draws the weapon (sprite) during an attack and reports
// whether it did.
func (g *game) drawAttack(sprite string, alpha float64, bobX, bobY float32, sky, blk uint8) bool {
	a, t := g.attack, float32(0)
	switch {
	case g.animName != "":
		a, t = g.debugAttack()
	case a == nil || g.dead:
		return false
	default:
		t = g.animTick(float32(g.attackT) + float32(alpha))
	}
	if a == nil {
		return false
	}
	aspect := float32(g.cam.Aspect)

	// The body bone, with Better Combat's first-person adjustment
	// (AttackAnimationStack): the camera's pitch added to its pitch, and
	// looking up a little lift and lean, faded in up to beginTick and out
	// after endTick (AdjustmentModifier). PlayerAnimationLib turns it about
	// 0.75 above the feet; the camera stays at the eye.
	up := float32(g.cam.Pitch)
	fade := min(t/a.begin, (a.stop-t)/(a.stop-a.end), 1)
	tv := a.bone(0, t, [6]float32{})
	tv[3] += up * fade
	if up > 0 {
		s := float32(math.Sin(float64(up))) * fade
		tv[1] += s * 0.5 / 16
		tv[2] -= s / 16
	}
	torso := mgl32.Translate3D(tv[0], tv[1]+0.75, tv[2]).
		Mul4(mgl32.HomogRotate3DZ(tv[5])).Mul4(mgl32.HomogRotate3DY(tv[4])).Mul4(mgl32.HomogRotate3DX(tv[3])).
		Mul4(mgl32.Translate3D(0, -0.75, 0))
	const eye = 1.62 // the player model's
	view := mgl32.Translate3D(bobX, bobY, 0)
	if g.tp {
		view, sky, blk = tpCamera(), 15, 15
	}
	body := view.Mul4(mgl32.HomogRotate3DX(-up)).
		Mul4(mgl32.Translate3D(0, -eye, 0)).Mul4(torso).
		Mul4(mgl32.Scale3D(-1, -1, 1)).Mul4(mgl32.Scale3D(15.0/16, 15.0/16, 15.0/16)).Mul4(mgl32.Translate3D(0, -1.501, 0))

	right := body.Mul4(partMat(a.bone(1, t, [6]float32{-5, 2, 0, -math.Pi / 10, 0, 0})))
	render.BeginHands()
	defer render.EndHands()
	if g.tp {
		left := body.Mul4(partMat(a.bone(2, t, [6]float32{5, 2, 0, 0, 0, 0})))
		g.drawTPBody(body, right, left, aspect)
	}

	// ItemInHandLayer: into the hand, the item bone, the display transform
	item := right.
		Mul4(rotX(-90)).Mul4(rotY(180)).Mul4(mgl32.Translate3D(1.0/16, 2.0/16, -10.0/16)).
		Mul4(partMat(a.bone(3, t, [6]float32{})))
	d, ok := thirdPerson[sprite]
	if !ok {
		d = handheld3
	}
	tr := d.trans.Mul(1.0 / 16)
	item = item.Mul4(mgl32.Translate3D(tr[0], tr[1], tr[2])).
		Mul4(rotX(d.rot[0])).Mul4(rotY(d.rot[1])).Mul4(rotZ(d.rot[2])).
		Mul4(mgl32.Scale3D(d.scale[0], d.scale[1], d.scale[2])).Mul4(spriteTurn(sprite))
	g.itemRend.DrawHandModel(sprite, aspect, item, sky, blk)
	return true
}
