/**
 * SkillRangeIndicator - 技能范围虚线指示器
 *
 * 管理技能范围指示器的生命周期、更新和渲染。
 * 支持 fan（扇形）、ellipse（椭圆）、circle（圆形）三种形状。
 *
 * 使用方式：
 *   this.skillRangeIndicator = new SkillRangeIndicator();
 *   this.skillRangeIndicator.show({ areaType, rx, ry, direction, halfAngle, ... });
 *   // update 中：
 *   this.skillRangeIndicator.update(deltaTime);
 *   // render 中（相机变换内部）：
 *   this.skillRangeIndicator.render(ctx, footCenter);
 */
export class SkillRangeIndicator {
    constructor() {
        /** @type {Array} 活跃指示器列表 */
        this.indicators = [];
    }

    /**
     * 触发显示一个范围指示器
     * @param {Object} opts
     * @param {string} opts.areaType - 'fan' | 'ellipse' | 'circle'
     * @param {number} opts.rx - 水平半径
     * @param {number} opts.ry - 垂直半径
     * @param {number} [opts.direction=0] - 扇形朝向（弧度）
     * @param {number} [opts.halfAngle=Math.PI/4] - 扇形半角（弧度）
     * @param {number} [opts.targetX] - circle 类型的目标 X
     * @param {number} [opts.targetY] - circle 类型的目标 Y
     * @param {string} [opts.color] - 描边颜色
     * @param {string} [opts.fillColor] - 填充颜色
     * @param {number} [opts.duration=1.0] - 持续时间（秒）
     */
    show(opts) {
        this.indicators.push({
            areaType: opts.areaType,
            rx: opts.rx,
            ry: opts.ry,
            targetX: opts.targetX,
            targetY: opts.targetY,
            direction: opts.direction || 0,
            halfAngle: opts.halfAngle || Math.PI / 4,
            color: opts.color,
            fillColor: opts.fillColor,
            life: opts.duration || 1.0,
            maxLife: opts.duration || 1.0,
            dashOffset: 0
        });
    }

    /**
     * 每帧更新（倒计时 + 虚线动画）
     * @param {number} deltaTime
     */
    update(deltaTime) {
        this.indicators = this.indicators.filter(ind => {
            ind.life -= deltaTime;
            ind.dashOffset += 60 * deltaTime;
            return ind.life > 0;
        });
    }

    /**
     * 渲染所有活跃指示器（需在相机变换内部调用）
     * @param {CanvasRenderingContext2D} ctx
     * @param {{ x: number, y: number }} footCenter - 玩家脚下坐标（实时跟随）
     */
    render(ctx, footCenter) {
        if (this.indicators.length === 0) return;
        if (!footCenter) return;

        ctx.save();

        for (const ind of this.indicators) {
            const alpha = ind.life < 0.5 ? ind.life / 0.5 : 1;
            ctx.globalAlpha = alpha * 0.8;
            ctx.setLineDash([6, 4]);
            ctx.lineDashOffset = ind.dashOffset || 0;
            ctx.lineWidth = 1.5;

            const cx = footCenter.x;
            const cy = footCenter.y;

            if (ind.areaType === 'fan') {
                const halfAngle = ind.halfAngle || Math.PI / 4;
                const steps = 32;
                ctx.strokeStyle = ind.color || 'rgba(255, 160, 50, 0.85)';
                ctx.fillStyle = ind.fillColor || 'rgba(255, 160, 50, 0.10)';
                ctx.beginPath();
                ctx.moveTo(cx, cy);
                for (let i = 0; i <= steps; i++) {
                    const a = (ind.direction - halfAngle) + (i / steps) * halfAngle * 2;
                    ctx.lineTo(cx + Math.cos(a) * ind.rx, cy + Math.sin(a) * ind.ry);
                }
                ctx.closePath();
                ctx.fill();
                ctx.stroke();

            } else if (ind.areaType === 'ellipse') {
                ctx.strokeStyle = ind.color || 'rgba(100, 200, 255, 0.85)';
                ctx.fillStyle = ind.fillColor || 'rgba(100, 200, 255, 0.08)';
                ctx.beginPath();
                ctx.ellipse(cx, cy, ind.rx, ind.ry, 0, 0, Math.PI * 2);
                ctx.fill();
                ctx.stroke();

            } else if (ind.areaType === 'circle') {
                const tcx = ind.targetX || cx;
                const tcy = ind.targetY || cy;
                ctx.strokeStyle = ind.color || 'rgba(100, 200, 255, 0.85)';
                ctx.fillStyle = ind.fillColor || 'rgba(100, 200, 255, 0.08)';
                ctx.beginPath();
                ctx.ellipse(tcx, tcy, ind.rx, ind.ry, 0, 0, Math.PI * 2);
                ctx.fill();
                ctx.stroke();
            }
        }

        ctx.setLineDash([]);
        ctx.lineDashOffset = 0;
        ctx.globalAlpha = 1;
        ctx.restore();
    }

    /** 清空所有指示器 */
    clear() {
        this.indicators = [];
    }
}
