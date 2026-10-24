# weapon_attributes

One file per melee weapon (its name with underscores): the weapon's Better Combat
attacks (`animation`: an emote of assets/bettercombat, `upswing`: the fraction of
the animation at which the blow lands), listed in the order of its attack patterns:
the sideways swing (its horizontal slashes alternate as a combo), the overhead strike,
the stab. presets.go binds them to the left button, the right button and F; a weapon
without a file (the launchers, the missiles) has the vanilla swing.

| weapon | hands | patterns |
|---|---|---|
| dagger | 1 | swing, stab |
| axe | 1 | swing, overhead |
| mace | 1 | swing, overhead |
| spear | 1 | stab |
| long sword | 1 | swing, overhead, stab |
| halberd | 2 | swing, overhead, stab |
| maul | 2 | swing, overhead |
| two handed sword | 2 | swing, overhead, stab |
| battle axe | 2 | swing, overhead |
