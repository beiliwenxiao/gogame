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
        const isNPC = targetId < 0;
        return isNPC
            ? this.scene.npcEntities.get(targetId)
            : this.scene.remotePlayers.get(targetId);
    }

    _isNPCTarget(targetId) {
        return targetId < 0;
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

        const maxRange = this._getWeaponRange();
        let closestId = null;
        let closestDist = maxRange;

        // 检查 NPC
        for (const [id, entity] of scene.npcEntities) {
            if (entity.dead) continue;
            const t = entity.getComponent('transform');
            if (!t) continue;
            const dx = t.position.x - selfT.position.x;
            const dy = t.position.y - selfT.position.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist < closestDist) { closestDist = dist; closestId = id; }
        }

        // 检查远程玩家
        for (const [id, entity] of scene.remotePlayers) {
            if (entity.dead) continue;
            const t = entity.getComponent('transform');
            if (!t) continue;
            const dx = t.position.x - selfT.position.x;
            const dy = t.position.y - selfT.position.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
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
            console.log('NetworkCombatSystem.attackTarget: 提前返回，selectedTarget=', scene.selectedTarget, 'ws=', !!scene.ws);
            return;
        }
        if (scene.playerEntity && scene.playerEntity.dead) {
            console.log('NetworkCombatSystem.attackTarget: 灵魂状态，禁止攻击');
            return;
        }
        // 昏迷检查
        if (scene.playerEntity && scene.playerEntity.stunUntil && Date.now() < scene.playerEntity.stunUntil) {
            return;
        }

        const isNPC = this._isNPCTarget(scene.selectedTarget);
        const entity = this._getTargetEntity(scene.selectedTarget);
        if (!entity || entity.dead) {
            console.log('NetworkCombatSystem.attackTarget: 目标不存在或已死亡，isNPC=', isNPC, 'entity=', !!entity, 'dead=', entity?.dead);
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
                console.log('NetworkCombatSystem.attackTarget: 范围预判，maxRange=', maxRange, 'dist=', dist.toFixed(1));

                if (!combat.isInSkillRange(selfTransform.position, targetTransform.position, { range: maxRange, area_type: 'single' })) {
                    console.log('NetworkCombatSystem.attackTarget: 超出攻击范围');
                    if (scene.floatingTextManager) {
                        scene.floatingTextManager.addText(selfTransform.position.x, selfTransform.position.y - 20, '超出攻击范围', '#ff6600');
                    }
                    return;
                }
            }
        }

        const msgType = isNPC ? 'attack_npc' : 'attack';
        console.log('NetworkCombatSystem.attackTarget: 发送攻击消息，msgType=', msgType, 'target_id=', scene.selectedTarget);
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
            console.log('NetworkCombatSystem.attackTarget: 触发弯刀攻击动画');
        }
    }

    // ─── 施放技能 ───
    castSkill(skillId) {
        const scene = this.scene;
        console.log('[NetworkCombat] castSkill called, skillId=', skillId, 'ws=', !!scene.ws, 'playerEntity=', !!scene.playerEntity, 'dead=', scene.playerEntity?.dead);
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

        // circle 类型技能（弓箭手 AOE）：以鼠标世界坐标为目标点
        if (skill && skill.area_type === 'circle' && scene.inputManager && scene.camera) {
            const mouseWorld = scene.inputManager.getMouseWorldPosition(scene.camera);
            if (mouseWorld) {
                targetX = mouseWorld.x;
                targetY = mouseWorld.y;
            }
            // 范围预判：施法者到目标点的距离不能超过技能 range
            const dx = targetX - transform.position.x;
            const dy2d = (targetY - transform.position.y) * 2;
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
            const isNPC = scene.selectedTarget && scene.selectedTarget < 0;
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

        const isNPC = scene.selectedTarget && scene.selectedTarget < 0;
        if (scene.selectedTarget) {
            const target = this._getTargetEntity(scene.selectedTarget);
            if (target) {
                const tTransform = target.getComponent('transform');
                if (tTransform) {
                    targetX = tTransform.position.x;
                    targetY = tTransform.position.y;
                }
                targetId = scene.selectedTarget;

                // 范围预判（按技能类型）
                if (skill) {
                    const sx = transform.position.x;
                    const sy = transform.position.y;
                    const tx = targetX;
                    const ty = targetY;
                    let inRange = true;

                    if (skill.area_type === 'fan') {
                        // 猛击：扇形判定（2.5D，半角 45°）
                        const mas = scene.meleeAttackSystem;
                        const range = skill.range > 0 ? skill.range : (mas ? mas.sliceAttackRange : 100);
                        const dx = tx - sx;
                        const dy2d = (ty - sy) * 2;
                        const dist = Math.sqrt(dx * dx + dy2d * dy2d);
                        if (dist > range) inRange = false;
                        // 扇形角度判定：朝目标方向，半角 45°
                        // 前端发送时 target 就是选中目标，方向即为目标方向，直接用距离判定即可
                    } else if (skill.area_type === 'ellipse') {
                        // 旋风斩/战吼：以玩家为中心的椭圆，不需要选中目标，直接放行
                        inRange = true;
                    } else {
                        // single / circle：原有逻辑
                        const combat = scene.playerEntity.getComponent('combat');
                        if (combat && !combat.isInSkillRange(transform.position, { x: targetX, y: targetY }, skill)) {
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
        console.log('NetworkCombatSystem.onDamage: 收到伤害数据', data);

        // 如果攻击者是 NPC，但该 NPC 已经死亡/不存在，忽略此伤害
        // （时序竞态：npcAITick 锁内收集攻击列表，解锁后广播；此时 NPC 可能已被玩家击杀）
        if (data.attacker_is_npc) {
            const attackerNPC = scene.npcEntities.get(data.attacker_id);
            if (!attackerNPC || attackerNPC.dead || attackerNPC.isDead) {
                console.log('NetworkCombatSystem.onDamage: 攻击者 NPC 已死亡，忽略伤害，attacker_id=', data.attacker_id);
                return;
            }
        }

        let targetEntity;
        if (data.target_is_npc) {
            targetEntity = scene.npcEntities.get(data.target_id);
            console.log('NetworkCombatSystem.onDamage: NPC目标，id=', data.target_id, 'entity=', !!targetEntity);
        } else {
            targetEntity = data.target_id === scene.selfId
                ? scene.playerEntity
                : scene.remotePlayers.get(data.target_id);
        }

        if (targetEntity) {
            const stats = targetEntity.getComponent('stats');
            if (stats) {
                console.log('NetworkCombatSystem.onDamage: 更新HP，旧HP=', stats.hp, '新HP=', data.target_hp);
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
                    areaSize: data.area_size || 0
                });
            }

            // 战士技能范围指示器（只给自己的技能显示）
            console.log('[SkillRange] onSkillCasted check:', 'caster=', data.caster_id, 'self=', scene.selfId, 'area_type=', data.area_type, 'area_size=', data.area_size, '_showSkillRange=', !!scene._showSkillRange);
            if (data.caster_id === scene.selfId && transform && scene._showSkillRange) {
                const footCenter = scene._getFootCenter && scene._getFootCenter();
                console.log('[SkillRange] footCenter=', footCenter, 'areaType=', data.area_type);
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
                            duration: 1.0
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
                    }
                }
            }
        }
    }

    // ─── 群攻：攻击范围内所有敌人 ───
    attackAllInRange() {
        const scene = this.scene;
        console.log('NetworkCombatSystem.attackAllInRange: 进入, ws=', !!scene.ws, 'playerEntity=', !!scene.playerEntity, 'dead=', scene.playerEntity?.dead);
        if (!scene.ws || !scene.playerEntity) return;
        if (scene.playerEntity.dead) return;
        // 恐惧检查
        if (scene.playerEntity.fearUntil && Date.now() < scene.playerEntity.fearUntil) return;

        const selfTransform = scene.playerEntity.getComponent('transform');
        if (!selfTransform) return;

        // 安全区检查
        if (scene.isInSafeZone(selfTransform.position.x, selfTransform.position.y)) {
            console.log('NetworkCombatSystem.attackAllInRange: 安全区内，return');
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

        // 获取攻击参数
        const sectorDir = mas ? mas.sectorDirection : 0;
        let sectorHalfAngle = mas ? mas.sectorAngle / 2 : (Math.PI / 6);
        let sectorRadius = maxRange;
        const isRanged = mas ? mas.sectorIsRanged : false;

        // 远程攻击消耗箭矢
        if (isRanged) {
            const equipComp = scene.playerEntity.getComponent('equipment');
            if (equipComp) {
                const offhand = equipComp.getEquipment('offhand');
                if (!offhand || offhand.subType !== 'ammo' || !offhand.quantity || offhand.quantity <= 0) {
                    // 尝试从背包自动补充箭矢
                    const inventory = scene.playerEntity.getComponent('inventory');
                    let refilled = false;
                    if (inventory) {
                        for (const { slot, index } of inventory.getAllItems()) {
                            if (slot.item && (slot.item.subType === 'ammo' || slot.item.type === 'ammo')) {
                                const newAmmo = { ...slot.item, quantity: slot.quantity };
                                inventory.removeFromSlot(index, slot.quantity);
                                equipComp.equip('offhand', newAmmo);
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
                                '没有箭矢！',
                                '#ff6666'
                            );
                        }
                        return;
                    }
                }
                // 消耗1支箭矢
                const currentOffhand = equipComp.getEquipment('offhand');
                if (currentOffhand && currentOffhand.quantity > 0) {
                    currentOffhand.quantity -= 1;
                    if (currentOffhand.quantity <= 0) {
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
                    // 近战：attackRange 是角度（度数），转半角弧度
                    if (!isRanged && mainhand.attackRange != null) sectorHalfAngle = (mainhand.attackRange * Math.PI / 180) / 2;
                    if (mainhand.attackDistance != null) sectorRadius = mainhand.attackDistance;
                }
            }
            if (sectorRadius === maxRange) sectorRadius = mas.sliceAttackRange;
        }

        // 收集范围内所有存活目标
        const allTargets = [
            ...Array.from(scene.npcEntities.entries()).map(([id, e]) => ({ id, entity: e, isNPC: true })),
            ...Array.from(scene.remotePlayers.entries()).map(([id, e]) => ({ id, entity: e, isNPC: false }))
        ];

        for (const { id, entity, isNPC } of allTargets) {
            if (entity.dead) continue;
            const targetTransform = entity.getComponent('transform');
            if (!targetTransform) continue;

            const dx = targetTransform.position.x - selfTransform.position.x;
            const dy = targetTransform.position.y - selfTransform.position.y;
            const dy2d = dy * 2; // 还原 2.5D Y 轴压缩
            const dist2d = Math.sqrt(dx * dx + dy2d * dy2d);
            if (dist2d > sectorRadius) continue;

            if (!isRanged) {
                // 近战：扇形角度判定
                const angle = Math.atan2(dy2d, dx);
                let angleDiff = angle - sectorDir;
                while (angleDiff > Math.PI) angleDiff -= Math.PI * 2;
                while (angleDiff < -Math.PI) angleDiff += Math.PI * 2;
                if (Math.abs(angleDiff) > sectorHalfAngle) continue;
            }
            // 远程：只做距离判定，全方向都能攻击到

            const msgType = isNPC ? 'attack_npc' : 'attack';
            scene.ws.send(msgType, { target_id: id });

            if (!attacked) {
                firstTargetTransform = targetTransform;
                scene.selectedTarget = id;
            }
            attacked = true;
        }

        // 触发弯刀攻击动画 + 刀光/箭光特效
        // 更新武器冷却时间（与鼠标攻击共享）
        if (mas) {
            mas.sliceLastAttackTime = performance.now() / 1000;
            mas.sliceCooldownShown = false;
        }
        if (scene.weaponRenderer) {
            if (attacked && firstTargetTransform) {
                // 有命中目标：朝第一个目标方向
                const dx = firstTargetTransform.position.x - selfTransform.position.x;
                const dy = firstTargetTransform.position.y - selfTransform.position.y;
                scene.weaponRenderer.currentMouseAngle = Math.atan2(dy, dx);
            }
            // 无论是否命中目标都触发弯刀特效（无目标时朝当前鼠标方向）
            scene.weaponRenderer.startAttack('thrust');
        }

        // 触发刀光/箭光特效（复用 MeleeAttackSystem 的扇形特效）
        if (mas && scene.playerEntity) {
            const sprite = scene.playerEntity.getComponent('sprite');
            const spriteHeight = sprite?.height || 64;
            const playerCenter = {
                x: selfTransform.position.x,
                y: selfTransform.position.y - spriteHeight / 2
            };
            // 攻击方向：有目标时朝目标，无目标时用当前鼠标方向
            let dir = sectorDir;
            if (attacked && firstTargetTransform) {
                const dx = firstTargetTransform.position.x - selfTransform.position.x;
                const dy = firstTargetTransform.position.y - selfTransform.position.y;
                dir = Math.atan2(dy, dx);
                mas.sectorDirection = dir;
            }
            mas.spawnSectorSlashEffect(
                playerCenter, dir,
                playerCenter.x, playerCenter.y,
                sectorRadius
            );
        }

        if (!attacked) {
            console.log('NetworkCombatSystem.attackAllInRange: 范围内无目标');
        }
    }

    // ─── 空格键攻击（在 update 中调用） ───
    handleSpaceAttack() {
        const scene = this.scene;
        if (!scene.inputManager || !scene.inputManager.isKeyPressed('space')) return;

        console.log('NetworkCombatSystem: 空格键按下, weaponRenderer=', !!scene.weaponRenderer, 'ws=', !!scene.ws, 'playerEntity=', !!scene.playerEntity, 'dead=', scene.playerEntity?.dead);
        this.attackAllInRange();
    }
}
