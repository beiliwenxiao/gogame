/**
 * SkillParticleEffects - 技能粒子特效库
 *
 * 将各技能的粒子配置从场景中解耦，提供统一的调用接口。
 * 使用方式：
 *   SkillParticleEffects.emit(particleSystem, skillName, params)
 *
 * params: { casterX, casterY, targetX, targetY, areaSize }
 */
export class SkillParticleEffects {
    /**
     * 发射技能粒子效果
     * @param {Object} particleSystem - ParticleSystem 实例
     * @param {string} skillName - 技能名称
     * @param {Object} params - { casterX, casterY, targetX, targetY, areaSize }
     */
    static emit(particleSystem, skillName, params) {
        if (!particleSystem) return;
        const { casterX = 0, casterY = 0, targetX = 0, targetY = 0, areaSize = 80 } = params;

        switch (skillName) {
            case '猛击':
                SkillParticleEffects._emitSlash(particleSystem, targetX, targetY);
                break;
            case '旋风斩':
                SkillParticleEffects.emitWhirlwind(particleSystem, casterX, casterY, areaSize);
                break;
            case '战吼':
                SkillParticleEffects._emitWarCry(particleSystem, casterX, casterY);
                break;
            case '多重射击':
                SkillParticleEffects._emitMultiArrow(particleSystem, casterX, casterY, targetX, targetY, areaSize);
                break;
            case '闪电箭':
                SkillParticleEffects._emitLightningArrow(particleSystem, casterX, casterY, targetX, targetY);
                break;
            case '天降箭雨':
                SkillParticleEffects._emitArrowRain(particleSystem, targetX, targetY, areaSize);
                break;
        }
    }

    /** 猛击：血色溅射 */
    static _emitSlash(ps, tx, ty) {
        ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 600, size: 6, color: '#cc0000', alpha: 0.95, gravity: 80, friction: 0.90 },
            15, { velocityRange: { min: 60, max: 150 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 4, max: 8 }, lifeRange: { min: 400, max: 700 } });
        ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 400, size: 3, color: '#880000', alpha: 0.7, gravity: 20, friction: 0.95 },
            12, { velocityRange: { min: 20, max: 80 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 2, max: 5 }, lifeRange: { min: 300, max: 500 } });
        ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 250, size: 4, color: '#ff3333', alpha: 1.0, gravity: 0, friction: 0.85 },
            8, { velocityRange: { min: 80, max: 180 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 2, max: 5 }, lifeRange: { min: 150, max: 300 } });
    }

    /**
     * 旋风斩：旋转风刃粒子（可单独调用，用于持续 tick）
     * @param {Object} ps - ParticleSystem
     * @param {number} cx - 中心 X
     * @param {number} cy - 中心 Y
     * @param {number} radius - 旋风半径
     */
    static emitWhirlwind(ps, cx, cy, radius) {
        for (let i = 0; i < 30; i++) {
            const angle = (i / 30) * Math.PI * 2;
            const r = radius * (0.4 + Math.random() * 0.6);
            ps.emit({ position: { x: cx + Math.cos(angle) * r, y: cy + Math.sin(angle) * r * 0.5 },
                velocity: { x: Math.cos(angle + Math.PI / 2) * 120, y: Math.sin(angle + Math.PI / 2) * 60 },
                life: 600, size: 3 + Math.random() * 4, color: '#aaddff', alpha: 0.85, gravity: 0, friction: 0.93 });
        }
        for (let i = 0; i < 16; i++) {
            const angle = (i / 16) * Math.PI * 2 + Math.random() * 0.3;
            const r = radius * 0.3;
            ps.emit({ position: { x: cx + Math.cos(angle) * r, y: cy + Math.sin(angle) * r * 0.5 },
                velocity: { x: Math.cos(angle + Math.PI / 2) * 160, y: Math.sin(angle + Math.PI / 2) * 80 },
                life: 400, size: 2 + Math.random() * 2, color: '#ffffff', alpha: 0.9, gravity: 0, friction: 0.90 });
        }
        ps.emitBurst({ position: { x: cx, y: cy }, velocity: { x: 0, y: 0 }, life: 500, size: 4, color: '#998866', alpha: 0.5, gravity: -10, friction: 0.92 },
            10, { velocityRange: { min: 30, max: 80 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 3, max: 6 }, lifeRange: { min: 300, max: 600 } });
    }

    /** 战吼：红色恐惧冲击波 */
    static _emitWarCry(ps, cx, cy) {
        ps.emitBurst({ position: { x: cx, y: cy }, velocity: { x: 0, y: 0 }, life: 700, size: 6, color: '#ff4444', alpha: 0.9, gravity: 0, friction: 0.94 },
            25, { velocityRange: { min: 80, max: 160 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 4, max: 8 }, lifeRange: { min: 500, max: 800 } });
        ps.emitBurst({ position: { x: cx, y: cy }, velocity: { x: 0, y: 0 }, life: 500, size: 4, color: '#880022', alpha: 0.7, gravity: -15, friction: 0.92 },
            15, { velocityRange: { min: 40, max: 100 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 3, max: 6 }, lifeRange: { min: 400, max: 600 } });
    }

    /** 多重射击：5支箭粒子轨迹 */
    static _emitMultiArrow(ps, cx, cy, tx, ty, areaSize) {
        for (let i = 0; i < 5; i++) {
            const ox = (Math.random() - 0.5) * (areaSize || 10) * 2;
            const oy = (Math.random() - 0.5) * (areaSize || 10) * 2;
            const dx = (tx + ox) - cx, dy = (ty + oy) - cy;
            const dist = Math.sqrt(dx * dx + dy * dy) || 1;
            for (let j = 0; j < 3; j++) {
                ps.emit({ position: { x: cx, y: cy },
                    velocity: { x: (dx / dist) * 200 * (0.9 + j * 0.05), y: (dy / dist) * 200 * (0.9 + j * 0.05) },
                    life: 400, size: 3 - j * 0.5, color: '#ffee44', alpha: 0.9, gravity: 0, friction: 0.98 });
            }
        }
    }

    /** 闪电箭：电弧 + 目标闪光 */
    static _emitLightningArrow(ps, cx, cy, tx, ty) {
        const dx = tx - cx, dy = ty - cy;
        for (let i = 0; i < 10; i++) {
            const t = i / 10;
            ps.emit({ position: { x: cx + dx * t + (Math.random() - 0.5) * 20, y: cy + dy * t + (Math.random() - 0.5) * 20 },
                velocity: { x: (Math.random() - 0.5) * 60, y: (Math.random() - 0.5) * 60 },
                life: 300 + Math.random() * 200, size: 3 + Math.random() * 4, color: '#44aaff', alpha: 0.95, gravity: 0, friction: 0.9 });
        }
        ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 350, size: 4, color: '#ffffff', alpha: 1.0, gravity: 0, friction: 0.88 },
            18, { velocityRange: { min: 40, max: 80 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 2, max: 6 }, lifeRange: { min: 250, max: 400 } });
    }

    /** 天降箭雨：延迟落箭 + 落地溅射 */
    static _emitArrowRain(ps, tx, ty, areaSize) {
        const rainRadius = areaSize || 30;
        for (let i = 0; i < 20; i++) {
            const ox = (Math.random() - 0.5) * rainRadius * 2;
            const oy = (Math.random() - 0.5) * rainRadius * 2;
            setTimeout(() => {
                if (!ps) return;
                ps.emit({ position: { x: tx + ox, y: ty + oy - 100 },
                    velocity: { x: (Math.random() - 0.5) * 10, y: 150 + Math.random() * 50 },
                    life: 500, size: 2 + Math.random() * 2, color: '#ffcc33', alpha: 0.9, gravity: 80, friction: 0.98 });
            }, Math.random() * 500);
        }
        setTimeout(() => {
            if (!ps) return;
            ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 300, size: 3, color: '#ff8833', alpha: 0.7, gravity: 40, friction: 0.9 },
                15, { velocityRange: { min: 20, max: 50 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 2, max: 4 }, lifeRange: { min: 200, max: 400 } });
        }, 400);
    }

    /**
     * 复活光环粒子
     * @param {Object} ps - ParticleSystem
     * @param {number} x
     * @param {number} y
     */
    static emitRespawn(ps, x, y) {
        if (!ps) return;
        const colors = ['#e0e8ff', '#ffffff', '#c0d0ff', '#a0c0ff'];
        for (let i = 0; i < 8; i++) {
            const angle = (Math.PI * 2 * i) / 8;
            const speed = 60 + Math.random() * 40;
            ps.createEmitter({
                position: { x, y }, rate: 12, duration: 800,
                particleConfig: {
                    position: { x, y },
                    velocity: { x: Math.cos(angle) * speed, y: Math.sin(angle) * speed * 0.5 },
                    life: 600, size: 3 + Math.random() * 2,
                    color: colors[i % colors.length], alpha: 0.9, gravity: 0, friction: 0.92
                }
            });
        }
    }

    /**
     * 掉落物出现粒子
     * @param {Object} ps - ParticleSystem
     * @param {number} x
     * @param {number} y
     * @param {string} dropType - 掉落物类型
     */
    static emitDropAppear(ps, x, y, dropType) {
        if (!ps) return;
        const colorMap = { health_potion: '#ff3333', mana_potion: '#3388ff', iron_arrow: '#88aacc' };
        const color = colorMap[dropType] || '#3388ff';
        ps.emitBurst({ position: { x, y }, velocity: { x: 0, y: -20 }, life: 500, size: 4, color, alpha: 0.9, gravity: 20, friction: 0.92 },
            10, { velocityRange: { min: 20, max: 50 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 3, max: 5 }, lifeRange: { min: 400, max: 600 } });
    }
}
