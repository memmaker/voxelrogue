package rogue

import "testing"

// TestHitScale pins the swing roll's average as a damage scale: level 1
// against armour 7 with +1 was 9 in 20, a +100 bolt always lands, and a
// blow the roll could never win does nothing.
func TestHitScale(t *testing.T) {
	for _, c := range []struct{ d, lvl, arm, plus, want int }{{10, 1, 7, 1, 5}, {6, 1, 7, 100, 6}, {6, 0, -5, 0, 0}, {3, 1, 7, 1, 1}} {
		if got := HitScale(c.d, c.lvl, c.arm, c.plus); got != c.want {
			t.Errorf("HitScale(%d, %d, %d, %d) = %d, want %d", c.d, c.lvl, c.arm, c.plus, got, c.want)
		}
	}
}
