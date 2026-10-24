package main

// Monsters in real time. Each monster owns a map tile and walks to the
// next one over moveTicks, so Rogue's grid rules (doors, no corner
// cutting, one monster per square) still hold while the model glides.
// Running monsters step down a distance field from the hero and attack
// from an adjacent square every attackTicks, with fight.c's dice.

import (
	"cmp"
	"fmt"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxellib/voxel/collide"
	"image"
	"math"
	"slices"

	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

const (
	moveTicks   = 14 // ticks per 3 blocks of step: about the hero's walking pace
	attackTicks = 16
	unreachable = 1 << 30
)

type monster struct {
	letter     byte
	st         rogue.Stats
	flags      int
	tile, from dungeon.Coord // standing on tile; from is where a move began
	moveT      int           // ticks into the move from -> tile, 0 = still
	running    bool          // chasing the hero (ISRUN)
	attackCD   int
	lunge      int // ticks left of the attack lunge
	hurt       int
	held       int // hold monster / sleep scroll
	confused   bool
	cancelled  bool
	speed      int            // -1 slowed, 1 hasted
	disguise   *rogue.Object  // a mimic's look until it is found out
	found      bool           // ISFOUND: an umber hulk's gaze is spent
	dest       *dungeon.Coord // a dragon's gold to guard; nil: the hero
	pack       *rogue.Object
	dying      int        // ticks since the killing blow
	dead       [4]float64 // where it fell (x, y, z) and its yaw then
	pin        *pin       // pinned to a wall by the missile that killed it (pin.go)
	leanX      float64    // offset from the tile centre toward the hero
	leanZ      float64    // while fighting, so both sides are in reach
}

type monsterState struct {
	monsters  []*monster
	modelRend *render.ModelRenderer
	monSprite map[byte]monSprite
	boxes     map[byte]monsterBox
	corpses   map[int][]*monster // by depth; they stay
	showBoxes bool               // F3: every monster's hit box
	dist      [dungeon.Lines][dungeon.Cols]int
	distFrom  dungeon.Coord
	distTick  int
	insts     []render.ItemPose
}

func (m *monster) kind() *rogue.MonsterKind { return &rogue.Monsters[m.letter-'A'] }
func (m *monster) name() string             { return m.kind().Name }
func (m *monster) alive() bool              { return m.dying == 0 }

// newMonster puts a monster on a tile (new_monster).
func (g *game) newMonster(letter byte, c dungeon.Coord) *monster {
	m := &monster{letter: letter, tile: c, from: c}
	g.initMonster(m, letter)
	if g.wearing(rogue.RAggr) {
		m.running = true
	}
	g.monsters = append(g.monsters, m)
	return m
}

// mimicLooks are what a mimic passes for (the stairs a mimic may imitate
// are blocks in this world, so it passes for gold there).
var mimicLooks = []rogue.ObjType{rogue.Gold, rogue.Potion, rogue.Scroll, rogue.Gold, rogue.Weapon, rogue.Armor,
	rogue.Ring, rogue.Stick, rogue.Amulet}

// initMonster rolls a monster's kind, stats and a mimic's disguise.
func (g *game) initMonster(m *monster, letter byte) {
	m.letter = letter
	m.st = rogue.NewMonsterStats(g.rng, letter)
	m.flags = m.kind().Flags
	m.disguise, m.found, m.dest = nil, false, nil
	if letter == 'M' {
		n := 8
		if g.depth > 25 {
			n = 9
		}
		t := mimicLooks[rogue.Rnd(g.rng, n)]
		m.disguise = &rogue.Object{Type: t, Count: 1, Launch: -1, GoldVal: 1}
		if t == rogue.Armor {
			m.disguise.Which = rogue.Rnd(g.rng, len(rogue.Armors))
		}
	}
}

// unmask shows a mimic for what it is.
func (g *game) unmask(m *monster) {
	if m.disguise != nil && g.blind == 0 {
		m.disguise = nil
		g.msg("Wait! That's a mimic!")
	}
}

// wanderer starts a monster in another room, already hunting the hero.
func (g *game) wanderer() {
	l := g.level()
	hr := l.RoomAt(g.heroTile())
	for tries := 0; tries < 100; tries++ {
		rm := &l.Rooms[l.RndRoom(g.rng)]
		if rm == hr {
			continue
		}
		c := l.RndPos(rm, g.rng)
		if !l.At(c).Walkable() || g.monsterAt(c) != nil {
			continue
		}
		m := g.newMonster(rogue.RandMonster(g.rng, g.depth, true), c)
		m.running = true
		return
	}
}

// sleepScale is a blow's damage multiplier on a sleeping monster: double,
// a dagger's five times. (Rogue itself gives no bonus: its +4 to hit on a
// sleeper has no meaning without the to-hit roll.)
func sleepScale(weap *rogue.Object) float64 {
	if weap != nil && weap.Type == rogue.Weapon && rogue.Weapons[weap.Which].Backstab > 0 {
		return rogue.Weapons[weap.Which].Backstab
	}
	return 2
}

// How far a noise carries, in blocks.
const (
	fightNoise  = 8   // a blow landing
	shotNoise   = 5   // a bow's twang, a crossbow's clack
	throwNoise  = 3   // a throw
	zapNoise    = 8   // a wand
	spanNoise   = 3   // spanning a crossbow
	walkNoise   = 2.5 // a step
	sprintNoise = 6   // a running step; a sneaking step is silent
)

// noise wakes every sleeping monster within r blocks of x, z.
func (g *game) noise(x, z, r float64) {
	for _, m := range g.monsters {
		mx, _, mz := g.monsterPos(m, 0)
		if !m.running && m.held == 0 && m.alive() && math.Hypot(mx-x, mz-z) <= r {
			m.running, m.dest = true, nil
		}
	}
}

// zz floats a Z (a 3x3 of little squares, facing the hero) up from above
// a sleeping monster's head: sleepZs sends one every second.
func (g *game) zz(m *monster) {
	x, y, z := g.monsterPos(m, 0)
	b := g.box(m.letter)
	y += float64(b.hover+b.h) + 0.2
	rx, rz := math.Cos(g.cam.Yaw), math.Sin(g.cam.Yaw) // the view's right
	const s = 0.08
	for _, c := range [10][2]float64{{-1.5, 1.5}, {-0.5, 1.5}, {0.5, 1.5}, {1.5, 1.5}, {0.5, 0.5}, {-0.5, -0.5}, {-1.5, -1.5}, {-0.5, -1.5}, {0.5, -1.5}, {1.5, -1.5}} { // a 4x4 Z (a 3x3 one reads as an I)
		g.parts.Add(particles.Particle{X: x + rx*c[0]*s, Y: y + c[1]*s, Z: z + rz*c[0]*s, VX: rx * 0.1, VY: 0.3, VZ: rz * 0.1,
			Life: 1.5, Size0: s, Size1: s, Col0: fxWhite, Col1: fade(fxWhite), Emit: 15})
	}
}

// sleepZs marks the sleeping monsters in sight with Zs.
func (g *game) sleepZs() {
	if g.tick%20 != 0 {
		return
	}
	for _, m := range g.monsters {
		if !m.running && m.alive() && m.disguise == nil && g.canSee(m.tile) {
			g.zz(m)
		}
	}
}

// wakeMonster is what happens when the hero sees a monster (wake_monster).
func (g *game) wakeMonster(m *monster) {
	if !m.running && m.flags&rogue.IsMean != 0 && m.held == 0 && !g.wearing(rogue.RStealth) && rogue.Rnd(g.rng, 100) > 33 {
		m.running, m.dest = true, nil
	}
	if m.letter == 'U' && g.blind == 0 {
		h := g.heroTile()
		rm := g.level().RoomAt(h)
		dx, dy := m.tile.X-h.X, m.tile.Y-h.Y
		if rm != nil && !rm.Dark || dx*dx+dy*dy < 3 {
			if !m.found && !rogue.Save(g.rng, rogue.VSMagic, g.stats.Lvl) {
				g.msg("The umber hulk's gaze has confused you.")
				g.confused += (rogue.Rnd(g.rng, 20) + 20) * turnTicks
			}
			m.found = true
		}
	}
	// greedy ones guard the gold of the hero's room
	if m.flags&rogue.IsGreed != 0 && !m.running {
		if rm := g.level().RoomAt(g.heroTile()); rm != nil {
			for _, f := range g.floor {
				if f.o.Type == rogue.Gold && rm.Inside(f.tile) {
					c := f.tile
					m.dest, m.running = &c, true
					break
				}
			}
		}
	}
}

// scared reports whether a scare monster scroll lies on c: no monster
// steps there, nor attacks a hero standing on it.
func (g *game) scared(c dungeon.Coord) bool {
	for _, f := range g.floor {
		if f.tile == c && f.o.Type == rogue.Scroll && f.o.Which == rogue.SScare {
			return true
		}
	}
	return false
}

// runto sets a monster after the hero.
func (m *monster) runto() { m.running, m.dest, m.held = true, nil, 0 }

func (g *game) monsterAt(c dungeon.Coord) *monster {
	for _, m := range g.monsters {
		if m.alive() && m.tile == c {
			return m
		}
	}
	return nil
}

// stepOK is Rogue's movement rule between neighbouring squares: no
// diagonal moves through doors or around corners.
func stepOK(l *dungeon.Level, a, b dungeon.Coord) bool {
	if !l.At(b).Walkable() {
		return false
	}
	if a.X != b.X && a.Y != b.Y {
		if l.At(a) == dungeon.Door || l.At(b) == dungeon.Door {
			return false
		}
		if !l.At(dungeon.Coord{X: b.X, Y: a.Y}).Walkable() || !l.At(dungeon.Coord{X: a.X, Y: b.Y}).Walkable() {
			return false
		}
	}
	return true
}

var dirs8 = []dungeon.Coord{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}, {X: 1, Y: -1}, {X: -1, Y: -1}}

// updateDist rebuilds the distance field from the hero's square.
func (g *game) updateDist() {
	h := g.heroTile()
	if h == g.distFrom && g.distTick == g.depth {
		return
	}
	g.distFrom, g.distTick = h, g.depth
	l := g.level()
	for y := range g.dist {
		for x := range g.dist[y] {
			g.dist[y][x] = unreachable
		}
	}
	if !l.At(h).Walkable() {
		return
	}
	g.dist[h.Y][h.X] = 0
	q := []dungeon.Coord{h}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for _, d := range dirs8 {
			n := dungeon.Coord{X: c.X + d.X, Y: c.Y + d.Y}
			if stepOK(l, c, n) && g.dist[n.Y][n.X] == unreachable {
				g.dist[n.Y][n.X] = g.dist[c.Y][c.X] + 1
				q = append(q, n)
			}
		}
	}
}

func (g *game) tickMonsters() {
	g.updateDist()
	l, h := g.level(), g.heroTile()
	live := g.monsters[:0]
	for _, m := range g.monsters {
		if m.dying > 0 { // out of the monster logic, a corpse for good
			g.loseMonster(m)
			if g.corpses == nil {
				g.corpses = map[int][]*monster{}
			}
			g.corpses[g.depth] = append(g.corpses[g.depth], m)
			continue
		}
		live = append(live, m)
		for _, t := range []*int{&m.attackCD, &m.lunge, &m.hurt, &m.held} {
			if *t > 0 {
				*t--
			}
		}
		g.lean(m, h)
		if m.moveT > 0 {
			if m.moveT++; m.moveT > m.moveTicks() {
				m.moveT, m.from = 0, m.tile
			}
			continue
		}
		if !m.running || m.held > 0 || g.dead {
			continue
		}
		if m.dest != nil { // a dragon off to its gold
			if !g.chaseTo(m, *m.dest) {
				m.running = false
			}
			continue
		}
		adjacent := abs(m.tile.X-h.X) <= 1 && abs(m.tile.Y-h.Y) <= 1 && m.tile != h && stepOK(l, m.tile, h)
		if adjacent {
			_, _, d := g.slab(m).nearest(g.player.X, g.player.Z)
			adjacent = d <= monsterReach
		}
		if adjacent && m.attackCD > 0 {
			continue
		}
		if g.wandersOff(m) {
			g.randomStep(m)
			if adjacent {
				m.attackCD = attackTicks
			}
			continue
		}
		if adjacent {
			if !g.scared(h) {
				m.attackCD, m.lunge = attackTicks, 8
				g.monsterAttack(m)
			}
			continue
		}
		if m.letter == 'F' { // fungi stay put
			continue
		}
		g.monsterStep(m)
	}
	g.monsters = live
	g.tickCorpses() // after the newly dead joined: a blow's second tick is the fall's second
}

// wandersOff decides whether a monster moves at random this time
// (chase.c): confused ones mostly, invisible stalkers and bats often.
func (g *game) wandersOff(m *monster) bool {
	if m.confused && rogue.Rnd(g.rng, 10) < 8 || m.letter == 'I' && rogue.Rnd(g.rng, 100) < 20 ||
		m.letter == 'B' && rogue.Rnd(g.rng, 100) < 50 {
		if rogue.Rnd(g.rng, 1000) < 50 {
			m.confused = false
		}
		return true
	}
	return false
}

// randomStep moves a monster to a random open neighbouring square
// (rndmove).
func (g *game) randomStep(m *monster) {
	if m.letter == 'F' {
		return
	}
	l, h := g.level(), g.heroTile()
	for _, i := range g.rng.Perm(len(dirs8)) {
		n := dungeon.Coord{X: m.tile.X + dirs8[i].X, Y: m.tile.Y + dirs8[i].Y}
		if n != h && stepOK(l, m.tile, n) && g.monsterAt(n) == nil && !g.scared(n) {
			m.from, m.tile, m.moveT = m.tile, n, 1
			return
		}
	}
}

// chaseTo steps a monster toward c by straight-line distance (chase.c
// chase); false once it is there.
func (g *game) chaseTo(m *monster, c dungeon.Coord) bool {
	l, h := g.level(), g.heroTile()
	dist := func(a dungeon.Coord) int { return (a.X-c.X)*(a.X-c.X) + (a.Y-c.Y)*(a.Y-c.Y) }
	best, bd := m.tile, dist(m.tile)
	if bd == 0 {
		return false
	}
	for _, d := range dirs8 {
		n := dungeon.Coord{X: m.tile.X + d.X, Y: m.tile.Y + d.Y}
		if n != h && stepOK(l, m.tile, n) && g.monsterAt(n) == nil && !g.scared(n) && dist(n) < bd {
			best, bd = n, dist(n)
		}
	}
	if best != m.tile {
		m.from, m.tile, m.moveT = m.tile, best, 1
	}
	return true
}

// moveTicks is how long the current step takes: steps along a map row
// cover tileW blocks, along a column tileD.
func (m *monster) moveTicks() int {
	blocks := math.Hypot(float64((m.tile.X-m.from.X)*tileW), float64((m.tile.Y-m.from.Y)*tileD))
	t := int(moveTicks*blocks/3 + 0.5)
	switch m.speed {
	case 1:
		return max(t/2, 1)
	case -1:
		return t * 2
	}
	return max(t, 1)
}

// monsterReach is how far a monster's attack reaches past its box: from
// the edge of its square to beyond the middle of the next (a dagger's
// reach, the hero's shortest).
const monsterReach = 2.1

// lean moves a monster that is fighting the hero from the next square to
// the side of its tile facing the hero (staying inside the tile), and
// back to the centre otherwise.
func (g *game) lean(m *monster, h dungeon.Coord) {
	var tx, tz float64
	if m.running && m.moveT == 0 && abs(m.tile.X-h.X) <= 1 && abs(m.tile.Y-h.Y) <= 1 && m.held == 0 {
		cx, _, cz := tileCenter(g.slot(), m.tile)
		// the turned box's half extents: its width across the view, its depth along it
		b, yaw := g.box(m.letter), math.Atan2(g.player.X-cx, -(g.player.Z-cz))
		w, d, c, sn := float64(b.r), float64(b.d), math.Abs(math.Cos(yaw)), math.Abs(math.Sin(yaw))
		ex, ez := math.Max(0, tileW/2.0-(c*w+sn*d)-0.05), math.Max(0, tileD/2.0-(sn*w+c*d)-0.05)
		tx = math.Max(-ex, math.Min(ex, g.player.X-cx))
		tz = math.Max(-ez, math.Min(ez, g.player.Z-cz))
	}
	m.leanX += (tx - m.leanX) * 0.2
	m.leanZ += (tz - m.leanZ) * 0.2
}

// monsterStep moves a running monster one square toward the hero.
func (g *game) monsterStep(m *monster) {
	l, h := g.level(), g.heroTile()
	best, bestD := m.tile, g.dist[m.tile.Y][m.tile.X]
	for _, i := range g.rng.Perm(len(dirs8)) {
		d := dirs8[i]
		n := dungeon.Coord{X: m.tile.X + d.X, Y: m.tile.Y + d.Y}
		if n == h || !stepOK(l, m.tile, n) || g.monsterAt(n) != nil || g.scared(n) {
			continue
		}
		if nd := g.dist[n.Y][n.X]; nd < bestD {
			best, bestD = n, nd
		}
	}
	if best != m.tile {
		m.from, m.tile, m.moveT = m.tile, best, 1
	}
}

// monsterAttack is fight.c attack(): a monster swings at the hero.
func (g *game) monsterAttack(m *monster) {
	g.quiet = 0
	if m.letter == 'M' {
		g.unmask(m)
	}
	defer func() { // regenerating monsters heal as they fight
		if m.flags&rogue.IsRegen != 0 && rogue.Rnd(g.rng, 100) < 33 {
			m.st.HP++
		}
	}()
	rogue.RollEm(g.rng, &m.st, &g.stats, g.armorClass(), rogue.Attack{})
	if m.letter != 'E' {
		g.msg("The " + m.name() + " hits.")
		g.hurt = 6
	}
	if g.stats.HP <= 0 {
		g.die(m.name())
		return
	}
	if m.cancelled {
		return
	}
	switch m.letter {
	case 'R':
		if g.armor != nil && g.armor.AC < 9 {
			g.armor.AC++
			g.msg("Your armor appears to be weaker now. Oh my!")
		}
	case 'E':
		if g.blind > 0 {
			break
		}
		if g.noCommand == 0 {
			g.msg("You are transfixed by the gaze of the floating eye.")
		}
		g.noCommand += (rogue.Rnd(g.rng, 2) + 2) * turnTicks
		if g.noCommand > 100*turnTicks && g.foodLeft <= 0 {
			g.die(m.name())
		}
	case 'A':
		if !rogue.Save(g.rng, rogue.VSPoison, g.stats.Lvl) {
			if g.wearing(rogue.RSustStr) {
				g.msg("A sting momentarily weakens you.")
			} else {
				g.chgStr(-1)
				g.msg("You feel a sting in your arm and now feel weaker.")
			}
		}
	case 'W':
		if rogue.Rnd(g.rng, 100) < 15 {
			if g.stats.Exp == 0 {
				g.die(m.name())
				return
			}
			g.msg("You suddenly feel weaker.")
			if g.stats.Lvl--; g.stats.Lvl == 0 {
				g.stats.Exp, g.stats.Lvl = 0, 1
			} else {
				g.stats.Exp = rogue.ELevels[g.stats.Lvl-1] + 1
			}
			fewer := rogue.Roll(g.rng, 1, 10)
			g.stats.HP -= fewer
			g.stats.MaxHP -= fewer
			g.stats.HP = max(g.stats.HP, 1)
			if g.stats.MaxHP < 1 {
				g.die(m.name())
			}
		}
	case 'F':
		g.held = true
		g.fungHit++
		rogue.Monsters['F'-'A'].Dmg = fmt.Sprintf("%dd1", g.fungHit)
	case 'L':
		last := g.purse
		g.purse -= rogue.Rnd(g.rng, 50+10*g.depth) + 2
		if !rogue.Save(g.rng, rogue.VSMagic, g.stats.Lvl) {
			for i := 0; i < 4; i++ {
				g.purse -= rogue.Rnd(g.rng, 50+10*g.depth) + 2
			}
		}
		g.purse = max(g.purse, 0)
		if g.purse != last {
			g.msg("Your purse feels lighter.")
		}
		g.removeMonster(m)
	case 'N':
		var steal *rogue.Object
		n := 0
		for _, o := range g.pack() {
			if o != g.armor && o != g.weapon && o != g.rings[0] && o != g.rings[1] && isMagic(o) {
				if n++; rogue.Rnd(g.rng, n) == 0 {
					steal = o
				}
			}
		}
		if steal != nil {
			g.removeMonster(m)
			if steal.Count > 1 && steal.Group == 0 {
				one := *steal
				one.Count = 1
				g.msg("She stole " + lowerFirst(g.rg.Name(&one)) + "!")
				g.removeFromPack(steal, false)
			} else {
				g.msg("She stole " + lowerFirst(g.rg.Name(steal)) + "!")
				g.removeFromPack(steal, true)
			}
		}
	}
}

// removeMonster takes a monster off the level without a corpse.
func (g *game) removeMonster(m *monster) {
	g.loseMonster(m)
	for i, o := range g.monsters {
		if o == m {
			g.monsters = append(g.monsters[:i], g.monsters[i+1:]...)
			return
		}
	}
}

// sweeps reports whether a blow strikes every monster in its path: a
// sweeping weapon's (an axe's) sideways swing (pattern 0).
func sweeps(w *rogue.Object, pattern int) bool {
	return w != nil && pattern == 0 && rogue.Weapons[meleeKind(w)].Sweeps
}

// bareSwing and bareReach are the bare hands' ticks between blows and
// reach in blocks (plus the target's size).
const bareSwing, bareReach = 12, 2.4

// heroAttack swings the wielded weapon in one of its attack patterns
// (0 left button, 1 right button, 2 F): a Better Combat attack whose hit
// lands at its upswing (bettercombat.go).
func (g *game) heroAttack(pattern int) {
	cd, reach := bareSwing, bareReach
	w := g.meleeItem()
	if w != nil {
		k := rogue.Weapons[meleeKind(w)]
		cd, reach = k.Swing, k.Reach
	}
	if g.startAttack(cd, reach, pattern) {
		g.attackCD = cd
		if w != nil {
			g.play(rogue.Weapons[meleeKind(w)].Sound)
		}
	}
}

// meleeItem is what the hero strikes with: a held wand, else the wielded
// weapon (nil = bare hands).
func (g *game) meleeItem() *rogue.Object {
	if g.zapping != nil {
		return g.zapping
	}
	return g.weapon
}

// meleeKind is the weapon kind an item handles and animates like: a wand
// as a spear.
func meleeKind(o *rogue.Object) int {
	if o.Type == rogue.Stick {
		return rogue.WeaponID("spear")
	}
	return o.Which
}

// magic reports whether a blow was struck with a magic item (a wand or
// a bolt), which draws no blood.
func magic(o *rogue.Object) bool { return o != nil && o.Type == rogue.Stick }

// impact is where a melee blow meets a monster: where the view line
// enters its box, else its face nearest the hero at eye height.
func (g *game) impact(m *monster) (x, y, z float64) {
	s := g.slab(m)
	cp := math.Cos(g.cam.Pitch)
	const far = 20
	ex, ey, ez := g.cam.X+math.Sin(g.cam.Yaw)*cp*far, g.cam.Y+math.Sin(g.cam.Pitch)*far, g.cam.Z-math.Cos(g.cam.Yaw)*cp*far
	if f, ok := s.enter(g.cam.X, g.cam.Y, g.cam.Z, ex, ey, ez); ok {
		return g.cam.X + (ex-g.cam.X)*f, g.cam.Y + (ey-g.cam.Y)*f, g.cam.Z + (ez-g.cam.Z)*f
	}
	x, z, _ = s.nearest(g.cam.X, g.cam.Z)
	return x, math.Min(math.Max(g.cam.Y, s.y), s.y+float64(s.h)), z
}

// bleed sprays blood from a point.
func (g *game) bleed(x, y, z float64) { g.parts.Emit(x, y, z, particles.Blood) }

// monsterInFront finds the living monster whose box is nearest, within
// reach of the hero and in a cone around the view direction (any way
// within a block).
func (g *game) monsterInFront(reach float64) *monster {
	if ms := g.monstersInFront(reach); len(ms) > 0 {
		return ms[0]
	}
	return nil
}

// monstersInFront is every living monster within reach in front of the
// hero, the nearest first.
func (g *game) monstersInFront(reach float64) []*monster {
	p := g.player
	fx, fz := math.Sin(g.cam.Yaw), -math.Cos(g.cam.Yaw)
	var ms []*monster
	dist := map[*monster]float64{}
	for _, m := range g.monsters {
		if !m.alive() {
			continue
		}
		nx, nz, d := g.slab(m).nearest(p.X, p.Z)
		if d > reach {
			continue
		}
		if d > 1 && ((nx-p.X)*fx+(nz-p.Z)*fz)/d < 0.6 {
			continue
		}
		ms, dist[m] = append(ms, m), d
	}
	slices.SortFunc(ms, func(a, b *monster) int { return cmp.Compare(dist[a], dist[b]) })
	return ms
}

// fight is fight.c fight(): the hero hits a monster with weap (nil =
// bare hands), thrown or not. Reaching it was the hit; nothing misses.
func (g *game) fight(m *monster, weap *rogue.Object, thrown bool) bool {
	asleep := !m.running
	g.quiet = 0
	m.runto()
	g.noise(g.player.X, g.player.Z, fightNoise)
	if m.disguise != nil && g.blind == 0 {
		g.unmask(m)
		if !thrown {
			return false
		}
	}
	a := rogue.Attack{}
	if weap != nil {
		a = rogue.Attack{Dmg: weap.Damage, HPlus: weap.HPlus, DPlus: weap.DPlus}
		if weap.Enemy == m.letter { // vorpally enchanted against it
			a.HPlus += 4
			a.DPlus += 4
		}
		if weap.Type == rogue.Stick && weap.Damage == "" { // a held wand fights as a spear at 3/5 the damage
			a = rogue.Attack{Dmg: rogue.Weapons[rogue.WeaponID("spear")].Damage, Scale: 0.6}
		}
		if thrown {
			a.Dmg = weap.HurlDmg
			if weap.Flags&rogue.IsMissile != 0 && g.weapon != nil && rogue.Launches(g.weapon.Which, weap) { // shot from its launcher
				a.HPlus += g.weapon.HPlus
				a.DPlus += g.weapon.DPlus
			} else if weap.Launch >= 0 { // ammo thrown by hand
				a.Dmg = weap.Damage
			}
		}
		if weap == g.weapon {
			for _, r := range g.rings {
				if r != nil && r.Which == rogue.RAddDam {
					a.DPlus += r.AC
				} else if r != nil && r.Which == rogue.RAddHit {
					a.HPlus += r.AC
				}
			}
		}
	}
	if asleep {
		if a.Scale == 0 {
			a.Scale = 1
		}
		a.Scale *= sleepScale(weap)
	}
	rogue.RollEm(g.rng, &g.stats, &m.st, m.st.Arm, a)
	m.hurt = 6
	if g.confuseHit {
		g.confuseHit = false
		m.confused = true
		g.msg("Your hands stop glowing red. The " + m.name() + " appears confused.")
	}
	if m.st.HP <= 0 {
		g.killed(m)
		return true
	}
	if thrown {
		g.msg("The missile hits the " + m.name() + ".")
	} else {
		g.msg("You hit the " + m.name() + ".")
	}
	return true
}

// killed is fight.c killed(): experience, and the monster's treasure
// falls where it stood.
func (g *game) killed(m *monster) {
	g.msg("You defeated the " + m.name() + ".")
	g.gainExp(rogue.KillExp(&m.st))
	m.dying = 1
	x, y, z := g.monsterPos(m, 0)
	m.dead = [4]float64{x, y, z, math.Atan2(g.player.X-x, -(g.player.Z - z))}
	if m.letter == 'F' {
		g.held, g.fungHit = false, 0
		rogue.Monsters['F'-'A'].Dmg = "000d0"
	}
	if m.letter == 'L' {
		gold := rogue.Rnd(g.rng, 50+10*g.depth) + 2
		if rogue.Save(g.rng, rogue.VSMagic, g.stats.Lvl) {
			for i := 0; i < 4; i++ {
				gold += rogue.Rnd(g.rng, 50+10*g.depth) + 2
			}
		}
		g.floor = append(g.floor, &floorObj{o: &rogue.Object{Type: rogue.Gold, GoldVal: gold, Count: 1}, tile: m.tile})
	}
	if m.pack != nil {
		g.floor = append(g.floor, &floorObj{o: m.pack, tile: m.tile})
		m.pack = nil
	}
}

// monsterPos is where a monster is drawn: between its squares while it
// walks, its lean on top. A step's first tick (moveT 1) draws it still
// leaving its square, and the lean eases off over the step, so the
// position never jumps.
func (g *game) monsterPos(m *monster, alpha float64) (x, y, z float64) {
	x, y, z = tileCenter(g.slot(), m.tile)
	if m.moveT > 0 {
		fx, _, fz := tileCenter(g.slot(), m.from)
		t := (float64(m.moveT-1) + alpha) / float64(m.moveTicks())
		t = math.Min(t, 1)
		x, z = fx+(x-fx)*t, fz+(z-fz)*t
	}
	return x + m.leanX, y, z + m.leanZ
}

// pushOutOfMonsters keeps the hero's body out of the monsters' boxes,
// backing him out through the world w only as far as its blocks let his
// box go: a push can never put him into a wall.
func (g *game) pushOutOfMonsters(w collide.Solids) {
	p := g.player
	r := p.Width/2 + 0.05 // ponytail: the hero as a circle; the box faces him, so his square's corners hardly matter
	for _, m := range g.monsters {
		if !m.alive() {
			continue
		}
		s := g.slab(m)
		if p.Y >= s.y+float64(s.h) || p.Y+p.Height <= s.y { // a flyer overhead
			continue
		}
		nx, nz, d := s.nearest(p.X, p.Z)
		if d >= r {
			continue
		}
		if m.disguise != nil { // walked into a mimic
			g.unmask(m)
			m.runto()
		}
		ux, uz := math.Sin(s.yaw), -math.Cos(s.yaw) // inside: out through the face, toward the hero
		if d > 1e-6 {
			ux, uz = (p.X-nx)/d, (p.Z-nz)/d
		}
		// back out, his box swept against the blocks, and only to where it
		// fits (a box already in a block would sweep through it)
		mx, _, mz := collide.Move(w, p.Box(), nx+ux*r-p.X, 0, nz+uz*r-p.Z)
		if collide.Fits(w, p.Box().Offset(mx, 0, mz)) {
			p.X, p.Z = p.X+mx, p.Z+mz
		}
	}
}

// monSprite is how a monster's 16px sprite fills its look: the opaque
// rows and their centre row, so the creature (not the cell) stands h high.
type monSprite struct{ rows, cy float32 }

// monsterScale makes the looks blocks, alike for all so they keep their
// sizes relative to each other: the tallest, the dragon (2.8), fills a
// room's headroom. Passages are lower; a giant's head goes into their
// ceiling.
const monsterScale = (roomHeight - 0.2) / 2.8

// monsterBox is a monster's hit box from its sprite's opaque outline,
// in the frame of its slab turned to the hero (slab): x0..x1 across, h
// high above hover, -d..d (one texel) through. r is its widest half,
// which a leaning monster keeps inside its square.
type monsterBox struct{ r, h, hover, x0, x1, d float32 }

// outlineBox is the box of a 16px sprite whose opaque bounds are bb,
// drawn with its opaque rows h high, turning about the cell's centre
// column.
func outlineBox(bb image.Rectangle, h, hover float32) monsterBox {
	px := h / float32(max(bb.Dy(), 1))
	return monsterBox{r: float32(max(8-bb.Min.X, bb.Max.X-8)) * px, h: h, hover: hover,
		x0: float32(bb.Min.X-8) * px, x1: float32(bb.Max.X-8) * px, d: px / 2}
}

// slab is a monster's hit box where it stands: its sprite's opaque
// outline (monsterBox) from the foot x, y, z, turned to the hero by yaw
// as drawMonsters draws it. Arrows, blows and the hero's body all meet
// this box.
type slab struct {
	x, y, z, yaw float64
	monsterBox
}

func (g *game) slab(m *monster) slab {
	b := g.box(m.letter)
	x, y, z := g.monsterPos(m, 0)
	return slab{x, y + float64(b.hover), z, math.Atan2(g.player.X-x, -(g.player.Z - z)), b}
}

// local is world point x, y, z in the slab's frame: the box spans
// x0..x1, 0..h, -d..d, and the hero is toward -z.
func (s slab) local(x, y, z float64) (lx, ly, lz float64) {
	c, sn := math.Cos(s.yaw), math.Sin(s.yaw)
	dx, dz := x-s.x, z-s.z
	return c*dx + sn*dz, y - s.y, -sn*dx + c*dz
}

// world is local's inverse.
func (s slab) world(lx, ly, lz float64) (x, y, z float64) {
	c, sn := math.Cos(s.yaw), math.Sin(s.yaw)
	return s.x + c*lx - sn*lz, s.y + ly, s.z + sn*lx + c*lz
}

// enter is how far (0..1) along the segment it enters the box.
func (s slab) enter(x0, y0, z0, x1, y1, z1 float64) (float64, bool) {
	ax, ay, az := s.local(x0, y0, z0)
	bx, by, bz := s.local(x1, y1, z1)
	lo, hi := [3]float64{float64(s.x0), 0, -float64(s.d)}, [3]float64{float64(s.x1), float64(s.h), float64(s.d)}
	p, d := [3]float64{ax, ay, az}, [3]float64{bx - ax, by - ay, bz - az}
	enter, exit := 0.0, 1.0
	for i := range 3 {
		if d[i] == 0 {
			if p[i] < lo[i] || p[i] > hi[i] {
				return 0, false
			}
			continue
		}
		t0, t1 := (lo[i]-p[i])/d[i], (hi[i]-p[i])/d[i]
		enter, exit = max(enter, min(t0, t1)), min(exit, max(t0, t1))
	}
	return enter, enter <= exit
}

// nearest is the point of the box's footprint nearest x, z (on its face
// toward the hero when x, z is inside) and how far x, z is from it.
func (s slab) nearest(x, z float64) (nx, nz, d float64) {
	lx, _, lz := s.local(x, 0, z)
	cx, cz := math.Max(float64(s.x0), math.Min(float64(s.x1), lx)), math.Max(-float64(s.d), math.Min(float64(s.d), lz))
	d = math.Hypot(lx-cx, lz-cz)
	if d == 0 {
		cz = -float64(s.d)
	}
	nx, _, nz = s.world(cx, 0, cz)
	return nx, nz, d
}

// box is a monster's hit box; a letter without a sprite gets the whole cell.
func (g *game) box(letter byte) monsterBox {
	if b, ok := g.boxes[letter]; ok {
		return b
	}
	bb := image.Rect(0, 0, 16, 16)
	if img := g.sprite["m:"+rogue.Monsters[letter-'A'].Name]; img != nil {
		bb = bbox(img)
	}
	lk := monsterLooks[letter]
	b := outlineBox(bb, lk.h*monsterScale, lk.hover)
	if g.boxes == nil {
		g.boxes = map[byte]monsterBox{}
	}
	g.boxes[letter] = b
	return b
}

// drawMonsters draws every visible monster as its extruded sprite, a
// billboard turned to the hero, its creature as tall as its look.
func (g *game) drawMonsters(alpha float64) {
	g.insts = g.insts[:0]
	seeInvis := g.seeInvis > 0 || g.wearing(rogue.RSeeInvis)
	for _, m := range g.monsters {
		if m.flags&rogue.IsInvis != 0 && !seeInvis || m.disguise != nil {
			continue
		}
		lk := g.box(m.letter)
		if m.dying > 0 {
			g.insts = append(g.insts, g.corpsePose(m, alpha))
			continue
		}
		x, y, z := g.monsterPos(m, alpha)
		y += float64(lk.hover)
		if lk.hover > 0 {
			y += 0.1 * math.Sin(float64(g.tick)*0.15+x+z) // a phase of its own that drifts, never jumps, as it moves
		}
		dx, dz := g.player.X-x, g.player.Z-z
		yaw := math.Atan2(dx, -dz)
		if m.lunge > 0 { // jab toward the hero
			d := math.Hypot(dx, dz) + 1e-6
			s := 0.5 * math.Sin(float64(8-m.lunge)/8*math.Pi)
			x, z = x+dx/d*s, z+dz/d*s
		}
		if m.hurt > 0 {
			x += 0.06 * math.Sin(float64(m.hurt)*2.5)
		}
		in := g.monsterPose(m.letter, x, y, z, yaw, 0)
		in.Sky, in.Block = g.light(x, y+float64(lk.h)/2, z)
		g.insts = append(g.insts, in)
	}
	for _, m := range g.corpses[g.depth] {
		if m.flags&rogue.IsInvis == 0 || seeInvis {
			g.insts = append(g.insts, g.corpsePose(m, alpha))
		}
	}
	g.itemRend.DrawPosed(&g.cam, g.insts)
	if g.showBoxes {
		for _, m := range g.monsters {
			s := g.slab(m)
			mat := mgl32.HomogRotate3DY(float32(-s.yaw)).Mul4(mgl32.Translate3D(s.x0, 0, -s.d)).Mul4(mgl32.Scale3D(s.x1-s.x0, s.h, 2*s.d))
			g.eng.Renderer.DrawOrientedOutline(&g.cam, s.x, s.y, s.z, mat, mgl32.Vec4{1, 0.2, 0.2, 1})
		}
	}
}

// monsterPose is a monster's sprite slab at x, y, z turned to yaw, tipped
// back tilt radians about its feet.
func (g *game) monsterPose(letter byte, x, y, z, yaw float64, tilt float32) render.ItemPose {
	lk, sp := g.box(letter), g.monSprite[letter]
	s := lk.h * 16 / max(sp.rows, 1)
	// the slab pivots at the feet; its centre sits where the creature's centre is h/2 up
	mat := mgl32.HomogRotate3DY(float32(-yaw)).Mul4(mgl32.HomogRotate3DX(tilt)).
		Mul4(mgl32.Translate3D(0, lk.h/2-s*(0.5-sp.cy/16), 0)).Mul4(mgl32.Scale3D(s, s, s))
	return render.ItemPose{Item: "m:" + rogue.Monsters[letter-'A'].Name, X: x, Y: y, Z: z, M: mat}
}

// fallTicks is how long a killed monster takes to tip over (a flyer to
// drop to the floor).
const fallTicks = 10

// tickCorpses runs the fall; then a corpse lies still.
func (g *game) tickCorpses() {
	for _, m := range g.corpses[g.depth] {
		m.dying = min(m.dying+1, fallTicks)
	}
}

// corpsePose is a killed monster where it fell, turned as it was at the
// blow, tipping over until it lies flat; nothing about it follows the
// hero.
func (g *game) corpsePose(m *monster, alpha float64) render.ItemPose {
	if m.pin != nil {
		return g.pinnedPose(m, alpha)
	}
	t := 1.0
	if m.dying < fallTicks {
		t = (float64(m.dying) + alpha) / fallTicks
	}
	y := m.dead[1] + float64(g.box(m.letter).hover)*(1-t)
	in := g.monsterPose(m.letter, m.dead[0], y, m.dead[2], m.dead[3], float32(t*math.Pi/2))
	if g.eng != nil {
		in.Sky, in.Block = g.light(m.dead[0], m.dead[1]+0.5, m.dead[2])
	}
	return in
}

// lineUp (debug, -mobs) stands one monster per letter on the hero's
// room's nearest free tiles two or more away, asleep, and turns the
// camera to the first.
func (g *game) lineUp(letters string) {
	l, h := g.level(), g.heroTile()
	rm := l.RoomAt(h)
	var free []dungeon.Coord
	for y := rm.Pos.Y; y < rm.Pos.Y+rm.Max.Y; y++ {
		for x := rm.Pos.X; x < rm.Pos.X+rm.Max.X; x++ {
			c := dungeon.Coord{X: x, Y: y}
			if d := max(abs(c.X-h.X), abs(c.Y-h.Y)); d >= 2 && l.At(c).Walkable() && g.monsterAt(c) == nil {
				free = append(free, c)
			}
		}
	}
	slices.SortFunc(free, func(a, b dungeon.Coord) int {
		return (a.X-h.X)*(a.X-h.X) + (a.Y-h.Y)*(a.Y-h.Y) - (b.X-h.X)*(b.X-h.X) - (b.Y-h.Y)*(b.Y-h.Y)
	})
	for i := 0; i < len(letters) && i < len(free); i++ {
		m := g.newMonster(letters[i], free[i])
		m.held = 1 << 30
		if i == 0 {
			x, _, z := tileCenter(g.depth, free[i])
			g.cam.Yaw = math.Atan2(x-g.player.X, -(z - g.player.Z))
		}
	}
}
