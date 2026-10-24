# Golden data generators

Both files run the reference code itself, headless, and print JSON.

- `Harness.java` → `golden.json.gz`: loads `assets/bettercombat/*.json`
  through ZigyTheBird's PlayerAnimationLib 1.2.5 core
  (`com.zigythebird.playeranimcore`, the `player-animation-lib-core` jar and
  its dependencies: gson, fastutil, joml, mochafloats) and drives a
  `HumanoidAnimationController` with Better Combat 3.2.2's modifiers exactly
  as `AbstractClientPlayerEntityMixin.playAttackAnimation` does.

      javac -cp "$CP" -d out Harness.java
      java -cp "$CP:out" Harness ../../assets/bettercombat | gzip > ../golden.json.gz

- `Vanilla.java` → `vanilla.json`: Minecraft 26.2's own `HumanoidModel`,
  `WalkAnimationState`, `Mth` and `PoseStack`/`ItemTransform`, from the
  client jar plus the launcher's `libraries` directory on the classpath.

      javac -cp "$VCP" -d vout Vanilla.java
      java -cp "$VCP:vout" Vanilla > ../vanilla.json
