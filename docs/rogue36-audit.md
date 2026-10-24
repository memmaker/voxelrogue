# VoxelRogue vs Rogue 3.6 (source: ~/Games/rogue3.6), 24 Oct 2026

Read-only audit of the port against the C source, by area. "Adapted" lists the
deliberate real-time/3D changes; "Missing" and "Different" are the gaps.
Verified identical and not listed: the monster table (all 26 entries, flags,
exp, level, armour, damage), lvl_mons/wand_mons, new_monster HP, wanderers
and rollwand, wake logic, umber hulk gaze, the special attacks of
R E A W F L N, swing/StrPlus/save_throw, e_levels (with the 40920 typo),
doctor and ring_eat rates, hunger thresholds, eat, init_player, room and
passage maths, dark rooms, GOLDCALC, trap counts and kinds, MAXOBJ odds,
amulet placement, object tables and worth, new_thing curses, potion and
scroll effects, ring effects, drain life, bolt bounce-back distance.

## Bugs (unintended differences)

- A found secret door stays a wall for monsters: `search` only sets
  `gen.found`, `dungeon.Walkable` excludes SecretDoor, so stepOK/updateDist
  never path through it and a hero standing in one empties the distance
  field (monsters stop chasing). C rewrites the tile to DOOR.
- Hand-thrown missiles use the launcher damage: C `fight.c roll_em` uses
  `o_damage` for an ISMISL weapon thrown without its launcher (dagger 1d6,
  dart 1d1, spear 1d8); Go uses HurlDmg (1d4, 1d3, 1d6) unless Launch >= 0.
- add_dam 18/xx bands: C 100→6, >90→5, >75→4, ≠0→3, else 2; `rules.go AddDam`
  has >80→4, >50→3 (18/01-18/50 give 2, 18/76-18/80 give 3).
- Starvation kills after 850 more turns (`hero.go`); C `daemons.c stomach`
  never kills, the hero faints 20%/turn for ever at food ≤ 0.
- Haste while hasted: C `misc.c add_haste` faints rnd(8) turns and leaves
  ISHASTE set for good; Go clears haste.
- chg_str: C steps 18/xx through bands (up: 0→rnd(50)+1→51+rnd(24)→76+rnd(14)
  →91→+1; down: <51→0, <76→1+rnd(50), <91→51+rnd(25), <100→76+rnd(14), else
  91+rnd(8)); Go adds Rnd(50)+1 capped at 100 and subtracts 10.
- Polymorph keeps confused/cancelled/speed/held/pack; C `new_monster` resets
  t_flags and t_pack.
- Magic missile bolt has `o_dplus = 1` in C; Go HPlus 100, DPlus 0.
- Wand charge is spent before the "too weak" check of drain life; C spends
  it after the effect.
- Wandering monsters: C arms the swander fuse (70 turns) at game start and
  never resets it on a new level; Go rolls at once and resets to 0 on every
  level change.
- Rings: C searches once per ring of searching and rolls teleport
  rnd(100)<2 per ring; Go once. Protection: C counts one ring, Go both.
  Bare hands get no increase-damage/accuracy ring bonus in Go.
- Gone rooms: C picks rnd(4) distinct rooms; Go may pick the same twice and
  un-gones the landing room. Room monsters: C retries rnd_pos until FLOOR,
  Go rolls once and skips (fewer monsters). no_food counts only first visits.
- Dart trap drains strength before damage (C after). Trap found 50% vs C 51%.
- Pack: C refuses at 22 items counting potions/scrolls/food singly; Go 23
  slots counting stacks once, and stacking pickups never fail. New items
  append instead of grouping by type.
- A thrown dagger/spear that hits lands on the monster's tile; C's is gone.
- Magic detection knowledge counts floor gold and monster packs; C only
  lvl_obj. Scare-monster dusts before the full-pack check. Amulet flag
  cleared on drop (C never).
- Confused monster next to the hero never attacks (randomStep excludes the
  hero tile). Held by a fungus, the hero can hit any monster (C only 'F').
- Slow/haste monster change only move speed, not attack rate (C both).
  Hasted hero only sprints, attack rate unchanged (C doubles all actions).
- Diagonal steps into/out of doors are forbidden (a 5.x rule; 3.6 allows).
- Blindness: searching works while blind (C returns), combat messages name
  the monster (C says "it").
- Trapdoor at the bottom level does nothing.

## Missing

- Score file and top ten, `quit` with confirmation, player name and year on
  the tombstone, "You can move again." when paralysis ends, "You feel a
  wrenching sensation in your gut." on climbing, "As you read the scroll, it
  vanishes.", "You are still stuck in the bear trap".
- Commands: call (naming unknown items), options, save, version, repeat
  message, identify symbol, picky inventory, per-key help. Wizard commands
  are flags and debug keys instead.
- "Left or right hand?" for rings; wielding non-weapons (a staff swung drains
  to 0d0); drop side effects (ring of add strength reverses chg_str, dropping
  worn armour costs a turn).
- Four phrasings each for hit and miss; "You hit" before "You have defeated";
  thrown messages naming the weapon; drain-life kill suppressed for unseen
  monsters.

## Different on purpose (real time, 3D)

- No to-hit roll (24 Oct 2026): every blow that reaches lands, as in an
  action RPG; the swing roll's average, (level + armour + bonus)/20, scales
  the damage instead (`rogue.HitScale`), so the balance in expectation is
  the C game's. Traps use the bare armour 10 as `move.c` does.
- Haste = sprint; daemons run every half-second turn (10 ticks); levels
  persist and stack; stairs are walked; searching is held F; confusion
  steers the camera; rnd is math/rand; death/win overlay with restart.
- Chase is a BFS distance field; attacks by reach cone and cooldowns instead
  of bump-to-fight; monsters move by ticks per block, attack every 16 ticks.
- Missiles and zaps aim along the view with a line-of-fire check; arrows fly
  as projectiles and land by stacking; the wielded weapon cannot be thrown.
- Bear trap blocks all commands, not just moves. Trapdoor keeps x,y when the
  floor below allows. Mimic disguise is gold. Wear costs a 2-turn freeze.
- Status line shows Str cur(max) and AC with protection rings; level entry
  message; starting pack adds the sling and rocks; random weapons draw from
  41 kinds including the mod weapons.
