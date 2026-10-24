package main

// The 2D layer: Rogue's message line and status line, the inventory, the
// map, help, the hurt/blind tint and the tombstone. Each panel is an
// image drawn with the 7x13 bitmap font into an Overlay, rebuilt only
// when its text changes.

import (
	"archive/zip"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/memmaker/voxellib/i18n"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/ui/guicanvas"
	"github.com/memmaker/voxellib/ui/guientity"
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
	{"Left mouse", "attack: the weapon's first pattern (sideways);"},
	{"", "fire a launcher: hold to draw a bow (let go to shoot),"},
	{"", "whirl the sling, shoot a spanned crossbow"},
	{"Right mouse", "the weapon's second pattern (overhead); zap held wand;"},
	{"", "use the held food, potion, scroll, ring or armor;"},
	{"", "cancel a bow's draw or a sling's whirl"},
	{"1-9, wheel", "hold a hotbar slot: a weapon is wielded, a wand held"},
	{"F", "the weapon's third attack pattern"},
	{"T", "throw the wielded weapon"},
	{"R", "span a crossbow (slow going)"},
	{"Shift (tap)", "sprint until you stop walking forward"},
	{"Ctrl (hold)", "sneak: slow, but silent to sleeping monsters"},
	{"Walk over", "pick up"},
	{"Walk down", "the stairs to descend"},
	{"< (Shift+,)", "on the stairs: climb, once you have the Amulet"},
	{"C (hold)", "search for secret doors and traps"},
	{"E / Tab / I", "inventory: drag items; the left column wears the"},
	{"", "Amulet, armor and rings; drop outside the window"},
	{"M", "map: on a panel, laid over the view, off"},
	{"F1", "this help"},
	{"F2", "the manual: the rules of the dungeon"},
	{"F3", "debug: show the monsters' hit boxes"},
	{"F9", "debug: take any weapon"},
	{"Esc", "close panel / release mouse / quit"},
}

// manualLines is the F2 manual: Rogue's rules in short.
var manualLines = []string{
	"Cursed equipment",
	"  Some armor, rings and weapons are cursed. Once worn or wielded a cursed",
	"  thing won't come off (\"You can't. It appears to be cursed.\"), and it is",
	"  usually a bad one: armor and weapons at -1 or worse, rings of aggravate",
	"  monster or teleportation. A scroll of remove curse lifts the curse from",
	"  everything you wear and wield (\"You feel as if somebody is watching over",
	"  you.\"); then it can be taken off. Reading an unknown scroll may turn out",
	"  to be it. A scroll of enchant armor or enchant weapon also uncurses the",
	"  piece it enchants. Until you know an item, its enchantment is hidden;",
	"  a scroll of identify tells.",
	"",
	"Armor",
	"  Armor class: the lower, the harder to hit, from plate mail (3) down to",
	"  leather (8); +1 is one class better. Only one piece is worn at a time,",
	"  plus two rings.",
	"",
	"Food",
	"  Every ration lasts a while; a ring eats more. Hungry, then weak, then",
	"  you faint on the spot. Eat before Weak.",
	"",
	"Potions and scrolls",
	"  Unknown until tried (or identified); the same colour or title is always",
	"  the same kind. Scrolls are read, potions quaffed, by holding them and",
	"  using the right mouse button.",
	"",
	"Wands and staves",
	"  Zap with the right mouse button; each has a few charges.",
	"",
	"Monsters",
	"  Capital letters in the map: A ant to Z zombie. Many are asleep until",
	"  you come near; some carry gold. Killing them earns experience.",
	"",
	"Sleep and noise",
	"  A sleeping monster floats Zs. A blow on it counts double, a dagger's",
	"  five times. Noise wakes the sleepers near it: a fight, a shot, a",
	"  throw, a wand, spanning a crossbow, and your steps (a sprint carries",
	"  far). Sneak (hold Ctrl) to pass them silently, slowly.",
	"",
	"Searching and traps",
	"  Hold C to find secret doors and traps in the walls and floor around",
	"  you. Known traps show on the map; a dart or arrow trap, a trap door",
	"  (to the next level), a bear trap, sleeping gas and teleport.",
	"",
	"The Amulet",
	"  The Amulet of Yendor lies on level 26 or deeper. With it, climb the",
	"  stairs (< on them) back up to the light of day to win.",
}

type panel struct {
	ov  *render.Overlay
	key string
}

type hudState struct {
	msgs                                                               []string
	msgAge                                                             int
	status, message, mapP, mini, help, manual, debug, tint, tomb, winP panel
	winLoot                                                            []string // total_winner's worth table
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
	if g.pg != nil {
		g.pg.Update()
	}
}

// armorSets are the VoxelHobbit armour textures (hobbit/armor/<set>_1.png
// the chest and boots, _2.png the leggings) the inventory's hero wears
// for each armour kind: the sets of gear.rec's armour icons.
var armorSets = [rogue.MaxArmors]string{"dunlending", "ranger", "corsair", "near_harad", "chainmail", "dale", "rohan", "iron"}

// loadNRGBA reads a png.
func loadNRGBA(path string) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	n := image.NewNRGBA(img.Bounds())
	draw.Draw(n, n.Rect, img, img.Bounds().Min, draw.Src)
	return n, nil
}

// armorLayers is what the inventory's hero wears over the skin: the worn
// armour's set as chest, leggings and boots (no helmet: the face stays
// the hero's).
func (g *game) armorLayers() []guientity.Armor {
	if g.armor == nil {
		return nil
	}
	l1, l2 := g.layers[armorSets[g.armor.Which]+"_1"], g.layers[armorSets[g.armor.Which]+"_2"]
	return []guientity.Armor{{Tex: l1, Piece: guientity.Chest}, {Tex: l2, Piece: guientity.Leggings}, {Tex: l1, Piece: guientity.Boots}}
}

// setupGUI loads the GUI textures, font and language from the client jar
// and makes the inventory's PlayerGUI: no crafting, no hearts and shanks
// (the status line has them), the player in hobbit/skin.png wearing the
// hobbit/armor layers.
func (g *game) setupGUI(jar, hobbit string) error {
	zr, err := zip.OpenReader(jar)
	if err != nil {
		return fmt.Errorf("GUI assets (-jar): %w", err)
	}
	cat := i18n.New()
	if err := cat.LoadFS(zr); err != nil {
		return err
	}
	cat.SetLanguage("en_US")
	ctx, err := ui.NewContext(guicanvas.New(), zr, cat)
	if err != nil {
		return err
	}
	ctx.InWorld = true
	g.pg = ui.NewPlayerGUI(ctx, g.inv, g.cont, g.itemStack)
	g.pg.Keys = []int{ui.KeyE, ui.KeyTab, int(glfw.KeyI)}
	g.pg.NoCrafting, g.pg.HUD.State.Creative = true, true
	hero := &guientity.Player{}
	if hero.Skin, err = loadNRGBA(filepath.Join(hobbit, "skin.png")); err != nil {
		return err
	}
	g.layers = map[string]*image.NRGBA{}
	for _, set := range armorSets {
		for _, l := range []string{set + "_1", set + "_2"} {
			if g.layers[l], err = loadNRGBA(filepath.Join(hobbit, "armor", l+".png")); err != nil {
				return err
			}
		}
	}
	g.pg.Model = func(ctx *ui.Context, x, y, mx, my int) {
		hero.Armor = g.armorLayers()
		hero.Draw(ctx, x, y, mx, my)
	}
	g.pg.EquipAt = rogueEquipAt
	ctx.SetTexture("rogue:ring_slot", slotIcon(g.sprite["agate ring"]))
	g.pg.OnOpen = func() { g.showDebug = false; g.capture(false) }
	g.pg.OnClose = func() {
		if g.prompt == 'i' {
			g.prompt = 0
		}
		g.capture(true)
	}
	g.icons = map[string]*ui.Sprite{}
	return nil
}

func (g *game) openInv() {
	if g.pg != nil {
		g.pg.Open()
	}
}

func (g *game) closeInv() {
	if g.pg != nil {
		g.pg.Close()
	}
}

// hotbarH is the hotbar's height in framebuffer pixels (22 GUI px).
func (g *game) hotbarH(fbW int) int {
	if g.pg == nil {
		return 0
	}
	ww, _ := g.win.GetSize()
	return 22 * g.pg.Ctx.Scale * fbW / max(1, ww)
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
	green  = color.NRGBA{120, 255, 120, 255}
	orange = color.NRGBA{255, 170, 40, 255}
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
	}, center(fbW), func(w, h int) int { return fbH - h - 6*s - g.hotbarH(fbW) }, s, fbW, fbH) // above the hotbar

	if !g.dead && !g.won {
		g.mini.show(g.minimapKey(), g.minimap, at(8*s), at(8*s), max(1, s/2), fbW, fbH)
	}

	if g.showMap != 0 || g.wizard && g.automated {
		lines := g.mapLines()
		bg := color.NRGBA{0, 0, 0, 200}
		if g.showMap == 2 { // laid over the view
			bg = color.NRGBA{}
		}
		g.mapP.show(fmt.Sprint(g.showMap, g.detect, strings.Join(lines, "\n")), func() *image.NRGBA {
			var ls []line
			for _, l := range lines {
				ls = append(ls, line{l, white})
			}
			return textImage(ls, bg)
		}, center(fbW), middle(fbH), max(1, min(fbW/(80*7+20), fbH/(25*14+20))), fbW, fbH)
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

	if g.showManual {
		g.manual.show("manual", func() *image.NRGBA {
			ls := []line{{"VoxelRogue - manual", white}, {"", white}}
			for _, l := range manualLines {
				c := cyan
				if l != "" && l[0] != ' ' {
					c = white
				}
				ls = append(ls, line{l, c})
			}
			return textImage(ls, panelBG)
		}, center(fbW), at(36*s), max(1, min(s, (fbH-g.hotbarH(fbW)-56*s)/((len(manualLines)+3)*14))), fbW, fbH) // between the message and status lines
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
