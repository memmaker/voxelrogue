// Command voxelrogue is Rogue 3.6 as a first-person, real-time voxel game:
// the original level generator, monsters, objects and dice, drawn and
// played in 3D with voxellib's WebGPU renderer.
//
//	./build_n_run.sh            build (WebGPU backend) and run
//	./build_n_run.sh -seed 42   a particular dungeon
//
// Files:
//
//	main.go      flags, window, engine, frame loop
//	world.go     levels as voxels (the chunk generator)
//	hero.go      the hero: stats, turns, daemons, traps, stairs
//	items.go     objects on the floor, the pack and using things
//	arrows.go    bows, the crossbow and arrows in flight
//	handanim.go  the held weapon's Minecraft first-person pose and swing
//	bettercombat.go  Better Combat's melee attack animations
//	debugview.go  -tp and -anim: attacks on the whole model, for checking
//	monsters.go  monster behaviour in real time and the fights
//	hud.go       messages, status line, inventory, map
//	input.go     keyboard and mouse
//	standins.go  placeholder monster models and item sprites
package main

import (
	"flag"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/respack"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

func init() { runtime.LockOSThread() }

const (
	tickDur = time.Second / 20
	fov     = 70 * math.Pi / 180
)

func main() {
	var (
		seed    = flag.Int64("seed", 0, "dungeon seed (0 = random)")
		libDir  = flag.String("voxellib", "../VoxelLib", "voxellib checkout (texture pack and item sprites)")
		shot    = flag.String("shot", "", "save a screenshot PNG at tick -shot-at and quit (never grabs input)")
		shotAt  = flag.Int("shot-at", 120, "tick for -shot")
		depth   = flag.Int("depth", 1, "debug: start on this level")
		look    = flag.Float64("look", -1, "debug: start yaw in degrees")
		pitch   = flag.Float64("pitch", 0, "debug: start pitch in degrees (up is positive)")
		wizard  = flag.Bool("wizard", false, "debug: show the whole map")
		dumpMap = flag.Bool("dump", false, "debug: print the levels and quit")
		wield   = flag.Int("wield", -1, "debug: start wielding this weapon kind")
		swingAt = flag.Int("swing", -1, "debug: with -shot, swing (draw a wielded launcher) this many ticks before the shot")
		combo   = flag.Int("combo", 0, "debug: with -swing, the attack of the weapon's combo to play")
		amulet  = flag.Bool("amulet", false, "debug: start with the Amulet of Yendor")
		renew   = flag.Int("renew", -1, "debug: start a new game (as R does) at this tick")
		renewTo = flag.Int64("renew-seed", 0, "debug: with -renew, the new game's seed (0 = random)")
		tp      = flag.Bool("tp", false, "debug: show attacks in third person, the whole model seen from the front")
		anim    = flag.String("anim", "", "debug: always show this Better Combat animation, at -anim-at")
		animAt  = flag.Float64("anim-at", 0, "debug: with -anim, the animation tick to show")
	)
	flag.Parse()
	if render.Backend != "wgpu" {
		log.Fatal("voxelrogue uses the WebGPU backend: build with -tags wgpu (see build_n_run.sh)")
	}
	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	levels := buildLevels(*seed)
	if *dumpMap {
		for _, l := range levels[:2] {
			os.Stdout.WriteString(l.String())
		}
		return
	}

	if err := glfw.Init(); err != nil {
		log.Fatal(err)
	}
	defer glfw.Terminate()
	render.WindowHints()
	if *shot != "" {
		glfw.WindowHint(glfw.Focused, glfw.False)
		glfw.WindowHint(glfw.FocusOnShow, glfw.False)
	}
	win, err := glfw.CreateWindow(1280, 800, "VoxelRogue", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	if err := render.Init(win, true); err != nil {
		log.Fatal(err)
	}
	pack, err := respack.Open(*libDir + "/assets/rpg16pack")
	if err != nil {
		log.Fatal(err)
	}
	defer pack.Close()
	reg := voxel.NewRegistry()
	reg.Resolver = pack.Resolver()
	gen := newLevelGen(reg, levels)
	maxChunk := voxel.ChunkPos{X: (dungeon.Cols*tileW - 1) / 16, Z: (dungeon.Lines*tileD - 1) / 16}
	eng, err := engine.New(engine.Config{
		Registry:       reg,
		RenderDistance: 6,
		Generator:      gen,
		MinY:           worldMinY,
		Height:         worldHeight,
		// A rim of rock two chunks wide: a chunk meshes only when its
		// neighbours are lit, which needs their neighbours loaded.
		Bounds:        &[2]voxel.ChunkPos{{X: -2, Z: -2}, maxChunk.Add(2, 2)},
		Textures:      pack,
		RenderOptions: render.Options{Missing: respack.MissingTexture, IsGreyscale: respack.IsGreyscale, BlockBrightness: 0.3},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()
	eng.Renderer.Opt.Daylight = 0
	eng.Renderer.Opt.Fog = &render.Fog{Color: mgl32.Vec3{0, 0, 0}, Start: 18, End: 56}

	rng := rand.New(rand.NewSource(*seed))
	g := &game{eng: eng, win: win, gen: gen, rng: rng, rg: rogue.NewGame(rng),
		automated: *shot != "", wizard: *wizard}
	g.terrain = &terrain{eng: eng}
	g.cam = render.Camera{FOV: fov, Near: 0.05}
	g.initHero()
	if err := g.setupArt(*libDir+"/assets/items", "assets/weapons"); err != nil {
		log.Fatal(err)
	}
	g.tp, g.animName, g.animAt = *tp, *anim, float32(*animAt)
	if err := g.setupDebugView(); err != nil {
		log.Fatal(err)
	}
	g.enterLevel(*depth)
	if *wield >= 0 {
		for _, o := range g.pack {
			if o.Type == rogue.Weapon && o.Which == *wield {
				g.weapon = o
			}
		}
		if g.weapon == nil || g.weapon.Which != *wield { // past the pack's limit
			g.weapon = rogue.NewWeapon(g.rng, *wield)
			g.weapon.Flags |= rogue.IsKnow
			g.pack = append(g.pack, g.weapon)
		}
		if isCrossbow(*wield) {
			g.debugWeapon(rogue.Bolt)
		}
	}
	if *amulet {
		g.addPack(&rogue.Object{Type: rogue.Amulet, Count: 1})
		g.amulet = true
	}
	g.combo, g.comboOf = *combo, g.weapon
	if *swingAt >= 0 {
		g.swingShot = *shotAt - *swingAt
	}
	g.renewAt, g.renewSeed = *renew, *renewTo
	if *look >= 0 {
		g.cam.Yaw = *look * math.Pi / 180
	}
	g.cam.Pitch = *pitch * math.Pi / 180
	if !g.automated {
		g.setupInput()
	}
	g.run(*shot, *shotAt)
}

// game is the whole running game.
type game struct {
	eng     *engine.Engine
	win     *glfw.Window
	cam     render.Camera
	terrain *terrain
	gen     *levelGen
	rng     *rand.Rand
	rg      *rogue.Game
	player  *character.Character

	depth int
	tick  int
	ready bool // terrain under the hero is loaded

	heroState
	monsterState
	itemState
	arrowState
	combatState
	hudState
	inputState
	debugView

	automated bool // -shot: no input
	wizard    bool
	swingShot int   // -swing: the tick to attack on
	renewAt   int   // -renew: the tick to start a new game on
	renewSeed int64 // its seed
}

// newGame starts over in this process (R after death or victory): a new
// dungeon from seed (0 = random) in place of the old one, which the
// engine drops and streams in again, and a new hero on level 1. What was
// loaded once (renderers, models, animations) stays.
func (g *game) newGame(seed int64) {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	g.gen.reset(buildLevels(seed))
	g.eng.Regenerate()
	g.rng = rand.New(rand.NewSource(seed))
	g.rg = rogue.NewGame(g.rng)
	g.depth, g.ready = 0, false
	g.heroState = heroState{}
	g.monsters, g.distTick = nil, -1
	g.floor, g.confuseHit, g.zapping = nil, false, nil
	g.clearArrows()
	g.drawing, g.drawTicks, g.fireHeld, g.reload = false, 0, false, 0
	g.handItem, g.handHeight, g.prevHandHeight, g.fovMod = nil, 0, 0, 0
	g.attack, g.hitIn, g.combo, g.comboOf, g.sinceAttack = nil, 0, 0, nil, 0
	if g.stack != nil {
		g.stack = newAttackStack()
	}
	g.msgs, g.winLoot = nil, nil
	g.showInv, g.showMap, g.showHelp, g.showDebug, g.prompt, g.detect = false, false, false, false, 0, 0
	g.initHero()
	g.enterLevel(1)
}

// slot is where the current level is in the world: its depth.
func (g *game) slot() int { return g.depth }

func (g *game) level() *dungeon.Level { return g.gen.level(g.slot()) }

// heroTile is the map square the hero stands on.
func (g *game) heroTile() dungeon.Coord { return tileAt(g.player.X, g.player.Z) }

func (g *game) run(shotPath string, shotAt int) {
	last := time.Now()
	acc := time.Duration(0)
	for !g.win.ShouldClose() {
		glfw.PollEvents()
		now := time.Now()
		acc = min(acc+now.Sub(last), 250*time.Millisecond)
		last = now
		for acc >= tickDur {
			g.step()
			acc -= tickDur
		}
		alpha := float64(acc) / float64(tickDur)
		ix, iy, iz := g.player.Lerp(alpha)
		g.eng.Update(ix, iy, iz)

		fw, fh := g.win.GetFramebufferSize()
		capture := shotPath != "" && g.tick >= shotAt
		if capture {
			render.CaptureNextFrame()
		}
		render.BeginFrame(fw, fh, 0, 0, 0)
		g.cam.X, g.cam.Y, g.cam.Z = ix, iy+g.player.EyeHeight, iz
		g.cam.Aspect = float64(fw) / float64(max(fh, 1))
		g.cam.FOV = fov * g.fovZoom(alpha) // a drawn bow zooms in
		g.eng.Render(&g.cam)
		g.drawMonsters(alpha)
		g.drawArrows(alpha)
		g.drawFloorItems(alpha)
		g.drawHand(alpha)
		g.eng.Renderer.DrawCrosshair(float32(g.cam.Aspect))
		g.drawHUD(fw, fh)
		if capture {
			if err := writePNG(shotPath, render.ReadFrame(fw, fh)); err != nil {
				log.Print(err)
			}
			return
		}
		render.EndFrame(g.win)
	}
}

// step is one 20 Hz tick of the whole game.
func (g *game) step() {
	g.tick++
	if g.tick == g.renewAt {
		g.newGame(g.renewSeed)
	}
	p := g.player
	if !g.eng.ChunkReady(voxel.ChunkPos{X: int(math.Floor(p.X)) >> 4, Z: int(math.Floor(p.Z)) >> 4}) {
		p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
		return
	}
	g.ready = true
	if g.dead || g.won {
		return
	}
	if g.tick == g.swingShot {
		if g.launcher() {
			g.startDraw()
		} else {
			g.sinceAttack = 0 // keep -combo's attack
			g.heroAttack()
		}
	}
	g.tickDraw()
	g.tickHero()
	g.tickMonsters()
	g.tickArrows()
	g.tickHUD()
}

// terrain adapts the engine's world to character.World; unloaded chunks
// are solid.
type terrain struct{ eng *engine.Engine }

func (t *terrain) Solid(x, y, z int) bool {
	w := t.eng.World
	if w.Chunk(voxel.ChunkPos{X: x >> 4, Z: z >> 4}) == nil {
		return true
	}
	return w.Reg.Info(w.Block(x, y, z)).Collides
}

func (t *terrain) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	return t.eng.World.Boxes(area, out)
}

func (t *terrain) Liquid(x, y, z int) character.Liquid { return character.LiquidNone }

func (g *game) light(x, y, z float64) (uint8, uint8) {
	return g.eng.World.Light(int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z)))
}
