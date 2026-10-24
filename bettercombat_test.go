package main

// Checks the vanilla side against Minecraft 26.2's own classes, run
// headless (testdata/vanilla.json): HumanoidModel poses,
// WalkAnimationState, LivingEntity's body yaw (with Mth.atan2) and the
// first-person render chain's matrices.

import (
	"encoding/json"
	"fmt"
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
		_, armM, itemM := bcChain(body, arm, item, c.BodyRot, disp)
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

// TestPresets checks the weapon kinds' Better Combat attacks against the
// mods' data: a registered item with a parent chain (hatchet), one with
// its own attacks (claymore through the game's data), the fallback by
// name (spear, rapier), and the kinds Better Combat leaves to vanilla
// (ranged items have no attacks, a pole matches nothing).
func TestPresets(t *testing.T) {
	if err := loadWeaponAttributes("assets/weapon_attributes"); err != nil {
		t.Fatal(err)
	}
	want := map[int][]bcAttack{
		rogue.Hatchet:       {{"one_handed_slash_horizontal_right", 0.3}, {"one_handed_slash_horizontal_left", 0.3}},
		rogue.TwoSword:      {{"two_handed_slash_horizontal_right", 0.5}, {"two_handed_stab_left", 0.5}, {"two_handed_slam", 0.5}},
		rogue.Spear:         {{"two_handed_stab_right", 0.5}},
		rogue.Rapier:        {{"one_handed_slash_horizontal_left", 0.5}, {"one_handed_stab", 0.5}},
		rogue.Bow:           nil,
		rogue.ChainCrossbow: nil,
		rogue.WoodenPole:    nil,
	}
	for k, w := range want {
		if got := weaponPresets[k]; fmt.Sprint(got) != fmt.Sprint(w) {
			t.Errorf("%s: %v, want %v", rogue.WeaponNames[k], got, w)
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
	g.weapon = &rogue.Object{Type: rogue.Weapon, Which: rogue.Sling}
	g.sling = slingAction{frame: 1}
	at := map[int]slingAction{8: {1, 1, 0, false}, 24: {2, 1, 17, true}, 25: {2, 2, 17, true}, 28: {3, 1, 17, true}, 31: {4, 1, 17, true}, 38: {}}
	for tick := 1; tick <= 38; tick++ {
		g.tickSling()
		if w, ok := at[tick]; ok && g.sling != w {
			t.Fatalf("tick %d: %+v, want %+v", tick, g.sling, w)
		}
	}
}

// TestChainCrossbow checks weapons_expanded's chain crossbow: a chamber
// loads over the 38-tick charge (an early release loads nothing), three
// fill the magazine, the left button shoots one per 8-tick cooldown and
// chambers the next at once, and melee waits while one is loaded.
func TestChainCrossbow(t *testing.T) {
	g := &game{}
	g.weapon = &rogue.Object{Type: rogue.Weapon, Which: rogue.ChainCrossbow}
	g.arrowObjs = map[*projectile.Arrow]*rogue.Object{}
	g.player = &character.Character{}
	for range 9 {
		b := rogue.NewWeapon(rand.New(rand.NewSource(1)), rogue.Bolt)
		b.Count = 1
		g.pack = append(g.pack, b)
	}
	load := func(ticks int) {
		g.startDraw()
		for range ticks {
			g.tickDraw()
		}
		g.releaseDraw()
	}
	load(37)
	if use, _ := g.handUse(0); len(g.chambers) != 0 || use != useNone {
		t.Fatalf("early release: %d chambers, use %v", len(g.chambers), use)
	}
	for i := 1; i <= 4; i++ {
		load(38)
		if len(g.chambers) != min(i, chainMagazine) || !g.chainLoaded() {
			t.Fatalf("load %d: %d chambers", i, len(g.chambers))
		}
	}
	if use, _ := g.handUse(0); use != useCrossbowHold {
		t.Fatalf("loaded: use %v, want hold", use)
	}
	g.fireChain()
	g.fireChain() // on the cooldown
	if len(g.chambers) != 2 || g.cooldown != chainCooldown || len(g.arrowObjs) != 1 {
		t.Fatalf("shot: %d chambers, cooldown %d, %d bolts", len(g.chambers), g.cooldown, len(g.arrowObjs))
	}
	g.startDraw() // topping up puts the loaded chambers aside
	if use, ut := g.handUse(0.5); use != useCrossbowCharge || ut != -0.5 || g.chainLoaded() {
		t.Fatalf("loading: use %v at %v, loaded %v", use, ut, g.chainLoaded())
	}
	g.releaseDraw() // at once: nothing loaded
	for range chainCooldown {
		g.tickDraw()
	}
	g.fireChain()
	g.fireChain()
	if len(g.chambers) != 1 || len(g.arrowObjs) != 2 || len(g.pack) != 6 {
		t.Fatalf("second shot: %d chambers, %d bolts, %d in the pack", len(g.chambers), len(g.arrowObjs), len(g.pack))
	}
}
