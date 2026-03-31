/**
 * StunEffectRenderer - 昏迷/恐惧头顶转圈星星效果渲染器
 *
 * 遍历实体列表，对 stunEffectUntil 有效的实体绘制头顶绕圈星星。
 *
 * 使用方式：
 *   this.stunEffectRenderer = new StunEffectRenderer();
 *   // update 中：
 *   this.stunEffectRenderer.update(deltaTime);
 *   // render 中（相机变换内部）：
 *   this.stunEffectRenderer.render(ctx, entities);
 */
export class StunEffectRenderer {
    constructor() {
        /** 旋转角度（弧度），每帧递增 */
        this.rotation = 0;
    }

    /**
     * 更新旋转角度
     * @param {number} deltaTime
     */
    update(deltaTime) {
        this.rotation = (this.rotation + deltaTime * Math.PI * 2.5) % (Math.PI * 2);
    }

    /**
     * 渲染所有有昏迷效果的实体头顶星星
     * @param {CanvasRenderingContext2D} ctx
     * @param {Array} entities - 实体数组（含 playerEntity、remotePlayers、npcEntities）
     */
    render(ctx, entities) {
        const now = Date.now();

        for (const entity of entities) {
            if (!entity.stunEffectUntil || now >= entity.stunEffectUntil) continue;
            if (entity.dead) continue;

            const transform = entity.getComponent('transform');
            if (!transform) continue;
            const sprite = entity.getComponent('sprite');
            const h = sprite?.height || 64;

            const cx = transform.position.x;
            const cy = transform.position.y - h - 8;

            const remaining = (entity.stunEffectUntil - now) / 1000;
            const alpha = remaining < 0.5 ? remaining / 0.5 : 1;

            const orbitRx = 14;
            const orbitRy = 7;
            const starCount = 3;

            ctx.save();
            ctx.globalAlpha = alpha;

            for (let i = 0; i < starCount; i++) {
                const angle = this.rotation + (i / starCount) * Math.PI * 2;
                const sx = cx + Math.cos(angle) * orbitRx;
                const sy = cy + Math.sin(angle) * orbitRy;

                ctx.save();
                ctx.translate(sx, sy);
                ctx.rotate(angle * 1.5);
                ctx.fillStyle = '#ffe066';
                ctx.strokeStyle = '#ff9900';
                ctx.lineWidth = 0.8;
                ctx.beginPath();
                const r1 = 4, r2 = 2, pts = 5;
                for (let p = 0; p < pts * 2; p++) {
                    const r = p % 2 === 0 ? r1 : r2;
                    const a = (p / (pts * 2)) * Math.PI * 2 - Math.PI / 2;
                    if (p === 0) ctx.moveTo(Math.cos(a) * r, Math.sin(a) * r);
                    else ctx.lineTo(Math.cos(a) * r, Math.sin(a) * r);
                }
                ctx.closePath();
                ctx.fill();
                ctx.stroke();
                ctx.restore();
            }

            ctx.restore();
        }
    }
}
