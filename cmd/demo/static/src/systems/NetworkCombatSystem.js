/**
 * NetworkCombatSystem - 联网战斗系统
 * 
 * 将联网场景中的攻击、技能、选敌、伤害处理等逻辑从场景中抽离，
 * 场景只需持有该系统实例并委托调用即可。
 * 
 * 依赖：场景需提供 scene.playerEntity, scene.ws, scene.remotePlayers,
 *        scene.npcEntities, scene.selectedTarget, scene.weaponRenderer,
 *        scene.floatingTextManager, scene.camera, scene.inputManager,
 *        scene.skills, scene.skillCooldowns, scene.particleSystem
 */
import { SkillParticleEffects } from '../rendering/SkillParticleEffects.js';

export class NetworkCombatSystem {
    /**
     * @param {object} scene - 持有该系统的场景实例
     */
    constructor(scene) {
        this.scene = scene;
    }

    // ─── 辅助：获取目标实体 ───
    _getTargetEntity(targetId) {
        if (!targetId) return null;
        const npc = this.scene.npcEntities.get(targetId);
        if (npc) return npc;
        return this.scene.remotePlayers.get(targetId);
    }

    _isNPCTarget(targetId) {
        return this.scene.npcEntities.has(targetId);
    }

    // ─── 辅助：获取武器攻击范围 ───
    _getWeaponRange() {
        const entity = this.scene.playerEntity;
        if (!entity) return 100;
        const equipment = entity.getComponent('equipment');
        if (equipment) {
            const weapon = equipment.getEquipment('mainhand');
            // 返回攻击距离（像素），不是角度
            if (weapon && weapon.attackDistance) return weapon.attackDistance;
            if (weapon && weapon.attackRange && weapon.subType !== 'bow') return weapon.attackRange;
        }
        const charClass = entity.class || 'warrior';
        return charClass === 'archer' ? 250 : 100;
    }

    // ─── 自动选中最近敌人 ───
    autoSelectNearestEnemy() {
        const scene = this.scene;
        const selfT = scene.playerEntity?.getComponent('transform');
        if (!selfT) return;
        if (scene.isInSafeZone(selfT.position.x, selfT.position.y)) return;

        const selfSprite = scene.playerEntity.getComponent('sprite');
        const selfH = selfSprite?.height || 64;
        const selfCx = selfT.position.x;
        const selfCy = selfT.position.y - selfH / 10;

        const maxRange = this._getWeaponRange();
        let closestId = null;
        let closestDist = maxRange;

        // 检查 NPC
        for (const [id, entity] of scene.npcEntities) {
            if (entity.dead) continue;
            const t = entity.getComponent('transform');
            if (!t) continue;
            const s = entity.getComponent('sprite');
            const h = s?.height || 64;
            const dx = t.position.x - selfCx;
            const dy = (t.position.y - h / 10) - selfCy;
            const dy2d = dy * 2;
            const dist = Math.sqrt(dx * dx + dy2d * dy2d);
            if (dist < closestDist) { closestDist = dist; closestId = id; }
        }

        // 检查远程玩家
        for (const [id, entity] of scene.remotePlayers) {
            if (entity.dead) continue;
            const t = entity.getComponent('transform');
            if (!t) continue;
            const s = entity.getComponent('sprite');
            const h = s?.height || 64;
            const dx = t.position.x - selfCx;
            const dy = (t.position.y - h / 10) - selfCy;
            const dy2d = dy * 2;
            const dist = Math.sqrt(dx * dx + dy2d * dy2d);
            if (dist < closestDist) { closestDist = dist; closestId = id; }
        }

        if (closestId !== null) {
            scene.selectedTarget = closestId;
        }
    }

    // ─── 普攻目标 ───
    attackTarget() {
        const scene = this.scene;
        if (!scene.selectedTarget || !scene.ws) {
            return;
        }
        if (scene.playerEntity && scene.playerEntity.dead) {
            return;
        }
        // 昏迷检查
        if (scene.playerEntity && scene.playerEntity.stunUntil && Date.now() < scene.playerEntity.stunUntil) {
            return;
        }

        const isNPC = this._isNPCTarget(scene.selectedTarget);
        const entity = this._getTargetEntity(scene.selectedTarget);
        if (!entity || entity.dead) {
            return;
        }

        // 安全区检查
        if (scene.playerEntity) {
            const selfT = scene.playerEntity.getComponent('transform');
            if (selfT && scene.isInSafeZone(selfT.position.x, selfT.position.y)) {
                console.log('NetworkCombatSystem.attackTarget: 安全区内禁止攻击');
                if (scene.floatingTextManager) {
                    scene.floatingTextManager.addText(selfT.position.x, selfT.position.y - 20, '安全区内禁止攻击', '#ffaa00');
                }
                return;
            }
        }

        // 前端范围预判
        if (scene.playerEntity) {
            const selfTransform = scene.playerEntity.getComponent('transform');
            const targetTransform = entity.getComponent('transform');
            const combat = scene.playerEntity.getComponent('combat');
            if (selfTransform && targetTransform && combat) {
                const maxRange = this._getWeaponRange();
                const dx = targetTransform.position.x - selfTransform.position.x;
                const dy = targetTransform.position.y - selfTransform.position.y;
                const dist = Math.sqrt(dx * dx + dy * dy);

                if (!combat.isInSkillRange(selfTransform.position, targetTransform.position, { range: maxRange, area_type: 'single' })) {
                    if (scene.floatingTextManager) {
                        scene.floatingTextManager.addText(selfTransform.position.x, selfTransform.position.y - 20, '超出攻击范围', '#ff6600');
                    }
                    return;
                }
            }
        }

        const msgType = isNPC ? 'attack_npc' : 'attack';
        scene.ws.send(msgType, { target_id: scene.selectedTarget });

        // 触发弯刀攻击动画
        if (scene.weaponRenderer) {
            const selfTransform = scene.playerEntity?.getComponent('transform');
            const targetTransform = entity.getComponent('transform');
            if (selfTransform && targetTransform) {
                const dx = targetTransform.position.x - selfTransform.position.x;
                const dy = targetTransform.position.y - selfTransform.position.y;
                scene.weaponRenderer.currentMouseAngle = Math.atan2(dy, dx);
            }
            scene.weaponRenderer.startAttack('thrust');
        }
    }

    // ─── 施放技能 ───
    castSkill(skillId) {
        const scene = this.scene;
        if (!scene.ws || !scene.playerEntity) return;
        if (scene.playerEntity.dead) return;
        // 昏迷检查
        if (scene.playerEntity.stunUntil && Date.now() < scene.playerEntity.stunUntil) return;
        // 恐惧检查
        if (scene.playerEntity.fearUntil && Date.now() < scene.playerEntity.fearUntil) return;

        const now = Date.now();
        const cd = scene.skillCooldowns[skillId];
        if (cd && now < cd) return;

        const transform = scene.playerEntity.getComponent('transform');
        if (!transform) return;

        // 安全区检查
        if (scene.isInSafeZone(transform.position.x, transform.position.y)) {
            if (scene.floatingTextManager) {
                scene.floatingTextManager.addText(transform.position.x, transform.position.y - 20, '安全区内禁止使用技能', '#ffaa00');
            }
            return;
        }

        // MP 预判
        const skill = scene.skills.find(s => s.id === skillId);
        if (skill) {
            const stats = scene.playerEntity.getComponent('stats');
            if (stats && skill.mp_cost > 0 && stats.mp < skill.mp_cost) {
                if (scene.floatingTextManager) {
                    scene.floatingTextManager.addText(transform.position.x, transform.position.y - 20, 'MP不足', '#6699ff');
                }
                return;
            }
        }

        let targetX = transform.position.x;
        let targetY = transform.position.y;
        let targetId = 0;

        // 攻击圆心：脚下 1/10 高度
        const selfSpriteCS = scene.playerEntity.getComponent('sprite');
        const selfHCS = selfSpriteCS?.height || 64;
        const selfCxCS = transform.position.x;
        const selfCyCS = transform.position.y - selfHCS / 10;

        // ── 多重射击：5波次，以普攻力度作为技能发射，不消耗武器冷却 ──
        if (skill && skill.name === '多重射击') {
            scene.skillCooldowns[skillId] = now + skill.cooldown * 1000;
            const mas = scene.meleeAttackSystem;
            let count = 0;
            const fireWave = () => {
                if (!scene.playerEntity || scene.playerEntity.dead) return;

                const t = scene.playerEntity.getComponent('transform');
                if (!t) return;
                const sp = scene.playerEntity.getComponent('sprite');
                const h = sp?.height || 64;
                const cx = t.position.x;
                const cy = t.position.y - h / 10;

                // 鼠标方向
                let dir = mas ? mas.sectorDirection : 0;
                if (scene.inputManager && scene.camera) {
                    const mw = scene.inputManager.getMouseWorldPosition(scene.camera);
                    if (mw) dir = Math.atan2((mw.y - cy) * 2, mw.x - cx);
                }

                // 武器参数
                const eq = scene.playerEntity.getComponent('equipment');
                const wep = eq?.getEquipment('mainhand');
                const attackDist = wep?.attackDistance || 250;
                const multiArrow = wep?.multiArrow || 0;

                // 找范围内目标，发 cast_skill_npc（伤害由后端按 skill.Damage=1.0 计算）
                const allTargets = [
                    ...Array.from(scene.npcEntities.entries()).map(([id, e]) => ({ id, entity: e, isNPC: true })),
                    ...Array.from(scene.remotePlayers.entries()).map(([id, e]) => ({ id, entity: e, isNPC: false }))
                ];
                for (const { id, entity, isNPC } of allTargets) {
                    if (entity.dead) continue;
                    const tt = entity.getComponent('transform');
                    if (!tt) continue;
                    const dx = tt.position.x - cx;
                    const dy = (tt.position.y - cy) * 2;
                    if (Math.sqrt(dx * dx + dy * dy) > attackDist) continue;
                    scene.ws.send(isNPC ? 'cast_skill_npc' : 'cast_skill', {
                        skill_id: skillId, target_id: id,
                        target_x: tt.position.x, target_y: tt.position.y
                    });
                }

                // 箭矢视觉特效（不消耗武器冷却）
                if (mas) {
                    const totalArrows = 1 + multiArrow;
                    const spread = 0.18;
                    for (let i = 0; i < totalArrows; i++) {
                        const arrowDir = dir + (i - (totalArrows - 1) / 2) * spread;
                        mas.sectorSlashEffects.push({
                            type: 'arrow', x: cx, y: cy,
                            dir: arrowDir, renderDir: arrowDir,
                            speed: 420, vy: -30, gravity: 220, friction: 0.998,
                            targetDist: attackDist, traveled: 0,
                            age: 0, maxAge: attackDist / 420 * 2.0 + 0.5,
                            damage: 0, pierce: 0, pierceCount: 0, hitEntities: [],
                            stuck: false, stuckAge: 0, stuckMaxAge: 5,
                            stuckAngle: (Math.random() - 0.5) * 0.3,
                            embedRatio: 0.2 + Math.random() * 0.6
                        });
                    }
                }

                count++;
                if (count < 5) setTimeout(fireWave, 100);
            };
            fireWave();
            return;
        }

        // ── 闪电箭：普攻1次，箭矢带闪电粒子特效 ──
        if (skill && skill.name === '闪电箭') {
            scene._lightningArrowActive = true; // 标记下次箭矢带闪电特效
            scene.skillCooldowns[skillId] = now + skill.cooldown * 1000;
            this.attackAllInRange();
            return;
        }

        // ── 天降箭雨：两阶段释放 ──
        if (skill && skill.name === '天降箭雨') {
            // 第二阶段：已有待释放状态，再次触发 = 释放
            if (scene._arrowRainPending) {
                const pending = scene._arrowRainPending;
                scene._arrowRainPending = null;
                scene.skillRangeIndicator.clear();
                // 检查是否在范围内
                const pdx = pending.x - selfCxCS;
                const pdy2d = (pending.y - selfCyCS) * 2;
                const pdist = Math.sqrt(pdx * pdx + pdy2d * pdy2d);
                if (pdist > (skill.range || 300)) {
                    // 超出范围，取消
                    return;
                }
                scene.ws.send('cast_skill_npc', { skill_id: skillId, target_id: 0, target_x: pending.x, target_y: pending.y });
                scene.skillCooldowns[skillId] = now + skill.cooldown * 1000;
                return;
            }
            // 第一阶段：进入选框模式，不倒计时
            scene._arrowRainPending = { skillId, x: selfCxCS, y: selfCyCS };
            scene._arrowRainSkill = skill;
            // 显示持续虚线选框（duration 设很大，手动清除）
            scene._arrowRainIndicatorActive = true;
            return;
        }

        // circle 类型技能（弓箭手 AOE）：以鼠标世界坐标为目标点
        if (skill && skill.area_type === 'circle' && scene.inputManager && scene.camera) {
            const mouseWorld = scene.inputManager.getMouseWorldPosition(scene.camera);
            if (mouseWorld) {
                targetX = mouseWorld.x;
                targetY = mouseWorld.y;
            }
            // 范围预判：施法者到目标点的距离不能超过技能 range
            const dx = targetX - selfCxCS;
            const dy2d = (targetY - selfCyCS) * 2;
            const dist = Math.sqrt(dx * dx + dy2d * dy2d);
            if (skill.range > 0 && dist > skill.range) {
                if (scene.floatingTextManager) {
                    scene.floatingTextManager.addText(transform.position.x, transform.position.y - 20, '超出技能范围', '#ff6600');
                }
                return;
            }
            // 如果有选中目标，也发送 target_id
            if (scene.selectedTarget) {
                targetId = scene.selectedTarget;
            }
            const isNPC = scene.selectedTarget && this._isNPCTarget(scene.selectedTarget);
            const msgType = isNPC ? 'cast_skill_npc' : 'cast_skill';
            scene.ws.send(msgType, { skill_id: skillId, target_id: targetId, target_x: targetX, target_y: targetY });

            scene.skillCooldowns[skillId] = now + skill.cooldown * 1000;
            const combat = scene.playerEntity.getComponent('combat');
            if (combat) {
                const combatSkillId = `backend_${skillId}`;
                combat.skillCooldowns.set(combatSkillId, performance.now());
                combat.startSkillPipeline({
                    ...skill,
                    phaseDurations: { windup: 100, hit: 50, settle: 50, recovery: 200 }
                });
            }
            return;
        }

        // fan 类型（猛击）：朝鼠标方向的扇形 AOE，不依赖 selectedTarget
        if (skill && skill.area_type === 'fan') {
            const mas = scene.meleeAttackSystem;
            // 用鼠标世界坐标作为方向目标点（与普攻 sectorDirection 一致）
            if (scene.inputManager && scene.camera) {
                const mouseWorld = scene.inputManager.getMouseWorldPosition(scene.camera);
                if (mouseWorld) {
                    targetX = mouseWorld.x;
                    targetY = mouseWorld.y;
                }
            } else if (mas) {
                // fallback：用 sectorDirection 反算目标点
                const dir = mas.sectorDirection;
                const range = skill.range > 0 ? skill.range : (mas.sliceAttackRange || 100);
                targetX = selfCxCS + Math.cos(dir) * range;
                targetY = selfCyCS + Math.sin(dir) * range / 2;
            }
            // fan 是 AOE，发 cast_skill_npc（后端遍历范围内所有 NPC）
            scene.ws.send('cast_skill_npc', { skill_id: skillId, target_id: 0, target_x: targetX, target_y: targetY });
            if (skill) {
                scene.skillCooldowns[skillId] = now + skill.cooldown * 1000;
                const combat = scene.playerEntity.getComponent('combat');
                if (combat) {
                    const combatSkillId = `backend_${skillId}`;
                    combat.skillCooldowns.set(combatSkillId, performance.now());
                    combat.startSkillPipeline({
                        ...skill,
                        phaseDurations: { windup: 100, hit: 50, settle: 50, recovery: 200 }
                    });
                }
            }
            return;
        }

        // ellipse 类型（旋风斩/战吼）：以玩家为中心的 AOE，不依赖 selectedTarget
        if (skill && skill.area_type === 'ellipse') {
            // 统一发 cast_skill_npc，后端战吼会同时处理玩家目标
            scene.ws.send('cast_skill_npc', { skill_id: skillId, target_id: 0, target_x: targetX, target_y: targetY });
            if (skill) {
                scene.skillCooldowns[skillId] = now + skill.cooldown * 1000;
                const combat = scene.playerEntity.getComponent('combat');
                if (combat) {
                    const combatSkillId = `backend_${skillId}`;
                    combat.skillCooldowns.set(combatSkillId, performance.now());
                    combat.startSkillPipeline({
                        ...skill,
                        phaseDurations: { windup: 100, hit: 50, settle: 50, recovery: 200 }
                    });
                }
            }
            return;
        }

        const isNPC = scene.selectedTarget && this._isNPCTarget(scene.selectedTarget);
        if (scene.selectedTarget) {
            const target = this._getTargetEntity(scene.selectedTarget);
            if (target) {
                const tTransform = target.getComponent('transform');
                if (tTransform) {
                    targetX = tTransform.position.x;
                    targetY = tTransform.position.y;
                }
                targetId = scene.selectedTarget;

                // 范围预判（ellipse/single/circle）
                if (skill) {
                    const tSpriteCS = target.getComponent('sprite');
                    const tHCS = tSpriteCS?.height || 64;
                    const tx = targetX;
                    const tyCtr = targetY - tHCS / 10;
                    let inRange = true;

                    if (skill.area_type === 'ellipse') {
                        // 旋风斩/战吼：以玩家为中心的椭圆，不需要选中目标，直接放行
                        inRange = true;
                    } else {
                        // single / circle
                        const combat = scene.playerEntity.getComponent('combat');
                        if (combat && !combat.isInSkillRange(
                            { x: selfCxCS, y: selfCyCS },
                            { x: tx, y: tyCtr },
                            skill
                        )) {
                            inRange = false;
                        }
                    }

                    if (!inRange) {
                        if (scene.floatingTextManager) {
                            scene.floatingTextManager.addText(transform.position.x, transform.position.y - 20, '超出技能范围', '#ff6600');
                        }
                        return;
                    }
                }
            }
        }

        const msgType = isNPC ? 'cast_skill_npc' : 'cast_skill';
        scene.ws.send(msgType, { skill_id: skillId, target_id: targetId, target_x: targetX, target_y: targetY });

        if (skill) {
            scene.skillCooldowns[skillId] = now + skill.cooldown * 1000;
            const combat = scene.playerEntity.getComponent('combat');
            if (combat) {
                const combatSkillId = `backend_${skillId}`;
                combat.skillCooldowns.set(combatSkillId, performance.now());
                combat.startSkillPipeline({
                    ...skill,
                    phaseDurations: { windup: 100, hit: 50, settle: 50, recovery: 200 }
                });
            }
        }
    }

    // ─── 点击选敌 ───
    handleEnemySelection() {
        const scene = this.scene;
        if (!scene.inputManager || !scene.playerEntity) return;
        if (scene.playerEntity.dead) return;
        if (!scene.inputManager.isMouseClicked() || scene.inputManager.getMouseButton() !== 0 || scene.inputManager.isMouseClickHandled()) return;

        // 左键点击 = 群攻范围内所有敌人（与空格键效果一致）
        scene.inputManager.markMouseClickHandled();
        this.attackAllInRange();
    }


    // ─── 处理伤害消息 ───
    onDamage(data) {
        const scene = this.scene;

        // 如果攻击者是 NPC，但该 NPC 已经死亡/不存在，忽略此伤害
        // （时序竞态：npcAITick 锁内收集攻击列表，解锁后广播；此时 NPC 可能已被玩家击杀）
        if (data.attacker_is_npc) {
            const attackerNPC = scene.npcEntities.get(data.attacker_id);
            if (!attackerNPC || attackerNPC.dead || attackerNPC.isDead) {
                return;
            }
            // NPC 攻击特效：在目标位置触发打击粒子
            const targetForFx = data.target_id === scene.selfId
                ? scene.playerEntity
                : scene.remotePlayers.get(data.target_id);
            if (targetForFx && scene.particleSystem) {
                const t = targetForFx.getComponent('transform');
                if (t) {
                    SkillParticleEffects.emitNPCHit(scene.particleSystem, t.position.x, t.position.y - 20, data.is_crit);
                }
            }
            // NPC 武器攻击动画（已禁用 EnemyWeaponRenderer，改用刀光特效）
            // if (scene.enemyWeaponRenderer && targetForFx) {
            //     const t = targetForFx.getComponent('transform');
            //     if (t) scene.enemyWeaponRenderer.startAttack(attackerNPC, t.position);
            // }
            // NPC 刀光特效：复用玩家近战光刃
            if (scene.meleeAttackSystem && targetForFx) {
                const npcT = attackerNPC.getComponent('transform');
                const tgtT = targetForFx.getComponent('transform');
                if (npcT && tgtT) {
                    const dx = tgtT.position.x - npcT.position.x;
                    const dy = tgtT.position.y - npcT.position.y;
                    const dir = Math.atan2(dy, dx);
                    const dist = Math.sqrt(dx * dx + dy * dy);
                    const radius = Math.min(dist * 0.7, 80);
                    scene.meleeAttackSystem.sectorSlashEffects.push({
                        cx: npcT.position.x,
                        cy: npcT.position.y,
                        radius,
                        dir,
                        halfAngle: Math.PI / 4,
                        age: 0,
                        maxAge: 0.22,
                        type: 'slash',
                        damage: 0,
                        hitEntities: [],
                        isNPC: true   // 标记为 NPC 刀光，颜色略有区别
                    });
                }
            }
        }
        let targetEntity;
        if (data.target_is_npc) {
            targetEntity = scene.npcEntities.get(data.target_id);
        } else {
            targetEntity = data.target_id === scene.selfId
                ? scene.playerEntity
                : scene.remotePlayers.get(data.target_id);
        }

        if (targetEntity) {
            const stats = targetEntity.getComponent('stats');
            if (stats) {
                stats.hp = data.target_hp;
                stats.maxHp = data.target_max_hp;
                if (data.target_is_npc && stats.hp <= 0) {
                    targetEntity.dead = true;
                    targetEntity.isDead = true;
                    targetEntity.isDying = true;
                }
            }

            // 浮动伤害文字
            const transform = targetEntity.getComponent('transform');
            if (transform && scene.floatingTextManager) {
                let color = '#ff0000';
                let text = `${Math.round(data.damage)}`;

                if (data.is_crit) {
                    color = '#ffd700';
                    text = `暴击! ${Math.round(data.damage)}`;
                }
                if (data.skill_name) {
                    color = data.is_crit ? '#ff8c00' : '#ff4500';
                    text = data.is_crit
                        ? `${data.skill_name} 暴击! ${Math.round(data.damage)}`
                        : `${data.skill_name} ${Math.round(data.damage)}`;
                }

                scene.floatingTextManager.addText(transform.position.x, transform.position.y - 20, text, color);
            }

            // 旋风斩：命中时同步触发粒子，并重置定时器（同一tick多目标只触发一次）
            if (data.skill_name === '旋风斩' && data.attacker_id === scene.selfId) {
                const now = Date.now();
                if (!scene._whirlwindLastParticleTime || now - scene._whirlwindLastParticleTime > 500) {
                    scene._whirlwindLastParticleTime = now;
                    scene._whirlwindNextParticle = now + 1000; // 重置定时器，避免1秒内重复
                    const t = scene.playerEntity?.getComponent('transform');
                    if (t) {
                        SkillParticleEffects.emitWhirlwind(scene.particleSystem, t.position.x, t.position.y, scene._whirlwindAreaSize || 80, scene.meleeAttackSystem);
                    }
                }
            }

            // 弓箭手普攻命中：创建预附着箭矢（隐藏），飞行箭矢靠近时再显示
            if (!data.skill_name && data.attacker_id === scene.selfId && transform) {
                const mas = scene.meleeAttackSystem;
                if (mas && mas.sectorIsRanged) {
                    // 找最近的飞行中箭矢（未附着、未插地、无预附着）
                    let flyingArrow = null, closestDist = Infinity;
                    for (const e of mas.sectorSlashEffects) {
                        if (e.type !== 'arrow' || e.stuck || e.attached || e.hidden || e.pendingAttachEntity) continue;
                        const dx = e.x - transform.position.x;
                        const dy = e.y - transform.position.y;
                        const d = Math.sqrt(dx * dx + dy * dy);
                        if (d < closestDist) { closestDist = d; flyingArrow = e; }
                    }
                    if (flyingArrow) {
                        // 创建预附着箭矢：位置在目标身体上，初始隐藏
                        const attachOffsetX = (Math.random() - 0.5) * 8;
                        const attachOffsetY = -16 + (Math.random() - 0.5) * 10;
                        const attachArrow = {
                            type: 'arrow',
                            x: transform.position.x + attachOffsetX,
                            y: transform.position.y + attachOffsetY,
                            dir: flyingArrow.dir,
                            renderDir: flyingArrow.renderDir ?? flyingArrow.dir,
                            speed: 0, vy: 0, gravity: 0, friction: 1,
                            traveled: 0, targetDist: 1,
                            age: 0, maxAge: 999,
                            damage: 0, pierce: 0, pierceCount: 0, hitEntities: [],
                            attached: true,
                            attachedEntity: targetEntity,
                            attachOffsetX,
                            attachOffsetY,
                            hidden: true,  // 等待飞行箭矢靠近后显示
                            stuck: false, stuckAge: 0, stuckMaxAge: 5,
                            stuckAngle: (Math.random() - 0.5) * 0.3,
                            embedRatio: 0.2 + Math.random() * 0.6
                        };
                        mas.sectorSlashEffects.push(attachArrow);
                        // 飞行箭矢关联预附着箭矢
                        flyingArrow.pendingAttachEntity = targetEntity;
                        flyingArrow.pendingAttachArrow = attachArrow;
                    }
                }
            }
        }
    }

    // ─── 处理技能施放反馈 ───
    onSkillCasted(data) {
        const scene = this.scene;

        // 战吼恐惧效果：给目标玩家设置恐惧状态（自动远离施法者逃跑）
        if (data.skill_name === '战吼_fear') {
            const targetEntity = data.target_id === scene.selfId
                ? scene.playerEntity
                : scene.remotePlayers.get(data.target_id);
            if (targetEntity) {
                targetEntity.fearUntil = Date.now() + 3000;
                targetEntity.fearDirX = data.fear_dir_x || 0;
                targetEntity.fearDirY = data.fear_dir_y || 0;
                const transform = targetEntity.getComponent('transform');
                if (transform && scene.floatingTextManager) {
                    scene.floatingTextManager.addText(transform.position.x, transform.position.y - 30, '恐惧!', '#ff4444');
                }
            }
            return;
        }

        if (data.caster_id === scene.selfId && scene.playerEntity) {
            const stats = scene.playerEntity.getComponent('stats');
            if (stats) {
                stats.mp = data.caster_mp;
                stats.maxMp = data.caster_max_mp;
            }
        }

        const caster = data.caster_id === scene.selfId
            ? scene.playerEntity
            : scene.remotePlayers.get(data.caster_id);
        if (caster) {
            const transform = caster.getComponent('transform');
            if (transform && scene.floatingTextManager) {
                scene.floatingTextManager.addText(transform.position.x, transform.position.y - 10, data.skill_name, '#ffab40');
            }
            if (scene.particleSystem && transform) {
                scene.createSkillParticles(data.skill_name, {
                    casterX: transform.position.x,
                    casterY: transform.position.y,
                    targetX: data.target_x,
                    targetY: data.target_y,
                    areaSize: data.area_size || 0,
                    isSelf: data.caster_id === scene.selfId
                });
            }

            // 战士技能范围指示器（只给自己的技能显示）
            if (data.caster_id === scene.selfId && transform && scene._showSkillRange) {
                const footCenter = scene._getFootCenter && scene._getFootCenter();
                if (footCenter) {
                    const areaType = data.area_type;
                    const equipment = scene.playerEntity.getComponent('equipment');
                    const weapon = equipment ? equipment.getEquipment('mainhand') : null;
                    const weaponDist = weapon ? (weapon.attackDistance || 100) : 100;

                    if (areaType === 'fan') {
                        // 猛击：扇形
                        const range = data.area_size || weaponDist;
                        const dir = Math.atan2((data.target_y || transform.position.y) - transform.position.y,
                                               (data.target_x || transform.position.x) - transform.position.x);
                        scene._showSkillRange({
                            areaType: 'fan', x: footCenter.x, y: footCenter.y,
                            rx: range, ry: range / 2, direction: dir,
                            halfAngle: Math.PI / 4,
                            color: 'rgba(255, 100, 30, 0.85)', fillColor: 'rgba(255, 100, 30, 0.12)',
                            duration: 1.0
                        });
                    } else if (areaType === 'ellipse') {
                        // 旋风斩 / 战吼：椭圆
                        const radius = data.area_size || weaponDist;
                        const isWarcry = data.skill_name === '战吼';
                        scene._showSkillRange({
                            areaType: 'ellipse', x: footCenter.x, y: footCenter.y,
                            rx: radius, ry: radius / 2,
                            color: isWarcry ? 'rgba(255, 60, 60, 0.85)' : 'rgba(100, 200, 255, 0.85)',
                            fillColor: isWarcry ? 'rgba(255, 60, 60, 0.12)' : 'rgba(100, 200, 255, 0.10)',
                            duration: isWarcry ? 1.0 : 5.0
                        });
                    } else if (areaType === 'circle') {
                        // 弓箭手技能：以鼠标点击位置为中心，武器攻击距离 1/2 为半径的椭圆
                        const equipment = scene.playerEntity.getComponent('equipment');
                        const weapon = equipment ? equipment.getEquipment('mainhand') : null;
                        const weaponDist = weapon ? (weapon.attackDistance || 250) : 250;
                        const radius = weaponDist / 2;
                        let circleColor, circleFill;
                        if (data.skill_name === '闪电箭') {
                            circleColor = 'rgba(68, 170, 255, 0.85)';
                            circleFill = 'rgba(68, 170, 255, 0.15)';
                        } else if (data.skill_name === '天降箭雨') {
                            circleColor = 'rgba(255, 140, 50, 0.85)';
                            circleFill = 'rgba(255, 140, 50, 0.15)';
                        } else {
                            circleColor = 'rgba(255, 200, 50, 0.85)';
                            circleFill = 'rgba(255, 200, 50, 0.12)';
                        }
                        scene._showSkillRange({
                            areaType: 'circle',
                            targetX: data.target_x, targetY: data.target_y,
                            rx: radius, ry: radius / 2,
                            color: circleColor, fillColor: circleFill,
                            duration: 1.2
                        });

                        // 天降箭雨：启动持续落箭（每1秒一批，持续5秒）
                        if (data.skill_name === '天降箭雨' && scene.meleeAttackSystem) {
                            const rainX = data.target_x;
                            const rainY = data.target_y;
                            // 分布范围用武器攻击距离
                            const equipment2 = scene.playerEntity.getComponent('equipment');
                            const weapon2 = equipment2 ? equipment2.getEquipment('mainhand') : null;
                            const weaponDist = weapon2?.attackDistance || 250;
                            const rainRadius = weaponDist / 2;
                            const multiArrow = (weapon2?.multiArrow || 0) + 1; // 普攻箭矢数
                            const rainCount = multiArrow * 5; // 5倍
                            let ticks = 0;
                            const maxTicks = 5; // 5秒 / 1秒 = 5次
                            const spawnBatch = () => {
                                if (!scene.meleeAttackSystem || ticks >= maxTicks) return;
                                ticks++;
                                for (let i = 0; i < rainCount; i++) {
                                    // 随机落点在椭圆范围内（等距视角 Y 轴压缩）
                                    const a = Math.random() * Math.PI * 2;
                                    const r = Math.sqrt(Math.random());
                                    const tx = rainX + Math.cos(a) * rainRadius * r;
                                    const ty = rainY + Math.sin(a) * (rainRadius * 0.5) * r;
                                    const startHeight = 180 + Math.random() * 80;
                                    const delay = Math.random() * 800;
                                    setTimeout(() => {
                                        if (!scene.meleeAttackSystem) return;
                                        scene.meleeAttackSystem.sectorSlashEffects.push({
                                            type: 'arrow',
                                            x: tx + (Math.random() - 0.5) * 20,
                                            y: ty - startHeight,
                                            dir: Math.PI / 2,
                                            renderDir: Math.PI / 2,
                                            speed: 0,
                                            vy: 380 + Math.random() * 80,
                                            gravity: 60,
                                            friction: 1,
                                            targetDist: startHeight + 20,
                                            traveled: 0,
                                            age: 0,
                                            maxAge: (startHeight + 20) / 400 + 0.3,
                                            damage: 0, pierce: 0, pierceCount: 0, hitEntities: [],
                                            stuck: false, stuckAge: 0, stuckMaxAge: 4,
                                            stuckAngle: (Math.random() - 0.5) * 0.25,
                                            embedRatio: 0.3 + Math.random() * 0.4,
                                            isRainArrow: true,
                                            groundY: ty
                                        });
                                    }, delay);
                                }
                                if (ticks < maxTicks) setTimeout(spawnBatch, 1000);
                            };
                            spawnBatch();
                        }
                    }
                }
            }
        }
    }

    // ─── 群攻：攻击范围内所有敌人 ───
    attackAllInRange() {
        const scene = this.scene;
        if (!scene.ws || !scene.playerEntity) return;
        if (scene.playerEntity.dead) return;
        // 恐惧检查
        if (scene.playerEntity.fearUntil && Date.now() < scene.playerEntity.fearUntil) return;

        const selfTransform = scene.playerEntity.getComponent('transform');
        if (!selfTransform) return;

        // 安全区检查
        if (scene.isInSafeZone(selfTransform.position.x, selfTransform.position.y)) {
            if (scene.floatingTextManager) {
                scene.floatingTextManager.addText(selfTransform.position.x, selfTransform.position.y - 20, '安全区内禁止攻击', '#ffaa00');
            }
            return;
        }

        const maxRange = this._getWeaponRange();
        const combat = scene.playerEntity.getComponent('combat');

        // 武器冷却检查（与鼠标攻击共享同一冷却状态）
        const mas = scene.meleeAttackSystem;
        if (mas) {
            const currentTime = performance.now() / 1000;
            let weaponCooldown = mas.sliceGlobalCooldown;
            const equipComp = scene.playerEntity.getComponent('equipment');
            if (equipComp) {
                const mainhand = equipComp.getEquipment('mainhand');
                const offhand = equipComp.getEquipment('offhand');
                if (mainhand && mainhand.attackSpeed != null) {
                    weaponCooldown = mainhand.attackSpeed;
                } else if (offhand && offhand.attackSpeed != null) {
                    weaponCooldown = offhand.attackSpeed;
                }
            }
            const timeSinceLastAttack = currentTime - mas.sliceLastAttackTime;
            if (timeSinceLastAttack < weaponCooldown) {
                return;
            }
        }
        let attacked = false;
        let firstTargetTransform = null;
        let pendingRangedTargets = null;

        // 获取攻击参数
        const sectorDir = mas ? mas.sectorDirection : 0;
        let sectorHalfAngle = mas ? mas.sectorAngle / 2 : (Math.PI / 6);
        let sectorRadius = maxRange;
        const isRanged = mas ? mas.sectorIsRanged : false;

        // 远程攻击消耗箭矢（从背包扣，副手只是类型标记）
        if (isRanged) {
            const equipComp = scene.playerEntity.getComponent('equipment');
            const inventory = scene.playerEntity.getComponent('inventory');
            if (equipComp) {
                const offhand = equipComp.getEquipment('offhand');
                const ammoId = offhand?.subType === 'ammo' ? offhand.id : null;
                // 检查背包中是否有箭矢
                const hasAmmo = ammoId && inventory &&
                    inventory.getAllItems().some(({ slot }) =>
                        slot.item && slot.item.id === ammoId && slot.quantity > 0
                    );
                if (!hasAmmo) {
                    // 尝试找背包中任意箭矢，自动切换副手类型
                    let refilled = false;
                    if (inventory) {
                        for (const { slot } of inventory.getAllItems()) {
                            if (slot.item && (slot.item.subType === 'ammo' || slot.item.type === 'ammo')) {
                                equipComp.equip('offhand', { ...slot.item, quantity: null });
                                refilled = true;
                                break;
                            }
                        }
                    }
                    if (!refilled) {
                        if (scene.floatingTextManager) {
                            scene.floatingTextManager.addText(
                                selfTransform.position.x,
                                selfTransform.position.y - 70,
                                '没有箭矢！', '#ff6666'
                            );
                        }
                        return;
                    }
                }
                // 从背包消耗1支箭矢
                const currentOffhand = equipComp.getEquipment('offhand');
                if (currentOffhand?.id && inventory) {
                    inventory.removeItem(currentOffhand.id, 1);
                    // 若背包中该类箭矢耗尽，清除副手类型标记
                    const remaining = inventory.getItemCount(currentOffhand.id);
                    if (remaining <= 0) {
                        equipComp.unequip('offhand');
                    }
                }
            }
        }

        if (mas) {
            const equipComp2 = scene.playerEntity.getComponent('equipment');
            if (equipComp2) {
                const mainhand = equipComp2.getEquipment('mainhand');
                if (mainhand) {
                    if (!isRanged && mainhand.attackRange != null) sectorHalfAngle = (mainhand.attackRange * Math.PI / 180) / 2;
                    if (mainhand.attackDistance != null && mainhand.attackDistance > 0) sectorRadius = mainhand.attackDistance;
                }
            }
            // fallback：仅在 sectorRadius 未从装备读到时才用 sliceAttackRange
            if (sectorRadius <= 0) sectorRadius = mas.sliceAttackRange;
        }

        // 攻击圆心：脚下 1/10 高度
        const selfSprite = scene.playerEntity.getComponent('sprite');
        const selfH = selfSprite?.height || 64;
        const selfCx = selfTransform.position.x;
        const selfCy = selfTransform.position.y - selfH / 10;

        // 收集范围内所有存活目标
        const allTargets = [
            ...Array.from(scene.npcEntities.entries()).map(([id, e]) => ({ id, entity: e, isNPC: true })),
            ...Array.from(scene.remotePlayers.entries()).map(([id, e]) => ({ id, entity: e, isNPC: false }))
        ];

        for (const { id, entity, isNPC } of allTargets) {
            if (entity.dead) continue;
            const targetTransform = entity.getComponent('transform');
            if (!targetTransform) continue;
            const tSprite = entity.getComponent('sprite');
            const tH = tSprite?.height || 64;

            const dx = targetTransform.position.x - selfCx;
            const dy = (targetTransform.position.y - tH / 10) - selfCy;
            const dy2d = dy * 2;
            const dist2d = Math.sqrt(dx * dx + dy2d * dy2d);
            if (dist2d > sectorRadius) continue;

            if (!isRanged) {
                // 近战：扇形角度判定
                const angle = Math.atan2(dy2d, dx);
                let angleDiff = angle - sectorDir;
                while (angleDiff > Math.PI) angleDiff -= Math.PI * 2;
                while (angleDiff < -Math.PI) angleDiff += Math.PI * 2;
                if (Math.abs(angleDiff) > sectorHalfAngle) continue;

                // 近战：立即发送攻击消息
                const msgType = isNPC ? 'attack_npc' : 'attack';
                scene.ws.send(msgType, { target_id: id });
            } else {
                // 远程：扇形角度判定（以鼠标方向为中心）
                const mouseDir = isRanged && scene.inputManager && scene.camera
                    ? (() => { const m = scene.inputManager.getMouseWorldPosition(scene.camera); return m ? Math.atan2((m.y - selfCy) * 2, m.x - selfCx) : sectorDir; })()
                    : sectorDir;
                const angle = Math.atan2(dy2d, dx);
                let angleDiff = angle - mouseDir;
                while (angleDiff > Math.PI) angleDiff -= Math.PI * 2;
                while (angleDiff < -Math.PI) angleDiff += Math.PI * 2;
                // 弓箭手扇形半角：约30°
                const archerHalfAngle = Math.PI / 6;
                if (Math.abs(angleDiff) > archerHalfAngle) continue;

                // 远程：不立即发消息，记录到 pendingRangedTargets，等箭矢碰撞时发送
                if (!pendingRangedTargets) pendingRangedTargets = [];
                pendingRangedTargets.push({ id, entity, isNPC, transform: targetTransform });
            }

            if (!attacked) {
                firstTargetTransform = targetTransform;
                scene.selectedTarget = id;
            }
            attacked = true;
        }

        // 弓箭手：pendingRangedTargets 有目标时也算 attacked
        if (isRanged && pendingRangedTargets && pendingRangedTargets.length > 0 && !attacked) {
            attacked = true;
        }

        // 触发弯刀攻击动画 + 刀光/箭光特效
        // 更新武器冷却时间（与鼠标攻击共享）
        if (mas) {
            mas.sliceLastAttackTime = performance.now() / 1000;
            mas.sliceCooldownShown = false;
        }
        if (scene.weaponRenderer) {
            if (!isRanged && attacked && firstTargetTransform) {
                // 近战有命中目标：朝第一个目标方向
                const fSprite = this._getTargetEntity(scene.selectedTarget)?.getComponent('sprite');
                const fH = fSprite?.height || 64;
                const dx = firstTargetTransform.position.x - selfCx;
                const dy = (firstTargetTransform.position.y - fH / 10) - selfCy;
                scene.weaponRenderer.currentMouseAngle = Math.atan2(dy, dx);
            } else if (isRanged && scene.inputManager && scene.camera) {
                // 弓箭手：始终朝鼠标实时位置
                const mouseWorld = scene.inputManager.getMouseWorldPosition(scene.camera);
                if (mouseWorld) {
                    scene.weaponRenderer.currentMouseAngle = Math.atan2(mouseWorld.y - selfCy, mouseWorld.x - selfCx);
                }
            }
            scene.weaponRenderer.startAttack('thrust');
        }

        // 触发刀光/箭光特效（复用 MeleeAttackSystem 的扇形特效）
        if (mas && scene.playerEntity) {
            const playerCenter = {
                x: selfCx,
                y: selfCy
            };
            // 弓箭手：始终朝鼠标实时位置方向；近战：有目标时朝目标，无目标时朝鼠标
            let dir = sectorDir;
            if (isRanged && scene.inputManager && scene.camera) {
                const mouseWorld = scene.inputManager.getMouseWorldPosition(scene.camera);
                if (mouseWorld) {
                    dir = Math.atan2(mouseWorld.y - selfCy, mouseWorld.x - selfCx);
                    mas.sectorDirection = dir;
                }
            } else if (!isRanged && attacked && firstTargetTransform) {
                const fSprite2 = this._getTargetEntity(scene.selectedTarget)?.getComponent('sprite');
                const fH2 = fSprite2?.height || 64;
                const dx = firstTargetTransform.position.x - selfCx;
                const dy = (firstTargetTransform.position.y - fH2 / 10) - selfCy;
                dir = Math.atan2(dy, dx);
                mas.sectorDirection = dir;
            }
            const prevCount = mas.sectorSlashEffects.length;
            mas.spawnSectorSlashEffect(
                playerCenter, dir,
                playerCenter.x, playerCenter.y,
                sectorRadius
            );
            // 弓箭手：把待命中目标列表挂到新生成的每支箭矢上
            if (isRanged && pendingRangedTargets && pendingRangedTargets.length > 0) {
                for (let ai = prevCount; ai < mas.sectorSlashEffects.length; ai++) {
                    const arrow = mas.sectorSlashEffects[ai];
                    if (arrow.type === 'arrow') {
                        arrow.rangedTargets = pendingRangedTargets.slice();
                        arrow.onHitCallback = (targetId, isNPC) => {
                            const msgType = isNPC ? 'attack_npc' : 'attack';
                            scene.ws.send(msgType, { target_id: targetId });
                        };
                        // 闪电箭：标记箭矢带闪电特效
                        if (scene._lightningArrowActive) {
                            arrow.isLightning = true;
                        }
                    }
                }
                if (scene._lightningArrowActive) scene._lightningArrowActive = false;
            }
        }

        if (!attacked) { /* 范围内无目标 */ }
    }

    // ─── 空格键攻击（在 update 中调用） ───
    handleSpaceAttack() {
        const scene = this.scene;
        if (!scene.inputManager || !scene.inputManager.isKeyPressed('space')) return;
        this.attackAllInRange();
    }
}
