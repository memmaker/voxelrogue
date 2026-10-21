package main

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxelrogue/rogue"
)

// Controls (keep helpLines in hud.go in sync):
//
//	WASD walk, mouse look, Space jump, LMB attack, RMB fire/zap (hold to
//	draw a bow; a crossbow repeats while held),
//	F search, < climb (with the Amulet), Tab/I inventory, M map, F1 help,
//	F9 debug weapon menu, 3 next weapon, Esc release mouse (again: quit), R new game
//	after death or victory.
type inputState struct {
	captured  bool
	showInv   bool
	showMap   bool
	showHelp  bool
	showDebug bool // F9: every weapon, to try them out
	prompt    byte // 'i' identify, 'g' genocide: the next key answers it
	detect    byte // map highlight after a detection potion/scroll
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
		if g.showDebug && k >= glfw.KeyA && k < glfw.KeyA+rogue.MaxWeapons {
			g.debugWeapon(int(k - glfw.KeyA))
			return
		}
		if g.showInv && k >= glfw.KeyA && k <= glfw.KeyW {
			g.invKey(int(k-glfw.KeyA), m&glfw.ModShift != 0)
			return
		}
		switch k {
		case glfw.KeyEscape:
			switch {
			case g.showInv || g.showMap || g.showHelp || g.showDebug:
				g.showInv, g.showMap, g.showHelp, g.showDebug, g.detect = false, false, false, false, 0
				if g.prompt == 'i' {
					g.prompt = 0
				}
			case g.captured:
				g.capture(false)
			default:
				w.SetShouldClose(true)
			}
		case glfw.KeyTab, glfw.KeyI:
			g.showDebug = false
			g.showInv = !g.showInv
		case glfw.KeyM:
			g.showMap = !g.showMap
			g.detect = 0
		case glfw.KeyF1:
			g.showHelp = !g.showHelp
		case glfw.KeyF9:
			g.showDebug = !g.showDebug
			g.showInv = false
		case glfw.KeyComma:
			if m&glfw.ModShift != 0 && g.noCommand == 0 {
				g.ascend()
			}
		case glfw.Key0 + 3:
			g.nextWeapon()
		}
	})
	g.win.SetMouseButtonCallback(func(w *glfw.Window, b glfw.MouseButton, a glfw.Action, m glfw.ModifierKey) {
		if b == glfw.MouseButtonRight && a == glfw.Release {
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
		switch {
		case b != glfw.MouseButtonRight || g.dead || g.won || g.noCommand > 0:
		case g.zapping == nil && g.launcher():
			g.startDraw()
		case g.attackCD == 0:
			g.fire()
		}
	})
	lastX, lastY := g.win.GetCursorPos()
	g.win.SetCursorPosCallback(func(w *glfw.Window, x, y float64) {
		dx, dy := x-lastX, y-lastY
		lastX, lastY = x, y
		if !g.captured || g.dead || g.won {
			return
		}
		const sens = 0.0025
		g.cam.Yaw += dx * sens
		g.cam.Pitch = math.Max(-1.55, math.Min(1.55, g.cam.Pitch-dy*sens))
	})
}

// invKey handles a pack letter on the inventory screen: use it, or drop
// it with Shift.
func (g *game) invKey(i int, drop bool) {
	if i >= len(g.pack) {
		return
	}
	o := g.pack[i]
	switch {
	case g.prompt == 'i':
		g.prompt = 0
		g.identify(o)
	case drop:
		g.dropItem(o)
	case g.noCommand > 0:
		g.msg("You can't move.")
		return
	default:
		g.useItem(o)
	}
	if g.prompt == 0 {
		g.showInv = false
	}
}

// debugWeapon puts a new, identified weapon of a kind into the pack
// (missiles come as a bundle, as NewWeapon rolls them).
func (g *game) debugWeapon(which int) {
	o := rogue.NewWeapon(g.rng, which)
	o.Flags |= rogue.IsKnow
	g.addPack(o)
}

// nextWeapon wields the pack's next weapon after the current one.
func (g *game) nextWeapon() {
	at := -1
	for i, o := range g.pack {
		if o == g.weapon {
			at = i
		}
	}
	for n := 1; n <= len(g.pack); n++ {
		if o := g.pack[(at+n+len(g.pack))%len(g.pack)]; o.Type == rogue.Weapon {
			g.weapon = o
			g.msg("You are now wielding " + lowerFirst(g.rg.Name(o)) + ".")
			return
		}
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
	if g.dead || g.won || g.showInv || g.showDebug {
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
	return in
}

func (g *game) wantAttack() bool {
	return !g.automated && g.captured && !g.won && !g.showInv && !g.showDebug && g.win.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press
}

func (g *game) wantSearch() bool { return g.key(glfw.KeyF) }
