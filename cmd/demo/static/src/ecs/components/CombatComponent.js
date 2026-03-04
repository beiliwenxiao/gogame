/**
 * CombatComponent.js
 * 战斗组件 - 管理实体的战斗相关数据
 * 对齐后端 SkillComponent + CombatSystem 的技能阶段流水线
 */

import { Component } from '../Component.js';
import { SKILL_PHASE, INTERRUPT_POLICY, TARGET_MODE_FROM_STRING } from '../ComponentTypes.js';

/**
 * 战斗组件
 * 存储战斗状态、目标、技能等信息
 * 对齐后端 ecs.SkillComponent（技能/冷却）+ combat.CombatSystem（阶段流水线）
 */
export class CombatComponent extends Component {
  /**
   * @param {Object} config - 配置
   * @param {number} config.attackRange - 攻击范围
   * @param {number} config.attackCooldown - 攻击冷却时间（毫秒）
   * @param {string} config.faction - 阵营 ('ally'|'enemy'|'neutral')
   */
  constructor(config = {}) {
    super('combat');
    
    // 目标
    this.target = null;
    
    // 攻击属性
    this.attackRange = config.attackRange || 50;
    this.attackCooldown = config.attackCooldown || 1000;
    this.lastAttackTime = 0;
    
    // 技能列表（对齐后端 SkillComponent.Skills）
    this.skills = [];
    this.skillCooldowns = new Map(); // 技能ID -> 上次使用时间
    
    // 阵营（用于判断敌我）
    this.faction = config.faction || 'neutral';
    
    // 战斗状态
    this.inCombat = false;
    this.isAttacking = false;
    this.isCasting = false;
    this.castingSkill = null;
    this.castStartTime = 0;
    
    // 技能阶段流水线（对齐后端 SkillPhase: Windup→Hit→Settle→Recovery）
    this.currentPhase = null;       // 当前阶段 SKILL_PHASE.*
    this.phaseStartTime = 0;        // 当前阶段开始时间
    this.phaseSkill = null;         // 正在执行阶段的技能
    this.interruptPolicy = INTERRUPT_POLICY.CANCEL; // 默认打断取消
  }

  /**
   * 设置攻击目标
   * @param {Entity} target - 目标实体
   */
  setTarget(target) {
    this.target = target;
    if (target) {
      this.inCombat = true;
    }
  }

  /**
   * 清除目标
   */
  clearTarget() {
    this.target = null;
    this.inCombat = false;
  }

  /**
   * 检查是否有目标
   * @returns {boolean}
   */
  hasTarget() {
    return this.target !== null;
  }

  /**
   * 检查攻击冷却是否完成
   * @param {number} currentTime - 当前时间（毫秒）
   * @returns {boolean}
   */
  canAttack(currentTime) {
    // 如果从未攻击过（lastAttackTime === 0），则可以攻击
    if (this.lastAttackTime === 0) return true;
    return currentTime - this.lastAttackTime >= this.attackCooldown;
  }

  /**
   * 执行攻击
   * @param {number} currentTime - 当前时间（毫秒）
   */
  attack(currentTime) {
    if (this.canAttack(currentTime)) {
      this.lastAttackTime = currentTime;
      this.isAttacking = true;
      return true;
    }
    return false;
  }

  /**
   * 获取攻击冷却剩余时间
   * @param {number} currentTime - 当前时间（毫秒）
   * @returns {number} 剩余时间（毫秒）
   */
  getAttackCooldownRemaining(currentTime) {
    const elapsed = currentTime - this.lastAttackTime;
    return Math.max(0, this.attackCooldown - elapsed);
  }

  /**
   * 添加技能
   * @param {Object} skill - 技能数据
   */
  addSkill(skill) {
    this.skills.push(skill);
    this.skillCooldowns.set(skill.id, 0);
  }

  /**
   * 检查技能是否可用
   * @param {string} skillId - 技能ID
   * @param {number} currentTime - 当前时间（毫秒）
   * @returns {boolean}
   */
  canUseSkill(skillId, currentTime) {
    const skill = this.skills.find(s => s.id === skillId);
    if (!skill) return false;
    
    const lastUseTime = this.skillCooldowns.get(skillId) || 0;
    // 如果从未使用过（lastUseTime === 0），则可以使用
    if (lastUseTime === 0) return true;
    // 技能冷却时间是秒，需要转换为毫秒
    return currentTime - lastUseTime >= skill.cooldown * 1000;
  }

  /**
   * 使用技能
   * @param {string} skillId - 技能ID
   * @param {number} currentTime - 当前时间（毫秒）
   * @returns {Object|null} 技能数据或null
   */
  useSkill(skillId, currentTime) {
    const skill = this.skills.find(s => s.id === skillId);
    if (!skill) return null;
    
    if (this.canUseSkill(skillId, currentTime)) {
      this.skillCooldowns.set(skillId, currentTime);
      
      // 如果有施法时间，进入施法状态
      if (skill.castTime > 0) {
        this.isCasting = true;
        this.castingSkill = skill;
        this.castStartTime = currentTime;
      }
      
      return skill;
    }
    
    return null;
  }

  /**
   * 获取技能冷却剩余时间
   * @param {string} skillId - 技能ID
   * @param {number} currentTime - 当前时间（毫秒）
   * @returns {number} 剩余时间（毫秒）
   */
  getSkillCooldownRemaining(skillId, currentTime) {
    const skill = this.skills.find(s => s.id === skillId);
    if (!skill) return 0;
    
    const lastUseTime = this.skillCooldowns.get(skillId) || 0;
    const elapsed = currentTime - lastUseTime;
    // 技能冷却时间是秒，需要转换为毫秒
    return Math.max(0, skill.cooldown * 1000 - elapsed);
  }

  /**
   * 检查施法是否完成
   * @param {number} currentTime - 当前时间（毫秒）
   * @returns {boolean}
   */
  isCastComplete(currentTime) {
    if (!this.isCasting || !this.castingSkill) return false;
    return currentTime - this.castStartTime >= this.castingSkill.castTime;
  }

  /**
   * 完成施法
   */
  completeCast() {
    this.isCasting = false;
    const skill = this.castingSkill;
    this.castingSkill = null;
    this.castStartTime = 0;
    return skill;
  }

  /**
   * 中断施法
   */
  interruptCast() {
    this.isCasting = false;
    this.castingSkill = null;
    this.castStartTime = 0;
  }

  /**
   * 更新战斗组件
   * @param {number} deltaTime - 帧间隔时间
   */
  update(deltaTime) {
    // 重置攻击状态
    if (this.isAttacking) {
      this.isAttacking = false;
    }
  }

  // ---- 技能阶段流水线（对齐后端 combat.CombatSystem） ----

  /**
   * 开始技能阶段流水线
   * 后端流程：Windup → Hit → Settle → Recovery
   * @param {Object} skill - 技能数据
   */
  startSkillPipeline(skill) {
    this.phaseSkill = skill;
    this.currentPhase = SKILL_PHASE.WINDUP;
    this.phaseStartTime = Date.now();
    this.interruptPolicy = skill.interruptPolicy || INTERRUPT_POLICY.CANCEL;
  }

  /**
   * 推进到下一个阶段
   * @returns {number|null} 新阶段或 null（流水线结束）
   */
  advancePhase() {
    if (this.currentPhase === null) return null;
    
    const nextPhase = this.currentPhase + 1;
    if (nextPhase > SKILL_PHASE.RECOVERY) {
      // 流水线结束
      this.currentPhase = null;
      this.phaseSkill = null;
      this.phaseStartTime = 0;
      return null;
    }
    
    this.currentPhase = nextPhase;
    this.phaseStartTime = Date.now();
    return this.currentPhase;
  }

  /**
   * 打断技能流水线
   * @returns {boolean} 是否成功打断
   */
  interruptPipeline() {
    if (this.currentPhase === null) return false;
    
    if (this.interruptPolicy === INTERRUPT_POLICY.CONTINUE) {
      return false; // 不可打断
    }
    
    this.currentPhase = null;
    this.phaseSkill = null;
    this.phaseStartTime = 0;
    this.interruptCast();
    return true;
  }

  /**
   * 检查当前阶段是否完成
   * @returns {boolean}
   */
  isPhaseComplete() {
    if (!this.phaseSkill || this.currentPhase === null) return false;
    
    const elapsed = Date.now() - this.phaseStartTime;
    const durations = this.phaseSkill.phaseDurations || {};
    
    switch (this.currentPhase) {
      case SKILL_PHASE.WINDUP:
        return elapsed >= (durations.windup || 0);
      case SKILL_PHASE.HIT:
        return elapsed >= (durations.hit || 0);
      case SKILL_PHASE.SETTLE:
        return elapsed >= (durations.settle || 0);
      case SKILL_PHASE.RECOVERY:
        return elapsed >= (durations.recovery || 0);
      default:
        return true;
    }
  }

  /**
   * 判断目标是否在技能范围内
   * 对齐后端 arena.go 的距离判定逻辑
   * @param {Object} selfPos - 自身位置 {x, y}
   * @param {Object} targetPos - 目标位置 {x, y}
   * @param {Object} skill - 技能数据
   * @returns {boolean}
   */
  isInSkillRange(selfPos, targetPos, skill) {
    const dx = selfPos.x - targetPos.x;
    const dy = selfPos.y - targetPos.y;
    const dist = Math.sqrt(dx * dx + dy * dy);
    
    const areaType = skill.area_type || skill.areaType || 'single';
    const range = skill.range || this.attackRange;
    
    switch (areaType) {
      case 'single':
        return dist <= range;
      case 'circle':
        return dist <= (skill.area_size || skill.areaSize || range);
      case 'fan':
        return dist <= range;
      default:
        return dist <= range;
    }
  }

  /**
   * 判断两个实体是否敌对
   * @param {string} otherFaction - 对方阵营
   * @returns {boolean}
   */
  isHostile(otherFaction) {
    if (this.faction === 'neutral' || otherFaction === 'neutral') return false;
    return this.faction !== otherFaction;
  }
}
