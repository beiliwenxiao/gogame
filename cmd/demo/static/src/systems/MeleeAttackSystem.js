/**
 * MeleeAttackSystem - 近战/远程扇形攻击系统
 * 
 * 从 BaseGameScene 提取的扇形攻击逻辑，包括：
 * - 扇形攻击检测与执行
 * - 刀光/箭光特效管理
 * - 战斗警示圆圈渲染
 * - 刀光轨迹渲染
 * - 远程箭矢消耗与穿刺机制
 */
export class MeleeAttackSystem {
  /**
   * @param {Object} config - 配置参数
   * @param {number} [config.sliceAttackRange=100] - 近战攻击距离
   * @param {number} [config.sliceHitRadius=40] - 命中半径
   * @param {number} [config.sliceGlobalCooldown=3.0] - 全局武器冷却（秒）
   * @param {number} [config.sectorAngle=Math.PI/3] - 扇形张角（弧度）
   * @param {number} [config.sliceTrailMaxAge=0.3] - 刀光轨迹最大存活时间
   */
  constructor(config = {}) {
    // 刀光轨迹
    this.sliceTrail = [];
    this.sliceTrailMaxAge = config.sliceTrailMaxAge ?? 0.3;
    this.isSlicing = false;
    this.slicedEnemies = new Set();
    
    // 攻击参数
    this.sliceAttackRange = config.sliceAttackRange ?? 100;
    this.sliceHitRadius = config.sliceHitRadius ?? 40;
    this.sliceCooldowns = new Map();
    this.sliceCooldownTime = config.sliceCooldownTime ?? 0.5;
    this.sliceMouseSpeed = 0;
    this.sliceGlobalCooldown = config.sliceGlobalCooldown ?? 3.0;
    this.sliceLastAttackTime = 0;
    this.sliceCooldownShown = false;
    
    // 扇形参数
    this.sectorAngle = config.sectorAngle ?? (Math.PI / 3);
    this.sectorDirection = 0;
    this.sectorIsRanged = false;
    this.sectorRangedOffset = config.sectorRangedOffset ?? 256;
    this.sectorRangedRadius = config.sectorRangedRadius ?? 120;
    this.sectorAttackFlash = 0;
    this.sectorSlashEffects = [];
    
    // 依赖（通过 init 注入）
    this.inputManager = null;
    this.combatSystem = null;
    this.floatingTextManager = null;
    this.playerEntity = null;
    this.entities = [];
  }

  /**
   * 初始化依赖
   * @param {Object} deps
   * @param {Object} deps.inputManager - 输入管理器
   * @param {Object} deps.combatSystem - 战斗系统
   * @param {Object} [deps.floatingTextManager] - 飘动文字管理器
   */
  init(deps) {
    this.inputManager = deps.inputManager;
    this.combatSystem = deps.combatSystem;
    this.floatingTextManager = deps.floatingTextManager || null;
  }

  /**
   * 设置玩家实体
   * @param {Object} entity
   */
  setPlayerEntity(entity) {
    this.playerEntity = entity;
  }

  /**
   * 设置实体列表引用
   * @param {Array} entities
   */
  setEntities(entities) {
    this.entities = entities;
  }

  /**
   * 更新扇形攻击（每帧调用）
   * @param {Object} mouseWorldPos - 鼠标世界坐标
   * @param {Object} playerCenter - 玩家中心坐标
   * @param {number} currentTime - 当前时间（秒）
   */
  update(mouseWorldPos, playerCenter, currentTime, deltaTime = 1/60) {
    if (!this.inputManager || !this.combatSystem) return;

    // 清理过期的轨迹点
    this.sliceTrail = this.sliceTrail.filter(p => currentTime - p.time < this.sliceTrailMaxAge);

    // 更新攻击闪光动画
    if (this.sectorAttackFlash > 0) {
      this.sectorAttackFlash -= deltaTime;
    }

    // 更新刀光/箭光特效（使用真实 deltaTime）
    this.updateSectorSlashEffects(deltaTime);

    // 计算鼠标方向角度（2.5D 等距视角：Y 轴 ×2 还原压缩）
    const dx = mouseWorldPos.x - playerCenter.x;
    const dy = mouseWorldPos.y - playerCenter.y;
    this.sectorDirection = Math.atan2(dy * 2, dx);

    // 判断近战/远程
    this.sectorIsRanged = this.checkIsRangedWeapon();

    // 战斗状态下才能攻击
    const inCombat = this.combatSystem && this.combatSystem.isInCombat();
    if (!inCombat) {
      this.isSlicing = false;
      return;
    }

    // 安全区内禁止攻击（由 ArenaScene 设置）
    if (this._safeZoneDisabled) return;

    // 鼠标左键按住触发攻击（右键不触发，联网模式由 NetworkCombatSystem 管理）
    if (!this._arenaMode && this.inputManager.isMouseDown() && this.inputManager.getMouseButton() === 0 && !this.inputManager.isMouseClickHandled()) {
      this.performSectorAttack(playerCenter, currentTime);
    }
  }

  /**
   * 检查是否装备了远程武器
   * @returns {boolean}
   */
  checkIsRangedWeapon() {
    if (!this.playerEntity) return false;
    const equipComp = this.playerEntity.getComponent('equipment');
    if (!equipComp) return false;
    const mainhand = equipComp.getEquipment('mainhand');
    if (mainhand && (mainhand.subType === 'bow' || mainhand.subType === 'crossbow' || mainhand.subType === 'staff' || mainhand.ranged === true)) {
      return true;
    }
    return false;
  }

  /**
   * 执行扇形攻击
   * @param {Object} playerCenter - 玩家中心坐标
   * @param {number} currentTime - 当前时间（秒）
   */
  performSectorAttack(playerCenter, currentTime) {
    if (!this.playerEntity || !this.combatSystem) return;
    
    const playerStats = this.playerEntity.getComponent('stats');
    if (!playerStats) return;
    
    // 获取武器冷却时间
    let weaponCooldown = this.sliceGlobalCooldown;
    const equipComp = this.playerEntity.getComponent('equipment');
    if (equipComp) {
      const mainhand = equipComp.getEquipment('mainhand');
      const offhand = equipComp.getEquipment('offhand');
      if (mainhand && mainhand.attackSpeed != null) {
        weaponCooldown = mainhand.attackSpeed;
      } else if (offhand && offhand.attackSpeed != null) {
        weaponCooldown = offhand.attackSpeed;
      }
    }
    
    // 检查全局武器冷却
    const timeSinceLastAttack = currentTime - this.sliceLastAttackTime;
    if (timeSinceLastAttack < weaponCooldown) {
      if (!this.sliceCooldownShown) {
        this.sliceCooldownShown = true;
        const remaining = (weaponCooldown - timeSinceLastAttack).toFixed(1);
        if (this.floatingTextManager) {
          const playerTransform = this.playerEntity.getComponent('transform');
          if (playerTransform) {
            this.floatingTextManager.addText(
              playerTransform.position.x,
              playerTransform.position.y - 70,
              `冷却中 ${remaining}s`,
              '#888888'
            );
          }
        }
      }
      return;
    }
    
    // 远程攻击需要消耗箭矢（联网模式下跳过，由 NetworkCombatSystem 管理）
    if (this.sectorIsRanged && !this._arenaMode) {
      const equipComp2 = this.playerEntity.getComponent('equipment');
      const inventory2 = this.playerEntity.getComponent('inventory');
      if (equipComp2) {
        const offhand = equipComp2.getEquipment('offhand');
        const ammoId = offhand?.subType === 'ammo' ? offhand.id : null;
        const hasAmmo = ammoId && inventory2 && inventory2.getItemCount(ammoId) > 0;
        if (!hasAmmo) {
          if (this.floatingTextManager) {
            const playerTransform = this.playerEntity.getComponent('transform');
            if (playerTransform) {
              this.floatingTextManager.addText(
                playerTransform.position.x, playerTransform.position.y - 70,
                '没有箭矢！', '#ff6666'
              );
            }
          }
          return;
        }
        // 从背包消耗1支
        inventory2.removeItem(ammoId, 1);
        if (inventory2.getItemCount(ammoId) <= 0) {
          equipComp2.unequip('offhand');
        }
      }
    }
    
    // 计算扇形参数
    const dir = this.sectorDirection;
    let weaponAttackRange = this.sectorAngle;
    let weaponAttackDistance = this.sliceAttackRange;
    
    if (equipComp) {
      const mainhand = equipComp.getEquipment('mainhand');
      if (mainhand) {
        if (mainhand.attackRange != null) {
          weaponAttackRange = mainhand.attackRange * Math.PI / 180;
        }
        if (mainhand.attackDistance != null) {
          weaponAttackDistance = mainhand.attackDistance;
        }
      }
    }
    
    const sectorCenterX = playerCenter.x;
    const sectorCenterY = playerCenter.y;
    const sectorRadius = weaponAttackDistance;
    
    // 记录攻击时间
    this.sliceLastAttackTime = currentTime;
    this.sliceCooldownShown = false;
    
    // 触发攻击闪光
    this.sectorAttackFlash = 0.2;
    
    // 生成刀光/箭光特效（含碰撞伤害，单机模式）
    this.spawnSectorSlashWithDamage(playerCenter, dir, sectorCenterX, sectorCenterY, sectorRadius);
    
    // 播放攻击动画
    const sprite = this.playerEntity.getComponent('sprite');
    if (sprite) {
      sprite.playAnimation('attack');
      setTimeout(() => {
        if (sprite.currentAnimation === 'attack') sprite.playAnimation('idle');
      }, 300);
    }
  }

  /**
   * 判断点是否在扇形内
   */
  isPointInSector(px, py, cx, cy, radius, dir, halfAngle) {
    const dx = px - cx;
    const dy = py - cy;
    const dist = Math.sqrt(dx * dx + dy * dy);
    if (dist > radius) return false;
    
    const angle = Math.atan2(dy, dx);
    let angleDiff = angle - dir;
    while (angleDiff > Math.PI) angleDiff -= Math.PI * 2;
    while (angleDiff < -Math.PI) angleDiff += Math.PI * 2;
    
    return Math.abs(angleDiff) <= halfAngle;
  }

  /**
   * 生成扇形攻击纯视觉特效（不含碰撞伤害）
   * 联网模式直接调用此方法，伤害由后端权威计算
   */
  spawnSectorSlashEffect(playerCenter, dir, sectorCenterX, sectorCenterY, sectorRadius) {
    const type = this.sectorIsRanged ? 'arrow' : 'slash';
    
    let weaponAttackRange = this.sectorAngle;
    const equipComp = this.playerEntity?.getComponent('equipment');
    if (equipComp) {
      const mainhand = equipComp.getEquipment('mainhand');
      if (mainhand && mainhand.attackRange != null) {
        weaponAttackRange = mainhand.attackRange * Math.PI / 180;
      }
    }
    const halfAngle = weaponAttackRange / 2;
    
    if (type === 'slash') {
      this.sectorSlashEffects.push({
        cx: sectorCenterX,
        cy: sectorCenterY,
        radius: sectorRadius * 0.8,
        dir: dir,
        halfAngle: halfAngle,
        age: 0,
        maxAge: 0.25,
        type: 'slash',
        damage: 0,
        hitEntities: []
      });
    } else {
      const speed = 420;  // 初速度降低，有减速感
      // 飞行距离与武器攻击距离一致
      const flyDist = sectorRadius;
      
      const mainhandWeapon = equipComp ? equipComp.getEquipment('mainhand') : null;
      const multiArrow = mainhandWeapon?.multiArrow || 0;
      const pierce = mainhandWeapon?.pierce || 0;
      
      const totalArrows = 1 + multiArrow;
      const spreadAngle = 0.18;
      
      for (let i = 0; i < totalArrows; i++) {
        let arrowDir = dir;
        if (totalArrows > 1) {
          const offset = (i - (totalArrows - 1) / 2) * spreadAngle;
          arrowDir = dir + offset;
        }
        
        this.sectorSlashEffects.push({
          x: playerCenter.x,
          y: playerCenter.y,
          dir: arrowDir,
          speed: speed,
          vy: -30,                // 初始轻微上扬
          gravity: 220,           // 重力（px/s²），让箭有明显下坠
          friction: 0.998,        // 极小阻力，几乎不减速，只靠重力产生弧线
          targetDist: flyDist,
          traveled: 0,
          age: 0,
          maxAge: flyDist / speed * 2.0 + 0.5,  // 足够长，确保能插地
          type: 'arrow',
          damage: 0,
          pierce: pierce,
          pierceCount: 0,
          hitEntities: [],
          stuck: false,
          stuckAge: 0,
          stuckMaxAge: 5,
          stuckAngle: (Math.random() - 0.5) * 0.3,
          embedRatio: 0.2 + Math.random() * 0.6
        });
      }
    }
  }

  /**
   * 生成扇形攻击特效（含碰撞伤害），单机模式使用
   * 在纯视觉特效基础上，为最近添加的特效对象挂载伤害数据
   */
  spawnSectorSlashWithDamage(playerCenter, dir, sectorCenterX, sectorCenterY, sectorRadius) {
    const prevCount = this.sectorSlashEffects.length;
    this.spawnSectorSlashEffect(playerCenter, dir, sectorCenterX, sectorCenterY, sectorRadius);
    
    // 计算伤害值
    const playerStats = this.playerEntity.getComponent('stats');
    const baseDmg = playerStats ? (playerStats.attack || 15) : 15;
    
    // 为新增的特效对象挂载伤害数据
    const equipComp = this.playerEntity?.getComponent('equipment');
    const mainhandWeapon = equipComp ? equipComp.getEquipment('mainhand') : null;
    
    for (let i = prevCount; i < this.sectorSlashEffects.length; i++) {
      const e = this.sectorSlashEffects[i];
      e.damage = baseDmg;
      if (e.type === 'arrow') {
        e.pierce = mainhandWeapon?.pierce || 0;
      }
    }
  }


  /**
   * 更新扇形攻击特效
   * @param {number} deltaTime
   */
  updateSectorSlashEffects(deltaTime) {
    for (let i = this.sectorSlashEffects.length - 1; i >= 0; i--) {
      const e = this.sectorSlashEffects[i];
      e.age += deltaTime;
      
      // 安全区内跳过碰撞伤害检测
      if (this._safeZoneDisabled) {
        if (e.type === 'arrow' && !e.stuck) {
          e.traveled += e.speed * deltaTime;
          e.x += Math.cos(e.dir) * e.speed * deltaTime;
          e.y += Math.sin(e.dir) * e.speed * deltaTime;
          e.speed *= Math.pow(e.friction ?? 0.96, deltaTime * 60);
          e.vy = (e.vy ?? 0) + (e.gravity ?? 180) * deltaTime;
          e.y += e.vy * deltaTime;
          const vx2 = Math.cos(e.dir) * e.speed;
          if (Math.abs(vx2) > 0.1 || Math.abs(e.vy) > 0.1) {
            e.renderDir = Math.atan2(e.vy, vx2);
          }
          if (e.traveled >= e.targetDist || e.age >= e.maxAge) {
            e.stuck = true;
            e.stuckAge = 0;
          }
        } else if (e.type === 'arrow' && e.stuck) {
          e.stuckAge += deltaTime;
          if (e.stuckAge >= e.stuckMaxAge) {
            this.sectorSlashEffects.splice(i, 1);
          }
          continue;
        }
        if (e.age >= e.maxAge && !e.stuck) {
          this.sectorSlashEffects.splice(i, 1);
        }
        continue;
      }
      
      // 剑光碰撞检测（联网模式下跳过本地伤害，由后端权威计算）
      if (e.type === 'slash' && e.damage && this.combatSystem && !this._arenaMode) {
        const progress = e.age / e.maxAge;
        const sweepRadius = e.radius * (0.3 + progress * 0.7);
        for (const entity of this.entities) {
          if (entity.type !== 'enemy') continue;
          if (entity.isDead || entity.isDying) continue;
          if (e.hitEntities && e.hitEntities.includes(entity)) continue;
          
          const targetTransform = entity.getComponent('transform');
          const targetStats = entity.getComponent('stats');
          if (!targetTransform || !targetStats || targetStats.hp <= 0) continue;
          
          const ex = targetTransform.position.x;
          const ey = targetTransform.position.y - 32;
          
          if (this.isPointInSector(ex, ey, e.cx, e.cy, sweepRadius + 25, e.dir, e.halfAngle)) {
            const dx = ex - e.cx;
            const dy = ey - e.cy;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist >= sweepRadius - 25) {
              const finalDamage = Math.max(1, Math.floor(e.damage * (0.8 + Math.random() * 0.4)));
              this.combatSystem.applyDamage(entity, finalDamage, null, '斩击');
              if (this.combatSystem.createSliceEffect) {
                this.combatSystem.createSliceEffect(targetTransform.position);
              }
              if (e.hitEntities) e.hitEntities.push(entity);
            }
          }
        }
      }
      
      // 箭矢碰撞检测
      if (e.type === 'arrow') {
        // 插地状态：只计时，不移动
        if (e.stuck) {
          e.stuckAge += deltaTime;
          if (e.stuckAge >= e.stuckMaxAge) {
            this.sectorSlashEffects.splice(i, 1);
          }
          continue;
        }

        // 附着状态：跟随目标实体，目标死亡则落地
        if (e.attached) {
          const t = e.attachedEntity?.getComponent('transform');
          if (!t || e.attachedEntity.dead || e.attachedEntity.isDead) {
            // 目标死亡，落在当前位置插地
            e.attached = false;
            e.attachedEntity = null;
            e.stuck = true;
            e.stuckAge = 0;
            // 落地时随机偏移一点，像从身上掉落
            e.x += (Math.random() - 0.5) * 12;
            e.y += (Math.random() - 0.5) * 6;
          } else {
            // 跟随目标
            e.x = t.position.x + (e.attachOffsetX || 0);
            e.y = t.position.y + (e.attachOffsetY || 0);
          }
          continue;
        }

        e.traveled += e.speed * deltaTime;
        e.x += Math.cos(e.dir) * e.speed * deltaTime;
        e.y += Math.sin(e.dir) * e.speed * deltaTime;
        
        // 减速（空气阻力）
        e.speed *= Math.pow(e.friction ?? 0.99, deltaTime * 60);
        // 重力：vy 每帧增加，Y 轴下坠
        e.vy = (e.vy ?? 0) + (e.gravity ?? 100) * deltaTime;
        e.y += e.vy * deltaTime;
        // 实时更新箭矢朝向（跟随速度方向）
        const vx = Math.cos(e.dir) * e.speed;
        const vy = e.vy ?? 0;
        if (Math.abs(vx) > 0.1 || Math.abs(vy) > 0.1) {
          e.renderDir = Math.atan2(vy, vx);
        }

        // 联网模式：检测是否靠近预附着目标，靠近时飞行箭矢消失，附着箭矢显示
        if (e.pendingAttachEntity && e.pendingAttachArrow) {
          const pt = e.pendingAttachEntity.getComponent('transform');
          if (pt) {
            const pdx = e.x - pt.position.x;
            const pdy = e.y - pt.position.y;
            const pdist = Math.sqrt(pdx * pdx + pdy * pdy);
            if (pdist <= 35) {
              // 飞行箭矢消失，附着箭矢显示
              e.pendingAttachArrow.hidden = false;
              this.sectorSlashEffects.splice(i, 1);
              continue;
            }
          } else {
            // 目标已消失，附着箭矢直接显示并插地
            e.pendingAttachArrow.hidden = false;
            e.pendingAttachArrow.attached = false;
            e.pendingAttachArrow.stuck = true;
            e.pendingAttachArrow.stuckAge = 0;
            this.sectorSlashEffects.splice(i, 1);
            continue;
          }
        }

        // 联网模式：检测箭矢与 rangedTargets 的碰撞，碰撞时触发伤害回调
        if (e.rangedTargets && e.onHitCallback) {
          const hitRadius = 30;
          for (let ti = e.rangedTargets.length - 1; ti >= 0; ti--) {
            const tgt = e.rangedTargets[ti];
            if (tgt.entity.dead || tgt.entity.isDead) {
              e.rangedTargets.splice(ti, 1);
              continue;
            }
            const tt = tgt.transform || tgt.entity.getComponent('transform');
            if (!tt) continue;
            const hdx = e.x - tt.position.x;
            const hdy = e.y - tt.position.y;
            if (Math.sqrt(hdx * hdx + hdy * hdy) <= hitRadius) {
              e.onHitCallback(tgt.id, tgt.isNPC);
              e.rangedTargets.splice(ti, 1); // 每个目标只触发一次
            }
          }
        }
        
        if (e.damage && this.combatSystem && !this._arenaMode) {
          const hitRadius = 20;
          for (const entity of this.entities) {
            if (entity.type !== 'enemy') continue;
            if (entity.isDead || entity.isDying) continue;
            if (e.hitEntities && e.hitEntities.includes(entity)) continue;
            
            const targetTransform = entity.getComponent('transform');
            const targetStats = entity.getComponent('stats');
            if (!targetTransform || !targetStats || targetStats.hp <= 0) continue;
            
            const ex = targetTransform.position.x;
            const ey = targetTransform.position.y - 32;
            const dx = e.x - ex;
            const dy = e.y - ey;
            const dist = Math.sqrt(dx * dx + dy * dy);
            
            if (dist <= hitRadius) {
              const finalDamage = Math.max(1, Math.floor(e.damage * (0.8 + Math.random() * 0.4)));
              this.combatSystem.applyDamage(entity, finalDamage, null, '远程攻击');
              if (this.combatSystem.createSliceEffect) {
                this.combatSystem.createSliceEffect(targetTransform.position);
              }
              if (e.hitEntities) e.hitEntities.push(entity);
              
              e.pierceCount = (e.pierceCount || 0) + 1;
              if (e.pierceCount > (e.pierce || 0)) {
                // 命中且穿刺耗尽：附着在目标身上
                e.attached = true;
                e.attachedEntity = entity;
                e.attachOffsetX = e.x - targetTransform.position.x;
                e.attachOffsetY = e.y - targetTransform.position.y;
                break;
              }
            }
          }
        }

        // 飞行到达终点或超时：插地
        // 雨箭：到达 groundY 时插地
        const hitGround = e.isRainArrow ? e.y >= e.groundY : false;
        if (!e.attached && (e.traveled >= e.targetDist || e.age >= e.maxAge || hitGround)) {
          if (e.isRainArrow && hitGround) e.y = e.groundY; // 精确落地
          e.stuck = true;
          e.stuckAge = 0;
          continue;
        }      }
      if (e.age >= e.maxAge && !e.stuck && !e.attached) {
        this.sectorSlashEffects.splice(i, 1);
      }
    }
  }

  /**
   * 渲染扇形攻击特效（刀光/箭光）
   * @param {CanvasRenderingContext2D} ctx
   */
  renderSectorSlashEffects(ctx) {
    if (this.sectorSlashEffects.length === 0) return;
    
    ctx.save();
    
    for (const e of this.sectorSlashEffects) {
      const alpha = Math.max(0, 1 - e.age / e.maxAge);
      
      if (e.type === 'slash') {
        const progress = e.age / e.maxAge;
        const sweepR = e.radius * (0.3 + progress * 0.7);
        const startAngle = e.dir - e.halfAngle;
        const endAngle = e.dir + e.halfAngle;
        const maxThick = 12 * (1 - progress * 0.4);
        
        const tipStartX = e.cx + Math.cos(startAngle) * sweepR;
        const tipStartY = e.cy + Math.sin(startAngle) * sweepR;
        const tipEndX = e.cx + Math.cos(endAngle) * sweepR;
        const tipEndY = e.cy + Math.sin(endAngle) * sweepR;
        const midAngle = e.dir;
        const outerMidX = e.cx + Math.cos(midAngle) * (sweepR + maxThick);
        const outerMidY = e.cy + Math.sin(midAngle) * (sweepR + maxThick);
        const innerMidX = e.cx + Math.cos(midAngle) * (sweepR - maxThick * 0.4);
        const innerMidY = e.cy + Math.sin(midAngle) * (sweepR - maxThick * 0.4);
        
        // 外层光晕
        ctx.beginPath();
        ctx.moveTo(tipStartX, tipStartY);
        ctx.quadraticCurveTo(e.cx + Math.cos(midAngle) * (sweepR + maxThick + 5), e.cy + Math.sin(midAngle) * (sweepR + maxThick + 5), tipEndX, tipEndY);
        ctx.quadraticCurveTo(e.cx + Math.cos(midAngle) * (sweepR - maxThick * 0.4 - 3), e.cy + Math.sin(midAngle) * (sweepR - maxThick * 0.4 - 3), tipStartX, tipStartY);
        ctx.closePath();
        ctx.fillStyle = e.isNPC ? `rgba(255, 180, 80, ${alpha * 0.2})` : `rgba(200, 230, 255, ${alpha * 0.2})`;
        ctx.fill();
        
        // 中层光芒
        ctx.beginPath();
        ctx.moveTo(tipStartX, tipStartY);
        ctx.quadraticCurveTo(e.cx + Math.cos(midAngle) * (sweepR + maxThick + 2), e.cy + Math.sin(midAngle) * (sweepR + maxThick + 2), tipEndX, tipEndY);
        ctx.quadraticCurveTo(e.cx + Math.cos(midAngle) * (sweepR - maxThick * 0.3), e.cy + Math.sin(midAngle) * (sweepR - maxThick * 0.3), tipStartX, tipStartY);
        ctx.closePath();
        ctx.fillStyle = e.isNPC ? `rgba(255, 120, 40, ${alpha * 0.5})` : `rgba(220, 240, 255, ${alpha * 0.5})`;
        ctx.fill();
        
        // 内层白色核心
        ctx.beginPath();
        ctx.moveTo(tipStartX, tipStartY);
        ctx.quadraticCurveTo(outerMidX, outerMidY, tipEndX, tipEndY);
        ctx.quadraticCurveTo(innerMidX, innerMidY, tipStartX, tipStartY);
        ctx.closePath();
        ctx.fillStyle = e.isNPC ? `rgba(255, 200, 100, ${alpha * 0.8})` : `rgba(255, 255, 255, ${alpha * 0.8})`;
        ctx.fill();
        
      } else if (e.type === 'arrow') {
        if (e.hidden) continue;  // 预附着箭矢，等待飞行箭矢靠近后再显示
        if (e.attached) {
          // 附着在目标身上：保持飞行时的方向，扎在身体上
          const attachDir = e.renderDir ?? e.dir;
          const len = 22;
          const tailX = e.x - Math.cos(attachDir) * len;
          const tailY = e.y - Math.sin(attachDir) * len;

          ctx.beginPath();
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(e.x, e.y);
          ctx.strokeStyle = `rgba(120, 75, 20, 0.92)`;
          ctx.lineWidth = 2.5;
          ctx.lineCap = 'round';
          ctx.stroke();

          ctx.beginPath();
          ctx.arc(e.x, e.y, 2.5, 0, Math.PI * 2);
          ctx.fillStyle = `rgba(190, 160, 60, 0.95)`;
          ctx.fill();

          const featherLen = 6;
          ctx.beginPath();
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(tailX - Math.cos(attachDir - 0.5) * featherLen, tailY - Math.sin(attachDir - 0.5) * featherLen);
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(tailX - Math.cos(attachDir + 0.5) * featherLen, tailY - Math.sin(attachDir + 0.5) * featherLen);
          ctx.strokeStyle = `rgba(230, 220, 190, 0.88)`;
          ctx.lineWidth = 1.5;
          ctx.stroke();

        } else if (e.stuck) {
          const stuckAlpha = Math.max(0, 1 - e.stuckAge / e.stuckMaxAge);
          const embedRatio = e.embedRatio ?? 0.5;
          const totalLen = 30;
          const visibleLen = totalLen * (1 - embedRatio); // 露出地面的长度

          // 等距视角下竖直插地：箭杆接近垂直，略带随机倾斜
          // stuckAngle 是小幅随机偏转（±0.15rad），基准方向向上（-π/2）
          const stuckDir = -Math.PI / 2 + e.stuckAngle;

          // 地面接触点（箭头嵌入处）
          const groundX = e.x;
          const groundY = e.y;
          // 露出部分的顶端（尾羽位置）
          const tailX = groundX + Math.cos(stuckDir) * visibleLen;
          const tailY = groundY + Math.sin(stuckDir) * visibleLen;

          // 箭杆（只画露出地面的部分）
          ctx.beginPath();
          ctx.moveTo(groundX, groundY);
          ctx.lineTo(tailX, tailY);
          ctx.strokeStyle = `rgba(160, 120, 60, ${stuckAlpha * 0.9})`;
          ctx.lineWidth = 2.5;
          ctx.lineCap = 'round';
          ctx.stroke();

          // 尾羽（在顶端）
          const featherLen = 7;
          ctx.beginPath();
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(tailX + Math.cos(stuckDir + Math.PI / 2) * featherLen * 0.6,
                     tailY + Math.sin(stuckDir + Math.PI / 2) * featherLen * 0.6);
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(tailX - Math.cos(stuckDir + Math.PI / 2) * featherLen * 0.6,
                     tailY - Math.sin(stuckDir + Math.PI / 2) * featherLen * 0.6);
          ctx.strokeStyle = `rgba(220, 200, 140, ${stuckAlpha * 0.8})`;
          ctx.lineWidth = 1.5;
          ctx.stroke();
        } else {
          // 飞行中：实体箭矢，方向跟随速度
          const drawDir = e.renderDir ?? e.dir;
          const len = 28;
          const tailX = e.x - Math.cos(drawDir) * len;
          const tailY = e.y - Math.sin(drawDir) * len;

          if (e.isLightning) {
            // 闪电箭：蓝白色电弧
            ctx.beginPath();
            ctx.moveTo(tailX, tailY);
            ctx.lineTo(e.x, e.y);
            ctx.strokeStyle = `rgba(80, 180, 255, ${alpha * 0.35})`;
            ctx.lineWidth = 8;
            ctx.lineCap = 'round';
            ctx.stroke();

            ctx.beginPath();
            ctx.moveTo(tailX, tailY);
            ctx.lineTo(e.x, e.y);
            ctx.strokeStyle = `rgba(200, 240, 255, ${alpha * 0.9})`;
            ctx.lineWidth = 2;
            ctx.stroke();

            ctx.beginPath();
            ctx.arc(e.x, e.y, 4, 0, Math.PI * 2);
            ctx.fillStyle = `rgba(255, 255, 255, ${alpha})`;
            ctx.fill();

            // 电弧锯齿
            ctx.beginPath();
            ctx.moveTo(tailX, tailY);
            for (let si = 1; si < 6; si++) {
              const t = si / 6;
              const mx = tailX + (e.x - tailX) * t + (Math.random() - 0.5) * 8;
              const my = tailY + (e.y - tailY) * t + (Math.random() - 0.5) * 8;
              ctx.lineTo(mx, my);
            }
            ctx.lineTo(e.x, e.y);
            ctx.strokeStyle = `rgba(150, 220, 255, ${alpha * 0.6})`;
            ctx.lineWidth = 1;
            ctx.stroke();
          } else {
          // 细光晕（轻微，不要太虚）
          ctx.beginPath();
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(e.x, e.y);
          ctx.strokeStyle = `rgba(255, 200, 60, ${alpha * 0.25})`;
          ctx.lineWidth = 5;
          ctx.lineCap = 'round';
          ctx.stroke();

          // 实体箭杆（深棕色）
          ctx.beginPath();
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(e.x, e.y);
          ctx.strokeStyle = `rgba(120, 75, 20, ${alpha * 0.95})`;
          ctx.lineWidth = 2.5;
          ctx.stroke();

          // 箭头（金属色）
          ctx.beginPath();
          ctx.arc(e.x, e.y, 3, 0, Math.PI * 2);
          ctx.fillStyle = `rgba(190, 160, 60, ${alpha})`;
          ctx.fill();

          // 尾羽
          const featherLen = 7;
          ctx.beginPath();
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(tailX - Math.cos(drawDir - 0.5) * featherLen, tailY - Math.sin(drawDir - 0.5) * featherLen);
          ctx.moveTo(tailX, tailY);
          ctx.lineTo(tailX - Math.cos(drawDir + 0.5) * featherLen, tailY - Math.sin(drawDir + 0.5) * featherLen);
          ctx.strokeStyle = `rgba(230, 220, 190, ${alpha * 0.9})`;
          ctx.lineWidth = 1.5;
          ctx.stroke();
          } // end isLightning else
        }
      }
    }
    
    ctx.restore();
  }

  /**
   * 渲染战斗警示圆圈
   * @param {CanvasRenderingContext2D} ctx
   * @param {Object} camera - 相机对象（用于获取鼠标世界坐标）
   */
  renderCombatAlertCircle(ctx, camera) {
    const transform = this.playerEntity.getComponent('transform');
    if (!transform) return;
    
    const sprite = this.playerEntity.getComponent('sprite');
    const spriteHeight = sprite?.height || 64;
    const cx = transform.position.x;
    const cy = transform.position.y - spriteHeight / 10;
    
    ctx.save();
    
    const dir = this.sectorDirection;
    const time = performance.now() / 1000;
    const dashOffset = (time * 20) % 20;
    
    // 从武器数据获取攻击范围和距离
    let weaponAttackRange = this.sectorAngle;
    let weaponAttackDistance = this.sliceAttackRange;
    const equipComp = this.playerEntity.getComponent('equipment');
    if (equipComp) {
      const mainhand = equipComp.getEquipment('mainhand');
      if (mainhand) {
        if (mainhand.attackRange != null) {
          weaponAttackRange = mainhand.attackRange * Math.PI / 180;
        }
        if (mainhand.attackDistance != null) {
          weaponAttackDistance = mainhand.attackDistance;
        }
      }
    }
    const halfAngle = weaponAttackRange / 2;
    const r = weaponAttackDistance;
    
    // 攻击闪光效果
    const flashAlpha = Math.max(0, this.sectorAttackFlash / 0.2);
    
    if (this.sectorIsRanged) {
      // 远程：扇形范围指示器（和近战一样的扇形，颜色区分）
      const rx = r;
      const ry = r / 2;
      const steps = 32;

      // 填充
      ctx.beginPath();
      ctx.moveTo(cx, cy);
      for (let i = 0; i <= steps; i++) {
        const a = (dir - halfAngle) + (i / steps) * halfAngle * 2;
        ctx.lineTo(cx + Math.cos(a) * rx, cy + Math.sin(a) * ry);
      }
      ctx.closePath();
      if (flashAlpha > 0) {
        ctx.fillStyle = `rgba(100, 200, 100, ${0.12 + flashAlpha * 0.4})`;
      } else {
        ctx.fillStyle = 'rgba(100, 200, 100, 0.12)';
      }
      ctx.fill();

      // 描边
      ctx.beginPath();
      ctx.moveTo(cx, cy);
      for (let i = 0; i <= steps; i++) {
        const a = (dir - halfAngle) + (i / steps) * halfAngle * 2;
        ctx.lineTo(cx + Math.cos(a) * rx, cy + Math.sin(a) * ry);
      }
      ctx.closePath();
      ctx.strokeStyle = flashAlpha > 0 ? `rgba(150, 255, 150, ${0.6 + flashAlpha * 0.4})` : 'rgba(100, 200, 100, 0.6)';
      ctx.lineWidth = 1.5;
      ctx.setLineDash([8, 5]);
      ctx.lineDashOffset = -dashOffset;
      ctx.stroke();
      ctx.setLineDash([]);
    } else {
      // 近战椭圆扇形（2.5D 等距视角：ry = r/2）
      const rx = r;
      const ry = r / 2;
      const steps = 32;

      // 填充
      ctx.beginPath();
      ctx.moveTo(cx, cy);
      for (let i = 0; i <= steps; i++) {
        const a = (dir - halfAngle) + (i / steps) * halfAngle * 2;
        ctx.lineTo(cx + Math.cos(a) * rx, cy + Math.sin(a) * ry);
      }
      ctx.closePath();
      if (flashAlpha > 0) {
        ctx.fillStyle = `rgba(255, 100, 100, ${0.12 + flashAlpha * 0.4})`;
      } else {
        ctx.fillStyle = 'rgba(255, 100, 100, 0.12)';
      }
      ctx.fill();

      // 描边
      ctx.beginPath();
      ctx.moveTo(cx, cy);
      for (let i = 0; i <= steps; i++) {
        const a = (dir - halfAngle) + (i / steps) * halfAngle * 2;
        ctx.lineTo(cx + Math.cos(a) * rx, cy + Math.sin(a) * ry);
      }
      ctx.closePath();
      ctx.strokeStyle = flashAlpha > 0 ? `rgba(255, 150, 150, ${0.6 + flashAlpha * 0.4})` : 'rgba(255, 100, 100, 0.6)';
      ctx.lineWidth = 1.5;
      ctx.setLineDash([8, 5]);
      ctx.lineDashOffset = -dashOffset;
      ctx.stroke();
      ctx.setLineDash([]);
    }
    
    // 武器冷却时间条
    const currentTime = performance.now() / 1000;
    const timeSinceLastAttack = currentTime - this.sliceLastAttackTime;
    
    let weaponCooldown = this.sliceGlobalCooldown;
    if (equipComp) {
      const mainhand2 = equipComp.getEquipment('mainhand');
      const offhand2 = equipComp.getEquipment('offhand');
      if (mainhand2 && mainhand2.attackSpeed != null) {
        weaponCooldown = mainhand2.attackSpeed;
      } else if (offhand2 && offhand2.attackSpeed != null) {
        weaponCooldown = offhand2.attackSpeed;
      }
    }
    
    if (timeSinceLastAttack < weaponCooldown && this.sliceLastAttackTime > 0) {
      const barWidth = 40;
      const barHeight = 5;
      const barX = cx - barWidth / 2;
      const barY = transform.position.y - spriteHeight - 18;
      const cooldownRatio = timeSinceLastAttack / weaponCooldown;
      
      ctx.fillStyle = 'rgba(0, 0, 0, 0.6)';
      ctx.fillRect(barX, barY, barWidth, barHeight);
      
      const gradient = ctx.createLinearGradient(barX, barY, barX + barWidth, barY);
      gradient.addColorStop(0, '#ff6600');
      gradient.addColorStop(1, '#ffcc00');
      ctx.fillStyle = gradient;
      ctx.fillRect(barX, barY, barWidth * cooldownRatio, barHeight);
      
      ctx.strokeStyle = 'rgba(255, 255, 255, 0.5)';
      ctx.lineWidth = 0.5;
      ctx.strokeRect(barX, barY, barWidth, barHeight);
      
      const remaining = (weaponCooldown - timeSinceLastAttack).toFixed(1);
      ctx.font = '9px Arial';
      ctx.textAlign = 'center';
      ctx.fillStyle = '#ffffff';
      ctx.fillText(`${remaining}s`, cx, barY - 2);
    }
    
    // 远程武器时显示剩余箭矢数量（从背包读）
    if (this.sectorIsRanged) {
      const eqComp = this.playerEntity.getComponent('equipment');
      const invComp = this.playerEntity.getComponent('inventory');
      if (eqComp) {
        const offhand = eqComp.getEquipment('offhand');
        const ammoId = offhand?.subType === 'ammo' ? offhand.id : null;
        const arrowCount = (ammoId && invComp) ? invComp.getItemCount(ammoId) : 0;
        ctx.font = 'bold 11px Arial';
        ctx.textAlign = 'center';
        ctx.fillStyle = arrowCount > 10 ? '#88ccff' : arrowCount > 0 ? '#ffaa44' : '#ff4444';
        ctx.fillText(`🏹 ${arrowCount}`, cx, transform.position.y - spriteHeight - 24);
      }
    }
    
    ctx.restore();
  }

  /**
   * 渲染滑动刀光轨迹
   * @param {CanvasRenderingContext2D} ctx
   */
  renderSliceTrail(ctx) {
    if (this.sliceTrail.length < 2) return;
    
    const currentTime = performance.now() / 1000;
    
    ctx.save();
    
    for (let i = 1; i < this.sliceTrail.length; i++) {
      const p0 = this.sliceTrail[i - 1];
      const p1 = this.sliceTrail[i];
      
      const age = currentTime - p1.time;
      const alpha = Math.max(0, 1 - age / this.sliceTrailMaxAge);
      if (alpha <= 0) continue;
      
      const posRatio = i / this.sliceTrail.length;
      const widthCurve = Math.sin(posRatio * Math.PI);
      const width = widthCurve * 5 * alpha + 0.5;
      
      ctx.beginPath();
      ctx.moveTo(p0.x, p0.y);
      ctx.lineTo(p1.x, p1.y);
      
      ctx.strokeStyle = `rgba(200, 230, 255, ${alpha * 0.3})`;
      ctx.lineWidth = width + 6;
      ctx.lineCap = 'round';
      ctx.stroke();
      
      ctx.strokeStyle = `rgba(180, 220, 255, ${alpha * 0.6})`;
      ctx.lineWidth = width + 2;
      ctx.stroke();
      
      ctx.strokeStyle = `rgba(255, 255, 255, ${alpha * 0.9})`;
      ctx.lineWidth = width;
      ctx.stroke();
    }
    
    ctx.restore();
  }

  /**
   * 获取扇形方向（供外部使用）
   * @returns {number}
   */
  getSectorDirection() {
    return this.sectorDirection;
  }

  /**
   * 获取是否远程攻击
   * @returns {boolean}
   */
  getIsRanged() {
    return this.sectorIsRanged;
  }

  /**
   * 清理系统状态
   */
  cleanup() {
    this.sliceTrail = [];
    this.sectorSlashEffects = [];
    this.slicedEnemies.clear();
    this.sliceCooldowns.clear();
    this.playerEntity = null;
    this.entities = [];
  }
}
