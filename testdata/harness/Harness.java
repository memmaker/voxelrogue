// Golden-data generator: drives the real PlayerAnimationLib core (the jar
// Better Combat 3.2.2 ships with) exactly as Better Combat's
// AttackAnimationStack does, and prints every bone value the game's Go
// port must reproduce. Usage: Harness <animationDir> > golden.json
import com.google.gson.*;
import com.zigythebird.playeranimcore.animation.*;
import com.zigythebird.playeranimcore.animation.keyframe.*;
import com.zigythebird.playeranimcore.animation.layered.modifier.*;
import com.zigythebird.playeranimcore.bones.PlayerAnimBone;
import com.zigythebird.playeranimcore.enums.PlayState;
import com.zigythebird.playeranimcore.loading.PlayerAnimatorLoader;
import com.zigythebird.playeranimcore.math.Vec3f;
import com.zigythebird.playeranimcore.molang.MolangLoader;
import java.io.*;
import java.util.*;

public class Harness {
    // --- Better Combat's TransmissionSpeedModifier, verbatim ---
    static class TransmissionSpeedModifier extends SpeedModifier {
        private float elapsed = 0;
        public List<Gear> gears = List.of();
        public TransmissionSpeedModifier(float speed) { super(speed); }
        public record Gear(float time, float speed) {}
        public void set(float speed, List<Gear> gears) { this.speed = speed; this.gears = gears; this.elapsed = 0; }
        private float elapsed(float delta) { return elapsed + delta; }
        @Override public void tick(AnimationData state) { super.tick(state); this.elapsed += 1; }
        @Override public void setupAnim(AnimationData state) {
            float tickDelta = state.getPartialTick();
            var time = elapsed(tickDelta);
            for (var gear : gears) { if (time > gear.time) { speed = gear.speed(); } }
            super.setupAnim(state);
        }
    }

    static final String[] BONES = {"body", "torso", "head", "right_arm", "left_arm", "right_leg", "left_leg", "right_item", "left_item"};
    static float pitchDeg; // the player's xRot

    static HumanoidAnimationController ctrl;
    static TransmissionSpeedModifier speed;
    static MirrorModifier mirror;

    static void build() {
        ctrl = new HumanoidAnimationController((c, s, setter) -> PlayState.STOP, MolangLoader::createNewEngine);
        speed = new TransmissionSpeedModifier(1F);
        mirror = new MirrorModifier();
        ctrl.addModifier(mirror, 0);
        ctrl.addModifier(speed, 0);
        // AttackAnimationStack.createAttackAdjustment, first and third person
        ctrl.addModifierLast(new AdjustmentModifier((partName, data) -> {
            float rotationX = 0, rotationY = 0, rotationZ = 0, offsetX = 0, offsetY = 0, offsetZ = 0;
            var pitch = pitchDeg;
            if (data.isFirstPersonPass()) {
                pitch = (float) Math.toRadians(pitch);
                if (partName.equals("body")) {
                    rotationX += pitch;
                    if (pitch < 0) {
                        var offset = Math.abs(Math.sin(pitch));
                        offsetY += offset * 0.5;
                        offsetZ -= offset;
                    }
                } else return Optional.empty();
            } else {
                pitch = (float) Math.toRadians(pitch);
                if (partName.equals("body")) rotationX += pitch * 0.75F;
                else if (partName.equals("right_arm") || partName.equals("left_arm")) rotationX += pitch * 0.25F;
                else if (partName.equals("right_leg") || partName.equals("left_leg")) rotationX -= pitch * 0.75;
                else return Optional.empty();
            }
            return Optional.of(new AdjustmentModifier.PartModifier(new Vec3f(rotationX, rotationY, rotationZ), new Vec3f(offsetX, offsetY, offsetZ)));
        }));
        ctrl.setPostAnimationSetupConsumer((func) -> {
            func.apply("torso").setEnabled(true);
            func.apply("head").rotXEnabled = false;
        });
    }

    // AbstractClientPlayerEntityMixin.playAttackAnimation (upswing multiplier 0.5, right handed main hand)
    static void playAttackAnimation(Animation animation, boolean offHand, float length, float upswing) {
        var endTick = animation.data().<Float>get(ExtraAnimationData.END_TICK_KEY).orElse(animation.length());
        var spd = endTick / length;
        var mir = offHand;
        var trueUpswingRatio = upswing / 0.5F;
        float upswingSpeed = spd / trueUpswingRatio;
        float downwindSpeed = (float) (spd * lerp(Math.max(0.5 - 0.5, 0) / 0.5, (1F - upswing), upswing / (1F - upswing)));
        speed.speed = spd;
        mirror.enabled = mir;
        speed.set(upswingSpeed, List.of(new TransmissionSpeedModifier.Gear(length * upswing, downwindSpeed), new TransmissionSpeedModifier.Gear(length, spd)));
        ctrl.triggerAnimation(animation);
    }
    static double lerp(double d, double a, double b) { return a + d * (b - a); }

    // AvatarAnimManager.handleAnimations for a stack with this one layer
    static void handleAnimations(float partialTick, boolean fullTick, boolean firstPerson, float velocity) {
        AnimationData data = new AnimationData(velocity, partialTick, firstPerson);
        if (fullTick) ctrl.tick(data.copy());
        if (ctrl.isActive()) ctrl.setupAnim(data.copy());
    }

    static JsonArray v3(org.joml.Vector3f v) { JsonArray a = new JsonArray(); a.add(v.x); a.add(v.y); a.add(v.z); return a; }
    static JsonObject boneJson(PlayerAnimBone b) {
        JsonObject o = new JsonObject(); o.add("pos", v3(b.position)); o.add("rot", v3(b.rotation)); o.add("scale", v3(b.scale)); return o;
    }

    // The vanilla model values fed in per bone (RenderUtil.copyVanillaPart semantics), deterministic per scenario.
    static Map<String, float[]> vanilla(Random r) {
        Map<String, float[]> m = new LinkedHashMap<>();
        for (String n : BONES) {
            float[] v = new float[9];
            if (n.equals("body") || n.equals("right_item") || n.equals("left_item")) { v[6] = v[7] = v[8] = 1; }
            else { for (int i = 0; i < 6; i++) v[i] = (r.nextFloat() - 0.5f) * (i < 3 ? 4 : 2); for (int i = 6; i < 9; i++) v[i] = 1; }
            m.put(n, v);
        }
        return m;
    }

    static JsonObject sample(int tick, float pt, Map<String, float[]> van) {
        JsonObject s = new JsonObject();
        s.addProperty("tick", tick); s.addProperty("pt", pt);
        s.addProperty("active", ctrl.isActive());
        s.addProperty("animTicks", ctrl.getAnimationTicks());
        s.addProperty("speed", speed.speed);
        JsonObject bones = new JsonObject();
        for (String n : BONES) {
            PlayerAnimBone b = new PlayerAnimBone(n);
            float[] v = van.get(n);
            b.position.set(v[0], v[1], v[2]); b.rotation.set(v[3], v[4], v[5]); b.scale.set(v[6], v[7], v[8]);
            if (ctrl.isActive()) ctrl.get3DTransform(b);
            bones.add(n, boneJson(b));
        }
        s.add("bones", bones);
        return s;
    }

    public static void main(String[] args) throws Exception {
        File dir = new File(args[0]);
        Map<String, Animation> anims = new TreeMap<>();
        for (File f : Objects.requireNonNull(dir.listFiles((d, n) -> n.endsWith(".json")))) {
            try (Reader r = new FileReader(f)) {
                anims.put(f.getName().replace(".json", ""), PlayerAnimatorLoader.GSON.fromJson(r, Animation.class));
            }
        }
        JsonObject out = new JsonObject();
        // 1. the loaded keyframes (what the loader port must produce)
        JsonObject loaded = new JsonObject();
        for (var e : anims.entrySet()) {
            Animation a = e.getValue();
            JsonObject ao = new JsonObject();
            ao.addProperty("length", a.length());
            ao.addProperty("beginTick", a.data().<Float>get("beginTick").orElse(null));
            ao.addProperty("endTick", a.data().<Float>get("endTick").orElse(null));
            ao.addProperty("loop", a.loopType().shouldPlayAgain(null, a));
            JsonObject bones = new JsonObject();
            for (var be : a.boneAnimations().entrySet()) {
                JsonObject bo = new JsonObject();
                BoneAnimation ba = be.getValue();
                String[] names = {"px", "py", "pz", "rx", "ry", "rz", "sx", "sy", "sz"};
                List<List<Keyframe>> lists = List.of(ba.positionKeyFrames().xKeyframes(), ba.positionKeyFrames().yKeyframes(), ba.positionKeyFrames().zKeyframes(),
                        ba.rotationKeyFrames().xKeyframes(), ba.rotationKeyFrames().yKeyframes(), ba.rotationKeyFrames().zKeyframes(),
                        ba.scaleKeyFrames().xKeyframes(), ba.scaleKeyFrames().yKeyframes(), ba.scaleKeyFrames().zKeyframes());
                for (int i = 0; i < 9; i++) {
                    JsonArray ka = new JsonArray();
                    for (Keyframe k : lists.get(i)) {
                        JsonObject ko = new JsonObject();
                        ko.addProperty("len", k.length());
                        ko.addProperty("start", MolangLoader.MOCHA_ENGINE.eval(k.startValue()));
                        ko.addProperty("end", MolangLoader.MOCHA_ENGINE.eval(k.endValue()));
                        ko.addProperty("ease", k.easingType().name);
                        ka.add(ko);
                    }
                    bo.add(names[i], ka);
                }
                bo.addProperty("bend", ba.bendKeyFrames().size());
                bones.add(be.getKey(), bo);
            }
            ao.add("bones", bones);
            loaded.add(e.getKey(), ao);
        }
        out.add("animations", loaded);

        // 2. scenarios: a schedule of attacks and render partial ticks
        JsonArray scenarios = new JsonArray();
        Random rng = new Random(12345);
        List<String> names = new ArrayList<>(anims.keySet());
        List<String> attackNames = names.stream().filter(n -> !n.startsWith("pose_")).toList();
        int sc = 0;
        for (String name : attackNames) {
            for (int variant = 0; variant < 3; variant++) {
                build();
                Random r = new Random(1000L * sc + variant);
                boolean firstPerson = variant != 2;
                float length = switch (variant) { case 0 -> 10; case 1 -> 18; default -> 2 + r.nextInt(20) + r.nextFloat(); };
                float upswing = variant == 1 ? 0.35f : 0.5f;
                pitchDeg = variant == 0 ? -30 : (variant == 1 ? 25 : (r.nextFloat() - 0.5f) * 170);
                boolean offHand = variant == 2 && r.nextBoolean();
                Map<String, float[]> van = vanilla(r);
                int ticks = 40 + r.nextInt(20);
                // attacks: at tick 1, and a second one part way through (re-trigger), sometimes a third after it ends
                List<int[]> attackAt = new ArrayList<>();
                attackAt.add(new int[]{1, r.nextInt(attackNames.size())});
                if (variant != 0) attackAt.add(new int[]{2 + r.nextInt(8), r.nextInt(attackNames.size())});
                attackAt.add(new int[]{ticks - 8, r.nextInt(attackNames.size())});
                JsonObject s = new JsonObject();
                s.addProperty("name", name + "/" + variant);
                s.addProperty("firstPerson", firstPerson);
                s.addProperty("length", length);
                s.addProperty("upswing", upswing);
                s.addProperty("pitchDeg", pitchDeg);
                s.addProperty("offHand", offHand);
                JsonObject vj = new JsonObject();
                for (var e : van.entrySet()) { JsonArray a = new JsonArray(); for (float f : e.getValue()) a.add(f); vj.add(e.getKey(), a); }
                s.add("vanilla", vj);
                JsonArray attacks = new JsonArray();
                JsonArray samples = new JsonArray();
                float velocity = r.nextFloat() * 0.2f;
                for (int tick = 0; tick < ticks; tick++) {
                    for (int[] at : attackAt) {
                        if (at[0] == tick) {
                            String an = tick == 1 ? name : attackNames.get(at[1]);
                            JsonObject aj = new JsonObject(); aj.addProperty("tick", tick); aj.addProperty("anim", an); attacks.add(aj);
                            playAttackAnimation(anims.get(an), offHand, length, upswing);
                        }
                    }
                    handleAnimations(0, true, firstPerson, velocity);
                    samples.add(sample(tick, 0, van));
                    int frames = 1 + r.nextInt(3);
                    float[] pts = new float[frames];
                    for (int i = 0; i < frames; i++) pts[i] = r.nextFloat();
                    Arrays.sort(pts);
                    for (float pt : pts) {
                        handleAnimations(pt, false, firstPerson, velocity);
                        samples.add(sample(tick, pt, van));
                    }
                }
                s.add("attacks", attacks);
                s.add("samples", samples);
                scenarios.add(s);
                sc++;
            }
        }
        out.add("scenarios", scenarios);
        System.out.println(new GsonBuilder().create().toJson(out));
    }
}
