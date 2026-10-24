package main

import (
	"log"
	"os"
	"testing"

	"github.com/memmaker/voxelrogue/rogue"
)

// TestMain loads the weapon and armour kinds (weapons.rec, gear.rec) the
// game and its tests count on.
func TestMain(m *testing.M) {
	if err := rogue.LoadKinds("assets/tiles"); err != nil {
		log.Fatal(err)
	}
	os.Exit(m.Run())
}
