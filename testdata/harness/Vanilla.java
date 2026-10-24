import com.mojang.blaze3d.vertex.PoseStack;
import com.mojang.math.Axis;
import net.minecraft.client.model.HumanoidModel;
import net.minecraft.client.model.geom.ModelPart;
import net.minecraft.client.model.geom.builders.*;
import net.minecraft.client.renderer.entity.state.HumanoidRenderState;
import net.minecraft.client.resources.model.cuboid.ItemTransform;
import net.minecraft.util.Mth;
import net.minecraft.world.entity.HumanoidArm;
import net.minecraft.world.entity.WalkAnimationState;
import org.joml.Quaternionf;
import org.joml.Vector3f;
import java.util.Random;

// Golden data from Minecraft 26.2's own classes (run headless): the vanilla
// HumanoidModel poses, WalkAnimationState, the LivingEntity body yaw and the
// first-person render chain of Player Animation Lib + Better Combat
// (LivingEntityRenderer.submit, the PAL mixins, ItemInHandLayer,
// ItemTransform), for the Go port's tests. Usage: Vanilla > vanilla.json
public class Vanilla {
    static StringBuilder out = new StringBuilder();
    static void f(float v) { out.append(Float.toString(v)); }
    static void arr(float... v) { out.append("["); for (int i = 0; i < v.length; i++) { if (i > 0) out.append(","); f(v[i]); } out.append("]"); }

    public static void main(String[] a) {
        ModelPart root = LayerDefinition.create(HumanoidModel.createMesh(CubeDeformation.NONE, 0), 64, 64).bakeRoot();
        HumanoidModel<HumanoidRenderState> m = new HumanoidModel<>(root);
        Random r = new Random(7);
        out.append("{\"poses\":[");
        for (int i = 0; i < 200; i++) {
            HumanoidRenderState s = new HumanoidRenderState();
            s.walkAnimationPos = r.nextFloat() * 200; s.walkAnimationSpeed = r.nextFloat(); s.ageInTicks = r.nextFloat() * 5000;
            s.xRot = (r.nextFloat() - 0.5f) * 180; s.yRot = (r.nextFloat() - 0.5f) * 100;
            s.rightArmPose = HumanoidModel.ArmPose.ITEM; s.leftArmPose = HumanoidModel.ArmPose.EMPTY; s.mainArm = HumanoidArm.RIGHT;
            m.setupAnim(s);
            ModelPart[] parts = {m.head, m.body, m.rightArm, m.leftArm, m.rightLeg, m.leftLeg};
            out.append(i == 0 ? "" : ",").append("{\"in\":"); arr(s.walkAnimationPos, s.walkAnimationSpeed, s.ageInTicks, s.xRot, s.yRot);
            out.append(",\"parts\":[");
            for (int p = 0; p < parts.length; p++) { ModelPart q = parts[p]; out.append(p == 0 ? "" : ","); arr(q.x, q.y, q.z, q.xRot, q.yRot, q.zRot); }
            out.append("]}\n");
        }
        // WalkAnimationState driven like LivingEntity.calculateEntityAnimation: distance per tick, sampled at partial ticks
        out.append("],\"walk\":[");
        for (int i = 0; i < 20; i++) {
            WalkAnimationState w = new WalkAnimationState();
            out.append(i == 0 ? "" : ",").append("[");
            for (int t = 0; t < 30; t++) {
                float dist = r.nextInt(3) == 0 ? 0 : r.nextFloat() * 0.3f;
                w.update(Math.min(dist * 4.0F, 1.0F), 0.4F, 1.0F);
                float pt = r.nextFloat();
                out.append(t == 0 ? "" : ","); arr(dist, pt, w.position(pt), w.speed(pt), w.position(), w.speed());
            }
            out.append("]\n");
        }
        // LivingEntity.tick body yaw (plus Better Combat's extra tickHeadTurn while an attack plays, Player.tick TAIL)
        out.append("],\"yaw\":[");
        for (int i = 0; i < 20; i++) {
            float yBodyRot = (r.nextFloat() - 0.5f) * 720, yBodyRotO = yBodyRot, yRot = yBodyRot;
            out.append(i == 0 ? "" : ",").append("{\"start\":"); f(yBodyRot); out.append(",\"ticks\":[");
            for (int t = 0; t < 40; t++) {
                yBodyRotO = yBodyRot;
                yRot += (r.nextFloat() - 0.5f) * 90;
                double xd = r.nextInt(3) == 0 ? 0 : (r.nextDouble() - 0.5) * 0.4, zd = r.nextInt(3) == 0 ? 0 : (r.nextDouble() - 0.5) * 0.4;
                boolean attacking = r.nextBoolean();
                float sideDist = (float)(xd * xd + zd * zd);
                float yBodyRotT = yBodyRot;
                if (sideDist > 0.0025000002F) {
                    float walkDirection = (float)Mth.atan2(zd, xd) * (180.0F / (float)Math.PI) - 90.0F;
                    float diff = Mth.abs(Mth.wrapDegrees(yRot) - walkDirection);
                    yBodyRotT = 95.0F < diff && diff < 265.0F ? walkDirection - 180.0F : walkDirection;
                }
                yBodyRot = tickHeadTurn(yBodyRot, yRot, yBodyRotT);
                while (yBodyRot - yBodyRotO < -180.0F) yBodyRotO -= 360.0F;
                while (yBodyRot - yBodyRotO >= 180.0F) yBodyRotO += 360.0F;
                if (attacking) yBodyRot = tickHeadTurn(yBodyRot, yRot, yRot);
                out.append(t == 0 ? "" : ",").append("["); f(yRot); out.append(",").append(xd).append(",").append(zd).append(",").append(attacking).append(","); f(yBodyRotO); out.append(","); f(yBodyRot); out.append("]");
            }
            out.append("]}\n");
        }
        // The first-person render chain: entity space relative to the camera at the eye -> right arm -> right item model space
        out.append("],\"chain\":[");
        for (int i = 0; i < 100; i++) {
            float[] body = rnd(r, 9), arm = rnd(r, 9), item = rnd(r, 9);
            if (r.nextInt(4) == 0) { item[3] = 0; }
            if (r.nextInt(4) == 0) { item[4] = 0; item[5] = 0; }
            float bodyRot = (r.nextFloat() - 0.5f) * 720;
            float[] disp = {(r.nextFloat() - 0.5f) * 360, (r.nextFloat() - 0.5f) * 360, (r.nextFloat() - 0.5f) * 360, (r.nextFloat() - 0.5f) * 20, (r.nextFloat() - 0.5f) * 20, (r.nextFloat() - 0.5f) * 20, r.nextFloat() * 2, r.nextFloat() * 2, r.nextFloat() * 2};
            float eyeHeight = 1.62f;
            PoseStack ps = new PoseStack();
            ps.translate(0, -eyeHeight, 0); // EntityRenderDispatcher: the entity's position relative to the camera
            // LivingEntityRenderer.submit
            ps.scale(1, 1, 1);
            ps.mulPose(Axis.YP.rotationDegrees(180.0F - bodyRot)); // setupRotations
            ps.scale(-1.0F, -1.0F, 1.0F);
            // PAL LivingEntityRendererMixin.doTranslations (first person, not following the camera)
            ps.scale(-1.0F, -1.0F, 1.0F);
            ps.translate(0.0F, eyeHeight, 0.0F);
            ps.translate(0.0F, -eyeHeight, 0.0F);
            ps.translate(-body[0] / 16, body[1] / 16 + 0.75, body[2] / 16);
            float brx = body[3] * -1, bry = body[4] * -1;
            ps.last().pose().rotateZYX(body[5], bry, brx);
            ps.scale(body[6], body[7], body[8]);
            ps.translate(0, -0.75, 0);
            ps.scale(-1.0F, -1.0F, 1.0F);
            ps.scale(0.9375F, 0.9375F, 0.9375F); // AvatarRenderer.scale
            ps.translate(0.0F, -1.501F, 0.0F);
            // ItemInHandLayer.submitArmWithItem: translateToHand with the PAL PlayerModelMixin replacement
            root.translateAndRotate(ps);
            ModelPart part = m.rightArm;
            part.setPos(arm[0] + part.getInitialPose().x(), -arm[1] + part.getInitialPose().y(), arm[2] + part.getInitialPose().z()); // RenderUtil.translatePartToBone
            part.setRotation(arm[3], arm[4] + part.getInitialPose().yRot(), arm[5]);
            part.xScale = arm[6]; part.yScale = arm[7]; part.zScale = arm[8];
            ps.translate(part.x / 16.0F, part.y / 16.0F, part.z / 16.0F);
            if (part.xRot != 0.0F || part.yRot != 0.0F || part.zRot != 0.0F) ps.last().pose().rotateZYX(part.zRot, part.yRot, part.xRot);
            ps.translate(0, (part.yScale - 1) * 0.609375, (part.zScale - 1) * 0.0625);
            float[] armM = new float[16]; ps.last().pose().get(armM);
            ps.translate(item[0] / 16, -item[1] / 16, item[2] / 16); // PAL ItemInHandLayerMixin.changeItemLocation
            ps.mulPose(Axis.XP.rotationDegrees(-90.0F));
            ps.mulPose(Axis.YP.rotationDegrees(180.0F));
            ps.translate(1 * 1.0F / 16.0F, 2.0F / 16.0F, -10.0F / 16.0F);
            // PAL ItemInHandLayerMixin.changeItemRotationAndScale
            if (item[5] != 0) ps.mulPose(Axis.ZP.rotation(-item[4]));
            if (item[4] != 0) ps.mulPose(Axis.YP.rotation(-item[5]));
            if (item[3] != 0) ps.mulPose(Axis.XP.rotation(-item[3]));
            ps.scale(item[6], item[7], item[8]);
            // ItemStackRenderState.submit -> the model's thirdperson_righthand ItemTransform (as deserialized: translation / 16, clamped)
            Vector3f tr = new Vector3f(disp[3], disp[4], disp[5]).mul(0.0625F);
            tr.set(Mth.clamp(tr.x, -5.0F, 5.0F), Mth.clamp(tr.y, -5.0F, 5.0F), Mth.clamp(tr.z, -5.0F, 5.0F));
            new ItemTransform(new Vector3f(disp[0], disp[1], disp[2]), tr, new Vector3f(disp[6], disp[7], disp[8])).apply(false, ps.last());
            float[] itemM = new float[16]; ps.last().pose().get(itemM);
            out.append(i == 0 ? "" : ",").append("{\"body\":"); arr(body); out.append(",\"arm\":"); arr(arm); out.append(",\"item\":"); arr(item);
            out.append(",\"bodyRot\":"); f(bodyRot); out.append(",\"display\":"); arr(disp); out.append(",\"armM\":"); arr(armM); out.append(",\"itemM\":"); arr(itemM); out.append("}\n");
        }
        // The main hand's item in first person: ItemInHandRenderer.submitHandsWithItems' sway, submitArmWithItem's arm offset,
        // then its use pose (bow, crossbow charging) or swingArm (and the loaded crossbow's hold), then renderItem's firstperson_righthand ItemTransform
        out.append("],\"hand\":[");
        for (int i = 0; i < 100; i++) {
            float swayX = (r.nextFloat() - 0.5f) * 40, swayY = (r.nextFloat() - 0.5f) * 40, armHeight = r.nextFloat();
            int use = r.nextInt(4); // 0 swing, 1 bow, 2 crossbow charging, 3 crossbow loaded
            float t = use == 1 ? r.nextFloat() * 30 : use == 2 ? r.nextFloat() * 28 : r.nextInt(3) == 0 ? 0 : r.nextFloat(); // ticks held, or the swing's progress
            float[] disp = {(r.nextFloat() - 0.5f) * 360, (r.nextFloat() - 0.5f) * 360, (r.nextFloat() - 0.5f) * 360, (r.nextFloat() - 0.5f) * 20, (r.nextFloat() - 0.5f) * 20, (r.nextFloat() - 0.5f) * 20, r.nextFloat() * 2, r.nextFloat() * 2, r.nextFloat() * 2};
            PoseStack ps = new PoseStack();
            ps.mulPose(Axis.XP.rotationDegrees(swayX));
            ps.mulPose(Axis.YP.rotationDegrees(swayY));
            ps.translate(0.56F, -0.52F + armHeight * -0.6F, -0.72F); // applyItemArmTransform
            if (use == 1 || use == 2) {
                float power;
                if (use == 1) {
                    ps.translate(-0.2785682F, 0.18344387F, 0.15731531F);
                    ps.mulPose(Axis.XP.rotationDegrees(-13.935F));
                    ps.mulPose(Axis.YP.rotationDegrees(35.3F));
                    ps.mulPose(Axis.ZP.rotationDegrees(-9.785F));
                    power = t / 20.0F;
                    power = (power * power + power * 2.0F) / 3.0F;
                } else {
                    ps.translate(-0.4785682F, -0.094387F, 0.05731531F);
                    ps.mulPose(Axis.XP.rotationDegrees(-11.935F));
                    ps.mulPose(Axis.YP.rotationDegrees(65.3F));
                    ps.mulPose(Axis.ZP.rotationDegrees(-9.785F));
                    power = t / 25; // CrossbowItem.getChargeDuration without Quick Charge
                }
                if (power > 1.0F) power = 1.0F;
                if (power > 0.1F) { float shake = Mth.sin((t - 0.1F) * 1.3F) * (power - 0.1F); ps.translate(shake * 0.0F, shake * 0.004F, shake * 0.0F); }
                ps.translate(power * 0.0F, power * 0.0F, power * 0.04F);
                ps.scale(1.0F, 1.0F, 1.0F + power * 0.2F);
                ps.mulPose(Axis.YN.rotationDegrees(45.0F));
            } else {
                ps.translate(-0.4F * Mth.sin(Mth.sqrt(t) * (float) Math.PI), 0.2F * Mth.sin(Mth.sqrt(t) * (float) (Math.PI * 2)), -0.2F * Mth.sin(t * (float) Math.PI)); // swingArm
                ps.mulPose(Axis.YP.rotationDegrees(45.0F + Mth.sin(t * t * (float) Math.PI) * -20.0F)); // applyItemArmAttackTransform
                float xz = Mth.sin(Mth.sqrt(t) * (float) Math.PI);
                ps.mulPose(Axis.ZP.rotationDegrees(xz * -20.0F));
                ps.mulPose(Axis.XP.rotationDegrees(xz * -80.0F));
                ps.mulPose(Axis.YP.rotationDegrees(-45.0F));
                if (use == 3 && t < 0.001F) { ps.translate(-0.641864F, 0.0F, 0.0F); ps.mulPose(Axis.YP.rotationDegrees(10.0F)); }
            }
            Vector3f tr = new Vector3f(disp[3], disp[4], disp[5]).mul(0.0625F);
            tr.set(Mth.clamp(tr.x, -5.0F, 5.0F), Mth.clamp(tr.y, -5.0F, 5.0F), Mth.clamp(tr.z, -5.0F, 5.0F));
            new ItemTransform(new Vector3f(disp[0], disp[1], disp[2]), tr, new Vector3f(disp[6], disp[7], disp[8])).apply(false, ps.last());
            float[] handM = new float[16]; ps.last().pose().get(handM);
            out.append(i == 0 ? "" : ",").append("{\"in\":"); arr(swayX, swayY, armHeight, use, t); out.append(",\"display\":"); arr(disp); out.append(",\"m\":"); arr(handM); out.append("}\n");
        }
        out.append("]}");
        System.out.println(out);
    }
    static float[] rnd(Random r, int n) { float[] v = new float[n]; for (int i = 0; i < 6; i++) v[i] = (r.nextFloat() - 0.5f) * (i < 3 ? 8 : 6); for (int i = 6; i < 9; i++) v[i] = 0.5f + r.nextFloat(); return v; }
    // LivingEntity.tickHeadTurn
    static float tickHeadTurn(float yBodyRot, float yRot, float yBodyRotT) {
        float yBodyRotD = Mth.wrapDegrees(yBodyRotT - yBodyRot);
        yBodyRot += yBodyRotD * 0.3F;
        float headDiff = Mth.wrapDegrees(yRot - yBodyRot);
        if (Math.abs(headDiff) > 50.0F) yBodyRot = yBodyRot + (headDiff - Mth.sign(headDiff) * 50.0F);
        return yBodyRot;
    }
}
