/**
 * MultiplayerManager - 多人场景实体管理器
 *
 * 负责远程玩家和 NPC 的生命周期管理与位置插值，
 * 将通用多人逻辑从具体场景中解耦，让场景只需传参调用。
 *
 * 使用方式：
 *   this.multiplayerManager = new MultiplayerManager(this, {
 *       directionMap: { 'u': 'up', 'd': 'down', ... }
 *   });
 *   // 在 update 中调用：
 *   this.multiplayerManager.update(deltaTime);
 */

import { NameComponent } from '../ecs/components/NameComponent.js';

export class MultiplayerManager {
    /**
     * @param {Object} scene - 场景实例，需提供 entities, entityFactory, floatingTextManager 等
     * @param {Object} opts
     * @param {Object} opts.directionMap - 网络方向简写 -> 引擎方向名映射
     */
    constructor(scene, opts = {}) {
        this.scene = scene;

        /** 远程玩家 Map: charId -> entity */
        this.remotePlayers = new Map();
        /** NPC 实体 Map: npcId -> entity */
        this.npcEntities = new Map();

        // 网络方向简写 -> 引擎方向名
        this._netToDir = opts.directionMap || {
            'u': 'up', 'd': 'down', 'l': 'left', 'r': 'right',
            'ul': 'up-left', 'ur': 'up-right', 'dl': 'down-left', 'dr': 'down-right'
        };
    }

    // ─────────────────────────────────────────────
    // 远程玩家管理
    // ─────────────────────────────────────────────

    /**
     * 添加远程玩家实体
     * @param {Object} serverData - 服务端玩家数据
     */
    addRemotePlayer(serverData) {
        if (this.remotePlayers.has(serverData.char_id)) return;

        const entity = this.scene.entityFactory.createPlayer({
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

        const stats = entity.getComponent('stats');
        if (stats) {
            stats.hp = serverData.hp;
            stats.mp = serverData.mp;
            stats.maxMp = serverData.max_mp;
        }

        // 竞技场中远程玩家设为敌对阵营
        entity.faction = 'enemy';
        const combat = entity.getComponent('combat');
        if (combat) combat.faction = 'enemy';

        entity.isRemote = true;
        entity.charId = serverData.char_id;
        entity.dead = serverData.dead || false;
        entity.targetX = serverData.x;
        entity.targetY = serverData.y;

        entity.addComponent(new NameComponent(serverData.name, {
            color: '#ffffff',
            fontSize: 14,
            offsetY: -10
        }));

        this.scene.entities.push(entity);
        this.remotePlayers.set(serverData.char_id, entity);
    }

    /**
     * 移除远程玩家
     * @param {number} charId
     */
    removeRemotePlayer(charId) {
        const entity = this.remotePlayers.get(charId);
        if (entity) {
            const idx = this.scene.entities.indexOf(entity);
            if (idx >= 0) this.scene.entities.splice(idx, 1);
            this.remotePlayers.delete(charId);
        }
        // 通知场景清除选中目标
        if (this.scene.selectedTarget === charId) {
            this.scene.selectedTarget = null;
        }
    }

    /**
     * 处理远程玩家移动消息
     * @param {Object} data - { char_id, x, y, direction }
     */
    onPlayerMoved(data) {
        const entity = this.remotePlayers.get(data.char_id);
        if (!entity) return;

        entity.targetX = data.x;
        entity.targetY = data.y;

        const sprite = entity.getComponent('sprite');
        if (sprite) {
            if (data.direction) {
                sprite.direction = this._netToDir[data.direction] || data.direction;
            }
            sprite.isWalking = true;
        }
        entity._lastMoveTime = Date.now();
    }

    // ─────────────────────────────────────────────
    // NPC 管理
    // ─────────────────────────────────────────────

    /**
     * 处理 NPC 刷新消息
     * @param {Object} data - { npcs: [...] }
     */
    onNPCSpawn(data) {
        if (!data || !data.npcs) return;

        for (const npc of data.npcs) {
            if (this.npcEntities.has(npc.id)) continue;

            const entity = this.scene.entityFactory.createEnemy({
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

            entity.npcId = npc.id;
            entity.dead = npc.dead || false;
            entity.isDead = entity.dead;
            entity.isDying = entity.dead;
            if (entity.dead) {
                const sprite = entity.getComponent('sprite');
                if (sprite) { sprite.alpha = 0.3; sprite.isWalking = false; }
            }

            this.scene.entities.push(entity);
            this.npcEntities.set(npc.id, entity);
        }

        console.log(`MultiplayerManager: 刷新 ${data.npcs.length} 个 NPC，当前总数: ${this.npcEntities.size}`);
    }

    /**
     * 处理 NPC 死亡消息
     * @param {Object} data - { npc_id/id, killer }
     * @param {Object} opts
     * @param {Array}  opts.boneCorpses - 白骨列表（可选，由场景传入）
     */
    onNPCDied(data, opts = {}) {
        const npcId = data.npc_id || data.id;
        const entity = this.npcEntities.get(npcId);
        if (!entity) return;

        entity.dead = true;
        entity.isDead = true;
        entity.isDying = true;

        const transform = entity.getComponent('transform');
        if (transform && this.scene.floatingTextManager) {
            this.scene.floatingTextManager.addText(
                transform.position.x,
                transform.position.y - 40,
                `${entity.name} 被击杀`,
                '#ff8800'
            );
        }

        if (this.scene.selectedTarget === npcId) {
            this.scene.selectedTarget = null;
        }

        // 记录白骨（由调用方传入列表）
        if (opts.boneCorpses && transform) {
            opts.boneCorpses.push({
                x: transform.position.x,
                y: transform.position.y,
                life: 10,
                maxLife: 10
            });
        }

        const idx = this.scene.entities.indexOf(entity);
        if (idx >= 0) this.scene.entities.splice(idx, 1);
        if (this.scene.engine && this.scene.engine.entityManager) {
            this.scene.engine.entityManager.removeEntity(entity);
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

            entity.targetX = npcData.x;
            entity.targetY = npcData.y;
            entity._lastMoveTime = Date.now();

            const sprite = entity.getComponent('sprite');
            if (sprite && npcData.direction) {
                sprite.direction = this._netToDir[npcData.direction] || npcData.direction;
                sprite.isWalking = true;
            }
        }
    }

    // ─────────────────────────────────────────────
    // 位置插值（每帧调用）
    // ─────────────────────────────────────────────

    /**
     * 更新所有远程实体的位置插值
     * @param {number} deltaTime - 帧时间（秒）
     */
    update(deltaTime) {
        this._interpolateRemotePlayers(deltaTime);
        this._interpolateNPCs(deltaTime);
    }

    _interpolateRemotePlayers(deltaTime) {
        for (const [, entity] of this.remotePlayers) {
            if (entity.targetX === undefined) continue;
            const transform = entity.getComponent('transform');
            if (!transform) continue;

            const dx = entity.targetX - transform.position.x;
            const dy = entity.targetY - transform.position.y;
            const dist = Math.sqrt(dx * dx + dy * dy);

            if (dist > 300) {
                transform.position.x = entity.targetX;
                transform.position.y = entity.targetY;
            } else if (dist < 1) {
                transform.position.x = entity.targetX;
                transform.position.y = entity.targetY;
            } else {
                const lerp = 1 - Math.pow(0.9, deltaTime * 60);
                transform.position.x += dx * lerp;
                transform.position.y += dy * lerp;
            }

            const sprite = entity.getComponent('sprite');
            if (sprite) {
                const timeSinceMove = Date.now() - (entity._lastMoveTime || 0);
                sprite.isWalking = dist > 2 && timeSinceMove < 200;
            }
        }
    }

    _interpolateNPCs(deltaTime) {
        for (const [, entity] of this.npcEntities) {
            if (entity.dead || entity.targetX === undefined) continue;
            const transform = entity.getComponent('transform');
            if (!transform) continue;

            const dx = entity.targetX - transform.position.x;
            const dy = entity.targetY - transform.position.y;
            const dist = Math.sqrt(dx * dx + dy * dy);

            if (dist > 400) {
                transform.position.x = entity.targetX;
                transform.position.y = entity.targetY;
            } else if (dist < 0.5) {
                transform.position.x = entity.targetX;
                transform.position.y = entity.targetY;
            } else {
                const stats = entity.getComponent('stats');
                const speed = (stats && stats.speed) ? stats.speed : 80;
                const maxStep = speed * deltaTime;
                if (dist <= maxStep) {
                    transform.position.x = entity.targetX;
                    transform.position.y = entity.targetY;
                } else {
                    transform.position.x += (dx / dist) * maxStep;
                    transform.position.y += (dy / dist) * maxStep;
                }
            }

            const sprite = entity.getComponent('sprite');
            if (sprite) {
                const timeSinceMove = Date.now() - (entity._lastMoveTime || 0);
                sprite.isWalking = dist > 1 && timeSinceMove < 800;
            }
        }
    }

    /**
     * 清理所有实体（场景退出时调用）
     */
    clear() {
        this.remotePlayers.clear();
        this.npcEntities.clear();
    }

    // ─────────────────────────────────────────────
    // 增量状态同步
    // ─────────────────────────────────────────────

    /**
     * 处理 state_sync 消息中的远程玩家和 NPC 部分
     * 自己的状态由场景自行处理（位置由本地控制）。
     *
     * @param {Object} data - { players: [...], npcs: [...] }
     * @param {number} selfId - 本玩家 charId（跳过自己）
     * @param {Function} addRemotePlayerFn - 新玩家出现时的回调 (serverData) => void
     * @param {Object} [sceneRef] - 场景引用，用于 selectedTarget 清理和 entities 操作
     */
    applyStateSync(data, selfId, addRemotePlayerFn, sceneRef) {
        // ── 远程玩家 ──────────────────────────────────────────────
        if (data.players) {
            for (const p of data.players) {
                if (p.char_id === selfId) continue;

                const entity = this.remotePlayers.get(p.char_id);
                if (entity) {
                    if (p.x !== undefined) entity.targetX = p.x;
                    if (p.y !== undefined) entity.targetY = p.y;
                    if (p.dead !== undefined) {
                        entity.dead = p.dead;
                        entity.isDead = p.dead;
                        entity.isDying = p.dead;
                    }
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
                    // 全量数据 → 新玩家
                    addRemotePlayerFn?.(p);
                }
            }
        }

        // ── NPC ──────────────────────────────────────────────────
        if (data.npcs && sceneRef) {
            for (const npc of data.npcs) {
                const entity = this.npcEntities.get(npc.id);
                if (!entity) continue;
                const stats = entity.getComponent('stats');
                if (stats) {
                    if (npc.hp !== undefined) stats.hp = npc.hp;
                    if (npc.max_hp !== undefined) stats.maxHp = npc.max_hp;
                }
                if (npc.dead) {
                    entity.isDead = true;
                    entity.isDying = true;
                    if (sceneRef.selectedTarget === npc.id) sceneRef.selectedTarget = null;
                    const idx = sceneRef.entities.indexOf(entity);
                    if (idx >= 0) sceneRef.entities.splice(idx, 1);
                    if (sceneRef.engine?.entityManager) {
                        sceneRef.engine.entityManager.removeEntity(entity);
                    }
                    this.npcEntities.delete(npc.id);
                }
            }
        }
    }
}
