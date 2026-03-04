/**
 * ArenaScene - 修罗斗场竞技场场景
 * 继承 BaseGameScene，复用引擎的等距地图、实体渲染、UI 面板
 * 通过 Go 后端 WebSocket 驱动多人对战
 */
import { BaseGameScene } from '../../prologue/scenes/BaseGameScene.js';
import { EntityFactory } from '../../ecs/EntityFactory.js';
import { NameComponent } from '../../ecs/components/NameComponent.js';
import { calcDamage, applyCrit, SKILL_PHASE, TARGET_MODE_FROM_STRING } from '../../ecs/ComponentTypes.js';

export class ArenaScene extends BaseGameScene {
    constructor() {
        super(1, {});
        this.name = 'ArenaScene';
        
        // 竞技场状态
        this.selfId = 0;
        this.remotePlayers = new Map(); // charId -> entity
        this.arenaSize = { width: 800, height: 600 };
        this.skills = [];
        this.skillCooldowns = {};
        this.selectedTarget = null;
        
        // 火堆（场景正中，默认点燃）
        this.campfire = {
            x: 0, y: 464,
            lit: true,
            fireImage: null,
            imageLoaded: false,
            frameWidth: 658 / 4,
            frameHeight: 712 / 3,
            frameCols: 4,
            frameRows: 3,
            frameCount: 12,
            currentFrame: 0,
            frameTime: 0,
            frameDuration: 0.16
        };
        
        // 网络同步
        this.lastMoveTime = 0;
        this.moveInterval = 30; // ms
        
        // WebSocket 引用（由外部注入）
        this.ws = null;
        
        // 浮动伤害文字
        this.floatingTexts = [];
        
        // 技能范围指示器
        this.skillRangeIndicators = [];
        
        // 覆盖 canvas ID（使用 engineCanvas 而非 gameCanvas）
        this.canvasId = 'engineCanvas';
        
        // 方向映射：引擎8方向 <-> 网络简写
        this._dirToNet = {
            'up': 'u', 'down': 'd', 'left': 'l', 'right': 'r',
            'up-left': 'ul', 'up-right': 'ur', 'down-left': 'dl', 'down-right': 'dr'
        };
        this._netToDir = {
            'u': 'up', 'd': 'down', 'l': 'left', 'r': 'right',
            'ul': 'up-left', 'ur': 'up-right', 'dl': 'down-left', 'dr': 'down-right'
        };
    }

    /**
     * 设置 WebSocket 连接
     */
    setWebSocket(ws) {
        this.ws = ws;
    }

    /**
     * 进入场景 - 使用后端数据初始化
     * @param {Object} data - 后端 arena_state 数据
     */
    enter(data = null) {
        // 覆盖 canvas 查找逻辑
        const origGetElement = document.getElementById.bind(document);
        const canvasId = this.canvasId;
        document.getElementById = function(id) {
            if (id === 'gameCanvas') return origGetElement(canvasId);
            return origGetElement(id);
        };
        
        try {
            super.enter(data);
        } finally {
            document.getElementById = origGetElement;
        }
        
        // 用实际 canvas 尺寸覆盖逻辑尺寸，避免双重压缩
        const canvas = document.getElementById(this.canvasId);
        if (canvas) {
            this.logicalWidth = canvas.width;
            this.logicalHeight = canvas.height;
            if (this.isometricRenderer) {
                this.isometricRenderer.canvasWidth = canvas.width;
                this.isometricRenderer.canvasHeight = canvas.height;
            }
            if (this.camera) {
                this.camera.width = canvas.width;
                this.camera.height = canvas.height;
            }
        }
        
        if (data) {
            this.selfId = data.self_id;
            // 只更新火堆坐标，保留动画参数
            if (data.campfire) {
                this.campfire.x = data.campfire.x !== undefined ? data.campfire.x : 0;
                this.campfire.y = data.campfire.y !== undefined ? data.campfire.y : 464;
            }
            this.arenaSize = data.arena || { width: 800, height: 600 };
            this.skills = data.skills || [];
            
            // 用后端数据更新玩家位置
            if (data.players) {
                for (const p of data.players) {
                    if (p.char_id === this.selfId) {
                        this.updateSelfFromServer(p);
                    } else {
                        this.addRemotePlayer(p);
                    }
                }
            }
            
            // 加载后端装备到前端 EquipmentComponent
            if (data.equipments && this.playerEntity) {
                this.loadBackendEquipments(data.equipments);
            }
        }
        
        console.log('ArenaScene: 进入竞技场', this.selfId);
        
        // 调整 BottomControlBar：居中贴底 + 扩大槽位
        this.setupBottomControlBar();
        
        // 将后端技能注入到 BottomControlBar
        this.injectBackendSkills();
        
        // 加载火焰图片
        this.loadFireImage();
        
        // 竞技场火堆默认点燃，创建粒子效果
        this.initCampfireParticles();
    }

    /**
     * 覆盖 loadActData - 竞技场不需要加载幕数据
     */
    loadActData() {
        // 不加载 ActData.json
    }

    /**
     * 覆盖 createPlayerEntity - 使用后端数据创建玩家
     */
    createPlayerEntity() {
        super.createPlayerEntity();
        // 给玩家添加名字组件（BaseGameScene 默认不添加）
        if (this.playerEntity && !this.playerEntity.getComponent('name')) {
            this.playerEntity.addComponent(new NameComponent('玩家', {
                color: '#4CAF50',
                fontSize: 14,
                offsetY: -10
            }));
        }
    }

    /**
     * 用服务端数据更新自己的实体
     */
    updateSelfFromServer(serverData) {
        if (!this.playerEntity) return;
        
        const transform = this.playerEntity.getComponent('transform');
        if (transform) {
            transform.position.x = serverData.x;
            transform.position.y = serverData.y;
        }
        
        const stats = this.playerEntity.getComponent('stats');
        if (stats) {
            stats.hp = serverData.hp;
            stats.maxHp = serverData.max_hp;
            stats.mp = serverData.mp;
            stats.maxMp = serverData.max_mp;
            stats.attack = serverData.attack;
            stats.defense = serverData.defense;
            stats.speed = serverData.speed;
            stats.level = serverData.level;
            // 同步暴击属性（对齐后端 CombatAttributeComponent）
            if (serverData.crit_rate !== undefined) stats.critRate = serverData.crit_rate;
            if (serverData.crit_damage !== undefined) stats.critDamage = serverData.crit_damage;
        }
        
        const nameComp = this.playerEntity.getComponent('name');
        if (nameComp) {
            nameComp.name = serverData.name;
        } else {
            this.playerEntity.addComponent(new NameComponent(serverData.name, {
                color: '#4CAF50',
                fontSize: 14,
                offsetY: -10
            }));
        }
    }

    /**
     * 添加远程玩家实体
     */
    addRemotePlayer(serverData) {
        if (this.remotePlayers.has(serverData.char_id)) return;
        
        const entity = this.entityFactory.createPlayer({
            name: serverData.name,
            class: serverData.class === 'warrior' ? 'warrior' : 'archer',
            level: serverData.level || 1,
            position: { x: serverData.x, y: serverData.y },
            stats: {
                maxHp: serverData.max_hp,
                attack: serverData.attack,
                defense: serverData.defense,
                speed: serverData.speed,
                critRate: serverData.crit_rate || 0.1,
                critDamage: serverData.crit_damage || 1.5
            }
        });
        
        // 设置当前 HP/MP
        const stats = entity.getComponent('stats');
        if (stats) {
            stats.hp = serverData.hp;
            stats.mp = serverData.mp;
            stats.maxMp = serverData.max_mp;
        }
        
        // 竞技场中远程玩家设为敌对阵营（对齐后端阵营判定）
        entity.faction = 'enemy';
        const combat = entity.getComponent('combat');
        if (combat) {
            combat.faction = 'enemy';
        }
        
        // 标记为远程玩家
        entity.isRemote = true;
        entity.charId = serverData.char_id;
        entity.dead = serverData.dead || false;
        entity.targetX = serverData.x;
        entity.targetY = serverData.y;
        
        // 添加名字组件
        entity.addComponent(new NameComponent(serverData.name, {
            color: '#ffffff',
            fontSize: 14,
            offsetY: -10
        }));
        
        this.entities.push(entity);
        this.remotePlayers.set(serverData.char_id, entity);
    }

    /**
     * 移除远程玩家
     */
    removeRemotePlayer(charId) {
        const entity = this.remotePlayers.get(charId);
        if (entity) {
            const idx = this.entities.indexOf(entity);
            if (idx >= 0) this.entities.splice(idx, 1);
            this.remotePlayers.delete(charId);
        }
        if (this.selectedTarget === charId) {
            this.selectedTarget = null;
        }
    }

    /**
     * 覆盖 update - 添加网络同步逻辑
     */
    update(deltaTime) {
        // 发送移动到服务端
        this.sendMovement();
        
        // 插值远程玩家位置 + 同步行走动画
        for (const [id, entity] of this.remotePlayers) {
            if (entity.dead) continue;
            if (entity.targetX !== undefined) {
                const transform = entity.getComponent('transform');
                if (transform) {
                    const dx = entity.targetX - transform.position.x;
                    const dy = entity.targetY - transform.position.y;
                    const dist = Math.sqrt(dx * dx + dy * dy);
                    
                    // 基于 deltaTime 的平滑插值，距离太远则瞬移，很近则snap
                    if (dist > 300) {
                        transform.position.x = entity.targetX;
                        transform.position.y = entity.targetY;
                    } else if (dist < 1) {
                        // 距离很近，直接到位，避免漂移
                        transform.position.x = entity.targetX;
                        transform.position.y = entity.targetY;
                    } else {
                        const lerp = 1 - Math.pow(0.9, deltaTime * 60);
                        transform.position.x += dx * lerp;
                        transform.position.y += dy * lerp;
                    }
                    
                    // 根据最近收到移动消息的时间判断行走状态
                    const sprite = entity.getComponent('sprite');
                    if (sprite) {
                        const timeSinceMove = Date.now() - (entity._lastMoveTime || 0);
                        if (dist > 2 && timeSinceMove < 200) {
                            sprite.isWalking = true;
                        } else if (dist <= 2 || timeSinceMove >= 200) {
                            sprite.isWalking = false;
                        }
                    }
                }
            }
        }
        
        // 更新技能范围指示器
        this.skillRangeIndicators = this.skillRangeIndicators.filter(ind => {
            ind.life -= deltaTime;
            ind.dashOffset += 60 * deltaTime;
            return ind.life > 0;
        });
        
        // 更新火堆动画
        this.updateCampfireAnimation(deltaTime);
        
        // 调用父类 update
        super.update(deltaTime);
    }

    /**
     * 发送移动数据到服务端
     */
    sendMovement() {
            if (!this.ws || !this.playerEntity) return;

            const now = Date.now();
            if (now - this.lastMoveTime < this.moveInterval) return;

            const transform = this.playerEntity.getComponent('transform');
            if (!transform) return;

            const x = Math.round(transform.position.x * 10) / 10;
            const y = Math.round(transform.position.y * 10) / 10;

            // 基于位置变化检测移动（兼容键盘和鼠标点击移动）
            const dx = x - (this._lastSentX || 0);
            const dy = y - (this._lastSentY || 0);
            const moved = Math.abs(dx) > 0.5 || Math.abs(dy) > 0.5;

            if (moved) {
                const sprite = this.playerEntity.getComponent('sprite');
                const engineDir = (sprite && sprite.direction) ? sprite.direction : 'down';
                const direction = this._dirToNet[engineDir] || 'd';

                this.ws.send('move', {
                    x: x,
                    y: y,
                    direction: direction
                });
                this._lastSentX = x;
                this._lastSentY = y;
                this.lastMoveTime = now;
                this._wasMoving = true;
            } else if (this._wasMoving) {
                // 停止移动时发送最终位置
                const sprite = this.playerEntity.getComponent('sprite');
                const engineDir = (sprite && sprite.direction) ? sprite.direction : 'down';
                const direction = this._dirToNet[engineDir] || 'd';
                this.ws.send('move', {
                    x: x,
                    y: y,
                    direction: direction
                });
                this._lastSentX = x;
                this._lastSentY = y;
                this._wasMoving = false;
                this.lastMoveTime = now;
            }
        }


    /**
     * 攻击选中目标
     */
    /**
     * 攻击目标 - 使用与后端一致的伤害公式进行前端预判
     * 后端公式：base = attack - defense*0.5, min 1, variance 0.85~1.15
     * 暴击率：普攻10%, 技能15%, 暴击倍率1.5x
     */
    attackTarget() {
        if (!this.selectedTarget || !this.ws) return;
        const entity = this.remotePlayers.get(this.selectedTarget);
        if (!entity || entity.dead) return;
        
        // 前端范围预判（对齐后端 handleAttack 的距离检查）
        if (this.playerEntity) {
            const selfTransform = this.playerEntity.getComponent('transform');
            const targetTransform = entity.getComponent('transform');
            const combat = this.playerEntity.getComponent('combat');
            if (selfTransform && targetTransform && combat) {
                const selfStats = this.playerEntity.getComponent('stats');
                const charClass = this.playerEntity.class || 'warrior';
                const maxRange = charClass === 'archer' ? 200 : 60;
                
                if (!combat.isInSkillRange(
                    selfTransform.position,
                    targetTransform.position,
                    { range: maxRange, area_type: 'single' }
                )) {
                    // 超出范围，不发送请求
                    if (this.floatingTextManager && selfStats) {
                        this.floatingTextManager.addText(
                            selfTransform.position.x,
                            selfTransform.position.y - 20,
                            '超出攻击范围',
                            '#ff6600'
                        );
                    }
                    return;
                }
            }
        }
        
        this.ws.send('attack', { target_id: this.selectedTarget });
    }

    /**
     * 释放技能
     */
    /**
     * 释放技能 - 对齐后端 handleCastSkill 的逻辑
     * 前端预判：范围检查、MP检查、冷却检查
     * 服务端权威：伤害计算、目标选择、状态变更
     */
    castSkill(skillId) {
        if (!this.ws || !this.playerEntity) return;
        
        const now = Date.now();
        const cd = this.skillCooldowns[skillId];
        if (cd && now < cd) return;
        
        const transform = this.playerEntity.getComponent('transform');
        if (!transform) return;
        
        // 前端 MP 预判（对齐后端 handleCastSkill 的 MP 检查）
        const skill = this.skills.find(s => s.id === skillId);
        if (skill) {
            const stats = this.playerEntity.getComponent('stats');
            if (stats && skill.mp_cost > 0 && stats.mp < skill.mp_cost) {
                if (this.floatingTextManager) {
                    this.floatingTextManager.addText(
                        transform.position.x,
                        transform.position.y - 20,
                        'MP不足',
                        '#6699ff'
                    );
                }
                return;
            }
        }
        
        let targetX = transform.position.x;
        let targetY = transform.position.y;
        let targetId = 0;
        
        if (this.selectedTarget) {
            const target = this.remotePlayers.get(this.selectedTarget);
            if (target) {
                const tTransform = target.getComponent('transform');
                if (tTransform) {
                    targetX = tTransform.position.x;
                    targetY = tTransform.position.y;
                }
                targetId = this.selectedTarget;
                
                // 前端范围预判（对齐后端距离检查）
                if (skill) {
                    const combat = this.playerEntity.getComponent('combat');
                    if (combat && !combat.isInSkillRange(
                        transform.position,
                        { x: targetX, y: targetY },
                        skill
                    )) {
                        if (this.floatingTextManager) {
                            this.floatingTextManager.addText(
                                transform.position.x,
                                transform.position.y - 20,
                                '超出技能范围',
                                '#ff6600'
                            );
                        }
                        return;
                    }
                }
            }
        }
        
        this.ws.send('cast_skill', {
            skill_id: skillId,
            target_id: targetId,
            target_x: targetX,
            target_y: targetY
        });
        
        if (skill) {
            this.skillCooldowns[skillId] = now + skill.cooldown * 1000;
            
            // 同步冷却到 combat 组件，让 BottomControlBar 显示冷却遮罩
            const combat = this.playerEntity.getComponent('combat');
            if (combat) {
                const combatSkillId = `backend_${skillId}`;
                combat.skillCooldowns.set(combatSkillId, now);
                
                // 启动技能阶段流水线（前端视觉反馈）
                combat.startSkillPipeline({
                    ...skill,
                    phaseDurations: {
                        windup: 100,
                        hit: 50,
                        settle: 50,
                        recovery: 200
                    }
                });
            }
        }
    }

    // ===== 网络事件处理 =====

    onPlayerJoined(data) {
        this.addRemotePlayer(data);
    }

    onPlayerLeft(data) {
        this.removeRemotePlayer(data.char_id);
    }

    onPlayerMoved(data) {
        const entity = this.remotePlayers.get(data.char_id);
        if (!entity) return;

        entity.targetX = data.x;
        entity.targetY = data.y;

        // 同步方向到精灵组件（网络简写 -> 引擎方向），并标记行走状态
        const sprite = entity.getComponent('sprite');
        if (sprite) {
            if (data.direction) {
                sprite.direction = this._netToDir[data.direction] || data.direction;
            }
            sprite.isWalking = true;
        }
        // 记录收到移动的时间，用于停止行走判断
        entity._lastMoveTime = Date.now();
    }


    /**
     * 处理伤害事件 - 服务端权威伤害结果
     * 后端伤害公式：base = attack - defense*0.5, variance 0.85~1.15
     * 暴击：普攻10%/技能15%, 倍率1.5x
     */
    onDamage(data) {
        // 更新目标 HP（服务端权威值）
        const targetEntity = data.target_id === this.selfId
            ? this.playerEntity
            : this.remotePlayers.get(data.target_id);
        
        if (targetEntity) {
            const stats = targetEntity.getComponent('stats');
            if (stats) {
                stats.hp = data.target_hp;
                stats.maxHp = data.target_max_hp;
            }
            
            // 浮动伤害文字（区分普攻/技能/暴击）
            const transform = targetEntity.getComponent('transform');
            if (transform && this.floatingTextManager) {
                let color = '#ff0000';
                let text = `${Math.round(data.damage)}`;
                
                if (data.is_crit) {
                    color = '#ffd700';
                    text = `暴击! ${Math.round(data.damage)}`;
                }
                
                // 技能伤害用不同颜色
                if (data.skill_name) {
                    color = data.is_crit ? '#ff8c00' : '#ff4500';
                    text = data.is_crit
                        ? `${data.skill_name} 暴击! ${Math.round(data.damage)}`
                        : `${data.skill_name} ${Math.round(data.damage)}`;
                }
                
                this.floatingTextManager.addText(
                    transform.position.x,
                    transform.position.y - 20,
                    text,
                    color
                );
            }
        }
    }

    onPlayerDied(data) {
        const entity = data.char_id === this.selfId
            ? this.playerEntity
            : this.remotePlayers.get(data.char_id);
        if (entity) {
            entity.dead = true;
            const stats = entity.getComponent('stats');
            if (stats) stats.hp = 0;
            // 死亡时精灵半透明
            const sprite = entity.getComponent('sprite');
            if (sprite) {
                sprite.alpha = 0.3;
                sprite.isWalking = false;
            }
        }
        // 在死亡实体位置显示击杀信息
        const deadEntity = entity || (data.char_id === this.selfId ? this.playerEntity : null);
        const transform = deadEntity ? deadEntity.getComponent('transform') : null;
        const textX = transform ? transform.position.x : 400;
        const textY = transform ? transform.position.y - 40 : 300;
        if (this.floatingTextManager) {
            this.floatingTextManager.addText(textX, textY, `${data.name} 被 ${data.killer} 击杀`, '#ff0000');
        }
    }

    onPlayerRespawn(data) {
        const entity = data.char_id === this.selfId
            ? this.playerEntity
            : this.remotePlayers.get(data.char_id);
        if (entity) {
            entity.dead = false;
            const transform = entity.getComponent('transform');
            if (transform) {
                transform.position.x = data.x;
                transform.position.y = data.y;
            }
            if (entity.targetX !== undefined) {
                entity.targetX = data.x;
                entity.targetY = data.y;
            }
            const stats = entity.getComponent('stats');
            if (stats) {
                stats.hp = data.hp;
                stats.maxHp = data.max_hp;
                stats.mp = data.mp;
                stats.maxMp = data.max_mp;
            }
            // 复活时恢复精灵透明度
            const sprite = entity.getComponent('sprite');
            if (sprite) {
                sprite.alpha = 1.0;
            }
        }
        if (this.floatingTextManager) {
            this.floatingTextManager.addText(data.x, data.y, `${data.name} 复活了`, '#00ff00');
        }
    }

    onStateSync(data) {
        if (!data || !data.players) return;
        const serverIds = new Set();

        for (const p of data.players) {
            serverIds.add(p.char_id);

            if (p.char_id === this.selfId) {
                // 自己：同步 HP/MP 和战斗属性（位置由本地控制）
                if (this.playerEntity) {
                    const stats = this.playerEntity.getComponent('stats');
                    if (stats) {
                        if (p.hp !== undefined) stats.hp = p.hp;
                        if (p.max_hp !== undefined) stats.maxHp = p.max_hp;
                        if (p.mp !== undefined) stats.mp = p.mp;
                        if (p.max_mp !== undefined) stats.maxMp = p.max_mp;
                        if (p.attack !== undefined) stats.attack = p.attack;
                        if (p.defense !== undefined) stats.defense = p.defense;
                        if (p.crit_rate !== undefined) stats.critRate = p.crit_rate;
                        if (p.crit_damage !== undefined) stats.critDamage = p.crit_damage;
                    }
                    if (p.dead !== undefined) {
                        if (p.dead && !this.playerEntity.dead) {
                            this.playerEntity.dead = true;
                            const sprite = this.playerEntity.getComponent('sprite');
                            if (sprite) { sprite.alpha = 0.3; sprite.isWalking = false; }
                        } else if (!p.dead && this.playerEntity.dead) {
                            this.playerEntity.dead = false;
                            const sprite = this.playerEntity.getComponent('sprite');
                            if (sprite) sprite.alpha = 1.0;
                        }
                    }
                }
                continue;
            }

            // 远程玩家
            const entity = this.remotePlayers.get(p.char_id);
            if (entity) {
                if (p.x !== undefined) entity.targetX = p.x;
                if (p.y !== undefined) entity.targetY = p.y;
                if (p.dead !== undefined) entity.dead = p.dead;
                const sprite = entity.getComponent('sprite');
                if (sprite) {
                    if (p.direction) sprite.direction = this._netToDir[p.direction] || p.direction;
                    if (p.dead !== undefined) {
                        sprite.alpha = p.dead ? 0.3 : 1.0;
                        if (p.dead) sprite.isWalking = false;
                    }
                }
                const stats = entity.getComponent('stats');
                if (stats) {
                    if (p.hp !== undefined) stats.hp = p.hp;
                    if (p.max_hp !== undefined) stats.maxHp = p.max_hp;
                    if (p.mp !== undefined) stats.mp = p.mp;
                    if (p.max_mp !== undefined) stats.maxMp = p.max_mp;
                    if (p.attack !== undefined) stats.attack = p.attack;
                    if (p.defense !== undefined) stats.defense = p.defense;
                    if (p.crit_rate !== undefined) stats.critRate = p.crit_rate;
                    if (p.crit_damage !== undefined) stats.critDamage = p.crit_damage;
                }
            } else if (p.name) {
                // 新玩家（有 name 字段说明是全量数据），添加到场景
                this.addRemotePlayer(p);
            }
        }

        // 玩家离开由 player_left 事件处理，增量同步不移除
    }

    onSkillCasted(data) {
        if (data.caster_id === this.selfId && this.playerEntity) {
            const stats = this.playerEntity.getComponent('stats');
            if (stats) {
                stats.mp = data.caster_mp;
                stats.maxMp = data.caster_max_mp;
            }
        }
        
        const caster = data.caster_id === this.selfId
            ? this.playerEntity
            : this.remotePlayers.get(data.caster_id);
        if (caster) {
            const transform = caster.getComponent('transform');
            if (transform && this.floatingTextManager) {
                this.floatingTextManager.addText(
                    transform.position.x,
                    transform.position.y - 10,
                    data.skill_name,
                    '#ffab40'
                );
            }
        }
    }

    /**
     * 初始化火堆粒子效果（竞技场火堆默认点燃）
     */
    initCampfireParticles() {
        if (!this.campfire || !this.campfire.lit || !this.particleSystem) return;
        
        const fireBaseY = this.campfire.y - 15;
        const firePoint = { x: this.campfire.x, y: fireBaseY };
        
        this.campfire.emitters = [];
        
        // 大火焰粒子
        this.campfire.emitters.push(this.particleSystem.createEmitter({
            position: { x: firePoint.x, y: firePoint.y },
            rate: 6, duration: Infinity,
            particleConfig: {
                position: { x: firePoint.x, y: firePoint.y },
                velocity: { x: 0, y: -50 }, life: 250, size: 8.5,
                color: '#ffaa22', alpha: 0.85, gravity: 0, friction: 0.95
            }
        }));
        
        // 中火焰粒子
        this.campfire.emitters.push(this.particleSystem.createEmitter({
            position: { x: firePoint.x, y: firePoint.y },
            rate: 8, duration: Infinity,
            particleConfig: {
                position: { x: firePoint.x, y: firePoint.y },
                velocity: { x: 0, y: -35 }, life: 200, size: 6,
                color: '#ff8833', alpha: 0.8, gravity: 0, friction: 0.95
            }
        }));
        
        // 白色亮点
        this.campfire.emitters.push(this.particleSystem.createEmitter({
            position: { x: firePoint.x, y: firePoint.y },
            rate: 4, duration: Infinity,
            particleConfig: {
                position: { x: firePoint.x, y: firePoint.y },
                velocity: { x: 0, y: -120 }, life: 400, size: 4.5,
                color: '#ffffee', alpha: 1.0, gravity: 0, friction: 0.95
            }
        }));
        
        // 亮黄色火星
        this.campfire.emitters.push(this.particleSystem.createEmitter({
            position: { x: firePoint.x, y: firePoint.y },
            rate: 10, duration: Infinity,
            particleConfig: {
                position: { x: firePoint.x, y: firePoint.y },
                velocity: { x: 0, y: -100 }, life: 350, size: 3.5,
                color: '#ffee44', alpha: 0.9, gravity: 0, friction: 0.95
            }
        }));
        
        // 橙色火星
        this.campfire.emitters.push(this.particleSystem.createEmitter({
            position: { x: firePoint.x, y: firePoint.y },
            rate: 8, duration: Infinity,
            particleConfig: {
                position: { x: firePoint.x, y: firePoint.y },
                velocity: { x: 0, y: -80 }, life: 300, size: 2.5,
                color: '#ff9933', alpha: 0.85, gravity: 0, friction: 0.95
            }
        }));
        
        // 红色火星
        this.campfire.emitters.push(this.particleSystem.createEmitter({
            position: { x: firePoint.x, y: firePoint.y },
            rate: 6, duration: Infinity,
            particleConfig: {
                position: { x: firePoint.x, y: firePoint.y },
                velocity: { x: 0, y: -60 }, life: 250, size: 2,
                color: '#ff5522', alpha: 0.8, gravity: 0, friction: 0.95
            }
        }));
        
        // 小火星
        this.campfire.emitters.push(this.particleSystem.createEmitter({
            position: { x: firePoint.x, y: firePoint.y },
            rate: 12, duration: Infinity,
            particleConfig: {
                position: { x: firePoint.x, y: firePoint.y },
                velocity: { x: 0, y: -40 }, life: 200, size: 2,
                color: '#ff6633', alpha: 0.7, gravity: 0, friction: 0.95
            }
        }));
        
        console.log('ArenaScene: 火堆粒子效果已创建');
    }

    /**
     * 更新火堆动画
     */
    updateCampfireAnimation(deltaTime) {
        if (!this.campfire) return;
        
        // 更新帧动画
        if (this.campfire.lit && this.campfire.imageLoaded) {
            this.campfire.frameTime += deltaTime;
            if (this.campfire.frameTime >= this.campfire.frameDuration) {
                this.campfire.frameTime = 0;
                this.campfire.currentFrame = (this.campfire.currentFrame + 1) % this.campfire.frameCount;
            }
        }
        
        // 更新火焰粒子效果
        if (this.campfire.lit && this.campfire.emitters) {
            const time = performance.now() / 1000;
            
            this.campfire.emitters.forEach((emitter, index) => {
                if (emitter) {
                    let swayAmount;
                    if (index < 2) {
                        swayAmount = (Math.random() - 0.5) * 10;
                    } else {
                        swayAmount = Math.sin(time * 2 + index * 0.5) * 4 + (Math.random() - 0.5) * 2;
                    }
                    
                    const baseX = this.campfire.x;
                    const baseY = this.campfire.y + 2;
                    
                    emitter.position.x = baseX + swayAmount;
                    emitter.position.y = baseY - 15;
                    emitter.particleConfig.velocity.x = (Math.random() - 0.5) * 10;
                    
                    this.particleSystem.updateEmitter(emitter, deltaTime);
                }
            });
        }
    }

    /**
     * 覆盖 renderWorldObjects - 添加火堆渲染
     */
    renderWorldObjects(ctx) {
        const renderQueue = [];

        for (const entity of this.entities) {
            const transform = entity.getComponent('transform');
            if (transform) {
                renderQueue.push({ type: 'entity', y: transform.position.y, entity });
            }
        }

        // 添加火堆
        if (this.campfire) {
            renderQueue.push({
                type: 'campfire',
                y: this.campfire.y,
                render: () => this.renderCampfire(ctx)
            });
        }

        renderQueue.sort((a, b) => a.y - b.y);

        for (const item of renderQueue) {
            if (item.type === 'entity') {
                this.renderEntity(ctx, item.entity);
            } else if (item.render) {
                item.render();
            }
        }
    }

    /**
     * 渲染火堆
     */
    renderCampfire(ctx) {
        const x = this.campfire.x;
        const y = this.campfire.y;

        // 燃烧的木材底座
        ctx.save();
        ctx.strokeStyle = '#3a2a1a';
        ctx.lineWidth = 8;
        ctx.lineCap = 'round';
        ctx.beginPath();
        ctx.moveTo(x - 20, y - 5);
        ctx.lineTo(x + 20, y - 25);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(x + 20, y - 5);
        ctx.lineTo(x - 20, y - 25);
        ctx.stroke();
        ctx.restore();

        // 发光效果
        const gradient = ctx.createRadialGradient(x, y - 15, 0, x, y - 15, 60);
        gradient.addColorStop(0, 'rgba(255, 200, 0, 0.4)');
        gradient.addColorStop(0.5, 'rgba(255, 100, 0, 0.2)');
        gradient.addColorStop(1, 'rgba(255, 50, 0, 0)');
        ctx.fillStyle = gradient;
        ctx.beginPath();
        ctx.arc(x, y - 15, 60, 0, Math.PI * 2);
        ctx.fill();

        // 火焰帧动画
        if (this.campfire.imageLoaded && this.campfire.fireImage) {
            const col = this.campfire.currentFrame % this.campfire.frameCols;
            const row = Math.floor(this.campfire.currentFrame / this.campfire.frameCols);
            const frameX = col * this.campfire.frameWidth;
            const frameY = row * this.campfire.frameHeight;
            const fireWidth = 40;
            const fireHeight = 60;
            const fireX = x - fireWidth / 2;
            const fireY = y - fireHeight - 5;

            ctx.globalAlpha = 0.9;
            ctx.drawImage(
                this.campfire.fireImage,
                frameX, frameY, this.campfire.frameWidth, this.campfire.frameHeight,
                fireX, fireY, fireWidth, fireHeight
            );
            ctx.globalAlpha = 1.0;
        }
    }

    /**
     * 调整 BottomControlBar 位置和槽位大小
     */
    setupBottomControlBar() {
        if (!this.bottomControlBar) return;
        
        // 槽位尺寸 60px
        const slotSize = 60;
        const slotGap = 8;
        const totalSlots = 7;
        const totalWidth = totalSlots * slotSize + (totalSlots - 1) * slotGap;
        const barWidth = totalWidth + 160; // 两侧留空给血蓝球
        const barHeight = 100;
        
        // 居中贴底
        this.bottomControlBar.width = barWidth;
        this.bottomControlBar.height = barHeight;
        this.bottomControlBar.x = (this.logicalWidth - barWidth) / 2;
        this.bottomControlBar.y = this.logicalHeight - barHeight;
        
        // 红蓝球各向外移 20px
        this.bottomControlBar.hpOrb.x = 40;
        this.bottomControlBar.mpOrb.x = barWidth - 40;
        
        // 重新计算槽位位置
        const startX = barWidth / 2 - totalWidth / 2 + slotSize / 2;
        for (let i = 0; i < this.bottomControlBar.skillSlots.length; i++) {
            const slot = this.bottomControlBar.skillSlots[i];
            slot.x = startX + i * (slotSize + slotGap);
            slot.size = slotSize;
        }
        
        // 设置技能点击回调 - 连接到后端技能释放
        this.bottomControlBar.onSkillClick = (skill) => {
            if (skill && skill.backendId) {
                this.castSkill(skill.backendId);
            }
        };
    }

    /**
     * 将后端技能注入到 playerEntity 的 combat 组件
     */
    injectBackendSkills() {
        if (!this.playerEntity || !this.skills || this.skills.length === 0) return;
        
        let combat = this.playerEntity.getComponent('combat');
        if (!combat) return;
        
        // 清空现有技能
        combat.skills = [];
        combat.skillCooldowns = new Map();
        
        // 后端技能映射到 combat 组件（跳过普通攻击，只取技能）
        const backendSkills = this.skills.filter(s => s.mp_cost > 0);
        
        // 技能图标映射
        const skillIconMap = {
            '猛击': '⚔️',
            '旋风斩': '🌀',
            '战吼': '📢',
            '射击': '🏹',
            '多重射击': '🎯',
            '闪避': '💨'
        };
        
        backendSkills.forEach(sk => {
            combat.addSkill({
                id: `backend_${sk.id}`,
                backendId: sk.id,
                name: sk.name,
                cooldown: sk.cooldown,
                manaCost: sk.mp_cost,
                effectType: sk.name, // 用名字做 effectType
                castTime: 0,
                icon: skillIconMap[sk.name] || '⚡'
            });
        });
        
        console.log('ArenaScene: 注入后端技能', backendSkills.map(s => s.name));
    }

    /**
     * 将后端装备数据转换为前端格式并装备到 EquipmentComponent
     * 后端格式: { id, slot_type, def: { id, name, slot_type, class, quality, level, attack, defense, hp, speed, crit_rate } }
     * 前端格式: { id, name, type, subType, rarity, stats: { attack, defense, maxHp, speed }, attackSpeed, attackRange, attackDistance }
     * @param {Array} backendEquips - 后端装备数据数组
     */
    loadBackendEquipments(backendEquips) {
        if (!this.playerEntity || !backendEquips || backendEquips.length === 0) return;
        
        const equipment = this.playerEntity.getComponent('equipment');
        if (!equipment) return;
        
        // 后端 slot_type -> 前端 slot 映射
        const SLOT_MAP = {
            'weapon': 'mainhand',
            'helmet': 'helmet',
            'armor': 'armor',
            'boots': 'boots'
        };
        
        // 后端 quality -> 前端 rarity 数值映射
        const QUALITY_MAP = {
            'normal': 0,
            'rare': 2,
            'epic': 3,
            'legendary': 4
        };
        
        // 职业默认武器属性（参考 EquipmentData.json）
        const charClass = this.playerEntity.class || 'warrior';
        const WEAPON_PROPS = {
            'warrior': { attackSpeed: 2.0, attackRange: 90, attackDistance: 100 },
            'archer': { attackSpeed: 3.0, attackRange: 200, attackDistance: 250 }
        };
        
        for (const eq of backendEquips) {
            const def = eq.def;
            if (!def) continue;
            
            const frontendSlot = SLOT_MAP[eq.slot_type];
            if (!frontendSlot) continue;
            
            // 构建前端装备对象
            const item = {
                id: def.id,
                name: def.name,
                type: eq.slot_type,
                subType: frontendSlot,
                rarity: QUALITY_MAP[def.quality] !== undefined ? QUALITY_MAP[def.quality] : 0,
                level: def.level,
                stats: {
                    attack: def.attack || 0,
                    defense: def.defense || 0,
                    maxHp: def.hp || 0,
                    speed: def.speed || 0
                }
            };
            
            // 武器添加攻击属性（参考 EquipmentData.json 的 attackSpeed/attackRange/attackDistance）
            if (eq.slot_type === 'weapon') {
                const weaponProps = WEAPON_PROPS[charClass] || WEAPON_PROPS['warrior'];
                item.attackSpeed = weaponProps.attackSpeed;
                item.attackRange = weaponProps.attackRange;
                item.attackDistance = weaponProps.attackDistance;
            }
            
            equipment.equip(frontendSlot, item);
        }
        
        console.log('ArenaScene: 加载后端装备完成', backendEquips.map(e => e.def?.name));
    }

    /**
     * 退出场景
     */
    exit() {
        this.remotePlayers.clear();
        this.selectedTarget = null;
        this.skillRangeIndicators = [];
        super.exit();
    }
}
