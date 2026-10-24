package main

import (
	"math"
	"slices"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxelrogue/rogue"
)

// Controls (keep helpLines in hud.go in sync):
//
//	WASD walk, Shift (tap) sprint until you stop, Ctrl (hold) sneak, mouse look, Space jump, LMB a melee weapon's first attack
//	pattern or fire a launcher (hold to draw a bow; a spanned crossbow
//	shoots), RMB the second pattern, cancel a bow's draw or a sling's
//	whirl, zap a held wand or use the held food, potion, scroll, ring or
//	armour, F the third pattern, T throw the wielded weapon, R span a
//	crossbow, 1-9 and the wheel hold a hotbar slot, C search, < climb
//	(with the Amulet), E/Tab/I inventory, M map (on a panel, over the
//	view, off), F1 help, F2 manual, F9 debug weapon menu, Esc release
//	mouse (again: quit), R new game after death or victory.
type inputState struct {
	captured   bool
	showMap    int // M cycles: 0 off, 1 the map on a dark panel, 2 the map laid over the view (Diablo style)
	showHelp   bool
	showManual bool // F2: the rules
	showDebug  bool // F9: every weapon, to try them out
	sprinting  bool // Shift tapped: sprint until the hero stops walking forward
	sneaking   bool // Ctrl held: slow and silent
	prompt     byte // 'i' identify, 'g' genocide: the next key answers it
	detect     byte // map highlight after a detection potion/scroll
}

func (g *game) setupInput() {
	g.capture(true)
	g.win.SetKeyCallback(func(w *glfw.Window, k glfw.Key, sc int, a glfw.Action, m glfw.ModifierKey) {
		if a != glfw.Press {
			return
		}
		if g.dead || g.won { // the game is over: only a new game or quitting
			switch k {
			case glfw.KeyR:
				g.newGame(0)
			case glfw.KeyEscape:
				w.SetShouldClose(true)
			}
			return
		}
		if g.prompt == 'g' {
			if k >= glfw.KeyA && k <= glfw.KeyZ {
				g.prompt = 0
				g.genocide(byte('A' + k - glfw.KeyA))
			}
			return
		}
		if g.pg.Key(int(k), int(a), int(m)) { // the inventory, its keys and the hotbar's
			return
		}
		if g.showDebug && k >= glfw.KeyA && k < glfw.KeyA+glfw.Key(len(rogue.Weapons)) {
			g.debugWeapon(int(k - glfw.KeyA))
			return
		}
		switch k {
		case glfw.KeyEscape:
			switch {
			case g.showMap != 0 || g.showHelp || g.showManual || g.showDebug:
				g.showMap, g.showHelp, g.showManual, g.showDebug, g.detect = 0, false, false, false, 0
			case g.captured:
				g.capture(false)
			default:
				w.SetShouldClose(true)
			}
		case glfw.KeyLeftShift, glfw.KeyRightShift:
			g.sprinting = true
		case glfw.KeyM:
			g.showMap = (g.showMap + 1) % 3
			g.detect = 0
		case glfw.KeyF1:
			g.showHelp = !g.showHelp
		case glfw.KeyF2:
			g.showManual = !g.showManual
		case glfw.KeyF3:
			g.showBoxes = !g.showBoxes
		case glfw.KeyF9:
			g.showDebug = !g.showDebug
		case glfw.KeyComma:
			if m&glfw.ModShift != 0 {
				g.sprinting = false // the Shift was for <, not a sprint tap
			}
			if m&glfw.ModShift != 0 && g.noCommand == 0 {
				g.ascend()
			}
		case glfw.KeyT:
			if g.attackCD == 0 && g.noCommand == 0 {
				g.throwItem()
			}
		case glfw.KeyR:
			if g.launcher() && g.noCommand == 0 {
				g.startReload()
			}
		}
	})
	g.win.SetMouseButtonCallback(func(w *glfw.Window, b glfw.MouseButton, a glfw.Action, m glfw.ModifierKey) {
		if g.pg.Button(int(b), int(a), int(m)) {
			return
		}
		if b == glfw.MouseButtonLeft && a == glfw.Release {
			g.releaseDraw()
			return
		}
		if a != glfw.Press {
			return
		}
		if !g.captured {
			g.capture(true)
			return
		}
		switch { // melee patterns are polled (wantPattern)
		case g.dead || g.won || g.noCommand > 0:
		case b == glfw.MouseButtonLeft && g.launcher():
			g.startDraw()
		case b == glfw.MouseButtonRight && (g.drawing || g.sling.state <= slingCharge && g.sling.frame != 0):
			g.cancelDraw()
		case b == glfw.MouseButtonRight && g.zapping != nil && g.attackCD == 0:
			g.zap(g.zapping)
		case b == glfw.MouseButtonRight && g.heldUse() != nil && g.attackCD == 0:
			g.useItem(g.heldUse())
		}
	})
	g.win.SetScrollCallback(func(w *glfw.Window, dx, dy float64) {
		if !g.dead && !g.won {
			g.pg.Scroll(dx, dy)
		}
	})
	lastX, lastY := g.win.GetCursorPos()
	g.win.SetCursorPosCallback(func(w *glfw.Window, x, y float64) {
		dx, dy := x-lastX, y-lastY
		lastX, lastY = x, y
		if g.pg.Cursor(x, y) {
			return
		}
		if !g.captured || g.dead || g.won {
			return
		}
		const sens = 0.0025
		g.cam.Yaw += dx * sens
		g.cam.Pitch = math.Max(-1.55, math.Min(1.55, g.cam.Pitch-dy*sens))
	})
}

// debugWeapon puts a new, identified weapon of a kind into the pack
// (missiles come as a bundle, as NewWeapon rolls them).
func (g *game) debugWeapon(which int) {
	o := rogue.NewWeapon(g.rng, which)
	o.Flags |= rogue.IsKnow
	g.addPack(o)
}

// nextWeapon (3, a debug key) wields the next weapon kind, with one of
// every kind to try: the pack's if it has one, else a fresh one.
func (g *game) nextWeapon() {
	which := -1 // bare hands (after a throw): the first kind
	if g.weapon != nil {
		which = g.weapon.Which
	}
	for which = (which + 1) % len(rogue.Weapons); rogue.IsAmmo(which); which = (which + 1) % len(rogue.Weapons) { // ammo
	}
	o := rogue.NewWeapon(g.rng, which)
	o.Flags |= rogue.IsKnow
	for _, p := range g.pack() {
		if p.Type == rogue.Weapon && p.Which == which {
			o = p
		}
	}
	if !slices.Contains(g.pack(), o) {
		if g.packCount() >= maxPack && g.weapon != nil { // ponytail: a full pack swaps the fresh weapon in for the wielded one
			g.removeFromPack(g.weapon, true)
		}
		g.give(o)
	}
	g.place(o, g.inv.Held)
	g.hold()
	g.giveAmmo()
}

// giveAmmo (debug) puts a bundle of the wielded launcher's ammunition
// (the first projectile kind it launches) in the pack if it has none, so
// every launcher can be tried.
func (g *game) giveAmmo() {
	a := slices.IndexFunc(rogue.Weapons, func(w rogue.WeaponKind) bool { return slices.Contains(w.Launchers, rogue.Weapons[g.weapon.Which].ID) })
	if f := g.findAmmo(); a >= 0 && (f == nil || !rogue.Launches(g.weapon.Which, f)) {
		g.debugWeapon(a)
	}
}

func (g *game) capture(on bool) {
	if g.automated {
		return
	}
	g.captured = on
	if on {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		if glfw.RawMouseMotionSupported() {
			g.win.SetInputMode(glfw.RawMouseMotion, glfw.True)
		}
	} else {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
}

func (g *game) key(k glfw.Key) bool {
	return !g.automated && g.captured && g.win.GetKey(k) == glfw.Press
}

// moveInput reads the walking keys.
func (g *game) moveInput() character.Input {
	in := character.Input{Yaw: g.cam.Yaw, Pitch: g.cam.Pitch}
	if g.dead || g.won || g.pg.IsOpen() || g.showDebug {
		return in
	}
	if g.key(glfw.KeyW) {
		in.Forward++
	}
	if g.key(glfw.KeyS) {
		in.Forward--
	}
	if g.key(glfw.KeyD) {
		in.Strafe++
	}
	if g.key(glfw.KeyA) {
		in.Strafe--
	}
	in.Jump = g.key(glfw.KeySpace)
	in.Sneak = g.key(glfw.KeyLeftControl) || g.key(glfw.KeyRightControl)
	g.sneaking = in.Sneak
	return in
}

func (g *game) wantAttack(b glfw.MouseButton) bool {
	return !g.automated && g.captured && !g.won && !g.pg.IsOpen() && !g.showDebug && g.win.GetMouseButton(b) == glfw.Press
}

// wantPattern is the melee attack pattern held down: 0 left button, 1
// right button, 2 F; -1 none (a launcher fires instead, and a held wand
// zaps on the right button).
func (g *game) wantPattern() int {
	switch {
	case g.launcher():
	case g.wantAttack(glfw.MouseButtonLeft):
		return 0
	case g.zapping == nil && g.heldUse() == nil && g.wantAttack(glfw.MouseButtonRight):
		return 1
	case g.key(glfw.KeyF):
		return 2
	}
	return -1
}

func (g *game) wantSearch() bool { return g.key(glfw.KeyC) }
