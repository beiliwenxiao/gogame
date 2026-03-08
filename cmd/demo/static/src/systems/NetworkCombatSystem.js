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
        if (!entity) return 60;
        const equipment = entity.getComponent('equipment');
        if (equipment) {
            const weapon = equipment.getEquipment('mainhand');
            if (weapon && weapon.attackRange) return weapon.attackRange;
        }
        const charClass = entity.class || 'warrior';
        return charClass === 'archer' ? 200 : 60;
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
        if (!scene.ws || !scene.playerEntity) return;
        if (scene.playerEntity.dead) return;

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

                // 范围预判
                if (skill) {
                    const combat = scene.playerEntity.getComponent('combat');
                    if (combat && !combat.isInSkillRange(transform.position, { x: targetX, y: targetY }, skill)) {
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
                combat.skillCooldowns.set(combatSkillId, now);
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

        const mouseWorldPos = scene.inputManager.getMouseWorldPosition(scene.camera);
        if (!mouseWorldPos) return;

        const clickRange = 40;
        let closestEntity = null;
        let closestDist = clickRange;

        const candidates = [
            ...Array.from(scene.npcEntities.values()).map(e => ({ entity: e, id: e.npcId })),
            ...Array.from(scene.remotePlayers.entries()).map(([id, e]) => ({ entity: e, id }))
        ];

        for (const { entity, id } of candidates) {
            if (entity.dead) continue;
            const transform = entity.getComponent('transform');
            if (!transform) continue;
            const dx = mouseWorldPos.x - transform.position.x;
            const dy = mouseWorldPos.y - transform.position.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist < closestDist) { closestDist = dist; closestEntity = { entity, id }; }
        }

        if (closestEntity) {
            const selfT = scene.playerEntity.getComponent('transform');
            if (selfT && scene.isInSafeZone(selfT.position.x, selfT.position.y)) {
                if (scene.floatingTextManager) {
                    scene.floatingTextManager.addText(selfT.position.x, selfT.position.y - 20, '安全区内禁止战斗', '#ffaa00');
                }
                scene.inputManager.markMouseClickHandled();
                return;
            }
            scene.selectedTarget = closestEntity.id;
            scene.inputManager.markMouseClickHandled();
        }
    }

    // ─── 处理伤害消息 ───
    onDamage(data) {
        const scene = this.scene;
        console.log('NetworkCombatSystem.onDamage: 收到伤害数据', data);

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
        }
    }

    // ─── 空格键攻击（在 update 中调用） ───
    handleSpaceAttack() {
        const scene = this.scene;
        if (!scene.inputManager || !scene.inputManager.isKeyPressed('space')) return;

        console.log('NetworkCombatSystem: 空格键按下，selectedTarget=', scene.selectedTarget, 'playerDead=', scene.playerEntity?.dead);

        if (!scene.selectedTarget && scene.playerEntity && !scene.playerEntity.dead) {
            this.autoSelectNearestEnemy();
            console.log('NetworkCombatSystem: 自动选中最近敌人，selectedTarget=', scene.selectedTarget);
        }
        this.attackTarget();
    }
}
