/**
 * ArenaScene - 修罗斗场竞技场场景
 * 继承 BaseGameScene，复用引擎的等距地图、实体渲染、UI 面板
 * 通过 Go 后端 WebSocket 驱动多人对战
 */
import { BaseGameScene } from '../../prologue/scenes/BaseGameScene.js';
import { EntityFactory } from '../../ecs/EntityFactory.js';
import { NameComponent } from '../../ecs/components/NameComponent.js';
import { calcDamage, applyCrit, SKILL_PHASE, TARGET_MODE_FROM_STRING } from '../../ecs/ComponentTypes.js';
import { NetworkCombatSystem } from '../../systems/NetworkCombatSystem.js';
import { MultiplayerManager } from '../../core/MultiplayerManager.js';
import { SkillParticleEffects } from '../../rendering/SkillParticleEffects.js';
import { SkillRangeIndicator } from '../../rendering/SkillRangeIndicator.js';
import { StunEffectRenderer } from '../../rendering/StunEffectRenderer.js';
import { GroundDropRenderer } from '../../rendering/GroundDropRenderer.js';
import { BoneCorpseRenderer } from '../../rendering/BoneCorpseRenderer.js';
import { MoveTargetIndicator } from '../../rendering/MoveTargetIndicator.js';
import { GroundDropPickupSystem } from '../../systems/GroundDropPickupSystem.js';
import { EquipmentSyncSystem } from '../../systems/EquipmentSyncSystem.js';
import { SkillSyncSystem } from '../../systems/SkillSyncSystem.js';

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
            radius: 200, // 安全区半径（由后端下发）
            fireImage: null,
            imageLoaded: false,
            frameWidth: 658 / 4,
            frameHeight: 712 / 3,
            frameCols: 4,
            frameRows: 3,
            frameCount: 12,
            currentFrame: 0,
            frameTime: 0,
            frameDuration: 0.16,
            countdown: 60  // 复活倒计时（秒）
        };
        
        // 网络同步
        this.lastMoveTime = 0;
        this.moveInterval = 30; // ms
        
        // WebSocket 引用（由外部注入）
        this.ws = null;
        
        // 浮动伤害文字
        this.floatingTexts = [];
        
        // 技能范围指示器（委托 SkillRangeIndicator）
        this.skillRangeIndicator = new SkillRangeIndicator();
        // 兼容旧代码访问 skillRangeIndicators 数组
        this.skillRangeIndicators = this.skillRangeIndicator.indicators;

        // 昏迷/恐惧转圈效果渲染器
        this.stunEffectRenderer = new StunEffectRenderer();

        // 移动目标指示器
        this.moveTargetIndicator = new MoveTargetIndicator();
        this.moveTargetIndicators = this.moveTargetIndicator.indicators;
        
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

        // NPC 死亡白骨列表 [{x, y, life, maxLife}]
        this.boneCorpses = [];

        // 右键移动目标指示器 [{x, y, life, maxLife}]
        // （已由 moveTargetIndicator 管理，此处保留空数组兼容旧引用）

        // 地面掉落物品 [{x, y, life, maxLife, dropType, dropName, dropCount, killerId}]
        this.groundDrops = [];

        // 联网战斗系统
        this.networkCombat = new NetworkCombatSystem(this);

        // 多人实体管理器（远程玩家 + NPC 生命周期 + 位置插值）
        this.multiplayerManager = new MultiplayerManager(this, {
            directionMap: this._netToDir
        });
        // 让 remotePlayers / npcEntities 直接引用 manager 的 Map，保持外部访问兼容
        this.remotePlayers = this.multiplayerManager.remotePlayers;
        this.npcEntities = this.multiplayerManager.npcEntities;

        // 旋风斩持续状态
        this._whirlwindUntil = 0;
        this._whirlwindAreaSize = 0;
        this._whirlwindNextParticle = 0;
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
        
        // 联网模式：禁用 CombatSystem 的单机自动复活
        if (this.combatSystem) {
            this.combatSystem._arenaMode = true;
            // 键盘技能快捷键走联网路径
            this.combatSystem.onNetworkSkillCast = (skillId) => {
                this.castSkill(skillId);
            };
        }
        // 联网模式：禁用 MeleeAttackSystem 的单机箭矢消耗
        if (this.meleeAttackSystem) {
            this.meleeAttackSystem._arenaMode = true;
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
                if (data.campfire.radius) this.campfire.radius = data.campfire.radius;
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
        
        // 同步职业（后端 class 覆盖 BaseGameScene 硬编码的 'refugee'）
        if (serverData.class) {
            this.playerEntity.class = serverData.class;
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
     * 添加远程玩家实体（委托 MultiplayerManager）
     */
    addRemotePlayer(serverData) {
        this.multiplayerManager.addRemotePlayer(serverData);
    }

    /**
     * 移除远程玩家（委托 MultiplayerManager）
     */
    removeRemotePlayer(charId) {
        this.multiplayerManager.removeRemotePlayer(charId);
    }

    /**
     * 覆盖 update - 添加网络同步逻辑
     */
    update(deltaTime) {
        // 昏迷状态检查（战吼效果）
        const nowMs = Date.now();
        const isStunned = this.playerEntity && this.playerEntity.stunUntil && nowMs < this.playerEntity.stunUntil;

        // 恐惧状态检查（战吼恐惧效果）
        const isFeared = this.playerEntity && this.playerEntity.fearUntil && nowMs < this.playerEntity.fearUntil;

        // 恐惧期间自动远离施法者方向逃跑
        if (isFeared && this.playerEntity) {
            const transform = this.playerEntity.getComponent('transform');
            const stats = this.playerEntity.getComponent('stats');
            if (transform) {
                const speed = (stats && stats.speed) ? stats.speed : 120;
                const fearSpeed = speed * 1.5; // 恐惧时跑得更快
                const fdx = this.playerEntity.fearDirX || 0;
                const fdy = this.playerEntity.fearDirY || 0;
                transform.position.x += fdx * fearSpeed * deltaTime;
                transform.position.y += fdy * fearSpeed * deltaTime;
                // 边界限制
                transform.position.x = Math.max(-960 + 30, Math.min(960 - 30, transform.position.x));
                transform.position.y = Math.max(30, Math.min(600 - 30, transform.position.y));
                // 设置行走动画
                const sprite = this.playerEntity.getComponent('sprite');
                if (sprite) sprite.isWalking = true;
            }
        }

        // 发送移动到服务端（昏迷时不发送，恐惧移动需要同步位置）
        if (!isStunned) this.sendMovement();
        
        // 插值远程玩家位置 + NPC 位置（委托 MultiplayerManager）
        this.multiplayerManager.update(deltaTime);
        
        // 更新技能范围指示器
        this.skillRangeIndicator.update(deltaTime);

        // 更新昏迷/恐惧转圈旋转角
        this.stunEffectRenderer.update(deltaTime);

        // 旋风斩持续粒子（每秒触发一次；命中时由 onDamage 额外触发，共享防重复标志）
        if (this._whirlwindUntil > 0 && this.playerEntity && !this.playerEntity.dead) {
            const now = Date.now();
            if (now >= this._whirlwindUntil) {
                this._whirlwindUntil = 0;
            } else if (now >= this._whirlwindNextParticle) {
                this._whirlwindNextParticle = now + 1000;
                this._whirlwindLastParticleTime = now;
                const t = this.playerEntity.getComponent('transform');
                if (t) {
                    SkillParticleEffects.emitWhirlwind(this.particleSystem, t.position.x, t.position.y, this._whirlwindAreaSize);
                }
            }
        }

        // 更新白骨倒计时
        this.boneCorpses = this.boneCorpses.filter(b => {
            b.life -= deltaTime;
            return b.life > 0;
        });

        // 更新地面掉落物品（倒计时 + 按E键/左键拾取）
        if (this.playerEntity && !this.playerEntity.dead && this.inputManager) {
            const pt = this.playerEntity.getComponent('transform');
            if (pt) {
                const px = pt.position.x, py = pt.position.y;
                const ePressed = this.inputManager.isKeyPressed('e') || this.inputManager.isKeyPressed('E');
                const leftClicked = this.inputManager.isMouseClicked() && this.inputManager.getMouseButton() === 0;
                if (ePressed || leftClicked) {
                    let nearest = null, nearestDist = 60;
                    for (const drop of this.groundDrops) {
                        if (drop.picked) continue;
                        const dx = drop.x - px, dy = drop.y - py;
                        const dist = Math.sqrt(dx * dx + dy * dy);
                        if (dist < nearestDist) {
                            if (leftClicked) {
                                const mouseWorld = this.inputManager.getMouseWorldPosition(this.camera);
                                if (mouseWorld) {
                                    const mdx = drop.x - mouseWorld.x, mdy = drop.y - mouseWorld.y;
                                    if (Math.sqrt(mdx * mdx + mdy * mdy) > 30) continue;
                                }
                            }
                            nearest = drop;
                            nearestDist = dist;
                        }
                    }
                    if (nearest) {
                        GroundDropPickupSystem.pickup(this.playerEntity, nearest, this.floatingTextManager);
                        if (leftClicked) this.inputManager.markMouseClickHandled();
                    }
                }
            }
        }
        this.groundDrops = this.groundDrops.filter(d => {
            d.life -= deltaTime;
            return d.life > 0 && !d.picked;
        });

        // 检测右键点击 → 添加移动目标指示器
        if (this.inputManager && this.inputManager.isMouseClicked() && this.inputManager.getMouseButton() === 2) {
            const clickPos = this.inputManager.getMouseWorldPosition(this.camera);
            if (clickPos) this.moveTargetIndicator.show(clickPos.x, clickPos.y);
        }

        // 更新移动目标指示器倒计时
        this.moveTargetIndicator.update(deltaTime);
        
        // 空格键触发普攻（委托 NetworkCombatSystem，昏迷/恐惧时禁止）
        if (!isStunned && !isFeared) this.networkCombat.handleSpaceAttack();

        // 更新火堆动画
        this.updateCampfireAnimation(deltaTime);
        
        // 安全区内禁用单机战斗系统（MeleeAttackSystem 滑动攻击 + CombatSystem 技能输入）
        let safeZoneDisabled = false;
        if (this.playerEntity) {
            const t = this.playerEntity.getComponent('transform');
            if (t && this.isInSafeZone(t.position.x, t.position.y)) {
                safeZoneDisabled = true;
            }
        }
        // 昏迷/恐惧时同样禁用单机战斗系统
        if (safeZoneDisabled || isStunned || isFeared) {
            if (this.meleeAttackSystem) this.meleeAttackSystem._safeZoneDisabled = true;
            if (this.combatSystem) this.combatSystem._safeZoneDisabled = true;
        }
        
        // 调用父类 update
        super.update(deltaTime);
        
        // 恢复单机战斗系统
        if (safeZoneDisabled || isStunned || isFeared) {
            if (this.meleeAttackSystem) this.meleeAttackSystem._safeZoneDisabled = false;
            if (this.combatSystem) this.combatSystem._safeZoneDisabled = false;
        }
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
    // 判断坐标是否在篝火安全区内
    isInSafeZone(x, y) {
        const dx = x - this.campfire.x;
        const dy = y - this.campfire.y;
        const rx = this.campfire.radius;
        const ry = this.campfire.radius / 2; // 2.5D 椭圆：垂直 = 水平 / 2
        return (dx * dx) / (rx * rx) + (dy * dy) / (ry * ry) <= 1;
    }

    /**
     * 自动选中最近的可攻击敌人（NPC 或远程玩家）
     */
    _autoSelectNearestEnemy() {
        this.networkCombat.autoSelectNearestEnemy();
    }


    attackTarget() {
        this.networkCombat.attackTarget();
    }

    /**
     * 释放技能（委托 NetworkCombatSystem）
     */
    castSkill(skillId) {
        this.networkCombat.castSkill(skillId);
    }

    // ===== 网络事件处理 =====

    onPlayerJoined(data) {
        this.addRemotePlayer(data);
    }

    onPlayerLeft(data) {
        this.removeRemotePlayer(data.char_id);
    }

    onPlayerMoved(data) {
        this.multiplayerManager.onPlayerMoved(data);
    }


    /**
     * 处理伤害事件（委托 NetworkCombatSystem）
     */
    onDamage(data) {
        this.networkCombat.onDamage(data);
        // 战吼命中：给目标实体设置转圈效果
        if (data.skill_name === '战吼') {
            let targetEntity;
            if (data.target_is_npc) {
                targetEntity = this.npcEntities.get(data.target_id);
            } else {
                targetEntity = data.target_id === this.selfId
                    ? this.playerEntity
                    : this.remotePlayers.get(data.target_id);
            }
            if (targetEntity && !targetEntity.dead) {
                targetEntity.stunEffectUntil = Date.now() + 3000;
            }
        }
    }

    onPlayerDied(data) {
        const entity = data.char_id === this.selfId
            ? this.playerEntity
            : this.remotePlayers.get(data.char_id);
        if (entity) {
            entity.dead = true;
            // 同时设置 isDead/isDying 阻止 CombatSystem 的单机复活逻辑
            entity.isDead = true;
            entity.isDying = true;
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

        // 自己死亡：清除选中目标、停止打坐、显示灵魂状态提示
        if (data.char_id === this.selfId) {
            this.selectedTarget = null;
            if (this.meditationSystem && this.meditationSystem.isActive()) {
                this.meditationSystem.stop();
            }
            this._showSoulOverlay();
        }
    }

    /** 显示灵魂状态遮罩提示 */
    _showSoulOverlay() {
        if (this._soulOverlay) return;
        const overlay = document.createElement('div');
        overlay.id = 'soul-overlay';
        overlay.style.cssText = [
            'position:fixed', 'top:0', 'left:0', 'width:100%',
            'display:flex', 'justify-content:center',
            'padding-top:24px',
            'pointer-events:none', 'z-index:999'
        ].join(';');
        overlay.innerHTML = `
            <div style="
                background:rgba(20,30,60,0.92);
                border:2px solid #7090ff;
                border-radius:10px;
                padding:16px 32px;
                text-align:center;
                color:#c0d0ff;
                font-family:sans-serif;
                box-shadow:0 4px 24px rgba(80,120,255,0.5);
                pointer-events:auto;
                display:flex;
                align-items:center;
                gap:20px;
            ">
                <div>
                    <span style="font-size:20px;">👻</span>
                    <span style="font-size:16px;font-weight:bold;margin-left:8px;">灵魂状态</span>
                    <span style="font-size:14px;color:#a0b0e0;margin-left:12px;">
                        你已阵亡，请前往 <span style="color:#ffd060;font-weight:bold;">篝火</span> 旁等待复活
                    </span>
                </div>
                <button id="soul-overlay-confirm" style="
                    background:#3a5acc;
                    border:1px solid #7090ff;
                    border-radius:6px;
                    color:#e0eaff;
                    font-size:14px;
                    padding:6px 18px;
                    cursor:pointer;
                    white-space:nowrap;
                ">知道了</button>
            </div>`;
        document.body.appendChild(overlay);
        this._soulOverlay = overlay;
        overlay.querySelector('#soul-overlay-confirm').addEventListener('click', () => {
            this._hideSoulOverlay();
        });
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
            entity.isDead = false;
            entity.isDying = false;
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

            // 复活银白色粒子光环
            this._spawnRespawnParticles(data.x, data.y);
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

        // 自己的状态同步（位置由本地控制）
        for (const p of data.players) {
            if (p.char_id !== this.selfId) continue;
            if (!this.playerEntity) continue;
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
                    this.playerEntity.isDead = true;
                    this.playerEntity.isDying = true;
                    const sprite = this.playerEntity.getComponent('sprite');
                    if (sprite) { sprite.alpha = 0.3; sprite.isWalking = false; }
                } else if (!p.dead && this.playerEntity.dead) {
                    this.playerEntity.dead = false;
                    this.playerEntity.isDead = false;
                    this.playerEntity.isDying = false;
                    const sprite = this.playerEntity.getComponent('sprite');
                    if (sprite) sprite.alpha = 1.0;
                }
            }
        }

        // 远程玩家 + NPC 同步（委托 MultiplayerManager）
        this.multiplayerManager.applyStateSync(
            data,
            this.selfId,
            (p) => this.addRemotePlayer(p),
            this
        );
    }

    onSkillCasted(data) {
        console.log('[ArenaScene] onSkillCasted received:', data.skill_name, 'caster=', data.caster_id, 'area_type=', data.area_type);
        this.networkCombat.onSkillCasted(data);
        // 战吼恐惧：给目标玩家设置转圈效果
        if (data.skill_name === '战吼_fear') {
            const targetEntity = data.target_id === this.selfId
                ? this.playerEntity
                : this.remotePlayers.get(data.target_id);
            if (targetEntity && !targetEntity.dead) {
                targetEntity.stunEffectUntil = Date.now() + 3000;
            }
        }
    }

    // ===== NPC 系统 =====

    /**
     * 处理 NPC 刷新事件 - 创建 NPC 实体加入场景
     * @param {Object} data - { npcs: [{ id, name, template, level, x, y, hp, max_hp, attack, defense, speed, dead }] }
     */
    onNPCSpawn(data) {
        this.multiplayerManager.onNPCSpawn(data);
    }

    /**
     * 处理 NPC 死亡事件（委托 MultiplayerManager）
     */
    onNPCDied(data) {
        this.multiplayerManager.onNPCDied(data, { boneCorpses: this.boneCorpses });
    }

    /**
     * 处理篝火倒计时广播
     */
    onCampfireTick(data) {
        if (!data || data.countdown === undefined) return;
        if (this.campfire) {
            this.campfire.countdown = data.countdown;
        }
    }

    /**
     * 处理 NPC 位置更新（委托 MultiplayerManager）
     */
    onNPCUpdate(data) {
        this.multiplayerManager.onNPCUpdate(data);
    }

    /**
     * 覆盖 handleEnemySelection - 支持点击选中远程玩家和 NPC
     * 父类 BaseGameScene 的 handleEnemySelection 为空（使用滑动攻击）
     * 竞技场需要点击选中目标
     */
    handleEnemySelection() {
        this.networkCombat.handleEnemySelection();
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

        // 添加白骨
        for (const bone of this.boneCorpses) {
            renderQueue.push({
                type: 'bone',
                y: bone.y,
                render: () => BoneCorpseRenderer.render(ctx, bone)
            });
        }

        // 添加地面掉落物品
        for (const drop of this.groundDrops) {
            if (!drop.picked) {
                renderQueue.push({
                    type: 'drop',
                    y: drop.y,
                    render: () => GroundDropRenderer.render(ctx, drop)
                });
            }
        }

        // 添加移动目标指示器
        for (const m of this.moveTargetIndicator.indicators) {
            renderQueue.push({
                type: 'moveTarget',
                y: m.y,
                render: () => MoveTargetIndicator.renderOne(ctx, m)
            });
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

        // 技能范围虚线（跟随玩家，始终在最上层）
        this.skillRangeIndicator.render(ctx, this._getFootCenter());

        // NPC 武器攻击动画（已改用刀光特效，EnemyWeaponRenderer 已禁用）
        // if (this.enemyWeaponRenderer) {
        //     for (const entity of this.npcEntities.values()) {
        //         if (!entity.dead && !entity.isDead) {
        //             this.enemyWeaponRenderer.render(ctx, entity, this.playerEntity);
        //         }
        //     }
        // }

        // 昏迷/恐惧转圈效果（在所有实体上层绘制）
        const allEntities = [
            ...(this.playerEntity ? [this.playerEntity] : []),
            ...Array.from(this.remotePlayers.values()),
            ...Array.from(this.npcEntities.values())
        ];
        this.stunEffectRenderer.render(ctx, allEntities);
    }

    /**
     * 渲染技能范围虚线指示器（委托 SkillRangeIndicator）
     * @deprecated 直接在 renderWorldObjects 中调用 skillRangeIndicator.render()
     */
    _renderSkillRangeIndicator(ctx) {
        this.skillRangeIndicator.render(ctx, this._getFootCenter());
    }

    /**
     * 触发技能范围指示器显示（委托 SkillRangeIndicator）
     */
    _showSkillRange(opts) {
        this.skillRangeIndicator.show(opts);
    }

    /**
     * 获取玩家脚下圆心坐标（sprite 底部 1/10 位置）
     */
    _getFootCenter() {
        if (!this.playerEntity) return null;
        const transform = this.playerEntity.getComponent('transform');
        if (!transform) return null;
        const sprite = this.playerEntity.getComponent('sprite');
        const h = sprite?.height || 64;
        return {
            x: transform.position.x,
            y: transform.position.y - h / 10
        };
    }

    /**
     * 渲染昏迷/恐惧转圈效果（委托 StunEffectRenderer）
     */
    _renderStunEffects(ctx) {
        const allEntities = [
            ...(this.playerEntity ? [this.playerEntity] : []),
            ...Array.from(this.remotePlayers.values()),
            ...Array.from(this.npcEntities.values())
        ];
        this.stunEffectRenderer.render(ctx, allEntities);
    }

    /**
     * 发射旋风斩粒子（委托 SkillParticleEffects）
     */
    _emitWhirlwindParticles(cx, cy, radius) {
        SkillParticleEffects.emitWhirlwind(this.particleSystem, cx, cy, radius);
    }

    /** 渲染 NPC 死亡白骨（委托 BoneCorpseRenderer） */
    _renderBoneCorpse(ctx, bone) {
        BoneCorpseRenderer.render(ctx, bone);
    }

    /** 渲染移动目标指示器（委托 MoveTargetIndicator） */
    _renderMoveTargetIndicator(ctx, m) {
        MoveTargetIndicator.renderOne(ctx, m);
    }

    /**
     * 渲染火堆
     */
    renderCampfire(ctx) {
        const x = this.campfire.x;
        const y = this.campfire.y;

        // 篝火安全区范围圈 - 2.5D 等距椭圆
        ctx.save();
        const rx = this.campfire.radius;
        const ry = this.campfire.radius / 2; // 2.5D：垂直方向压缩为水平的一半
        ctx.setLineDash([8, 5]);
        ctx.strokeStyle = 'rgba(255, 200, 80, 0.55)';
        ctx.lineWidth = 1.8;
        ctx.beginPath();
        ctx.ellipse(x, y - 10, rx, ry, 0, 0, Math.PI * 2);
        ctx.stroke();
        // 椭圆内填充淡光晕
        const ringGrad = ctx.createRadialGradient(x, y - 10, 0, x, y - 10, rx);
        ringGrad.addColorStop(0, 'rgba(255, 200, 80, 0.06)');
        ringGrad.addColorStop(0.7, 'rgba(255, 150, 30, 0.04)');
        ringGrad.addColorStop(1, 'rgba(255, 100, 0, 0)');
        ctx.fillStyle = ringGrad;
        ctx.beginPath();
        ctx.ellipse(x, y - 10, rx, ry, 0, 0, Math.PI * 2);
        ctx.fill();
        // "安全区" 文字标识
        ctx.font = '11px sans-serif';
        ctx.fillStyle = 'rgba(255, 220, 120, 0.5)';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText('— 安全区 —', x, y - 10 - ry - 8);
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

        // 倒计时显示（仅死亡状态可见）
        const playerEntity = this.entityManager?.getEntity(this.playerEntityId);
        if (!playerEntity || !playerEntity.dead) return;
        const countdown = this.campfire.countdown !== undefined ? this.campfire.countdown : 60;
        const fireTopY = y - 75; // 火焰顶部上方
        ctx.save();
        // 背景胶囊
        const cdText = `复活 ${countdown}s`;
        ctx.font = 'bold 13px sans-serif';
        const tw = ctx.measureText(cdText).width;
        const padX = 8, padY = 4;
        const bgX = x - tw / 2 - padX;
        const bgY = fireTopY - 18;
        const bgW = tw + padX * 2;
        const bgH = 22;
        const r = bgH / 2;
        ctx.fillStyle = 'rgba(0, 0, 0, 0.55)';
        ctx.beginPath();
        ctx.moveTo(bgX + r, bgY);
        ctx.arcTo(bgX + bgW, bgY, bgX + bgW, bgY + bgH, r);
        ctx.arcTo(bgX + bgW, bgY + bgH, bgX, bgY + bgH, r);
        ctx.arcTo(bgX, bgY + bgH, bgX, bgY, r);
        ctx.arcTo(bgX, bgY, bgX + bgW, bgY, r);
        ctx.closePath();
        ctx.fill();
        // 文字颜色：最后10秒变红闪烁
        if (countdown <= 10) {
            const blink = Math.floor(Date.now() / 400) % 2 === 0;
            ctx.fillStyle = blink ? '#ff4444' : '#ffaa44';
        } else {
            ctx.fillStyle = '#ffe066';
        }
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(cdText, x, bgY + bgH / 2);
        ctx.restore();
    }

    /**
     * 覆盖父类药水使用，发送消息给后端同步 HP/MP
     */
    usePotionFromHotbar(potionType) {
        if (!this.playerEntity || this.playerEntity.dead) return;
        // 调用父类逻辑（从背包扣除药水 + 本地恢复 HP/MP）
        super.usePotionFromHotbar(potionType);
        // 通知后端同步 HP/MP
        if (this.ws) {
            this.ws.send('use_potion', { potion_type: potionType });
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
    /**
     * 将后端技能注入到 playerEntity 的 combat 组件（委托 SkillSyncSystem）
     */
    injectBackendSkills() {
        SkillSyncSystem.loadFromBackend(this.playerEntity, this.skills);
    }

    /**
     * 将后端装备数据加载到前端 EquipmentComponent（委托 EquipmentSyncSystem）
     */
    loadBackendEquipments(backendEquips) {
        EquipmentSyncSystem.loadFromBackend(this.playerEntity, backendEquips);
    }

    /**
     * 创建技能粒子效果（委托 SkillParticleEffects）
     */
    createSkillParticles(skillName, params) {
        SkillParticleEffects.emit(this.particleSystem, skillName, params);
        // 旋风斩：额外设置持续状态
        if (skillName === '旋风斩' && params.isSelf) {
            const now = Date.now();
            this._whirlwindUntil = now + 5000;
            this._whirlwindAreaSize = params.areaSize || 80;
            this._whirlwindNextParticle = now + 600;
        }
    }


    /**
     * 处理 NPC 掉落物品
     */
    onNPCDrop(data) {
        if (!data) return;

        this.groundDrops.push({
            x: data.x + (Math.random() - 0.5) * 30,
            y: data.y + (Math.random() - 0.5) * 15,
            life: 15,
            maxLife: 15,
            dropType: data.drop_type,
            dropName: data.drop_name,
            dropCount: data.drop_count || 1,
            killerId: data.killer_id,
            picked: false
        });

        SkillParticleEffects.emitDropAppear(this.particleSystem, data.x, data.y, data.drop_type);
    }

    /** 拾取地面掉落物品（委托 GroundDropPickupSystem） */
    _pickupGroundDrop(drop) {
        GroundDropPickupSystem.pickup(this.playerEntity, drop, this.floatingTextManager);
    }

    /** 渲染地面掉落物品（委托 GroundDropRenderer） */
    _renderGroundDrop(ctx, drop) {
        GroundDropRenderer.render(ctx, drop);
    }

    /** 复活粒子光环（委托 SkillParticleEffects） */
    _spawnRespawnParticles(x, y) {
        SkillParticleEffects.emitRespawn(this.particleSystem, x, y);
    }


    // 覆写轻功：安全区内禁止使用
    handleTeleport() {
        if (this.playerEntity) {
            const t = this.playerEntity.getComponent('transform');
            if (t && this.isInSafeZone(t.position.x, t.position.y)) {
                // 仍需消费点击事件，避免触发移动
                if (this.inputManager && this.inputManager.isCtrlClick()) {
                    this.inputManager.markMouseClickHandled();
                    if (this.floatingTextManager) {
                        this.floatingTextManager.addText(
                            t.position.x, t.position.y - 20,
                            '安全区内禁止轻功', '#ffaa00'
                        );
                    }
                }
                return;
            }
        }
        super.handleTeleport();
    }

    // 覆写武器投掷：安全区内禁止使用
    handleWeaponThrow() {
        if (this.playerEntity) {
            const t = this.playerEntity.getComponent('transform');
            if (t && this.isInSafeZone(t.position.x, t.position.y)) {
                if (this.floatingTextManager) {
                    this.floatingTextManager.addText(
                        t.position.x, t.position.y - 20,
                        '安全区内禁止投掷', '#ffaa00'
                    );
                }
                return;
            }
        }
        super.handleWeaponThrow();
    }

    exit() {
        this.multiplayerManager.clear();
        this.selectedTarget = null;
        this.skillRangeIndicator.clear();
        this.moveTargetIndicator.clear();
        this.boneCorpses = [];
        this.groundDrops = [];
        this._whirlwindUntil = 0;
        this._whirlwindNextParticle = 0;
        this._hideSoulOverlay();
        if (this.combatSystem) {
            this.combatSystem._arenaMode = false;
            this.combatSystem.onNetworkSkillCast = null;
        }
        if (this.meleeAttackSystem) {
            this.meleeAttackSystem._arenaMode = false;
        }
        super.exit();
    }
}
