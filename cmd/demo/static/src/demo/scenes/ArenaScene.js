/**
 * ArenaScene - 修罗斗场竞技场场景
 * 继承 BaseGameScene，复用引擎的等距地图、实体渲染、UI 面板
 * 通过 Go 后端 WebSocket 驱动多人对战
 */
import { BaseGameScene } from '../../prologue/scenes/BaseGameScene.js';
import { EntityFactory } from '../../ecs/EntityFactory.js';
import { NameComponent } from '../../ecs/components/NameComponent.js';

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
        this.moveInterval = 50; // ms
        
        // WebSocket 引用（由外部注入）
        this.ws = null;
        
        // 浮动伤害文字
        this.floatingTexts = [];
        
        // 技能范围指示器
        this.skillRangeIndicators = [];
        
        // 覆盖 canvas ID（使用 engineCanvas 而非 gameCanvas）
        this.canvasId = 'engineCanvas';
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
                this.campfire.x = data.campfire.x || 400;
                this.campfire.y = data.campfire.y || 300;
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
                speed: serverData.speed
            }
        });
        
        // 设置当前 HP/MP
        const stats = entity.getComponent('stats');
        if (stats) {
            stats.hp = serverData.hp;
            stats.mp = serverData.mp;
            stats.maxMp = serverData.max_mp;
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
        
        // 插值远程玩家位置
        for (const [id, entity] of this.remotePlayers) {
            if (entity.targetX !== undefined) {
                const transform = entity.getComponent('transform');
                if (transform) {
                    transform.position.x += (entity.targetX - transform.position.x) * 0.3;
                    transform.position.y += (entity.targetY - transform.position.y) * 0.3;
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
        
        // 检测是否有移动输入
        if (!this.inputManager) return;
        const hasInput = this.inputManager.isKeyDown('w') || this.inputManager.isKeyDown('s') ||
                         this.inputManager.isKeyDown('a') || this.inputManager.isKeyDown('d') ||
                         this.inputManager.isKeyDown('arrowup') || this.inputManager.isKeyDown('arrowdown') ||
                         this.inputManager.isKeyDown('arrowleft') || this.inputManager.isKeyDown('arrowright');
        
        if (hasInput) {
            let direction = 'down';
            if (this.inputManager.isKeyDown('w') || this.inputManager.isKeyDown('arrowup')) direction = 'up';
            if (this.inputManager.isKeyDown('s') || this.inputManager.isKeyDown('arrowdown')) direction = 'down';
            if (this.inputManager.isKeyDown('a') || this.inputManager.isKeyDown('arrowleft')) direction = 'left';
            if (this.inputManager.isKeyDown('d') || this.inputManager.isKeyDown('arrowright')) direction = 'right';
            
            this.ws.send('move', {
                x: transform.position.x,
                y: transform.position.y,
                direction: direction
            });
            this.lastMoveTime = now;
        }
    }

    /**
     * 攻击选中目标
     */
    attackTarget() {
        if (!this.selectedTarget || !this.ws) return;
        const entity = this.remotePlayers.get(this.selectedTarget);
        if (!entity || entity.dead) return;
        this.ws.send('attack', { target_id: this.selectedTarget });
    }

    /**
     * 释放技能
     */
    castSkill(skillId) {
        if (!this.ws || !this.playerEntity) return;
        
        const now = Date.now();
        const cd = this.skillCooldowns[skillId];
        if (cd && now < cd) return;
        
        const transform = this.playerEntity.getComponent('transform');
        if (!transform) return;
        
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
            }
        }
        
        this.ws.send('cast_skill', {
            skill_id: skillId,
            target_id: targetId,
            target_x: targetX,
            target_y: targetY
        });
        
        const skill = this.skills.find(s => s.id === skillId);
        if (skill) {
            this.skillCooldowns[skillId] = now + skill.cooldown * 1000;
            
            // 同步冷却到 combat 组件，让 BottomControlBar 显示冷却遮罩
            const combat = this.playerEntity.getComponent('combat');
            if (combat) {
                const combatSkillId = `backend_${skillId}`;
                combat.skillCooldowns.set(combatSkillId, now);
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
        if (entity) {
            entity.targetX = data.x;
            entity.targetY = data.y;
        }
    }

    onDamage(data) {
        // 更新目标 HP
        const targetEntity = data.target_id === this.selfId
            ? this.playerEntity
            : this.remotePlayers.get(data.target_id);
        
        if (targetEntity) {
            const stats = targetEntity.getComponent('stats');
            if (stats) {
                stats.hp = data.target_hp;
                stats.maxHp = data.target_max_hp;
            }
            
            // 浮动伤害文字
            const transform = targetEntity.getComponent('transform');
            if (transform && this.floatingTextManager) {
                const color = data.is_crit ? '#ffd700' : '#ff0000';
                const text = data.is_crit ? `暴击! ${Math.round(data.damage)}` : `${Math.round(data.damage)}`;
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
        }
        if (this.floatingTextManager) {
            this.floatingTextManager.addText(400, 300, `${data.name} 被 ${data.killer} 击杀`, '#ff0000');
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
        }
        if (this.floatingTextManager) {
            this.floatingTextManager.addText(data.x, data.y, `${data.name} 复活了`, '#00ff00');
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
        
        // 扩大槽位尺寸：40 -> 64
        const slotSize = 64;
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
        
        // 更新血蓝球位置
        this.bottomControlBar.hpOrb.x = 60;
        this.bottomControlBar.mpOrb.x = barWidth - 60;
        
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
     * 退出场景
     */
    exit() {
        this.remotePlayers.clear();
        this.selectedTarget = null;
        this.skillRangeIndicators = [];
        super.exit();
    }
}
