/**
 * BoneCorpseRenderer - NPC 死亡白骨渲染器
 *
 * 纯渲染逻辑，无场景依赖。
 * 使用方式：BoneCorpseRenderer.render(ctx, bone)
 */
export class BoneCorpseRenderer {
    /**
     * 渲染单个白骨
     * @param {CanvasRenderingContext2D} ctx
     * @param {Object} bone - { x, y, life, maxLife }
     */
    static render(ctx, bone) {
        const { x, y } = bone;
        const alpha = bone.life < 3 ? bone.life / 3 : 1;

        ctx.save();
        ctx.globalAlpha = alpha * 0.85;
        ctx.translate(x, y - 8);

        // 头骨
        ctx.fillStyle = '#d4cfc0';
        ctx.beginPath();
        ctx.ellipse(0, -14, 7, 6, 0, 0, Math.PI * 2);
        ctx.fill();
        ctx.strokeStyle = '#a09880';
        ctx.lineWidth = 0.8;
        ctx.stroke();

        // 眼眶
        ctx.fillStyle = '#2a2520';
        ctx.beginPath();
        ctx.ellipse(-2.5, -15, 1.8, 1.5, 0, 0, Math.PI * 2);
        ctx.fill();
        ctx.beginPath();
        ctx.ellipse(2.5, -15, 1.8, 1.5, 0, 0, Math.PI * 2);
        ctx.fill();

        // 脊椎
        ctx.strokeStyle = '#c8c3b0';
        ctx.lineWidth = 2.5;
        ctx.lineCap = 'round';
        ctx.beginPath();
        ctx.moveTo(0, -8);
        ctx.lineTo(0, 2);
        ctx.stroke();

        // 肋骨
        ctx.lineWidth = 1.2;
        for (let i = 0; i < 3; i++) {
            const ry = -6 + i * 3;
            ctx.beginPath();
            ctx.moveTo(0, ry);
            ctx.quadraticCurveTo(-6, ry - 1, -7, ry + 2);
            ctx.stroke();
            ctx.beginPath();
            ctx.moveTo(0, ry);
            ctx.quadraticCurveTo(6, ry - 1, 7, ry + 2);
            ctx.stroke();
        }

        // 腿骨
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(-5, 3);
        ctx.lineTo(-9, 10);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(3, 3);
        ctx.lineTo(8, 9);
        ctx.stroke();

        ctx.restore();
    }
}
