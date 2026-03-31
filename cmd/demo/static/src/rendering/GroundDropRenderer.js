/**
 * GroundDropRenderer - 地面掉落物品渲染器
 *
 * 纯渲染逻辑，无场景依赖。
 * 使用方式：GroundDropRenderer.render(ctx, drop)
 */
export class GroundDropRenderer {
    /**
     * 渲染单个地面掉落物品
     * @param {CanvasRenderingContext2D} ctx
     * @param {Object} drop - { x, y, life, dropType, dropName, dropCount }
     */
    static render(ctx, drop) {
        const { x, y } = drop;
        const alpha = drop.life < 3 ? drop.life / 3 : 1;

        ctx.save();
        ctx.globalAlpha = alpha;

        // 地面光晕（脉冲）
        const colorMap = {
            health_potion: 'rgba(255,60,60,',
            mana_potion: 'rgba(60,100,255,',
            iron_arrow: 'rgba(140,200,220,'
        };
        const baseColor = colorMap[drop.dropType] || 'rgba(255,255,100,';
        const pulse = 0.6 + 0.4 * Math.sin(performance.now() / 300);

        ctx.beginPath();
        ctx.ellipse(x, y, 12, 6, 0, 0, Math.PI * 2);
        ctx.fillStyle = baseColor + (0.3 * pulse) + ')';
        ctx.fill();

        // 物品图标
        if (drop.dropType === 'health_potion') {
            GroundDropRenderer._drawPotion(ctx, x, y, '#cc2222', '#ff4444');
            GroundDropRenderer._drawCross(ctx, x, y);
        } else if (drop.dropType === 'mana_potion') {
            GroundDropRenderer._drawPotion(ctx, x, y, '#2244cc', '#4488ff');
            GroundDropRenderer._drawStar(ctx, x, y);
        } else if (drop.dropType === 'iron_arrow' || drop.dropType === 'wood_arrow') {
            GroundDropRenderer._drawArrow(ctx, x, y);
        }

        // 物品名称
        ctx.font = '9px Arial';
        ctx.textAlign = 'center';
        ctx.fillStyle = '#ffffff';
        ctx.strokeStyle = 'rgba(0,0,0,0.7)';
        ctx.lineWidth = 2;
        ctx.strokeText(drop.dropName, x, y + 10);
        ctx.fillText(drop.dropName, x, y + 10);

        ctx.restore();
    }

    static _drawPotion(ctx, x, y, dark, light) {
        ctx.fillStyle = dark;
        ctx.fillRect(x - 4, y - 14, 8, 12);
        ctx.fillStyle = light;
        ctx.fillRect(x - 3, y - 13, 6, 10);
        ctx.fillStyle = '#884400';
        ctx.fillRect(x - 2, y - 16, 4, 3);
    }

    static _drawCross(ctx, x, y) {
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(x - 1, y - 11, 2, 6);
        ctx.fillRect(x - 3, y - 9, 6, 2);
    }

    static _drawStar(ctx, x, y) {
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(x - 1, y - 11, 2, 6);
        ctx.fillRect(x - 3, y - 9, 6, 2);
    }

    static _drawArrow(ctx, x, y) {
        ctx.strokeStyle = '#aabbcc';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(x, y - 16);
        ctx.lineTo(x, y - 2);
        ctx.stroke();
        ctx.fillStyle = '#ccddee';
        ctx.beginPath();
        ctx.moveTo(x, y - 18);
        ctx.lineTo(x - 3, y - 14);
        ctx.lineTo(x + 3, y - 14);
        ctx.closePath();
        ctx.fill();
    }
}
