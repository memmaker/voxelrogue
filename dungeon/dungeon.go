// Package dungeon generates Rogue 3.6 levels: nine rooms in a 3x3 grid,
// some of them gone, joined by passages, plus stairs and traps. It is a
// port of rooms.c, passages.c and new_level() and is pure Go (no
// rendering), so levels can be built ahead of time from a seed.
//
// The levels are stacked: the staircase down from a level runs StairTiles
// columns east or west and lands in a room of the level below, which is
// built around that landing first.
package dungeon

import "math/rand"

// Map size, as Rogue's 80x24 terminal.
const (
	Cols     = 80
	Lines    = 24
	MaxRooms = 9
	// StairTiles is how many map columns the staircase down runs, east or
	// west from the stairs, before it lands on the level below; the last
	// StairRamp of them stand in the room there, under its foot.
	StairTiles = 6
	StairRamp  = 2
)

// Tile is one map square.
type Tile byte

const (
	Rock       Tile = ' '
	HWall      Tile = '-'
	VWall      Tile = '|'
	Floor      Tile = '.'
	Passage    Tile = '#'
	Door       Tile = '+'
	SecretDoor Tile = '&' // looks like wall until found
	Stairs     Tile = '%'
	StairsUp   Tile = '<' // the foot of the staircase from the level above
)

// Walkable reports whether a tile can be stood on.
func (t Tile) Walkable() bool {
	return t == Floor || t == Passage || t == Door || t == Stairs
}

// Coord is a map position (X column, Y row).
type Coord struct{ X, Y int }

// Room is one of the nine rooms. A gone room has only Pos, a spot that
// passages meet at.
type Room struct {
	Pos, Max Coord // top-left corner and size, walls included
	Gone     bool
	Dark     bool
	GoldVal  int
	Gold     Coord
	Exits    []Coord
}

// Inside reports whether c is on the room's floor (walls excluded).
func (r *Room) Inside(c Coord) bool {
	return !r.Gone && c.X > r.Pos.X && c.X < r.Pos.X+r.Max.X-1 && c.Y > r.Pos.Y && c.Y < r.Pos.Y+r.Max.Y-1
}

// TrapKind is what a trap does.
type TrapKind int

const (
	TrapDoor TrapKind = iota
	BearTrap
	SleepTrap
	ArrowTrap
	TeleTrap
	DartTrap
)

var trapNames = [...]string{"trapdoor", "beartrap", "sleeping gas trap", "arrow trap", "teleport trap", "poison dart trap"}

func (k TrapKind) String() string { return trapNames[k] }

// Trap is a hidden trap; Found once it went off or was searched out.
type Trap struct {
	Pos   Coord
	Kind  TrapKind
	Found bool
}

// Level is a generated dungeon level.
type Level struct {
	Depth     int
	Grid      [Lines][Cols]Tile
	Rooms     [MaxRooms]Room
	Stairs    Coord
	StairsDir int // +1: the staircase runs east, -1: west
	Traps     []Trap
}

// At returns the tile at c (Rock off the map).
func (l *Level) At(c Coord) Tile {
	if c.X < 0 || c.Y < 0 || c.X >= Cols || c.Y >= Lines {
		return Rock
	}
	return l.Grid[c.Y][c.X]
}

// RoomAt returns the room whose floor contains c, or nil.
func (l *Level) RoomAt(c Coord) *Room {
	for i := range l.Rooms {
		if l.Rooms[i].Inside(c) {
			return &l.Rooms[i]
		}
	}
	return nil
}

// TrapAt returns the trap at c, or nil.
func (l *Level) TrapAt(c Coord) *Trap {
	for i := range l.Traps {
		if l.Traps[i].Pos == c {
			return &l.Traps[i]
		}
	}
	return nil
}

// Landing is where the staircase down from l arrives on the level below.
func (l *Level) Landing() Coord {
	return Coord{l.Stairs.X + StairTiles*l.StairsDir, l.Stairs.Y}
}

// Ramp is the foot of the staircase down from l, on the level below: the
// tiles before the landing.
func (l *Level) Ramp() []Coord {
	land := l.Landing()
	r := make([]Coord, StairRamp)
	for i := range r {
		r[i] = Coord{land.X - (i+1)*l.StairsDir, land.Y}
	}
	return r
}

type gen struct {
	l   *Level
	rng *rand.Rand
	// the staircase from the level above, if any
	above   *Level
	ramp    []Coord
	landing Coord
}

// rnd is Rogue's rnd(): 0..n-1, 0 for n < 1.
func (g *gen) rnd(n int) int {
	if n < 1 {
		return 0
	}
	return g.rng.Intn(n)
}

// New builds level depth (1 = top) from rng, as Rogue's new_level(). The
// staircase from the level above, if any, comes down into this level:
// the room under its landing is placed around it first and the rest of
// the level is built from there. Rogue joins all rooms of a level, so
// every level reaches its stairs and the dungeon is connected from top
// to bottom.
func New(depth int, rng *rand.Rand, above *Level) *Level {
	g := &gen{l: &Level{Depth: depth}, rng: rng, above: above}
	if above != nil {
		g.ramp, g.landing = above.Ramp(), above.Landing()
	}
	for y := range g.l.Grid {
		for x := range g.l.Grid[y] {
			g.l.Grid[y][x] = Rock
		}
	}
	g.doRooms()
	g.doPassages()
	l := g.l
	l.Stairs, l.StairsDir = g.stairs()
	l.Grid[l.Stairs.Y][l.Stairs.X] = Stairs
	l.Grid[l.Stairs.Y][l.Stairs.X+l.StairsDir] = Stairs // the stairwell
	if g.rnd(10) < depth {
		n := min(g.rnd(depth/4)+1, 10)
		for i := 0; i < n; i++ {
			l.Traps = append(l.Traps, Trap{Pos: g.randomFloor(), Kind: TrapKind(g.rnd(6))})
		}
	}
	return l
}

// stairs picks the stairs as new_level() does, on a free floor spot, but
// one where the staircase can start down through the floor beside it and
// land where a room can stand on the level below. Every room has such a
// spot (its westmost or eastmost floor column).
func (g *gen) stairs() (Coord, int) {
	for {
		s := g.randomFloor()
		dirs := [2]int{1, -1}
		if g.rnd(2) == 0 {
			dirs = [2]int{-1, 1}
		}
		for _, d := range dirs {
			w := Coord{s.X + d, s.Y}
			if g.l.At(w) == Floor && !g.atFoot(s) && !g.atFoot(w) && landingFits(s.X+StairTiles*d, d) {
				return s, d
			}
		}
	}
}

// floorSpan is the floor columns a room needs for a landing at x reached
// going d: the landing, the ramp, and one tile behind the ramp so that no
// side-wall door opens onto it.
func floorSpan(x, d int) (lo, hi int) {
	if d > 0 {
		return x - StairRamp - 1, x
	}
	return x, x + StairRamp + 1
}

// landingFits reports whether a room can have the floorSpan of a landing:
// a room keeps to its cell of Cols/3 columns, with floor from the cell's
// second column to its fourth-to-last (do_rooms).
func landingFits(x, d int) bool {
	lo, hi := floorSpan(x, d)
	if lo < 1 {
		return false
	}
	w := Cols / 3
	left := (lo-1)/w*w + 1
	return lo >= left+1 && hi <= left+w-3
}

// atFoot reports whether c is in the floorSpan of the landing from above,
// where the staircase down from this level must not start.
func (g *gen) atFoot(c Coord) bool {
	if g.above == nil || c.Y != g.landing.Y {
		return false
	}
	lo, hi := floorSpan(g.landing.X, g.above.StairsDir)
	return c.X >= lo && c.X <= hi
}

// onRamp reports whether c is under the foot of the staircase from above.
func (g *gen) onRamp(c Coord) bool {
	for _, r := range g.ramp {
		if r == c {
			return true
		}
	}
	return false
}

// RandomFloor picks a free floor spot in a random room, as new_level()
// does for objects, stairs and the hero.
func (l *Level) RandomFloor(rng *rand.Rand) Coord {
	return (&gen{l: l, rng: rng}).randomFloor()
}

func (g *gen) randomFloor() Coord {
	for {
		c := g.rndPos(&g.l.Rooms[g.rndRoom()])
		if g.l.At(c) == Floor && g.l.TrapAt(c) == nil {
			return c
		}
	}
}

// RndRoom picks a room that is really there.
func (l *Level) RndRoom(rng *rand.Rand) int { return (&gen{l: l, rng: rng}).rndRoom() }

func (g *gen) rndRoom() int {
	for {
		if rm := g.rnd(MaxRooms); !g.l.Rooms[rm].Gone {
			return rm
		}
	}
}

// RndPos picks a random spot on a room's floor.
func (l *Level) RndPos(r *Room, rng *rand.Rand) Coord { return (&gen{l: l, rng: rng}).rndPos(r) }

func (g *gen) rndPos(r *Room) Coord {
	return Coord{r.Pos.X + g.rnd(r.Max.X-2) + 1, r.Pos.Y + g.rnd(r.Max.Y-2) + 1}
}

// doRooms is rooms.c's do_rooms() without the monsters (the game places
// those on arrival). The room under the landing of the staircase from
// above is never gone and is sized and placed around it.
func (g *gen) doRooms() {
	l := g.l
	bsze := Coord{Cols / 3, Lines / 3}
	landRoom := -1
	if g.above != nil {
		lo, _ := floorSpan(g.landing.X, g.above.StairsDir)
		landRoom = g.landing.Y/bsze.Y*3 + (lo-1)/bsze.X
	}
	for i := g.rnd(4); i > 0; i-- {
		l.Rooms[g.rnd(MaxRooms)].Gone = true
	}
	if landRoom >= 0 {
		l.Rooms[landRoom].Gone = false
	}
	for i := range l.Rooms {
		rp := &l.Rooms[i]
		top := Coord{(i%3)*bsze.X + 1, i / 3 * bsze.Y}
		if rp.Gone {
			for {
				rp.Pos = Coord{top.X + g.rnd(bsze.X-2) + 1, top.Y + g.rnd(bsze.Y-2) + 1}
				rp.Max = Coord{-Cols, -Lines}
				if rp.Pos.Y > 0 && rp.Pos.Y < Lines-1 {
					break
				}
			}
			continue
		}
		if g.rnd(10) < l.Depth-1 {
			rp.Dark = true
		}
		if i == landRoom {
			g.fitRoom(rp, top, bsze)
		} else {
			for {
				rp.Max = Coord{g.rnd(bsze.X-4) + 4, g.rnd(bsze.Y-4) + 4}
				rp.Pos = Coord{top.X + g.rnd(bsze.X-rp.Max.X), top.Y + g.rnd(bsze.Y-rp.Max.Y)}
				if rp.Pos.Y != 0 {
					break
				}
			}
		}
		if g.rnd(100) < 50 {
			rp.GoldVal = g.rnd(50+10*l.Depth) + 2 // GOLDCALC
			for rp.Gold = g.rndPos(rp); g.onRamp(rp.Gold); {
				rp.Gold = g.rndPos(rp)
			}
		}
		g.drawRoom(rp)
	}
	for _, c := range g.ramp {
		l.Grid[c.Y][c.X] = StairsUp
	}
}

// fitRoom sizes and places the landing's room in its cell (top, bsze) as
// do_rooms() would, but only among the rooms whose floor has the
// landing's floorSpan.
func (g *gen) fitRoom(rp *Room, top, bsze Coord) {
	lo, hi := floorSpan(g.landing.X, g.above.StairsDir)
	r := g.landing.Y
	minW := max(4, hi-lo+3)
	rp.Max.X = minW + g.rnd(bsze.X-minW)
	x0, x1 := max(top.X, hi+2-rp.Max.X), min(lo-1, top.X+bsze.X-rp.Max.X-1)
	rp.Pos.X = x0 + g.rnd(x1-x0+1)
	maxH := bsze.Y - 1
	if top.Y == 0 {
		maxH-- // no room starts on the first line
	}
	rp.Max.Y = 4 + g.rnd(maxH-3)
	y0, y1 := max(top.Y, r+2-rp.Max.Y, 1), min(r-1, top.Y+bsze.Y-rp.Max.Y-1)
	rp.Pos.Y = y0 + g.rnd(y1-y0+1)
}

func (g *gen) drawRoom(rp *Room) {
	for x := 0; x < rp.Max.X; x++ {
		g.l.Grid[rp.Pos.Y][rp.Pos.X+x] = HWall
		g.l.Grid[rp.Pos.Y+rp.Max.Y-1][rp.Pos.X+x] = HWall
	}
	for y := 1; y < rp.Max.Y-1; y++ {
		g.l.Grid[rp.Pos.Y+y][rp.Pos.X] = VWall
		g.l.Grid[rp.Pos.Y+y][rp.Pos.X+rp.Max.X-1] = VWall
		for x := 1; x < rp.Max.X-1; x++ {
			g.l.Grid[rp.Pos.Y+y][rp.Pos.X+x] = Floor
		}
	}
}

// adjacent is passages.c's rdes connection table.
var adjacent = [MaxRooms][MaxRooms]bool{
	{false, true, false, true},
	{true, false, true, false, true},
	{false, true, false, false, false, true},
	{true, false, false, false, true, false, true},
	{false, true, false, true, false, true, false, true},
	{false, false, true, false, true, false, false, false, true},
	{false, false, false, true, false, false, false, true},
	{false, false, false, false, true, false, true, false, true},
	{false, false, false, false, false, true, false, true},
}

func (g *gen) doPassages() {
	var ingraph [MaxRooms]bool
	var isconn [MaxRooms][MaxRooms]bool
	r1 := g.rnd(MaxRooms)
	ingraph[r1] = true
	for count := 1; count < MaxRooms; {
		r2, j := -1, 0
		for i := 0; i < MaxRooms; i++ {
			if adjacent[r1][i] && !ingraph[i] {
				if j++; g.rnd(j) == 0 {
					r2 = i
				}
			}
		}
		if j == 0 {
			for {
				if r1 = g.rnd(MaxRooms); ingraph[r1] {
					break
				}
			}
			continue
		}
		ingraph[r2] = true
		g.conn(r1, r2)
		isconn[r1][r2], isconn[r2][r1] = true, true
		count++
	}
	for n := g.rnd(5); n > 0; n-- {
		r1 := g.rnd(MaxRooms)
		r2, j := -1, 0
		for i := 0; i < MaxRooms; i++ {
			if adjacent[r1][i] && !isconn[r1][i] {
				if j++; g.rnd(j) == 0 {
					r2 = i
				}
			}
		}
		if j != 0 {
			g.conn(r1, r2)
			isconn[r1][r2], isconn[r2][r1] = true, true
		}
	}
}

// conn digs a corridor between two adjacent rooms (passages.c conn()).
func (g *gen) conn(r1, r2 int) {
	rm := min(r1, r2)
	down := max(r1, r2) != rm+1
	rpf := &g.l.Rooms[rm]
	var rpt *Room
	var pdelta, turnDelta, spos, epos Coord
	var distance, turnDistance int
	if down {
		rpt = &g.l.Rooms[rm+3]
		pdelta = Coord{0, 1}
		spos, epos = rpf.Pos, rpt.Pos
		if !rpf.Gone {
			spos.Y += rpf.Max.Y - 1
			spos.X += g.doorColumn(rpf, spos.Y)
		}
		if !rpt.Gone {
			epos.X += g.doorColumn(rpt, epos.Y)
		}
		distance = abs(spos.Y-epos.Y) - 1
		turnDelta = Coord{sign(epos.X - spos.X), 0}
		turnDistance = abs(spos.X - epos.X)
	} else {
		rpt = &g.l.Rooms[rm+1]
		pdelta = Coord{1, 0}
		spos, epos = rpf.Pos, rpt.Pos
		if !rpf.Gone {
			spos.X += rpf.Max.X - 1
			spos.Y += g.rnd(rpf.Max.Y-2) + 1
		}
		if !rpt.Gone {
			epos.Y += g.rnd(rpt.Max.Y-2) + 1
		}
		distance = abs(spos.X-epos.X) - 1
		turnDelta = Coord{0, sign(epos.Y - spos.Y)}
		turnDistance = abs(spos.Y - epos.Y)
	}
	turnSpot := g.rnd(distance-1) + 1
	g.door(rpf, spos)
	g.door(rpt, epos)
	curr := spos
	for ; distance > 0; distance-- {
		curr.X += pdelta.X
		curr.Y += pdelta.Y
		if distance == turnSpot {
			for ; turnDistance > 0; turnDistance-- {
				g.set(curr, Passage)
				curr.X += turnDelta.X
				curr.Y += turnDelta.Y
			}
		}
		g.set(curr, Passage)
	}
}

// doorColumn picks where along a room's top or bottom wall (at row y) a
// door goes, as conn() does, but never onto the foot of the staircase
// from above, which would block it.
func (g *gen) doorColumn(r *Room, y int) int {
	for {
		dx := g.rnd(r.Max.X-2) + 1
		inside := Coord{r.Pos.X + dx, y + 1}
		if y > r.Pos.Y {
			inside.Y = y - 1
		}
		if !g.onRamp(inside) {
			return dx
		}
	}
}

func (g *gen) set(c Coord, t Tile) {
	if c.X >= 0 && c.Y >= 0 && c.X < Cols && c.Y < Lines {
		g.l.Grid[c.Y][c.X] = t
	}
}

func (g *gen) door(r *Room, c Coord) {
	if r.Gone {
		g.set(c, Passage)
		return
	}
	t := Door
	if g.rnd(10) < g.l.Depth-1 && g.rnd(100) < 20 {
		t = SecretDoor
	}
	g.set(c, t)
	r.Exits = append(r.Exits, c)
}

// String draws the level as Rogue would (debugging).
func (l *Level) String() string {
	b := make([]byte, 0, (Cols+1)*Lines)
	for y := range l.Grid {
		for x := range l.Grid[y] {
			b = append(b, byte(l.Grid[y][x]))
		}
		b = append(b, '\n')
	}
	return string(b)
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// sign is Rogue's (a < b ? 1 : -1) for a = 0: never 0.
func sign(d int) int {
	if d > 0 {
		return 1
	}
	return -1
}
