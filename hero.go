package main

// The hero: Rogue's pstats in a first-person body. Real time is cut into
// Rogue turns of turnTicks; the daemons (hunger, healing, wandering
// monsters) and timed effects run once per turn.

import (
	"fmt"
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
)

const (
	turnTicks   = 10 // a Rogue turn is half a second
	hungerTime  = 1300
	moreTime    = 150
	stomachSize = 2000
	starveTime  = 850
	wanderTime  = 70
	healTime    = 30
	searchTicks = 10 // one search per this many ticks while F is held
	holdTime    = 2  // turns: potion of paralysis
	sleepTime   = 5  // turns: sleeping gas trap
	bearTime    = 3  // turns: bear trap
	seeDuration = 850
	amuletLevel = 26 // the Amulet lies on levels deeper than 25
)

type heroState struct {
	stats    rogue.Stats
	maxStr   rogue.Str
	pack     []*rogue.Object
	weapon   *rogue.Object
	armor    *rogue.Object
	rings    [2]*rogue.Object
	purse    int
	maxLevel int
	amulet   bool // carrying the Amulet of Yendor
	won      bool // made it out of the dungeon with it
	fungHit  int  // fung_hit: how hard the violet fungi's hold has grown

	foodLeft    int
	hungry      int // 0 fine, 1 hungry, 2 weak, 3 fainting
	quiet       int // turns without a fight (healing)
	between     int // wandering-monster daemon
	wanderPause int // turns until wandering monsters roll again

	// timed effects, in ticks
	noCommand, confused, blind, hasted, seeInvis int
	held                                         bool // a violet fungus has hold of the hero
	heldMsg                                      int  // ticks until "You are being held" again
	turnClock                                    int
	attackCD, swing                              int
	searchCD                                     int
	hurt                                         int // red flash
	lastTile                                     dungeon.Coord
	lantern                                      voxel.BlockPos
	hasLantern                                   bool
	dead                                         bool
	killer                                       string
	walkDist, prevWalk                           float64
	seen                                         [maxDepth][dungeon.Lines][dungeon.Cols]bool
	kept                                         map[int]levelThings // what is on the levels the hero left
}

// levelThings is what lives and lies on a level.
type levelThings struct {
	monsters []*monster
	floor    []*floorObj
}

// initHero rolls up the rogue (init_player) with the starting pack.
func (g *game) initHero() {
	g.stats = rogue.Stats{Lvl: 1, HP: 12, MaxHP: 12, Str: rogue.Str{Str: 16}, Dmg: "1d4", Arm: 10}
	if rogue.Rnd(g.rng, 100) == 7 {
		g.stats.Str = rogue.Str{Str: 18, Add: rogue.Rnd(g.rng, 100) + 1}
	}
	g.maxStr = g.stats.Str
	g.foodLeft = hungerTime
	mace := rogue.NewWeapon(g.rng, rogue.Mace)
	mace.HPlus, mace.DPlus, mace.Flags = 1, 1, mace.Flags|rogue.IsKnow
	bow := rogue.NewWeapon(g.rng, rogue.Bow)
	bow.HPlus, bow.Flags = 1, bow.Flags|rogue.IsKnow
	arrows := rogue.NewWeapon(g.rng, rogue.Arrow)
	arrows.Count, arrows.Flags, arrows.Group = 25+rogue.Rnd(g.rng, 15), arrows.Flags|rogue.IsKnow, -1
	mail := rogue.NewArmor(rogue.RingMail)
	mail.AC--
	mail.Flags |= rogue.IsKnow
	food := &rogue.Object{Type: rogue.Food, Count: 1, Launch: -1}
	sling := rogue.NewWeapon(g.rng, rogue.Sling)
	sling.Flags |= rogue.IsKnow
	rocks := rogue.NewWeapon(g.rng, rogue.RockW)
	rocks.Flags |= rogue.IsKnow
	g.pack = []*rogue.Object{mace, bow, arrows, sling, rocks, mail, food}
	g.weapon, g.armor = mace, mail
}

// enterLevel puts the hero on a level at a random spot (the start, and
// where falling or magic leaves them).
func (g *game) enterLevel(depth int) {
	g.switchLevel(depth)
	g.placeHero(g.level().RandomFloor(g.rng))
}

// switchLevel makes depth the level the hero is on, keeping what is on
// the one left. The first time, it is stocked as Rogue's new_level()
// does: objects and monsters.
func (g *game) switchLevel(depth int) {
	if g.depth > 0 {
		if g.kept == nil {
			g.kept = map[int]levelThings{}
		}
		g.kept[g.depth] = levelThings{g.monsters, g.floor}
	}
	g.depth = depth
	g.maxLevel = max(g.maxLevel, depth)
	g.distTick = -1
	g.clearArrows()
	g.held = false
	g.wanderPause = 0
	g.msg(fmt.Sprintf("You are on level %d of the Dungeons of Doom.", depth))
	if k, ok := g.kept[depth]; ok {
		g.monsters, g.floor = k.monsters, k.floor
		return
	}
	g.rg.NoFood++
	g.monsters, g.floor = nil, nil
	l := g.level()
	for i := range l.Rooms {
		rm := &l.Rooms[i]
		if rm.Gone {
			continue
		}
		// with the Amulet, the levels on the way up keep their gold
		if rm.GoldVal > 0 && (!g.amulet || depth >= g.maxLevel) {
			g.floor = append(g.floor, &floorObj{o: &rogue.Object{Type: rogue.Gold, GoldVal: rm.GoldVal, Count: 1}, tile: rm.Gold})
		}
		rm.GoldVal = 0
		chance := 25
		if g.goldIn(rm) {
			chance = 80
		}
		if rogue.Rnd(g.rng, 100) < chance {
			c := l.RndPos(rm, g.rng)
			if l.At(c) == dungeon.Floor {
				m := g.newMonster(rogue.RandMonster(g.rng, depth, false), c)
				if rogue.Rnd(g.rng, 100) < rogue.Monsters[m.letter-'A'].Carry {
					m.pack = g.rg.NewThing()
				}
			}
		}
	}
	g.putThings(l)
}

// putThings scatters the level's objects (put_things): none on the way
// back up with the Amulet, and the Amulet itself deep enough down.
func (g *game) putThings(l *dungeon.Level) {
	if g.amulet && g.depth < g.maxLevel {
		return
	}
	for i := 0; i < 9; i++ { // MAXOBJ tries
		if rogue.Rnd(g.rng, 100) < 35 {
			g.floor = append(g.floor, &floorObj{o: g.rg.NewThing(), tile: l.RandomFloor(g.rng)})
		}
	}
	if g.depth >= amuletLevel && !g.amulet {
		g.floor = append(g.floor, &floorObj{o: &rogue.Object{Type: rogue.Amulet, Count: 1, Launch: -1}, tile: l.RandomFloor(g.rng)})
	}
}

func (g *game) goldIn(rm *dungeon.Room) bool {
	for _, f := range g.floor {
		if f.o.Type == rogue.Gold && rm.Inside(f.tile) {
			return true
		}
	}
	return false
}

// placeHero stands the hero on a tile, facing into the room.
func (g *game) placeHero(c dungeon.Coord) {
	x, y, z := tileCenter(g.slot(), c)
	if g.player == nil {
		g.player = character.New(x, y, z)
	} else {
		g.player.Teleport(x, y, z)
	}
	g.lastTile = dungeon.Coord{X: -1}
	g.clearLantern()
}

// tickHero moves the hero and runs the per-tick and per-turn rules.
func (g *game) tickHero() {
	p := g.player
	for _, t := range []*int{&g.noCommand, &g.confused, &g.blind, &g.hasted, &g.seeInvis, &g.attackCD, &g.swing, &g.searchCD, &g.hurt, &g.heldMsg} {
		if *t > 0 {
			*t--
			if t == &g.confused && *t == 0 {
				g.msg("You feel less confused now.")
			}
			if t == &g.blind && *t == 0 {
				g.msg("The veil of darkness lifts.")
			}
			if t == &g.hasted && *t == 0 {
				g.msg("You feel yourself slowing down.")
			}
			if t == &g.seeInvis && *t == 0 {
				g.look()
			}
		}
	}
	in := g.moveInput()
	if g.noCommand > 0 || g.won {
		in.Forward, in.Strafe, in.Jump = 0, 0, false
	}
	if g.held && (in.Forward != 0 || in.Strafe != 0) && !g.towardFungus(in) {
		in.Forward, in.Strafe, in.Jump = 0, 0, false
		if g.heldMsg == 0 {
			g.msg("You are being held.")
			g.heldMsg = 2 * turnTicks
		}
	}
	if g.confused > 0 && (in.Forward != 0 || in.Strafe != 0) {
		in.Yaw += math.Sin(float64(g.tick)*0.37) * 1.4
	}
	in.Sprint = g.hasted > 0
	if g.drawing { // Minecraft slows the hero while a bow is drawn
		in.Forward *= 0.2
		in.Strafe *= 0.2
		in.Sprint = false
	}
	p.Tick(g.terrain, in)
	g.pushOutOfMonsters()
	g.prevWalk = g.walkDist
	if p.OnGround {
		g.walkDist += math.Hypot(p.X-p.PrevX, p.Z-p.PrevZ)
	}
	g.updateLantern()

	// halfway down a staircase the hero is on the level below; Rogue's
	// way up is shut without the Amulet
	if d := depthAt(p.Y); d != g.depth && d >= 1 && d <= maxDepth {
		if d < g.depth && !g.amulet {
			p.Teleport(p.PrevX, p.PrevY, p.PrevZ)
			if g.heldMsg == 0 {
				g.msg("You cannot climb without the Amulet.")
				g.heldMsg = 2 * turnTicks
			}
		} else {
			g.switchLevel(d)
		}
	}
	// on a staircase, under or over the map, no tile is underfoot
	onFloor := math.Abs(p.Y-float64(floorY(g.depth)+1)) < 0.6
	if t := g.heroTile(); t != g.lastTile && onFloor {
		g.lastTile = t
		g.look()
		g.pickUp()
		g.checkTrap()
	}
	if g.wantSearch() && g.searchCD == 0 {
		g.searchCD = searchTicks
		g.search()
	}
	if g.wantAttack() && g.attackCD == 0 && g.noCommand == 0 && !g.chainLoaded() {
		g.heroAttack()
	}
	g.tickCombat()
	g.tickHand()
	if g.turnClock++; g.turnClock >= turnTicks {
		g.turnClock = 0
		g.turn()
	}
}

// turn runs the daemons once per Rogue turn.
func (g *game) turn() {
	g.doctor()
	g.stomach()
	g.rollWander()
	if g.wearing(rogue.RSearch) {
		g.search()
	}
	if g.wearing(rogue.RTeleport) && rogue.Rnd(g.rng, 50) == 17 {
		g.teleport()
	}
}

// doctor regains hit points over time (daemons.c).
func (g *game) doctor() {
	lv, ohp := g.stats.Lvl, g.stats.HP
	g.quiet++
	if lv < 8 {
		if g.quiet > 20-lv*2 {
			g.stats.HP++
		}
	} else if g.quiet >= 3 {
		g.stats.HP += rogue.Rnd(g.rng, lv-7) + 1
	}
	for _, r := range g.rings {
		if r != nil && r.Which == rogue.RRegen {
			g.stats.HP++
		}
	}
	if ohp != g.stats.HP {
		g.stats.HP = min(g.stats.HP, g.stats.MaxHP)
		g.quiet = 0
	}
}

// stomach digests food (daemons.c).
func (g *game) stomach() {
	if g.foodLeft <= 0 {
		if g.foodLeft--; g.foodLeft < -starveTime {
			g.die("starvation")
			return
		}
		if g.noCommand > 0 || rogue.Rnd(g.rng, 100) > 20 {
			return
		}
		g.noCommand = (rogue.Rnd(g.rng, 8) + 4) * turnTicks
		g.hungry = 3
		g.msg("You faint from lack of food.")
		return
	}
	old := g.foodLeft
	amulet := 0
	if g.amulet {
		amulet = 1
	}
	g.foodLeft -= g.ringEat(0) + g.ringEat(1) + 1 - amulet
	switch {
	case g.foodLeft < moreTime && old >= moreTime:
		g.hungry = 2
		g.msg("You are starting to feel weak.")
	case g.foodLeft < 2*moreTime && old >= 2*moreTime:
		g.hungry = 1
		g.msg("You are starting to get hungry.")
	}
}

// ringEat is the extra food a ring costs each turn (ring_eat).
func (g *game) ringEat(hand int) int {
	r := g.rings[hand]
	if r == nil {
		return 0
	}
	switch r.Which {
	case rogue.RRegen:
		return 2
	case rogue.RSustStr:
		return 1
	case rogue.RSearch:
		if rogue.Rnd(g.rng, 100) < 33 {
			return 1
		}
	case rogue.RDigest:
		if rogue.Rnd(g.rng, 100) < 50 {
			return -1
		}
	}
	return 0
}

// rollWander may start a wandering monster (daemons.c rollwand).
func (g *game) rollWander() {
	if g.wanderPause > 0 {
		g.wanderPause--
		return
	}
	if g.between++; g.between >= 4 {
		if rogue.Roll(g.rng, 1, 6) == 4 {
			g.wanderer()
			g.wanderPause = wanderTime
		}
		g.between = 0
	}
}

// look marks what the hero sees from the current tile: a lit room whole,
// else the tiles around. Monsters in sight may wake.
func (g *game) look() {
	l, c := g.level(), g.heroTile()
	seen := &g.seen[g.slot()-1]
	if rm := l.RoomAt(c); rm != nil && !rm.Dark && g.blind == 0 {
		for y := rm.Pos.Y; y < rm.Pos.Y+rm.Max.Y; y++ {
			for x := rm.Pos.X; x < rm.Pos.X+rm.Max.X; x++ {
				seen[y][x] = true
			}
		}
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if y, x := c.Y+dy, c.X+dx; y >= 0 && x >= 0 && y < dungeon.Lines && x < dungeon.Cols {
				seen[y][x] = true
			}
		}
	}
	for _, m := range g.monsters {
		if g.canSee(m.tile) {
			g.wakeMonster(m)
		}
	}
}

// canSee reports whether the hero sees tile c: same lit room, or next to it.
func (g *game) canSee(c dungeon.Coord) bool {
	if g.blind > 0 {
		return false
	}
	h := g.heroTile()
	if abs(c.X-h.X) <= 1 && abs(c.Y-h.Y) <= 1 {
		return true
	}
	rm := g.level().RoomAt(h)
	return rm != nil && !rm.Dark && rm.Inside(c)
}

// search looks for secret doors and traps around the hero.
func (g *game) search() {
	l, h := g.level(), g.heroTile()
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			c := dungeon.Coord{X: h.X + dx, Y: h.Y + dy}
			if l.At(c) == dungeon.SecretDoor && !g.gen.isFound(g.slot(), c) && rogue.Rnd(g.rng, 100) < 20 {
				g.gen.markFound(g.slot(), c)
				g.redrawTile(c)
				g.msg("You found a secret door.")
			}
			if t := l.TrapAt(c); t != nil && !t.Found && rogue.Rnd(g.rng, 100) < 50 {
				g.foundTrap(t)
				g.msg("You found a " + t.Kind.String() + ".")
			}
		}
	}
}

func (g *game) foundTrap(t *dungeon.Trap) {
	t.Found = true
	g.gen.markFound(g.slot(), t.Pos)
	g.redrawTile(t.Pos)
}

// redrawTile regenerates the blocks of one tile after it changed (a found
// secret door or trap).
func (g *game) redrawTile(c dungeon.Coord) {
	l := g.level()
	f := floorY(g.slot())
	for sz := 0; sz < tileD; sz++ {
		for sx := 0; sx < tileW; sx++ {
			x, z := c.X*tileW+sx, c.Y*tileD+sz
			col := map[int]voxel.StateID{}
			g.gen.column(func(y int, id voxel.StateID) { col[y] = id }, g.slot(), l, c, sx, sz)
			for y := f; y <= f+roomHeight+1; y++ {
				if id, ok := col[y]; ok && g.eng.Block(x, y, z) != id {
					g.eng.SetBlock(x, y, z, id)
				}
			}
		}
	}
}

// checkTrap springs a trap under the hero (move.c be_trapped).
func (g *game) checkTrap() {
	t := g.level().TrapAt(g.heroTile())
	if t == nil {
		return
	}
	g.foundTrap(t)
	switch t.Kind {
	case dungeon.TrapDoor:
		g.msg("You fell into a trap!")
		if g.depth < maxDepth {
			c := g.heroTile()
			g.switchLevel(g.depth + 1)
			if g.level().At(c) != dungeon.Floor {
				c = g.level().RandomFloor(g.rng)
			}
			g.placeHero(c)
		}
	case dungeon.BearTrap:
		g.noCommand += bearTime * turnTicks
		g.msg("You are caught in a bear trap.")
	case dungeon.SleepTrap:
		g.noCommand += sleepTime * turnTicks
		g.msg("A strange white mist envelops you and you fall asleep.")
	case dungeon.ArrowTrap:
		if g.trapHits(g.stats.Lvl-1, "1d6") {
			g.msg("Oh no! An arrow shot you.")
			if g.stats.HP <= 0 {
				g.die("an arrow")
			}
		} else {
			arrow := rogue.NewWeapon(g.rng, rogue.Arrow)
			arrow.Count = 1
			g.dropMissile(arrow, g.heroTile())
			g.msg("An arrow shoots past you.")
		}
	case dungeon.TeleTrap:
		g.teleport()
	case dungeon.DartTrap:
		if g.trapHits(g.stats.Lvl+1, "1d4") {
			if !g.wearing(rogue.RSustStr) {
				g.chgStr(-1)
			}
			g.msg("A small dart just hit you in the shoulder.")
			if g.stats.HP <= 0 {
				g.die("a poison dart")
			}
		} else {
			g.msg("A small dart whizzes by your ear and vanishes.")
		}
	}
}

// trapHits is a trap's blow at the hero (move.c: swing(lvl, s_arm, 1)
// against the bare armour 10, plain dice, no strength) and its damage.
func (g *game) trapHits(lvl int, dmg string) bool {
	att := rogue.Stats{Lvl: lvl, Dmg: dmg, Str: rogue.Str{Str: 10}}
	rogue.RollEm(g.rng, &att, &g.stats, 10, rogue.Attack{HPlus: 1})
	g.hurt = 6
	return true
}

// teleport moves the hero to a random spot on the level, out of a
// fungus's grip (wizard.c teleport).
func (g *game) teleport() {
	g.vanish(g.player.X, g.player.Y, g.player.Z)
	g.placeHero(g.level().RandomFloor(g.rng))
	g.vanish(g.player.X, g.player.Y, g.player.Z)
	g.releaseFungus()
	g.msg("You feel a wrenching sensation.")
}

// releaseFungus ends a violet fungus's hold (killed, teleport).
func (g *game) releaseFungus() {
	g.held, g.fungHit = false, 0
	rogue.Monsters['F'-'A'].Dmg = "000d0"
}

// towardFungus reports whether the hero's step heads for the fungus that
// holds them: Rogue lets a held hero move only to fight it.
func (g *game) towardFungus(in character.Input) bool {
	yaw := in.Yaw + math.Atan2(in.Strafe, in.Forward)
	fx, fz := math.Sin(yaw), -math.Cos(yaw)
	for _, m := range g.monsters {
		if m.letter != 'F' || !m.alive() {
			continue
		}
		mx, _, mz := g.monsterPos(m, 0)
		dx, dz := mx-g.player.X, mz-g.player.Z
		if d := math.Hypot(dx, dz); d < 4 && (dx*fx+dz*fz)/d > 0.7 {
			return true
		}
	}
	return false
}

// ascend is the < command (u_level): the dungeon's levels are climbed on
// foot, but from the first level's stairs, with the Amulet, out.
func (g *game) ascend() {
	if g.depth != 1 || g.level().At(g.heroTile()) != dungeon.Stairs || !g.amulet {
		g.msg("I see no way up.")
		return
	}
	g.totalWinner()
}

// totalWinner ends the game in victory: the loot is sold (total_winner)
// and the worth table shown.
func (g *game) totalWinner() {
	g.won = true
	g.winLoot = g.winLoot[:0]
	gold := g.purse
	for _, o := range g.pack {
		w := g.rg.Worth(o)
		g.winLoot = append(g.winLoot, fmt.Sprintf("%c) %5d  %s", g.packLetter(o), w, g.rg.Name(o)))
		g.purse += w
	}
	g.winLoot = append(g.winLoot, fmt.Sprintf("   %5d  Gold Pieces", gold))
	g.showInv, g.showMap, g.showHelp, g.showDebug = false, false, false, false
	g.capture(false)
}

// armorClass is the hero's armour class with rings.
func (g *game) armorClass() int {
	ac := g.stats.Arm
	if g.armor != nil {
		ac = g.armor.AC
	}
	for _, r := range g.rings {
		if r != nil && r.Which == rogue.RProtect {
			ac -= r.AC
		}
	}
	return ac
}

func (g *game) wearing(which int) bool {
	for _, r := range g.rings {
		if r != nil && r.Which == which {
			return true
		}
	}
	return false
}

// chgStr changes strength, as chg_str.
func (g *game) chgStr(amt int) {
	s := &g.stats.Str
	if amt < 0 {
		for ; amt < 0; amt++ {
			if s.Str == 18 && s.Add > 0 {
				s.Add = max(0, s.Add-10)
			} else if s.Str > 3 {
				s.Str--
			}
		}
		return
	}
	for ; amt > 0; amt-- {
		if s.Str < 18 {
			s.Str++
		} else if s.Add < 100 {
			s.Add = min(100, s.Add+rogue.Rnd(g.rng, 50)+1)
		}
	}
	if s.Str > g.maxStr.Str || s.Str == g.maxStr.Str && s.Add > g.maxStr.Add {
		g.maxStr = *s
	}
}

// gainExp adds experience and raises the level (check_level).
func (g *game) gainExp(n int) {
	g.stats.Exp += n
	lv := rogue.LevelFor(g.stats.Exp)
	if lv > g.stats.Lvl {
		add := rogue.Roll(g.rng, lv-g.stats.Lvl, 10)
		g.stats.MaxHP += add
		g.stats.HP = min(g.stats.HP+add, g.stats.MaxHP)
		g.msg(fmt.Sprintf("Welcome to level %d", lv))
	}
	g.stats.Lvl = lv
}

// die ends the game.
func (g *game) die(killer string) {
	if g.dead {
		return
	}
	g.dead, g.killer = true, killer
	g.stats.HP = 0
	g.showInv = false
	g.capture(false)
}

// updateLantern keeps a faint light at the hero's head, so dark rooms
// and passages show the squares around them, as in Rogue.
func (g *game) updateLantern() {
	p := g.player
	pos := voxel.BlockPos{X: int(math.Floor(p.X)), Y: int(math.Floor(p.Y + 1.5)), Z: int(math.Floor(p.Z))}
	if g.hasLantern && pos == g.lantern {
		return
	}
	if g.eng.Block(pos.X, pos.Y, pos.Z) != voxel.Air {
		return
	}
	g.clearLantern()
	g.eng.SetBlock(pos.X, pos.Y, pos.Z, g.gen.b.lantern)
	g.lantern, g.hasLantern = pos, true
}

func (g *game) clearLantern() {
	if g.hasLantern && g.eng.Block(g.lantern.X, g.lantern.Y, g.lantern.Z) == g.gen.b.lantern {
		g.eng.SetBlock(g.lantern.X, g.lantern.Y, g.lantern.Z, voxel.Air)
	}
	g.hasLantern = false
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
