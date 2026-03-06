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
        this.npcEntities = new Map();   // npcId -> entity
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

        // 灵魂状态 UI
        this._soulOverlay = null;
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
            
            // 加载初始 NPC
            if (data.npcs && data.npcs.length > 0) {
                this.onNPCSpawn({ npcs: data.npcs });
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
        
        // 插值 NPC 位置 - 基于速度的线性插值，避免跳跃感
        for (const [id, entity] of this.npcEntities) {
            if (entity.dead) continue;
            if (entity.targetX !== undefined) {
                const transform = entity.getComponent('transform');
                if (transform) {
                    const dx = entity.targetX - transform.position.x;
                    const dy = entity.targetY - transform.position.y;
                    const dist = Math.sqrt(dx * dx + dy * dy);
                    
                    if (dist > 400) {
                        // 距离过远直接瞬移（传送/重生等情况）
                        transform.position.x = entity.targetX;
                        transform.position.y = entity.targetY;
                    } else if (dist > 0.5) {
                        // 基于 NPC speed 属性做线性插值，匀速移动
                        const stats = entity.getComponent('stats');
                        const speed = (stats && stats.speed) ? stats.speed : 60;
                        // 每帧最大移动距离 = speed * deltaTime，额外乘以1.5补偿网络延迟
                        const maxMove = speed * deltaTime * 1.5;
                        if (maxMove >= dist) {
                            transform.position.x = entity.targetX;
                            transform.position.y = entity.targetY;
                        } else {
                            transform.position.x += (dx / dist) * maxMove;
                            transform.position.y += (dy / dist) * maxMove;
                        }
                    }
                    
                    const sprite = entity.getComponent('sprite');
                    if (sprite) {
                        const timeSinceMove = Date.now() - (entity._lastMoveTime || 0);
                        if (dist > 2 && timeSinceMove < 800) {
                            sprite.isWalking = true;
                        } else if (dist <= 2 || timeSinceMove >= 800) {
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
        // 灵魂状态不能攻击
        if (this.playerEntity && this.playerEntity.dead) return;
        
        // 检查是否是 NPC 目标（负数 ID）
        const isNPC = this.selectedTarget < 0;
        const entity = isNPC
            ? this.npcEntities.get(this.selectedTarget)
            : this.remotePlayers.get(this.selectedTarget);
        if (!entity || entity.dead) return;
        
        // 前端范围预判
        if (this.playerEntity) {
            const selfTransform = this.playerEntity.getComponent('transform');
            const targetTransform = entity.getComponent('transform');
            const combat = this.playerEntity.getComponent('combat');
            if (selfTransform && targetTransform && combat) {
                // 使用武器攻击范围
                const equipment = this.playerEntity.getComponent('equipment');
                let maxRange = 60;
                if (equipment) {
                    const weapon = equipment.getEquipped('mainhand');
                    if (weapon && weapon.attackRange) {
                        maxRange = weapon.attackRange;
                    } else {
                        const charClass = this.playerEntity.class || 'warrior';
                        maxRange = charClass === 'archer' ? 200 : 60;
                    }
                } else {
                    const charClass = this.playerEntity.class || 'warrior';
                    maxRange = charClass === 'archer' ? 200 : 60;
                }
                
                if (!combat.isInSkillRange(
                    selfTransform.position,
                    targetTransform.position,
                    { range: maxRange, area_type: 'single' }
                )) {
                    if (this.floatingTextManager) {
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
        
        const msgType = isNPC ? 'attack_npc' : 'attack';
        this.ws.send(msgType, { target_id: this.selectedTarget });
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
        // 灵魂状态不能施法
        if (this.playerEntity.dead) return;
        
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
        
        // 查找目标：支持远程玩家和 NPC（负数 ID）
        const isNPC = this.selectedTarget && this.selectedTarget < 0;
        if (this.selectedTarget) {
            const target = isNPC
                ? this.npcEntities.get(this.selectedTarget)
                : this.remotePlayers.get(this.selectedTarget);
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
        
        const msgType = isNPC ? 'cast_skill_npc' : 'cast_skill';
        this.ws.send(msgType, {
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
        let targetEntity;
        if (data.target_is_npc) {
            targetEntity = this.npcEntities.get(data.target_id);
        } else {
            targetEntity = data.target_id === this.selfId
                ? this.playerEntity
                : this.remotePlayers.get(data.target_id);
        }
        
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

        // 自己死亡：显示灵魂状态提示
        if (data.char_id === this.selfId) {
            this._showSoulOverlay();
        }
    }

    /** 显示灵魂状态遮罩提示 */
    _showSoulOverlay() {
        if (this._soulOverlay) return;
        const overlay = document.createElement('div');
        overlay.id = 'soul-overlay';
        overlay.style.cssText = [
            'position:fixed', 'top:0', 'left:0', 'width:100%', 'height:100%',
            'display:flex', 'flex-direction:column', 'align-items:center', 'justify-content:center',
            'pointer-events:none', 'z-index:999',
            'background:rgba(80,100,180,0.25)'
        ].join(';');
        overlay.innerHTML = `
            <div style="
                background:rgba(20,30,60,0.85);
                border:2px solid #7090ff;
                border-radius:12px;
                padding:24px 40px;
                text-align:center;
                color:#c0d0ff;
                font-family:sans-serif;
                box-shadow:0 0 30px rgba(80,120,255,0.5);
            ">
                <div style="font-size:28px;margin-bottom:8px;">👻 灵魂状态</div>
                <div style="font-size:16px;line-height:1.8;">
                    你已阵亡，现在是灵魂状态<br>
                    请前往<span style="color:#ffd060;font-weight:bold;">篝火</span>旁等待复活<br>
                    <span style="font-size:13px;color:#8090cc;">篝火每 60 秒复活附近的灵魂</span>
                </div>
            </div>`;
        document.body.appendChild(overlay);
        this._soulOverlay = overlay;
    }

    /** 隐藏灵魂状态遮罩 */
    _hideSoulOverlay() {
        if (this._soulOverlay) {
            this._soulOverlay.remove();
            this._soulOverlay = null;
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

        // 自己复活：隐藏灵魂状态提示
        if (data.char_id === this.selfId) {
            this._hideSoulOverlay();
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
        
        // 同步 NPC 状态
        if (data.npcs) {
            for (const npc of data.npcs) {
                const entity = this.npcEntities.get(npc.id);
                if (entity) {
                    const stats = entity.getComponent('stats');
                    if (stats) {
                        if (npc.hp !== undefined) stats.hp = npc.hp;
                        if (npc.max_hp !== undefined) stats.maxHp = npc.max_hp;
                    }
                    // NPC 死亡时直接移除
                    if (npc.dead) {
                        if (this.selectedTarget === npc.id) this.selectedTarget = null;
                        const idx = this.entities.indexOf(entity);
                        if (idx >= 0) this.entities.splice(idx, 1);
                        if (this.engine && this.engine.entityManager) {
                            this.engine.entityManager.removeEntity(entity);
                        }
                        this.npcEntities.delete(npc.id);
                    }
                }
            }
        }
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
            
            // 技能粒子效果
            if (this.particleSystem && transform) {
                this.createSkillParticles(data.skill_name, {
                    casterX: transform.position.x,
                    casterY: transform.position.y,
                    targetX: data.target_x,
                    targetY: data.target_y,
                    areaSize: data.area_size || 0
                });
            }
        }
    }

    // ===== NPC 系统 =====

    /**
     * 处理 NPC 刷新事件 - 创建 NPC 实体加入场景
     * @param {Object} data - { npcs: [{ id, name, template, level, x, y, hp, max_hp, attack, defense, speed, dead }] }
     */
    onNPCSpawn(data) {
        if (!data || !data.npcs) return;
        
        for (const npc of data.npcs) {
            // 已存在则跳过
            if (this.npcEntities.has(npc.id)) continue;
            
            const entity = this.entityFactory.createEnemy({
                id: `npc_${npc.id}`,
                templateId: npc.template,
                name: npc.name,
                level: npc.level,
                position: { x: npc.x, y: npc.y },
                stats: {
                    maxHp: npc.max_hp,
                    hp: npc.hp,
                    attack: npc.attack,
                    defense: npc.defense,
                    speed: npc.speed
                },
                aiType: 'passive'
            });
            
            // 存储后端 NPC ID（负数）
            entity.npcId = npc.id;
            entity.dead = npc.dead || false;
            if (entity.dead) {
                const sprite = entity.getComponent('sprite');
                if (sprite) { sprite.alpha = 0.3; sprite.isWalking = false; }
            }
            
            this.entities.push(entity);
            this.npcEntities.set(npc.id, entity);
        }
        
        console.log(`ArenaScene: 刷新 ${data.npcs.length} 个 NPC，当前 NPC 总数: ${this.npcEntities.size}`);
    }

    /**
     * 处理 NPC 死亡事件
     * @param {Object} data - { npc_id, killer }
     */
    onNPCDied(data) {
        const npcId = data.npc_id || data.id;
        const entity = this.npcEntities.get(npcId);
        if (!entity) return;
        
        // 显示击杀信息
        const transform = entity.getComponent('transform');
        if (transform && this.floatingTextManager) {
            this.floatingTextManager.addText(
                transform.position.x,
                transform.position.y - 40,
                `${entity.name} 被击杀`,
                '#ff8800'
            );
        }
        
        // 取消选中
        if (this.selectedTarget === npcId) {
            this.selectedTarget = null;
        }
        
        // 立即移除死亡 NPC 实体
        const idx = this.entities.indexOf(entity);
        if (idx >= 0) this.entities.splice(idx, 1);
        if (this.engine && this.engine.entityManager) {
            this.engine.entityManager.removeEntity(entity);
        }
        this.npcEntities.delete(npcId);
    }

    /**
     * 处理 NPC 位置更新（AI 移动）
     * @param {Object} data - { npcs: [{ id, x, y, direction }] }
     */
    onNPCUpdate(data) {
        if (!data || !data.npcs) return;
        
        for (const npcData of data.npcs) {
            const entity = this.npcEntities.get(npcData.id);
            if (!entity || entity.dead) continue;
            
            // 设置目标位置用于插值
            entity.targetX = npcData.x;
            entity.targetY = npcData.y;
            entity._lastMoveTime = Date.now();
            
            // 同步方向
            const sprite = entity.getComponent('sprite');
            if (sprite && npcData.direction) {
                sprite.direction = this._netToDir[npcData.direction] || npcData.direction;
                sprite.isWalking = true;
            }
        }
    }

    /**
     * 覆盖 handleEnemySelection - 支持点击选中远程玩家和 NPC
     * 父类 BaseGameScene 的 handleEnemySelection 为空（使用滑动攻击）
     * 竞技场需要点击选中目标
     */
    handleEnemySelection() {
        if (!this.inputManager || !this.playerEntity) return;
        
        // 只在鼠标点击且未被 UI 处理时选中
        if (!this.inputManager.isMouseClicked() || this.inputManager.isMouseClickHandled()) return;
        
        const mouseWorldPos = this.inputManager.getMouseWorldPosition(this.camera);
        if (!mouseWorldPos) return;
        
        const clickRange = 40; // 点击判定范围
        let closestEntity = null;
        let closestDist = clickRange;
        
        // 检查所有 NPC 和远程玩家
        const candidates = [
            ...Array.from(this.npcEntities.values()).map(e => ({ entity: e, id: e.npcId })),
            ...Array.from(this.remotePlayers.entries()).map(([id, e]) => ({ entity: e, id }))
        ];
        
        for (const { entity, id } of candidates) {
            if (entity.dead) continue;
            const transform = entity.getComponent('transform');
            if (!transform) continue;
            
            const dx = mouseWorldPos.x - transform.position.x;
            const dy = mouseWorldPos.y - transform.position.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            
            if (dist < closestDist) {
                closestDist = dist;
                closestEntity = { entity, id };
            }
        }
        
        if (closestEntity) {
            this.selectedTarget = closestEntity.id;
            // 标记点击已处理，防止移动系统响应
            this.inputManager.markMouseClickHandled();
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

        // 篝火复活范围圈（50px，虚线）
        ctx.save();
        ctx.setLineDash([6, 4]);
        ctx.strokeStyle = 'rgba(255, 200, 80, 0.5)';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.arc(x, y - 15, 50, 0, Math.PI * 2);
        ctx.stroke();
        ctx.setLineDash([]);
        ctx.restore();

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
            '猛击': '🪓',
            '旋风斩': '🌀',
            '战吼': '📢',
            '射击': '🏹',
            '多重射击': '🎯',
            '闪电箭': '⚡',
            '天降箭雨': '🌧️'
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
                id: def.icon_id || def.id,
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
                // 优先使用后端传来的武器属性，否则用职业默认值
                item.attackSpeed = eq.def.attack_interval || weaponProps.attackSpeed;
                item.attackRange = eq.def.attack_range || weaponProps.attackRange;
                item.attackDistance = eq.def.attack_distance || weaponProps.attackDistance;
                item.pierce = eq.def.pierce || 0;
                item.multiArrow = eq.def.multi_arrow || 0;
            }
            
            equipment.equip(frontendSlot, item);
        }
        
        console.log('ArenaScene: 加载后端装备完成', backendEquips.map(e => e.def?.name));
    }

    /**
     * 退出场景
     */
    /**
     * 创建技能粒子效果
     * @param {string} skillName - 技能名称
     * @param {Object} params - 参数 {casterX, casterY, targetX, targetY, areaSize}
     */
    createSkillParticles(skillName, params) {
            if (!this.particleSystem) return;
            const { casterX, casterY, targetX, targetY, areaSize } = params;

            switch (skillName) {
                case '猛击': {
                    // 血色粒子效果 - 模拟血溅射
                    this.particleSystem.emitBurst({
                        position: { x: targetX, y: targetY },
                        velocity: { x: 0, y: 0 },
                        life: 500,
                        size: 5,
                        color: '#cc0000',
                        alpha: 0.9,
                        gravity: 50,
                        friction: 0.92
                    }, 20, {
                        velocityRange: { min: 30, max: 100 },
                        angleRange: { min: 0, max: Math.PI * 2 },
                        sizeRange: { min: 3, max: 7 },
                        lifeRange: { min: 300, max: 600 }
                    });
                    break;
                }
                case '旋风斩': {
                    // 旋风粒子效果 - 围绕施法者旋转
                    const radius = areaSize || 80;
                    for (let i = 0; i < 24; i++) {
                        const angle = (i / 24) * Math.PI * 2;
                        this.particleSystem.emit({
                            position: { x: casterX + Math.cos(angle) * radius * 0.5, y: casterY + Math.sin(angle) * radius * 0.5 },
                            velocity: { x: Math.cos(angle + Math.PI / 2) * 100, y: Math.sin(angle + Math.PI / 2) * 100 },
                            life: 500,
                            size: 3 + Math.random() * 3,
                            color: '#88ccff',
                            alpha: 0.8,
                            gravity: 0,
                            friction: 0.95
                        });
                    }
                    break;
                }
                case '战吼': {
                    // 冲击波粒子 - 从施法者向外扩散
                    this.particleSystem.emitBurst({
                        position: { x: casterX, y: casterY },
                        velocity: { x: 0, y: 0 },
                        life: 600,
                        size: 5,
                        color: '#ffaa00',
                        alpha: 0.85,
                        gravity: 0,
                        friction: 0.93
                    }, 30, {
                        velocityRange: { min: 60, max: 120 },
                        angleRange: { min: 0, max: Math.PI * 2 },
                        sizeRange: { min: 3, max: 7 },
                        lifeRange: { min: 400, max: 700 }
                    });
                    break;
                }
                case '多重射击': {
                    // 5支箭射向目标区域
                    for (let i = 0; i < 5; i++) {
                        const offsetX = (Math.random() - 0.5) * (areaSize || 10) * 2;
                        const offsetY = (Math.random() - 0.5) * (areaSize || 10) * 2;
                        const dx = (targetX + offsetX) - casterX;
                        const dy = (targetY + offsetY) - casterY;
                        const dist = Math.sqrt(dx * dx + dy * dy) || 1;
                        const speed = 200;
                        // 每支箭3个粒子形成轨迹
                        for (let j = 0; j < 3; j++) {
                            this.particleSystem.emit({
                                position: { x: casterX, y: casterY },
                                velocity: { x: (dx / dist) * speed * (0.9 + j * 0.05), y: (dy / dist) * speed * (0.9 + j * 0.05) },
                                life: 400,
                                size: 3 - j * 0.5,
                                color: '#ffee44',
                                alpha: 0.9,
                                gravity: 0,
                                friction: 0.98
                            });
                        }
                    }
                    break;
                }
                case '闪电箭': {
                    // 雷电粒子效果 - 电弧 + 闪光
                    const dx = targetX - casterX;
                    const dy = targetY - casterY;
                    // 主电弧
                    for (let i = 0; i < 10; i++) {
                        const t = i / 10;
                        const px = casterX + dx * t + (Math.random() - 0.5) * 20;
                        const py = casterY + dy * t + (Math.random() - 0.5) * 20;
                        this.particleSystem.emit({
                            position: { x: px, y: py },
                            velocity: { x: (Math.random() - 0.5) * 60, y: (Math.random() - 0.5) * 60 },
                            life: 300 + Math.random() * 200,
                            size: 3 + Math.random() * 4,
                            color: '#44aaff',
                            alpha: 0.95,
                            gravity: 0,
                            friction: 0.9
                        });
                    }
                    // 目标点闪光爆炸
                    this.particleSystem.emitBurst({
                        position: { x: targetX, y: targetY },
                        velocity: { x: 0, y: 0 },
                        life: 350,
                        size: 4,
                        color: '#ffffff',
                        alpha: 1.0,
                        gravity: 0,
                        friction: 0.88
                    }, 18, {
                        velocityRange: { min: 40, max: 80 },
                        angleRange: { min: 0, max: Math.PI * 2 },
                        sizeRange: { min: 2, max: 6 },
                        lifeRange: { min: 250, max: 400 }
                    });
                    break;
                }
                case '天降箭雨': {
                    // 箭雨粒子 - 从天空落下到目标区域
                    const rainRadius = areaSize || 30;
                    for (let i = 0; i < 20; i++) {
                        const offsetX = (Math.random() - 0.5) * rainRadius * 2;
                        const offsetY = (Math.random() - 0.5) * rainRadius * 2;
                        const delay = Math.random() * 500;
                        setTimeout(() => {
                            if (!this.particleSystem) return;
                            // 下落的箭
                            this.particleSystem.emit({
                                position: { x: targetX + offsetX, y: targetY + offsetY - 100 },
                                velocity: { x: (Math.random() - 0.5) * 10, y: 150 + Math.random() * 50 },
                                life: 500,
                                size: 2 + Math.random() * 2,
                                color: '#ffcc33',
                                alpha: 0.9,
                                gravity: 80,
                                friction: 0.98
                            });
                        }, delay);
                    }
                    // 落地溅射效果
                    setTimeout(() => {
                        if (!this.particleSystem) return;
                        this.particleSystem.emitBurst({
                            position: { x: targetX, y: targetY },
                            velocity: { x: 0, y: 0 },
                            life: 300,
                            size: 3,
                            color: '#ff8833',
                            alpha: 0.7,
                            gravity: 40,
                            friction: 0.9
                        }, 15, {
                            velocityRange: { min: 20, max: 50 },
                            angleRange: { min: 0, max: Math.PI * 2 },
                            sizeRange: { min: 2, max: 4 },
                            lifeRange: { min: 200, max: 400 }
                        });
                    }, 400);
                    break;
                }
            }
        }


    /**
     * 处理 NPC 掉落物品
     * @param {Object} data - {npc_id, x, y, drop_type, drop_name, killer_id}
     */
    onNPCDrop(data) {
        if (!data) return;
        
        // 显示掉落文字
        if (this.floatingTextManager) {
            const color = data.drop_type === 'health_potion' ? '#ff4444' : '#4488ff';
            this.floatingTextManager.addText(
                data.x,
                data.y - 30,
                `掉落 ${data.drop_name}`,
                color
            );
        }
        
        // 掉落粒子效果
        if (this.particleSystem) {
            const color = data.drop_type === 'health_potion' ? '#ff3333' : '#3388ff';
            this.particleSystem.emitBurst({
                position: { x: data.x, y: data.y },
                velocity: { x: 0, y: -20 },
                life: 500,
                size: 4,
                color: color,
                alpha: 0.9,
                gravity: 20,
                friction: 0.92
            }, 10, {
                velocityRange: { min: 20, max: 50 },
                angleRange: { min: 0, max: Math.PI * 2 },
                sizeRange: { min: 3, max: 5 },
                lifeRange: { min: 400, max: 600 }
            });
        }
        
        // 如果是自己击杀的，自动拾取（恢复HP/MP）
        if (data.killer_id === this.selfId && this.playerEntity) {
            const stats = this.playerEntity.getComponent('stats');
            if (stats) {
                if (data.drop_type === 'health_potion') {
                    const heal = Math.floor(stats.maxHp * 0.2);
                    stats.hp = Math.min(stats.maxHp, stats.hp + heal);
                    if (this.floatingTextManager) {
                        const transform = this.playerEntity.getComponent('transform');
                        if (transform) {
                            this.floatingTextManager.addText(
                                transform.position.x,
                                transform.position.y - 20,
                                `+${heal} HP`,
                                '#44ff44'
                            );
                        }
                    }
                } else if (data.drop_type === 'mana_potion') {
                    const mana = Math.floor(stats.maxMp * 0.2);
                    stats.mp = Math.min(stats.maxMp, stats.mp + mana);
                    if (this.floatingTextManager) {
                        const transform = this.playerEntity.getComponent('transform');
                        if (transform) {
                            this.floatingTextManager.addText(
                                transform.position.x,
                                transform.position.y - 20,
                                `+${mana} MP`,
                                '#44aaff'
                            );
                        }
                    }
                }
            }
        }
    }

    exit() {
        this.remotePlayers.clear();
        this.npcEntities.clear();
        this.selectedTarget = null;
        this.skillRangeIndicators = [];
        this._hideSoulOverlay();
        super.exit();
    }
}
