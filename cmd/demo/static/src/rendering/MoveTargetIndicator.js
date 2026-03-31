/**
 * MoveTargetIndicator - 右键移动目标指示器渲染器
 *
 * 管理移动目标指示器的生命周期和渲染（收缩椭圆圈）。
 *
 * 使用方式：
 *   this.moveTargetIndicator = new MoveTargetIndicator();
 *   this.moveTargetIndicator.show(x, y);
 *   // update 中：
 *   this.moveTargetIndicator.update(deltaTime);
 *   // render 中（相机变换内部）：
 *   this.moveTargetIndicator.render(ctx);
 */
export class MoveTargetIndicator {
    constructor() {
        /** @type {Array<{x,y,life,maxLife}>} */
        this.indicators = [];
    }

    /**
     * 显示移动目标指示器（自动替换旧的）
     * @param {number} x
     * @param {number} y
     * @param {number} [duration=1.0]
     */
    show(x, y, duration = 1.0) {
        this.indicators = [{ x, y, life: duration, maxLife: duration }];
    }

    /**
     * 每帧更新
     * @param {number} deltaTime
     */
    update(deltaTime) {
        this.indicators = this.indicators.filter(m => {
            m.life -= deltaTime;
            return m.life > 0;
        });
    }

    /**
     * 渲染所有指示器（需在相机变换内部调用）
     * @param {CanvasRenderingContext2D} ctx
     */
    render(ctx) {
        for (const m of this.indicators) {
            MoveTargetIndicator.renderOne(ctx, m);
        }
    }

    /**
     * 渲染单个指示器
     * @param {CanvasRenderingContext2D} ctx
     * @param {Object} m - { x, y, life, maxLife }
     */
    static renderOne(ctx, m) {
        const progress = 1 - m.life / m.maxLife;
        const alpha = m.life < 0.3 ? m.life / 0.3 : 1;

        const maxRx = 30, minRx = 8;
        const rx = maxRx - (maxRx - minRx) * progress;
        const ry = rx / 2;

        ctx.save();
        ctx.globalAlpha = alpha * 0.7;

        ctx.strokeStyle = '#00ff88';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.ellipse(m.x, m.y, rx, ry, 0, 0, Math.PI * 2);
        ctx.stroke();

        const fillAlpha = 0.1 + 0.2 * progress;
        ctx.fillStyle = `rgba(0, 255, 136, ${fillAlpha})`;
        ctx.beginPath();
        ctx.ellipse(m.x, m.y, rx, ry, 0, 0, Math.PI * 2);
        ctx.fill();

        ctx.restore();
    }

    /** 清空 */
    clear() {
        this.indicators = [];
    }
}
