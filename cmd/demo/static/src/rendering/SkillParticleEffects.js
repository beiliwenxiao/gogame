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
        const { casterX = 0, casterY = 0, targetX = 0, targetY = 0, areaSize = 80, mas = null } = params;

        switch (skillName) {
            case '猛击':
                SkillParticleEffects._emitSlash(particleSystem, targetX, targetY);
                break;
            case '旋风斩':
                SkillParticleEffects.emitWhirlwind(particleSystem, casterX, casterY, areaSize, mas);
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

    /**
     * NPC 近战打击特效：橙红色冲击 + 暗色残影
     * @param {Object} ps - ParticleSystem
     * @param {number} tx - 目标 X
     * @param {number} ty - 目标 Y
     * @param {boolean} isCrit - 是否暴击
     */
    static emitNPCHit(ps, tx, ty, isCrit = false) {
        if (!ps) return;
        // 主冲击：橙红色放射
        const mainColor = isCrit ? '#ff6600' : '#cc4400';
        const count = isCrit ? 18 : 12;
        ps.emitBurst(
            { position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 450, size: 5, color: mainColor, alpha: 0.9, gravity: 30, friction: 0.88 },
            count, { velocityRange: { min: 50, max: 130 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 3, max: 7 }, lifeRange: { min: 300, max: 500 } }
        );
        // 暗色残影
        ps.emitBurst(
            { position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 300, size: 3, color: '#441100', alpha: 0.6, gravity: 0, friction: 0.92 },
            8, { velocityRange: { min: 20, max: 60 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 2, max: 4 }, lifeRange: { min: 200, max: 350 } }
        );
        // 暴击时加白色闪光
        if (isCrit) {
            ps.emitBurst(
                { position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 200, size: 6, color: '#ffffff', alpha: 1.0, gravity: 0, friction: 0.82 },
                10, { velocityRange: { min: 80, max: 160 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 3, max: 6 }, lifeRange: { min: 150, max: 250 } }
            );
        }
    }

    /** 猛击：血色溅射 */
    static _emitSlash(ps, tx, ty) {        ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 600, size: 6, color: '#cc0000', alpha: 0.95, gravity: 80, friction: 0.90 },
            15, { velocityRange: { min: 60, max: 150 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 4, max: 8 }, lifeRange: { min: 400, max: 700 } });
        ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 400, size: 3, color: '#880000', alpha: 0.7, gravity: 20, friction: 0.95 },
            12, { velocityRange: { min: 20, max: 80 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 2, max: 5 }, lifeRange: { min: 300, max: 500 } });
        ps.emitBurst({ position: { x: tx, y: ty }, velocity: { x: 0, y: 0 }, life: 250, size: 4, color: '#ff3333', alpha: 1.0, gravity: 0, friction: 0.85 },
            8, { velocityRange: { min: 80, max: 180 }, angleRange: { min: 0, max: Math.PI * 2 }, sizeRange: { min: 2, max: 5 }, lifeRange: { min: 150, max: 300 } });
    }

    /**
     * 旋风斩：旋转刀光 + 风气流（可单独调用，用于持续 tick）
     * @param {Object} ps - ParticleSystem
     * @param {number} cx - 中心 X
     * @param {number} cy - 中心 Y
     * @param {number} radius - 旋风半径
     * @param {Object} [mas] - MeleeAttackSystem 实例（可选，有则生成扇形刀光）
     */
    static emitWhirlwind(ps, cx, cy, radius, mas = null) {
        const rx = radius;
        const ry = radius * 0.5;

        // ── 扇形刀光：6条均匀分布在圆周，沿切线方向 ──
        if (mas) {
            const bladeCount = 6;
            for (let i = 0; i < bladeCount; i++) {
                const angle = (i / bladeCount) * Math.PI * 2;
                // 刀光中心在圆周上
                const slashCx = cx + Math.cos(angle) * rx;
                const slashCy = cy + Math.sin(angle) * ry;
                // 切线方向（逆时针）
                const dir = angle + Math.PI / 2;
                mas.sectorSlashEffects.push({
                    cx: slashCx,
                    cy: slashCy,
                    radius: radius * 0.45,
                    dir,
                    halfAngle: Math.PI / 5,   // 约36°，细长刀光
                    age: 0,
                    maxAge: 0.2,
                    type: 'slash',
                    damage: 0,
                    hitEntities: [],
                    isNPC: false
                });
            }
        }

        // ── 风气流：沿圆周内侧螺旋扩散 ──
        const windCount = 28;
        for (let i = 0; i < windCount; i++) {
            const angle = (i / windCount) * Math.PI * 2 + Math.random() * 0.3;
            const r = rx * (0.2 + Math.random() * 0.7);
            const sx = cx + Math.cos(angle) * r;
            const sy = cy + Math.sin(angle) * r * 0.5;
            const tx = -Math.sin(angle);
            const ty =  Math.cos(angle) * 0.5;
            const speed = 60 + Math.random() * 60;
            ps.emit({
                position: { x: sx, y: sy },
                velocity: { x: tx * speed + Math.cos(angle) * 12, y: ty * speed + Math.sin(angle) * 6 },
                life: 250 + Math.random() * 150, size: 1 + Math.random() * 1.5,
                color: '#c8e8ff', alpha: 0.25 + Math.random() * 0.2,
                gravity: -5, friction: 0.94
            });
        }

        // ── 中心气旋：向上飘散的细小白点 ──
        for (let i = 0; i < 8; i++) {
            ps.emit({
                position: { x: cx + (Math.random() - 0.5) * rx * 0.5, y: cy + (Math.random() - 0.5) * ry * 0.5 },
                velocity: { x: (Math.random() - 0.5) * 30, y: -20 - Math.random() * 20 },
                life: 250, size: 1 + Math.random(),
                color: '#ffffff', alpha: 0.45, gravity: 0, friction: 0.92
            });
        }
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
