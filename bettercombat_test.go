package main

// Checks the vanilla side against Minecraft 26.2's own classes, run
// headless (testdata/vanilla.json): HumanoidModel poses,
// WalkAnimationState, LivingEntity's body yaw (with Mth.atan2) and the
// first-person render chain's matrices.

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxelrogue/rogue"
)

type vanillaData struct {
	Poses []struct {
		In    [5]float32
		Parts [6]part
	}
	Walk [][][6]float32
	Yaw  []struct {
		Start float32
		Ticks []json.RawMessage // [yRot, xd, zd, attacking, yBodyRotO, yBodyRot]
	}
	Chain []struct {
		Body, Arm, Item [9]float32
		BodyRot         float32
		Display         [9]float32
		ArmM, ItemM     [16]float32
	}
	Hand []struct {
		In      [5]float32 // swayX, swayY, armHeight, use (0 swing, 1 bow, 2 crossbow charging, 3 crossbow loaded), ticks held or swing progress
		Display [9]float32
		M       [16]float32
	}
}

func loadVanilla(t *testing.T) vanillaData {
	b, err := os.ReadFile("testdata/vanilla.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vanillaData
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVanillaPose(t *testing.T) {
	for i, p := range loadVanilla(t).Poses {
		var got [6]part
		got[0], got[1], got[2], got[3], got[4], got[5] = vanillaPose(p.In[0], p.In[1], p.In[2], p.In[3], p.In[4])
		for j := range got {
			for k := range got[j] {
				if !near(got[j][k], p.Parts[j][k], 1e-6) {
					t.Fatalf("pose %d part %d[%d]: %v, want %v (in %v)", i, j, k, got[j][k], p.Parts[j][k], p.In)
				}
			}
		}
	}
}

func TestWalkState(t *testing.T) {
	for i, seq := range loadVanilla(t).Walk {
		var w walkState
		for j, s := range seq {
			w.update(min(s[0]*4, 1), 0.4, 1)
			got := [4]float32{w.positionAt(s[1]), w.speedAt(s[1]), w.position, w.speed}
			for k, g := range got {
				if g != s[2+k] {
					t.Fatalf("walk %d tick %d [%d]: %v, want %v", i, j, k, g, s[2+k])
				}
			}
		}
	}
}

func TestBodyYaw(t *testing.T) {
	for i, seq := range loadVanilla(t).Yaw {
		g := &game{}
		g.bodyYaw, g.prevBodyYaw = seq.Start, seq.Start
		for j, raw := range seq.Ticks {
			var tk [6]json.Number
			var attacking bool
			var row []any
			json.Unmarshal(raw, &row)
			attacking = row[3].(bool)
			json.Unmarshal(raw, &[]any{&tk[0], &tk[1], &tk[2], &attacking, &tk[4], &tk[5]})
			f := func(n json.Number) float64 { v, _ := n.Float64(); return v }
			g.tickBodyYaw(float32(f(tk[0])), f(tk[1]), f(tk[2]), attacking)
			if g.prevBodyYaw != float32(f(tk[4])) || g.bodyYaw != float32(f(tk[5])) {
				t.Fatalf("yaw %d tick %d: %v %v, want %v %v", i, j, g.prevBodyYaw, g.bodyYaw, f(tk[4]), f(tk[5]))
			}
		}
	}
}

func TestChain(t *testing.T) {
	for i, c := range loadVanilla(t).Chain {
		disp := display{mgl32.Vec3{c.Display[0], c.Display[1], c.Display[2]}, mgl32.Vec3{c.Display[3], c.Display[4], c.Display[5]}, mgl32.Vec3{c.Display[6], c.Display[7], c.Display[8]}}
		var body, arm, item [axes]float32
		copy(body[:], c.Body[:])
		copy(arm[:], c.Arm[:])
		copy(item[:], c.Item[:])
		_, armM, itemM := bcChain(body, arm, item, c.BodyRot, 0, 0, disp)
		itemM = itemM.Mul4(mgl32.Translate3D(-0.5, -0.5, -0.5)) // the item model's centring
		for k := range 16 {
			if !near(armM[k], c.ArmM[k], 2e-4) {
				t.Fatalf("chain %d armM[%d]: %v, want %v", i, k, armM[k], c.ArmM[k])
			}
			if !near(itemM[k], c.ItemM[k], 2e-4) {
				t.Fatalf("chain %d itemM[%d]: %v, want %v", i, k, itemM[k], c.ItemM[k])
			}
		}
	}
}

func TestHand(t *testing.T) {
	for i, c := range loadVanilla(t).Hand {
		h := handPose{swayX: c.In[0], swayY: c.In[1], armHeight: c.In[2]}
		switch c.In[3] {
		case 0:
			h.swing = c.In[4]
		case 1:
			h.use, h.useT = useBow, c.In[4]
		case 2:
			h.use, h.useT, h.charge = useCrossbowCharge, c.In[4], 25
		case 3:
			h.use, h.swing = useCrossbowHold, c.In[4]
		}
		disp := display{mgl32.Vec3{c.Display[0], c.Display[1], c.Display[2]}, mgl32.Vec3{c.Display[3], c.Display[4], c.Display[5]}, mgl32.Vec3{c.Display[6], c.Display[7], c.Display[8]}}
		m := handArm(h).Mul4(displayMat(disp)).Mul4(mgl32.Translate3D(-0.5, -0.5, -0.5))
		for k := range 16 {
			if !near(m[k], c.M[k], 2e-4) {
				t.Fatalf("hand %d m[%d]: %v, want %v (in %v)", i, k, m[k], c.M[k], c.In)
			}
		}
	}
}

// TestPresets checks the weapon kinds' attacks against weapons.rec, the
// upswings from its animation records: a one-handed weapon's, a two-handed
// one's, and the kinds with the vanilla swing (launchers, missiles).
func TestPresets(t *testing.T) {
	want := map[string]string{
		"axe":            "[[{one_handed_slash_horizontal_right 0.5} {one_handed_slash_horizontal_left 0.5}] [{one_handed_slam 0.5}]]",
		"battle axe":     "[[{two_handed_slash_horizontal_right 0.5} {two_handed_spin 0.5}] [{two_handed_slash_vertical_right 0.5} {two_handed_slash_vertical_left 0.5}]]",
		"spear":          "[[{one_handed_stab 0.5}]]",
		"long bow":       "[]",
		"chain crossbow": "[]",
		"dart":           "[]",
	}
	for k, w := range want {
		if got := rogue.Weapons[rogue.WeaponID(k)].Attacks; fmt.Sprint(got) != w {
			t.Errorf("%s: %v, want %v", k, got, w)
		}
	}
}

// TestBowFrames checks items/bow's pulling pictures: use_duration (ticks
// used) scaled by 0.05 against the thresholds 0.65 and 0.9.
func TestBowFrames(t *testing.T) {
	for ticks, want := range map[int]int{0: 0, 12: 0, 13: 1, 17: 1, 18: 2, 25: 2} {
		if got := pullStage(float32(ticks)*0.05, 0.65, 0.9); got != want {
			t.Errorf("%d ticks: bow_pulling_%d, want %d", ticks, got, want)
		}
	}
}

// TestSling checks the sling against Nightfall's SlingThrow worked by
// hand: the action's keyframes through its AnimationCalculators and
// easings, and its ActionTracker's state machine.
func TestSling(t *testing.T) {
	dT, dR := animDefault(slingSwingT), animDefault(slingSwingR)
	raised := dT.Add(mgl32.Vec3{0, 0.35, 0})
	released := mgl32.Vec3{-6.0 / 16, -2.0 / 16, -2.0 / 16}
	inOut := func(p float32) float32 { return (1 - mthCos(math.Pi*p)) / 2 } // Easing.inOutSine, with Mth.cos
	cases := []struct {
		active   bool
		state, f int
		pt       float32
		t, r     mgl32.Vec3
	}{
		{false, 0, 0, 0, animDefault(slingRestT), animDefault(slingRestR)},
		{true, 0, 1, 0, dT, dR},
		{true, 0, 8, 1, raised, dR.Add(mgl32.Vec3{-240, 0, 0})},
		{true, 0, 4, 0, dT.Add(mgl32.Vec3{0, 0.35 * inOut(3.0/8), 0}), dR.Add(mgl32.Vec3{-240 * (1 - mthCos(3.0/8*math.Pi/2)), 0, 0})},
		{true, 1, 5, 0, raised, dR.Add(mgl32.Vec3{-240 - 360*0.4, 0, 0})},    // first loop, 10 ticks
		{true, 1, 15, 0.5, raised, dR.Add(mgl32.Vec3{-240 - 360*0.5, 0, 0})}, // second loop, 9 ticks from frame 10
		{true, 1, 40, 0, raised, dR.Add(mgl32.Vec3{-240 - 360*5.0/7, 0, 0})}, // the 7-tick loop repeats
		{true, 2, 4, 1, released, dR.Add(mgl32.Vec3{-840, 30, 0})},
		{true, 2, 2, 0, raised.Add(released.Sub(raised).Mul(0.578125)), dR.Add(mgl32.Vec3{-600 - 240*0.578125, 30 * 0.578125, 0})}, // outCubic(1/4)
		{true, 3, 2, 0.7, released, dR.Add(mgl32.Vec3{-840, 30, 0})},
		{true, 4, 1, 0, released, mgl32.Vec3{-170 + 0.00001, 120 + 0.00001, 0}}, // wrapped
		{true, 4, 7, 1, dT, dR},
		{true, 4, 4, 0, released.Add(dT.Sub(released).Mul(inOut(3.0 / 7))), mgl32.Vec3{-170 + 120*inOut(3.0/7), 120 - 30*inOut(3.0/7), 0}},
	}
	for i, c := range cases {
		gt, gr := slingPose(c.active, c.state, c.f, c.pt)
		for k := range 3 {
			if !near(gt[k], c.t[k], 1e-5) || !near(gr[k], c.r[k], 1e-3) {
				t.Fatalf("case %d (state %d frame %d): %v %v, want %v %v", i, c.state, c.f, gt, gr, c.t, c.r)
			}
		}
	}
	// The tracker: 8 ticks raising, the throw released at once comes at
	// the least charge (17) and launches on the release state's second
	// frame, then 3 ticks frozen and 7 returning.
	g := &game{}
	g.weapon = &rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("sling")}
	g.sling = slingAction{frame: 1}
	at := map[int]slingAction{8: {1, 1, 0, false}, 24: {2, 1, 17, true}, 25: {2, 2, 17, true}, 28: {3, 1, 17, true}, 31: {4, 1, 17, true}, 38: {}}
	for tick := 1; tick <= 38; tick++ {
		g.tickSling()
		if w, ok := at[tick]; ok && g.sling != w {
			t.Fatalf("tick %d: %+v, want %+v", tick, g.sling, w)
		}
	}
}

// TestCrossbowReload spans the chain crossbow with R: each press loads a
// chamber over the 38-tick charge (the hand showing the charge), three
// fill the magazine, the right button shoots one per 8-tick cooldown and
// chambers the next at once, and an empty crossbow shoots nothing.
func TestCrossbowReload(t *testing.T) {
	g := &game{}
	g.weapon = &rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("chain crossbow")}
	g.arrowObjs = map[*projectile.Arrow]*rogue.Object{}
	g.player = &character.Character{}
	g.resetInv()
	g.give(g.weapon) // into the held slot
	for range 9 {
		b := rogue.NewWeapon(rand.New(rand.NewSource(1)), rogue.WeaponID("crossbow bolt"))
		b.Count = 1
		g.give(b)
	}
	g.startDraw() // empty
	if use, _ := g.handUse(0); len(g.arrowObjs) != 0 || use != useNone {
		t.Fatalf("empty: %d bolts, use %v", len(g.arrowObjs), use)
	}
	for i := 1; i <= 3; i++ {
		g.startReload()
		if use, ut := g.handUse(0.5); use != useCrossbowCharge || ut != -0.5 || !g.spanning {
			t.Fatalf("spanning: use %v at %v", use, ut)
		}
		for range reloadTicks(rogue.WeaponID("chain crossbow")) {
			g.tickDraw()
		}
		if len(g.chambers) != i || g.spanning {
			t.Fatalf("load %d: %d chambers", i, len(g.chambers))
		}
	}
	g.startReload() // full
	if use, _ := g.handUse(0); use != useCrossbowHold || g.spanning {
		t.Fatalf("loaded: use %v, want hold", use)
	}
	g.startDraw()
	g.startDraw() // on the cooldown
	if len(g.chambers) != 2 || g.cooldown != rogue.Weapons[g.weapon.Which].Cooldown || len(g.arrowObjs) != 1 {
		t.Fatalf("shot: %d chambers, cooldown %d, %d bolts", len(g.chambers), g.cooldown, len(g.arrowObjs))
	}
	for range rogue.Weapons[g.weapon.Which].Cooldown {
		g.tickDraw()
	}
	g.startDraw()
	g.startDraw()
	if a := g.findAmmo(); len(g.chambers) != 1 || len(g.arrowObjs) != 2 || g.packCount() != 2 || a.Count != 6 { // the crossbow and six bolts
		t.Fatalf("second shot: %d chambers, %d bolts, %d in the pack, %d left", len(g.chambers), len(g.arrowObjs), g.packCount(), a.Count)
	}
}

// TestPatterns reads the patterns bound to the left button, the right
// button and F (attack_1..3): the horizontal slashes of a swing alternate.
func TestPatterns(t *testing.T) {
	want := map[string]string{
		"dagger":     "[[one_handed_slash_horizontal_right one_handed_slash_horizontal_left] [one_handed_stab]]",
		"long sword": "[[one_handed_slash_horizontal_right one_handed_slash_horizontal_left] [one_handed_slam] [one_handed_stab]]",
		"halberd":    "[[two_handed_slash_horizontal_right two_handed_slash_horizontal_left] [two_handed_slash_vertical_right] [two_handed_stab_right]]",
		"maul":       "[[two_handed_slash_horizontal_right two_handed_slash_horizontal_left] [two_handed_slam_heavy]]",
		"spear":      "[[one_handed_stab]]",
	}
	for k, w := range want {
		var got [][]string
		for _, pat := range rogue.Weapons[rogue.WeaponID(k)].Attacks {
			var names []string
			for _, a := range pat {
				names = append(names, a.Anim)
			}
			got = append(got, names)
		}
		if fmt.Sprint(got) != w {
			t.Errorf("%s: %v, want %v", k, got, w)
		}
	}
}

// TestWandMelee: a held staff strikes like a spear (reach, cooldown, 3/5
// the damage) with the staffs' patterns: the right button's overhead
// blow, and the left button's stab whose landing fires the zap.
func TestWandMelee(t *testing.T) {
	g := &game{}
	g.anims, g.weapon = loadAnims(t), &rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("dagger")}
	g.stack = newAttackStack()
	g.zapping = &rogue.Object{Type: rogue.Stick, Which: 2}
	if fmt.Sprint(presetFor(g.zapping)) != fmt.Sprint(rogue.StaffAttacks) || len(rogue.StaffAttacks) != 2 || rogue.StaffAttacks[1][0].Anim != "two_handed_slash_vertical_right" {
		t.Errorf("staff presets %v", presetFor(g.zapping))
	}
	g.heroAttack(1)
	if spear := rogue.Weapons[rogue.WeaponID("spear")]; g.hitReach != spear.Reach || g.attackCD != spear.Swing || g.comboOf != g.zapping || g.attack != g.anims["two_handed_slash_vertical_right"] {
		t.Errorf("reach %v cd %d combo of %v: the dagger, not the staff, fought overhead", g.hitReach, g.attackCD, g.comboOf)
	}
	g.attackCD, g.sinceAttack = 0, 100
	g.zapStab()
	if stab := rogue.StaffAttacks[0][0]; g.attack != g.anims[stab.Anim] || g.pendingZap != g.zapping || g.hitIn != int(math.Round(float64(float32(rogue.Weapons[rogue.WeaponID("spear")].Swing)*stab.Upswing))) {
		t.Errorf("stab %v pending %v hit in %d: the zap does not wait for the stab", g.attack, g.pendingZap, g.hitIn)
	}
	// the bolt leaves the top of the staff: the posed sprite's topmost
	// row, a staff's length before the eye, not the fixed spot by the hand
	g.rg, g.player, g.sprite = rogue.NewGame(rand.New(rand.NewSource(1))), &character.Character{EyeHeight: 1.62}, map[string]image.Image{}
	g.cam.Y = 1.62
	head := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	head.Set(1, 0, color.White) // the head top left, the stick down to the right
	head.Set(14, 15, color.White)
	g.sprite[spriteFor(g.rg, g.zapping)] = head
	for range g.hitIn {
		g.stack.handle(0, true, true)
	}
	x, y, z := g.wandTip()
	g.zapping = nil
	hx, hy, hz := g.wandTip()
	if d := math.Hypot(math.Hypot(x-hx, y-hy), z-hz); d < 0.3 || d > 2 || x == 0 && y == 0 && z == 0 {
		t.Errorf("tip at %.2f %.2f %.2f, %.2f from the hand spot %.2f %.2f %.2f", x, y, z, d, hx, hy, hz)
	}
}

// TestBleed: a hit bleeds VoxelLib's blood burst, and only wands count as
// magic (no blood).
func TestBleed(t *testing.T) {
	g := &game{}
	g.bleed(1, 2, 3)
	if len(g.parts.P) == 0 {
		t.Error("bleed emitted nothing")
	}
	if !magic(&rogue.Object{Type: rogue.Stick}) || magic(&rogue.Object{Type: rogue.Weapon, Which: rogue.WeaponID("dagger")}) || magic(nil) {
		t.Error("magic: wands only")
	}
}
