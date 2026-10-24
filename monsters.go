package main

// Monsters in real time. Each monster owns a map tile and walks to the
// next one over moveTicks, so Rogue's grid rules (doors, no corner
// cutting, one monster per square) still hold while the model glides.
// Running monsters step down a distance field from the hero and attack
// from an adjacent square every attackTicks, with fight.c's dice.

import (
	"fmt"
	"github.com/memmaker/voxellib/particles"
	"math"

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
	dying      int
	leanX      float64 // offset from the tile centre toward the hero
	leanZ      float64 // while fighting, so both sides are in reach
}

type monsterState struct {
	monsters  []*monster
	modelRend *render.ModelRenderer
	models    [26]int
	dist      [dungeon.Lines][dungeon.Cols]int
	distFrom  dungeon.Coord
	distTick  int
	insts     []render.ModelInstance
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
			m.disguise.Which = rogue.Rnd(g.rng, rogue.MaxArmors)
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
		if m.dying > 0 {
			if m.dying++; m.dying > 24 {
				g.loseMonster(m)
				continue
			}
			live = append(live, m)
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
			if m.moveT++; m.moveT >= m.moveTicks() {
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
			mx, _, mz := g.monsterPos(m, 0)
			adjacent = math.Hypot(mx-g.player.X, mz-g.player.Z) <= monsterReach+float64(monsterLooks[m.letter].w)/2
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

// monsterReach is how far a monster's attack reaches past its body.
const monsterReach = 1.6

// lean moves a monster that is fighting the hero from the next square to
// the side of its tile facing the hero (staying inside the tile), and
// back to the centre otherwise.
func (g *game) lean(m *monster, h dungeon.Coord) {
	var tx, tz float64
	if m.running && m.moveT == 0 && abs(m.tile.X-h.X) <= 1 && abs(m.tile.Y-h.Y) <= 1 && m.held == 0 {
		cx, _, cz := tileCenter(g.slot(), m.tile)
		half := float64(monsterLooks[m.letter].w) / 2
		ex, ez := math.Max(0, tileW/2.0-half-0.05), math.Max(0, tileD/2.0-half-0.05)
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
		for _, o := range g.pack {
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
				steal.Count--
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

// weaponFeel is how a weapon handles in real time: ticks between swings
// and reach in blocks (plus the target's size). Missing = 12, 2.4.
var weaponFeel = map[int]struct {
	cd    int
	reach float64
}{
	rogue.Dagger:     {8, 2.0},
	rogue.TwoSword:   {18, 2.6},
	rogue.Spear:      {13, 3.0},
	rogue.Broadsword: {14, 2.5},
	rogue.Glaive:     {16, 3.4},
	rogue.Greatsword: {18, 2.7},
	rogue.Halberd:    {18, 3.4},
	rogue.Katana:     {9, 2.5},
	rogue.Scythe:     {16, 3.0},
	rogue.WarSpear:   {13, 3.2},
	rogue.WarGlaive:  {16, 3.4},
	rogue.Scimitar:   {9, 2.4},
	rogue.WoodenPole: {11, 3.4},
}

// heroAttack swings the wielded weapon in one of its attack patterns
// (0 left button, 1 right button, 2 F): a Better Combat attack whose hit
// lands at its upswing (bettercombat.go).
func (g *game) heroAttack(pattern int) {
	cd, reach := 12, 2.4
	if w := g.meleeItem(); w != nil {
		if f, ok := weaponFeel[meleeKind(w)]; ok {
			cd, reach = f.cd, f.reach
		}
	}
	if g.startAttack(cd, reach, pattern) {
		g.attackCD = cd
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
		return rogue.Spear
	}
	return o.Which
}

// magic reports whether a blow was struck with a magic item (a wand or
// a bolt), which draws no blood.
func magic(o *rogue.Object) bool { return o != nil && o.Type == rogue.Stick }

// impact is where a melee blow meets a monster: on the view line, at the
// near side of its box.
func (g *game) impact(m *monster) (x, y, z float64) {
	mx, my, mz := g.monsterPos(m, 0)
	lk := monsterLooks[m.letter]
	d := math.Max(0, math.Hypot(mx-g.cam.X, mz-g.cam.Z)-float64(lk.w)/2)
	cp := math.Cos(g.cam.Pitch)
	x, y, z = g.cam.X+math.Sin(g.cam.Yaw)*cp*d, g.cam.Y+math.Sin(g.cam.Pitch)*d, g.cam.Z-math.Cos(g.cam.Yaw)*cp*d
	y = math.Min(math.Max(y, my+float64(lk.hover)), my+float64(lk.hover+lk.h))
	return
}

// bleed sprays blood from a point.
func (g *game) bleed(x, y, z float64) { g.parts.Emit(x, y, z, particles.Blood) }

// monsterInFront finds the nearest living monster within reach (plus its
// size) in a cone around the view direction.
func (g *game) monsterInFront(reach float64) *monster {
	p := g.player
	fx, fz := math.Sin(g.cam.Yaw), -math.Cos(g.cam.Yaw)
	var best *monster
	bd := math.MaxFloat64
	for _, m := range g.monsters {
		if !m.alive() {
			continue
		}
		mx, _, mz := g.monsterPos(m, 0)
		dx, dz := mx-p.X, mz-p.Z
		d := math.Hypot(dx, dz)
		if d > reach+float64(monsterLooks[m.letter].w)/2 || d < 1e-6 {
			continue
		}
		if (dx*fx+dz*fz)/d < 0.6 && d > 1 {
			continue
		}
		if d < bd {
			best, bd = m, d
		}
	}
	return best
}

// fight is fight.c fight(): the hero hits a monster with weap (nil =
// bare hands), thrown or not. Reaching it was the hit; nothing misses.
func (g *game) fight(m *monster, weap *rogue.Object, thrown bool) bool {
	g.quiet = 0
	m.runto()
	if m.disguise != nil && g.blind == 0 {
		g.unmask(m)
		if !thrown {
			return false
		}
	}
	a := rogue.Attack{}
	if weap != nil {
		a = rogue.Attack{Dmg: weap.Damage, HPlus: weap.HPlus, DPlus: weap.DPlus}
		if weap.Type == rogue.Stick && weap.Damage == "" { // a held wand fights as a spear at 3/5 the damage
			a = rogue.Attack{Dmg: rogue.WeaponDamage(rogue.Spear), Scale: 0.6}
		}
		if thrown {
			a.Dmg = weap.HurlDmg
			if weap.Flags&rogue.IsMissile != 0 && g.weapon != nil && rogue.Launches(g.weapon.Which, weap) {
				a.Dmg = rogue.LaunchDamage(g.weapon.Which, weap)
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
// walks.
func (g *game) monsterPos(m *monster, alpha float64) (x, y, z float64) {
	x, y, z = tileCenter(g.slot(), m.tile)
	x, z = x+m.leanX, z+m.leanZ
	if m.moveT > 0 {
		fx, _, fz := tileCenter(g.slot(), m.from)
		t := (float64(m.moveT) + alpha) / float64(m.moveTicks())
		t = math.Min(t, 1)
		x, z = fx+(x-fx)*t, fz+(z-fz)*t
	}
	return x, y, z
}

// pushOutOfMonsters keeps the hero from walking into monsters.
func (g *game) pushOutOfMonsters() {
	p := g.player
	for _, m := range g.monsters {
		if !m.alive() {
			continue
		}
		mx, _, mz := g.monsterPos(m, 0)
		r := float64(monsterLooks[m.letter].w)/2 + p.Width/2 + 0.05
		dx, dz := p.X-mx, p.Z-mz
		if d := math.Hypot(dx, dz); d < r {
			if m.disguise != nil { // walked into a mimic
				g.unmask(m)
				m.runto()
			}
			if d < 1e-6 {
				dx, dz, d = 1, 0, 1
			}
			// back out along the push, but never into a wall
			nx, nz := mx+dx/d*r, mz+dz/d*r
			if !g.terrain.Solid(int(math.Floor(nx)), int(math.Floor(p.Y+0.5)), int(math.Floor(nz))) {
				p.X, p.Z = nx, nz
			}
		}
	}
}

// setupMonsterModels uploads a stand-in model per letter.
func (g *game) setupMonsterModels() error {
	var err error
	if g.modelRend, err = render.NewModelRenderer(g.eng.Renderer); err != nil {
		return err
	}
	for i := range g.models {
		letter := byte('A' + i)
		lk := monsterLooks[letter]
		h, err := g.modelRend.Add(boxModel(fmt.Sprintf("monster_%c", letter), lk.w, lk.h, letterTexture(letter, lk.col)))
		if err != nil {
			return err
		}
		g.models[i] = h
	}
	return nil
}

func (g *game) drawMonsters(alpha float64) {
	g.insts = g.insts[:0]
	seeInvis := g.seeInvis > 0 || g.wearing(rogue.RSeeInvis)
	for _, m := range g.monsters {
		if m.flags&rogue.IsInvis != 0 && !seeInvis || m.disguise != nil {
			continue
		}
		lk := monsterLooks[m.letter]
		x, y, z := g.monsterPos(m, alpha)
		y += float64(lk.hover)
		if lk.hover > 0 {
			y += 0.1 * math.Sin(float64(g.tick)*0.15+float64(m.tile.X))
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
		in := render.ModelInstance{Model: g.models[m.letter-'A'], X: x, Y: y, Z: z, Yaw: yaw}
		if m.dying > 0 {
			in.Tilt = math.Min(1, float64(m.dying)/10) * math.Pi / 2
			in.TiltYaw = yaw + math.Pi
			in.Fade = float32(math.Max(0, float64(m.dying-10)/14))
		}
		in.Sky, in.Block = g.light(x, y+float64(lk.h)/2, z)
		g.insts = append(g.insts, in)
	}
	g.modelRend.Draw(&g.cam, g.insts)
}
