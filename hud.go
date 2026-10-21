package main

// The 2D layer: Rogue's message line and status line, the inventory, the
// map, help, the hurt/blind tint and the tombstone. Each panel is an
// image drawn with the 7x13 bitmap font into an Overlay, rebuilt only
// when its text changes.

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"strings"

	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxelrogue/dungeon"
	"github.com/memmaker/voxelrogue/rogue"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const msgTicks = 80 // a message stays 4 seconds

var helpLines = [][2]string{
	{"W A S D", "walk"},
	{"Mouse", "look"},
	{"Space", "jump"},
	{"Left mouse", "attack with the wielded weapon"},
	{"Right mouse", "throw missiles, zap held wand; hold to draw a bow"},
	{"", "(let go to shoot); a crossbow shoots, held: repeats"},
	{"Walk over", "pick up"},
	{"Walk down", "the stairs to descend"},
	{"< (Shift+,)", "on the stairs: climb, once you have the Amulet"},
	{"F (hold)", "search for secret doors and traps"},
	{"Tab / I", "inventory: letter uses, Shift+letter drops"},
	{"M", "map"},
	{"F1", "this help"},
	{"F9", "debug: take any weapon"},
	{"3", "wield the next weapon in the pack"},
	{"F9", "debug: take any weapon"},
	{"Esc", "close panel / release mouse / quit"},
}

type panel struct {
	ov  *render.Overlay
	key string
}

type hudState struct {
	msgs                                                      []string
	msgAge                                                    int
	status, message, inv, mapP, help, debug, tint, tomb, winP panel
	winLoot                                                   []string // total_winner's worth table
}

func (g *game) msg(s string) {
	if len(g.msgs) > 0 && g.msgAge < msgTicks/2 && len(g.msgs) < 3 {
		g.msgs = append(g.msgs, s)
	} else {
		g.msgs = []string{s}
	}
	g.msgAge = 0
}

func (g *game) tickHUD() {
	if g.msgAge++; g.msgAge > msgTicks {
		g.msgs = nil
	}
}

// show draws a panel, rebuilding its image when key changed.
func (p *panel) show(key string, build func() *image.NRGBA, x, y func(w, h int) int, scale, fbW, fbH int) {
	if p.ov == nil {
		ov, err := render.NewOverlay()
		if err != nil {
			log.Print(err)
			return
		}
		p.ov = ov
		p.key = "\x00"
	}
	if key != p.key {
		p.key = key
		p.ov.SetImage(build())
	}
	w, h := p.ov.Size()
	p.ov.Draw(x(w*scale, h*scale), y(w*scale, h*scale), scale, fbW, fbH)
}

type line struct {
	s string
	c color.NRGBA
}

var (
	white  = color.NRGBA{235, 235, 235, 255}
	yellow = color.NRGBA{255, 210, 90, 255}
	cyan   = color.NRGBA{140, 200, 255, 255}
	red    = color.NRGBA{255, 90, 80, 255}
	grey   = color.NRGBA{140, 140, 140, 255}
)

// textImage draws lines (each may hold several coloured runs separated
// by \x01c, but here one colour per line) on a translucent panel.
func textImage(lines []line, bg color.NRGBA) *image.NRGBA {
	const lineH, pad = 14, 6
	face := basicfont.Face7x13
	w := 1
	for _, l := range lines {
		w = max(w, font.MeasureString(face, l.s).Ceil())
	}
	img := image.NewNRGBA(image.Rect(0, 0, w+2*pad, len(lines)*lineH+2*pad))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	d := &font.Drawer{Dst: img, Face: face}
	for i, l := range lines {
		d.Src = image.NewUniform(l.c)
		d.Dot = fixed.P(pad, pad+i*lineH+11)
		d.DrawString(l.s)
	}
	return img
}

func center(fb int) func(w, h int) int { return func(w, h int) int { return (fb - w) / 2 } }
func middle(fb int) func(w, h int) int { return func(w, h int) int { return max(0, (fb-h)/2) } }
func at(v int) func(w, h int) int      { return func(int, int) int { return v } }

func (g *game) drawHUD(fbW, fbH int) {
	s := max(1, fbH/400)
	panelBG := color.NRGBA{10, 12, 20, 210}

	// tint: hurt flash, blindness, death
	var tint color.NRGBA
	switch {
	case g.dead:
		tint = color.NRGBA{40, 0, 0, 170}
	case g.blind > 0:
		tint = color.NRGBA{0, 0, 0, 235}
	case g.hurt > 0:
		tint = color.NRGBA{200, 0, 0, uint8(min(120, g.hurt*20))}
	}
	if tint.A > 0 {
		g.tint.show(fmt.Sprint(tint), func() *image.NRGBA {
			img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
			draw.Draw(img, img.Bounds(), image.NewUniform(tint), image.Point{}, draw.Src)
			return img
		}, at(0), at(0), max(fbW, fbH)/16+1, fbW, fbH)
	}

	if len(g.msgs) > 0 {
		key := strings.Join(g.msgs, "\n")
		g.message.show(key, func() *image.NRGBA {
			var ls []line
			for _, m := range g.msgs {
				ls = append(ls, line{m, white})
			}
			return textImage(ls, color.NRGBA{0, 0, 0, 140})
		}, center(fbW), at(8*s), s, fbW, fbH)
	}

	st := g.statusLine()
	g.status.show(st, func() *image.NRGBA {
		return textImage([]line{{st, yellow}}, color.NRGBA{0, 0, 0, 160})
	}, center(fbW), func(w, h int) int { return fbH - h - 6*s }, s, fbW, fbH)

	if g.showMap || g.wizard && g.automated {
		lines := g.mapLines()
		g.mapP.show(strings.Join(lines, "\n")+string(g.detect), func() *image.NRGBA {
			var ls []line
			for _, l := range lines {
				ls = append(ls, line{l, white})
			}
			return textImage(ls, color.NRGBA{0, 0, 0, 200})
		}, center(fbW), middle(fbH), max(1, min(fbW/(80*7+20), fbH/(25*14+20))), fbW, fbH)
	}

	if g.showInv {
		ls := g.invLines()
		key := fmt.Sprint(ls)
		g.inv.show(key, func() *image.NRGBA { return textImage(ls, panelBG) }, center(fbW), middle(fbH), s, fbW, fbH)
	}

	if g.showHelp {
		g.help.show("help", func() *image.NRGBA {
			ls := []line{{"VoxelRogue - controls", white}, {"", white}}
			for _, h := range helpLines {
				ls = append(ls, line{fmt.Sprintf("%-14s %s", h[0], h[1]), cyan})
			}
			return textImage(ls, panelBG)
		}, center(fbW), middle(fbH), s, fbW, fbH)
	}

	if g.showDebug {
		g.debug.show("debug", func() *image.NRGBA {
			ls := []line{{"Debug: take a weapon  (letter: take   Esc/F9: close)", white}, {"", white}}
			for i := 0; i < rogue.MaxWeapons; i++ {
				ls = append(ls, line{fmt.Sprintf("%c) %s", 'a'+i, rogue.WeaponNames[i]), cyan})
			}
			return textImage(ls, panelBG)
		}, center(fbW), middle(fbH), s, fbW, fbH)
	}

	if g.dead {
		ls := []line{
			{"REST IN PEACE", white},
			{"", white},
			{fmt.Sprintf("%d Au", g.purse), yellow},
			{"killed by " + g.killer, white},
			{fmt.Sprintf("on level %d", g.depth), white},
			{"", white},
			{"R: new game   Esc: quit", grey},
		}
		g.tomb.show(g.killer, func() *image.NRGBA { return textImage(ls, panelBG) }, center(fbW), middle(fbH), 2*s, fbW, fbH)
	}

	if g.won {
		ls := []line{
			{"Congratulations, you have made it to the light of day!", yellow},
			{"", white},
			{"You have joined the elite ranks of those who have escaped the", white},
			{"Dungeons of Doom alive.  You journey home and sell all your loot at", white},
			{"a great profit and are admitted to the fighters guild.", white},
			{"", white},
			{"   Worth  Item", white},
		}
		for _, s := range g.winLoot {
			ls = append(ls, line{s, cyan})
		}
		ls = append(ls, line{"", white}, line{fmt.Sprintf("Total: %d gold pieces", g.purse), yellow},
			line{"", white}, line{"R: new game   Esc: quit", grey})
		g.winP.show("win", func() *image.NRGBA { return textImage(ls, panelBG) }, center(fbW), middle(fbH), s, fbW, fbH)
	}
}

// statusLine is Rogue's bottom line.
func (g *game) statusLine() string {
	st := &g.stats
	str := func(s rogue.Str) string {
		if s.Str == 18 && s.Add > 0 {
			return fmt.Sprintf("18/%d", s.Add)
		}
		return fmt.Sprint(s.Str)
	}
	hunger := [...]string{"", "  Hungry", "  Weak", "  Fainting"}[min(g.hungry, 3)]
	return fmt.Sprintf("Level: %d  Gold: %-5d  Hp: %d(%d)  Str: %s(%s)  Ac: %d  Exp: %d/%d%s",
		g.depth, g.purse, st.HP, st.MaxHP, str(st.Str), str(g.maxStr), g.armorClass(), st.Lvl, st.Exp, hunger)
}

func (g *game) invLines() []line {
	title := "Inventory  (letter: use   Shift+letter: drop   Esc: close)"
	if g.prompt == 'i' {
		title = "Identify what?"
	}
	ls := []line{{title, white}, {"", white}}
	if len(g.pack) == 0 {
		ls = append(ls, line{"You are empty handed.", grey})
	}
	for i, o := range g.pack {
		s := fmt.Sprintf("%c) %s", 'a'+i, g.rg.Name(o))
		c := cyan
		switch {
		case o == g.weapon:
			s += " (weapon in hand)"
			c = yellow
		case o == g.armor:
			s += " (being worn)"
			c = yellow
		case o == g.rings[0]:
			s += " (on left hand)"
			c = yellow
		case o == g.rings[1]:
			s += " (on right hand)"
			c = yellow
		case o == g.zapping:
			s += " (held)"
			c = yellow
		}
		ls = append(ls, line{s, c})
	}
	return ls
}

// mapLines is the ASCII map of what the hero has seen.
func (g *game) mapLines() []string {
	l := g.level()
	seen := &g.seen[g.slot()-1]
	h := g.heroTile()
	var rows []string
	for y := 0; y < dungeon.Lines; y++ {
		var b strings.Builder
		for x := 0; x < dungeon.Cols; x++ {
			c := dungeon.Coord{X: x, Y: y}
			ch := byte(' ')
			if seen[y][x] || g.wizard {
				t := l.At(c)
				ch = byte(t)
				if t == dungeon.SecretDoor {
					ch = byte(dungeon.VWall)
					if g.gen.isFound(g.slot(), c) {
						ch = byte(dungeon.Door)
					} else if rm := l.RoomAt(dungeon.Coord{X: x, Y: y + 1}); rm != nil && rm.Pos.Y == y+1 {
						ch = byte(dungeon.HWall)
					} else if y > 0 && (l.At(dungeon.Coord{X: x - 1, Y: y}) == dungeon.HWall || l.At(dungeon.Coord{X: x + 1, Y: y}) == dungeon.HWall) {
						ch = byte(dungeon.HWall)
					}
				}
				if tr := l.TrapAt(c); tr != nil && (tr.Found || g.wizard) {
					ch = '^'
				}
			}
			for _, f := range g.floor {
				if f.tile == c && (seen[y][x] || g.wizard || g.detect == 'o' && isMagic(f.o) || g.detect == 'g' && f.o.Type == rogue.Gold) {
					ch = byte(f.o.Type)
				}
			}
			seeInvis := g.seeInvis > 0 || g.wearing(rogue.RSeeInvis)
			if m := g.monsterAt(c); m != nil && (g.wizard || g.detect == 'm' || g.canSee(c) && (m.flags&rogue.IsInvis == 0 || seeInvis)) {
				ch = m.letter
				if m.disguise != nil && g.detect != 'm' && !g.wizard {
					ch = byte(m.disguise.Type)
				}
			} else if m != nil && g.detect == 'o' && m.pack != nil && isMagic(m.pack) {
				ch = '$' // MAGIC: something magic in a monster's pack
			}
			if c == h {
				ch = '@'
			}
			b.WriteByte(ch)
		}
		rows = append(rows, b.String())
	}
	return rows
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
