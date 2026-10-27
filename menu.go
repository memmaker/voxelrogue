package main

// The menus, VoxelLib's ui screens on the inventory's screen stack: the
// main menu at the start, over the dungeon, the game menu Esc opens, and
// the options, kept in options.json beside the save (configDir).

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/ui"
)

// options are what the options menu sets.
type options struct {
	PixelMonsters bool // the sheet's 16px monster tiles instead of their remastered pictures
	GUIScale      int  // ui.Context.GUIScale: 0 auto, 1 small, 2 normal, 3 large
	Linear        bool // levels as one chain of rooms, from the next new game on
}

type menuState struct {
	opts    options
	menu    bool   // a menu is open: the world waits (paused)
	tileDir string // -tiles: the sheets and their rec mappings
	monArt  string // -monsters: the remastered pictures
}

func optionsPath() string { return filepath.Join(configDir(), "VoxelRogue", "options.json") }

func loadOptions() (o options) {
	b, err := os.ReadFile(optionsPath())
	if err == nil {
		err = json.Unmarshal(b, &o)
	}
	if err != nil && !os.IsNotExist(err) {
		log.Print(err)
	}
	return o
}

func (g *game) saveOptions() {
	b, _ := json.Marshal(g.opts)
	err := os.MkdirAll(filepath.Dir(optionsPath()), 0o755)
	if err == nil {
		err = os.WriteFile(optionsPath(), b, 0o644)
	}
	if err != nil {
		log.Print(err)
	}
}

// openMenu frees the mouse and opens the main menu (main: New Game, Load
// Game, Options, Quit) or the game menu (Back to Game, Save Game, Load
// Game, New Game, Options, Quit); the last screen closing resumes the
// game (setupGUI's OnEmpty).
func (g *game) openMenu(main bool) {
	ctx := g.pg.Ctx
	g.menu = true
	g.capture(false)
	resume := ctx.Screens.CloseAll
	_, noSave := os.Stat(g.saveFile)
	load := ui.MenuItem{Label: "Load Game", OnClick: func() { g.load(); resume() }, Disabled: noSave != nil}
	options := func() { ctx.Screens.Open(g.optionsScreen()) }
	quit := func() { g.win.SetShouldClose(true) }
	if main {
		m := ui.NewMainMenu(ctx, []ui.MenuItem{{Label: "New Game", OnClick: func() {
			if g.gen.level(1).Linear != g.opts.Linear {
				g.newGame(0) // the dungeon rolled at the start is the other kind
			}
			resume()
		}}, load}, options, quit, nil)
		m.Title, m.TitleY = "VoxelRogue", 40
		ctx.Screens.Open(mainMenu{m})
		return
	}
	ctx.Screens.Open(&ui.MenuScreen{Title: "menu.game", TitleY: 40, Origin: func(h int) int { return h/4 - 16 }, Rows: []ui.MenuRow{
		{Y: 24, Items: []ui.MenuItem{{Label: "menu.returnToGame", OnClick: resume}}},
		{Y: 48, Items: []ui.MenuItem{{Label: "Save Game", OnClick: func() { g.save(); resume() }}, load}},
		{Y: 72, Items: []ui.MenuItem{{Label: "New Game", OnClick: func() { g.newGame(0); resume() }}}},
		{Y: 96, Items: []ui.MenuItem{{Label: "menu.options", OnClick: options}}},
		{Y: 120, Items: []ui.MenuItem{{Label: "menu.quit", OnClick: quit}}},
	}})
}

// mainMenu is the main menu, which Esc does not leave.
type mainMenu struct{ *ui.MenuScreen }

func (m mainMenu) Input(ctx *ui.Context, ev ui.Event) bool {
	return ev.Type == ui.KeyPress && ev.Key == ui.KeyEscape || m.MenuScreen.Input(ctx, ev)
}

// optionsScreen is the options menu: the monsters' pictures and the GUI
// scale; Done keeps them.
func (g *game) optionsScreen() ui.Screen {
	return &ui.SettingsScreen{Title: "options.title", Settings: []ui.Setting{
		{Label: "Monsters", Text: func() string {
			if g.opts.PixelMonsters {
				return "Pixel"
			}
			return "Remastered"
		}, OnClick: func() {
			g.opts.PixelMonsters = !g.opts.PixelMonsters
			g.setMonsterArt()
		}},
		{Label: "Linear mode", Text: func() string {
			if g.opts.Linear {
				return "On"
			}
			return "Off"
		}, OnClick: func() { g.opts.Linear = !g.opts.Linear }},
		ui.GUIScaleSetting(g.pg.Ctx, func(s int) { g.opts.GUIScale = s }),
	}, OnDone: g.saveOptions}
}

// monsterDir is where the monsters' pictures come from: the remasters, or
// nothing for the sheet's tiles.
func (g *game) monsterDir() string {
	if g.opts.PixelMonsters {
		return ""
	}
	return g.monArt
}

// setMonsterArt reloads the sprites for the Monsters option (loadArt),
// dropping the old atlas.
func (g *game) setMonsterArt() {
	old := g.itemRend
	if err := g.loadArt(); err != nil {
		log.Print(err)
		return
	}
	old.Delete()
}
