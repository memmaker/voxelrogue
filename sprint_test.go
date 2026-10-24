package main

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/character"
)

// flat is a floor at y<0 and nothing else.
type flat struct{}

func (flat) Solid(x, y, z int) bool              { return y < 0 }
func (flat) Liquid(x, y, z int) character.Liquid { return character.LiquidNone }

// TestSprintDoubles checks a sprinting hero covers at least twice the
// ground of a walking one in the same ticks.
func TestSprintDoubles(t *testing.T) {
	run := func(sprint bool) float64 {
		p := character.New(0, 0, 0)
		p.Tick(flat{}, character.Input{}) // land first: the measure is ground speed
		for i := 0; i < 100; i++ {
			p.MoveSpeed = 0
			if sprint {
				p.MoveSpeed = sprintSpeed
			}
			p.Tick(flat{}, character.Input{Forward: 1, Sprint: sprint})
		}
		return math.Hypot(p.X, p.Z)
	}
	walk, run2 := run(false), run(true)
	if run2 < 2*walk {
		t.Fatalf("walk %.2f, sprint %.2f: less than double", walk, run2)
	}
	t.Logf("100 ticks: walk %.2f blocks, sprint %.2f", walk, run2)
}
