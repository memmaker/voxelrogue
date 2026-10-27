package dungeon

import (
	"math/rand"
	"testing"
)

// Every floor tile must be reachable from the stairs (secret doors count
// as open: they can be found), on every level of whole dungeons, and the
// staircase from above must land in a room: so the hero can always walk
// from the start to the bottom.
func TestConnected(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		var above *Level
		for depth := 1; depth <= 40; depth++ {
			l := New(depth, rand.New(rand.NewSource(seed*31+int64(depth))), above, false)
			checkLevel(t, seed, l)
			if above != nil {
				land := above.Landing()
				if l.At(land) != Floor || l.RoomAt(land) == nil || l.RoomAt(above.Ramp()[StairRamp-1]) != l.RoomAt(land) {
					t.Fatalf("seed %d depth %d: staircase lands on %q at %v\n%s", seed, depth, l.At(land), land, l)
				}
			}
			above = l
		}
	}
}

func checkLevel(t *testing.T, seed int64, l *Level) {
	depth := l.Depth
	seen := map[Coord]bool{l.Stairs: true}
	q := []Coord{l.Stairs}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for _, d := range []Coord{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := Coord{c.X + d.X, c.Y + d.Y}
			if tl := l.At(n); (tl.Walkable() || tl == SecretDoor) && !seen[n] {
				seen[n] = true
				q = append(q, n)
			}
		}
	}
	for y := range l.Grid {
		for x, tl := range l.Grid[y] {
			if tl.Walkable() && !seen[Coord{x, y}] {
				t.Fatalf("seed %d depth %d: %d,%d unreachable\n%s", seed, depth, x, y, l)
			}
		}
	}
}

// A linear level is one chain of rooms from Start to the stairs' room:
// every room has one door in and one out, the ends one each.
func TestLinear(t *testing.T) {
	for seed := int64(0); seed < 100; seed++ {
		var above *Level
		for depth := 1; depth <= 40; depth++ {
			l := New(depth, rand.New(rand.NewSource(seed*31+int64(depth))), above, true)
			checkLevel(t, seed, l)
			if above != nil && l.RoomAt(above.Landing()) != &l.Rooms[l.Start] {
				t.Fatalf("seed %d depth %d: staircase lands outside the start room\n%s", seed, depth, l)
			}
			end := l.RoomAt(l.Stairs)
			for i := range l.Rooms {
				r := &l.Rooms[i]
				if r.Gone {
					continue
				}
				want := 2
				if i == l.Start || r == end {
					want = 1
				}
				if len(r.Exits) != want {
					t.Fatalf("seed %d depth %d: room %d has %d doors\n%s", seed, depth, i, len(r.Exits), l)
				}
			}
			above = l
		}
	}
}
