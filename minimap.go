package main

// The minimap in the top left: the seen walls, corridors and doors of the
// level drawn as lines (not Rogue's letters, which the big map keeps), a
// dim fill for seen floor, a mark for the stairs, a bright cell where a
// seen corridor or floor leads on into the unseen (more to explore) and
// an arrow where the hero stands, pointing where the camera looks.

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/memmaker/voxelrogue/dungeon"
)

const mmCell = 4 // minimap pixels per map tile (before the panel's scale)

// seg draws a one pixel line (Bresenham).
func seg(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	for e := dx + dy; ; {
		img.SetNRGBA(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		if e2 := 2 * e; e2 >= dy {
			e, x0 = e+dy, x0+sx
		} else if e2 <= dx {
			e, y0 = e+dx, y0+sy
		}
	}
}

// minimapKey changes whenever the minimap has to be redrawn: a step, a
// turn, something newly seen.
func (g *game) minimapKey() string {
	n := 0
	for _, row := range g.seen[g.slot()-1] {
		for _, s := range row {
			if s {
				n++
			}
		}
	}
	return fmt.Sprintf("%d %d %.2f %.2f %.2f", g.depth, n, g.player.X, g.player.Z, g.cam.Yaw)
}

// minimap draws the level as the hero knows it.
func (g *game) minimap() *image.NRGBA {
	l, seen := g.level(), &g.seen[g.slot()-1]
	img := image.NewNRGBA(image.Rect(0, 0, dungeon.Cols*mmCell, dungeon.Lines*mmCell))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.NRGBA{0, 0, 0, 150}), image.Point{}, draw.Src)
	known := func(x, y int) bool {
		return x >= 0 && y >= 0 && x < dungeon.Cols && y < dungeon.Lines && (seen[y][x] || g.wizard)
	}
	// kind of a known tile: 'w' wall, '#' passage, '+' door, 0 nothing drawn
	kind := func(x, y int) byte {
		if !known(x, y) {
			return 0
		}
		switch t := l.At(dungeon.Coord{X: x, Y: y}); t {
		case dungeon.HWall, dungeon.VWall:
			return 'w'
		case dungeon.SecretDoor:
			if g.gen.isFound(g.slot(), dungeon.Coord{X: x, Y: y}) {
				return '+'
			}
			return 'w'
		case dungeon.Passage, dungeon.Door:
			return byte(t)
		}
		return 0
	}
	mid := func(x, y int) (int, int) { return x*mmCell + mmCell/2, y*mmCell + mmCell/2 }
	wall, pass, door, floor := color.NRGBA{220, 220, 220, 255}, color.NRGBA{150, 150, 150, 255}, yellow, color.NRGBA{255, 255, 255, 30}
	for y := 0; y < dungeon.Lines; y++ {
		for x := 0; x < dungeon.Cols; x++ {
			k := kind(x, y)
			if k == 0 && known(x, y) && l.At(dungeon.Coord{X: x, Y: y}).Walkable() {
				draw.Draw(img, image.Rect(x*mmCell, y*mmCell, (x+1)*mmCell, (y+1)*mmCell), image.NewUniform(floor), image.Point{}, draw.Over)
			}
			if k == 0 {
				continue
			}
			cx, cy := mid(x, y)
			if k == '+' {
				img.SetNRGBA(cx, cy, door)
			}
			for _, n := range [2][2]int{{x + 1, y}, {x, y + 1}} { // each edge once: to the right and down
				nk := kind(n[0], n[1])
				nx, ny := mid(n[0], n[1])
				switch {
				case k == 'w' && nk == 'w':
					seg(img, cx, cy, nx, ny, wall)
				case k != 'w' && nk != 0 && nk != 'w': // corridor to corridor or door
					seg(img, cx, cy, nx, ny, pass)
				}
			}
		}
	}
	// more to explore: seen ground next to unseen ground
	more := color.NRGBA{255, 60, 255, 255}
	for y := 0; y < dungeon.Lines; y++ {
		for x := 0; x < dungeon.Cols; x++ {
			if !known(x, y) || !l.At(dungeon.Coord{X: x, Y: y}).Walkable() {
				continue
			}
			for _, n := range [4][2]int{{x + 1, y}, {x - 1, y}, {x, y + 1}, {x, y - 1}} {
				if n[0] >= 0 && n[1] >= 0 && n[0] < dungeon.Cols && n[1] < dungeon.Lines && !known(n[0], n[1]) &&
					l.At(dungeon.Coord{X: n[0], Y: n[1]}).Walkable() {
					draw.Draw(img, image.Rect(x*mmCell, y*mmCell, (x+1)*mmCell, (y+1)*mmCell), image.NewUniform(more), image.Point{}, draw.Over)
					break
				}
			}
		}
	}
	if s := l.Stairs; known(s.X, s.Y) {
		seg(img, s.X*mmCell, s.Y*mmCell, s.X*mmCell+mmCell-1, s.Y*mmCell+mmCell-1, cyan)
		seg(img, s.X*mmCell+mmCell-1, s.Y*mmCell, s.X*mmCell, s.Y*mmCell+mmCell-1, cyan)
	}
	// the hero: an arrow at the exact position, pointing along the camera's yaw
	px, py := g.player.X/tileW*mmCell, g.player.Z/tileD*mmCell
	dx, dy := math.Sin(g.cam.Yaw), -math.Cos(g.cam.Yaw)
	const r = 4
	pt := func(f, s float64) (int, int) {
		return int(math.Round(px + dx*f*r - dy*s*r)), int(math.Round(py + dy*f*r + dx*s*r))
	}
	tx, ty := pt(1, 0)
	lx, ly := pt(-0.8, 0.7)
	rx, ry := pt(-0.8, -0.7)
	seg(img, tx, ty, lx, ly, red)
	seg(img, tx, ty, rx, ry, red)
	seg(img, lx, ly, rx, ry, red)
	return img
}
