package main

// The dungeon as voxels: every Rogue level is a slab of rock, the levels
// stacked downward, each built once and kept. A map tile is tileW blocks
// wide (x) and tileD deep (z): Rogue was drawn in text cells about twice
// as tall as wide, so this keeps rooms and passages in the proportions
// players saw. A level's stairs are a real staircase: from the stairs
// tile it runs east or west and down through the rock, levelPitch steps,
// and comes out of the ceiling of a room on the level below (dungeon.New
// builds that level around its landing). The engine streams chunks
// from levelGen, which reads only the levels plus what was found since
// (secret doors, traps), so a chunk generated again comes back the same.

import (
	"math"
	"math/rand"
	"strings"
	"sync"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxelrogue/dungeon"
)

const (
	tileW       = 2  // blocks per map column (x)
	tileD       = 4  // blocks per map row (z)
	levelPitch  = 12 // blocks between level floors: tileW*dungeon.StairTiles
	maxDepth    = 40 // levels in the world; Rogue's has no bottom
	topFloorY   = 436
	worldMinY   = -64
	worldHeight = 512
	roomHeight  = 4 // air above the floor in rooms
	passHeight  = 3 // in passages and doors
	stairAir    = 3 // headroom above a step of the staircase
	lanternName = "voxelrogue:lantern"
)

// floorY is the y of the floor blocks of a level; the hero stands at +1.
func floorY(depth int) int { return topFloorY - levelPitch*depth }

// depthAt is the level whose floor is nearest under a standing height y:
// halfway down a staircase the hero is on the level below.
func depthAt(y float64) int {
	return int(math.Floor((float64(topFloorY) - (y - 1) + levelPitch/2) / levelPitch))
}

// tileCenter is the world position of the middle of a tile's floor.
func tileCenter(depth int, c dungeon.Coord) (x, y, z float64) {
	return float64(c.X*tileW) + tileW/2.0, float64(floorY(depth) + 1), float64(c.Y*tileD) + tileD/2.0
}

// tileAt is the tile under world x,z.
func tileAt(x, z float64) dungeon.Coord {
	return dungeon.Coord{X: int(math.Floor(x / tileW)), Y: int(math.Floor(z / tileD))}
}

// buildLevels rolls the whole dungeon for a seed, each level under the
// one above; linear: as chains of rooms (the Linear option).
func buildLevels(seed int64, linear bool) []*dungeon.Level {
	levels := make([]*dungeon.Level, maxDepth)
	for d := range levels {
		var above *dungeon.Level
		if d > 0 {
			above = levels[d-1]
		}
		levels[d] = dungeon.New(d+1, rand.New(rand.NewSource(seed*31+int64(d))), above, linear)
	}
	return levels
}

type blocks struct {
	rock, wall, roomFloor, darkFloor, passFloor, ceiling, passCeiling, glow, trap, doorFrame, lantern voxel.StateID
	stairs                                                                                            [2]voxel.StateID // going down east, west
}

// levelGen implements worldgen.Generator for the stacked levels.
type levelGen struct {
	levels []*dungeon.Level // index depth-1
	b      blocks
	mu     sync.RWMutex
	found  map[[3]int]bool // depth, x, y of found secret doors and traps
	// sealed are the levels whose staircase down is filled in above its
	// halfway point: the hero went down without the Amulet.
	sealed map[int]bool
}

// cube registers a solid block whose every face shows the texture
// textures.rec maps under tex (remap_textures.sh).
func cube(reg *voxel.Registry, tex string, light uint8) voxel.StateID {
	name := "rogue:" + strings.ReplaceAll(tex, " ", "_")
	reg.Register(&voxel.BlockDef{Name: name, Shape: voxel.ShapeCube, Textures: [6]string{tex, tex, tex, tex, tex, tex},
		Opaque: true, Collides: true, LightOpacity: 15, LightEmission: light})
	return reg.Default(name)
}

func newLevelGen(reg *voxel.Registry, levels []*dungeon.Level) *levelGen {
	reg.Register(&voxel.BlockDef{Name: lanternName, Shape: voxel.ShapeNone, Replaceable: true, LightEmission: 8})
	return &levelGen{levels: levels, found: map[[3]int]bool{}, b: blocks{
		rock:        cube(reg, "corridor wall", 0),
		wall:        cube(reg, "room wall", 0),
		roomFloor:   cube(reg, "room floor", 0),
		darkFloor:   cube(reg, "dark room floor", 0),
		passFloor:   cube(reg, "corridor floor", 0),
		ceiling:     cube(reg, "room ceiling", 0),
		passCeiling: cube(reg, "corridor ceiling", 0),
		glow:        cube(reg, "room light", 15),
		trap:        cube(reg, "trap", 0),
		// the pack's models, their textures mapped to "stairs" and "door frame" (tiles.go)
		stairs:    [2]voxel.StateID{reg.Parse("cobblestone_stairs[facing=west,half=bottom]"), reg.Parse("cobblestone_stairs[facing=east,half=bottom]")},
		doorFrame: reg.Parse("oak_log[axis=y]"),
		lantern:   reg.Default(lanternName),
	}}
}

// markFound records a found secret door or trap so regenerated chunks
// show it.
func (g *levelGen) markFound(depth int, c dungeon.Coord) {
	g.mu.Lock()
	g.found[[3]int{depth, c.X, c.Y}] = true
	g.mu.Unlock()
}

func (g *levelGen) isFound(depth int, c dungeon.Coord) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.found[[3]int{depth, c.X, c.Y}]
}

// seal fills in (or with false reopens) the upper half of level depth's
// staircase down; the game redraws it (redrawStairs).
func (g *levelGen) seal(depth int, on bool) {
	g.mu.Lock()
	if g.sealed == nil {
		g.sealed = map[int]bool{}
	}
	g.sealed[depth] = on
	g.mu.Unlock()
}

func (g *levelGen) isSealed(depth int) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.sealed[depth]
}

func (g *levelGen) level(depth int) *dungeon.Level { return g.levels[depth-1] }

// reset swaps in a new dungeon (a new game); the engine must regenerate.
func (g *levelGen) reset(levels []*dungeon.Level) {
	g.mu.Lock()
	g.levels, g.found, g.sealed = levels, map[[3]int]bool{}, nil
	g.mu.Unlock()
}

// Generate implements worldgen.Generator.
func (g *levelGen) Generate(w *voxel.World, ch *voxel.Chunk) {
	g.mu.RLock()
	levels := g.levels // the game may reset it while workers generate
	g.mu.RUnlock()
	for lz := 0; lz < 16; lz++ {
		for lx := 0; lx < 16; lx++ {
			g.fill(levels, ch.Pos.X*16+lx, ch.Pos.Z*16+lz, func(y int, id voxel.StateID) { ch.SetRaw(lx, y, lz, id) })
		}
	}
	ch.RecomputeHeights(w.Reg.Table())
}

// fill makes block column x,z of the stacked levels through set.
func (g *levelGen) fill(levels []*dungeon.Level, x, z int, set func(y int, id voxel.StateID)) {
	top := floorY(1) + levelPitch + 4
	bottom := floorY(len(levels)) - levelPitch
	for y := bottom; y < top; y++ {
		set(y, g.b.rock)
	}
	if x < 0 || z < 0 {
		return
	}
	c := dungeon.Coord{X: x / tileW, Y: z / tileD}
	sx, sz := x%tileW, z%tileD
	for d := 1; d <= len(levels); d++ {
		g.column(set, d, levels[d-1], c, sx, sz)
	}
	// the staircases last: they cut through the levels below
	for d := 1; d <= len(levels); d++ {
		g.staircase(set, d, levels[d-1], x, z)
	}
}

// staircase fills block column x,z with its part of level depth's
// staircase down, if it has one there. It runs along the stairs' row:
// step i is i blocks east (or west) of the stairs tile's near edge and i
// blocks down; step levelPitch is the landing on the floor below. Under
// the steps that stand in the room below (its ramp), rock down to its
// floor.
func (g *levelGen) staircase(set func(y int, id voxel.StateID), depth int, l *dungeon.Level, x, z int) {
	s := l.Stairs
	i, id := x-s.X*tileW, g.b.stairs[0]
	if l.StairsDir < 0 {
		i, id = s.X*tileW+tileW-1-x, g.b.stairs[1]
	}
	if z/tileD != s.Y || i < 0 || i >= levelPitch || i < sealedSteps && g.isSealed(depth) {
		return
	}
	f := floorY(depth)
	step := f - i
	set(step, id)
	for y := step + 1; y <= step+stairAir; y++ {
		set(y, voxel.Air)
	}
	if depth < maxDepth && i >= tileW*(dungeon.StairTiles-dungeon.StairRamp) {
		for y := f - levelPitch + 1; y < step; y++ {
			set(y, g.b.rock)
		}
	}
}

// sealedSteps are the steps a sealed staircase fills in: down past its
// halfway point, so even a jump from the last open step stays below it.
const sealedSteps = levelPitch/2 + 2

// opens reports whether a passage reaches over into tile t.
func opens(t dungeon.Tile) bool {
	return t == dungeon.Passage || t == dungeon.Door || t == dungeon.SecretDoor
}

// column fills one block column of one level through set. sx, sz are the
// block's offsets inside its tile.
func (g *levelGen) column(set func(y int, id voxel.StateID), depth int, l *dungeon.Level, c dungeon.Coord, sx, sz int) {
	t := l.At(c)
	if t == dungeon.SecretDoor && g.isFound(depth, c) {
		t = dungeon.Door
	}
	f := floorY(depth)
	carve := func(from, h int) {
		for y := from; y <= f+h; y++ {
			set(y, voxel.Air)
		}
	}
	mid := sx == tileW/2 && sz == tileD/2
	// passages are a 2 block wide strip down the middle of their row,
	// reaching across the row where the passage turns north or south
	strip := sz == tileD/2-1 || sz == tileD/2 ||
		sz < tileD/2-1 && opens(l.At(dungeon.Coord{X: c.X, Y: c.Y - 1})) ||
		sz > tileD/2 && opens(l.At(dungeon.Coord{X: c.X, Y: c.Y + 1}))
	switch t {
	case dungeon.HWall, dungeon.VWall, dungeon.SecretDoor:
		for y := f; y <= f+roomHeight+1; y++ {
			set(y, g.b.wall)
		}
	case dungeon.Stairs, dungeon.StairsUp:
		// a room's floor; staircase draws the steps
		set(f, g.b.roomFloor)
		carve(f+1, roomHeight)
		set(f+roomHeight+1, g.b.ceiling)
	case dungeon.Floor:
		rm := l.RoomAt(c)
		floor := g.b.roomFloor
		if rm != nil && rm.Dark {
			floor = g.b.darkFloor
		}
		if mid && l.TrapAt(c) != nil && g.isFound(depth, c) {
			floor = g.b.trap
		}
		set(f, floor)
		carve(f+1, roomHeight)
		set(f+roomHeight+1, g.b.ceiling)
		if rm != nil && !rm.Dark && mid && (c.X+c.Y)%2 == 0 {
			set(f+roomHeight+1, g.b.glow)
		}
	case dungeon.Passage:
		if strip {
			set(f, g.b.passFloor)
			carve(f+1, passHeight)
			set(f+passHeight+1, g.b.passCeiling)
		}
	case dungeon.Door:
		set(f, g.b.roomFloor)
		// In a side wall the passage comes along the row: posts frame the
		// strip. In a top or bottom wall it comes down the column, which
		// is just the opening's width: only the lintel.
		side := l.At(dungeon.Coord{X: c.X - 1, Y: c.Y}).Walkable() || l.At(dungeon.Coord{X: c.X + 1, Y: c.Y}).Walkable()
		if side && !(sz == tileD/2-1 || sz == tileD/2) {
			for y := f + 1; y <= f+roomHeight+1; y++ {
				set(y, g.b.doorFrame)
			}
			return
		}
		carve(f+1, passHeight)
		set(f+passHeight+1, g.b.doorFrame)
		for y := f + passHeight + 2; y <= f+roomHeight+1; y++ {
			set(y, g.b.wall)
		}
	}
}
