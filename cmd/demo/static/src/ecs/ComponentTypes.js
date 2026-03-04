/**
 * ComponentTypes.js
 * ECS 组件类型常量 - 与后端 gfgame/internal/ecs 保持一致
 * 
 * 后端 Go 定义：
 *   CompPosition        = 1
 *   CompMovement        = 2
 *   CompCombatAttribute = 3
 *   CompEquipment       = 4
 *   CompSkill           = 5
 *   CompBuff            = 6
 *   CompNetwork         = 7
 *   CompAOI             = 8
 */

/** 组件类型数字常量（与后端一致） */
export const COMP_TYPE = {
  POSITION: 1,
  MOVEMENT: 2,
  COMBAT_ATTRIBUTE: 3,
  EQUIPMENT: 4,
  SKILL: 5,
  BUFF: 6,
  NETWORK: 7,
  AOI: 8
};

/** 前端组件字符串名 → 后端数字类型 映射 */
export const COMP_NAME_TO_TYPE = {
  'transform': COMP_TYPE.POSITION,
  'movement': COMP_TYPE.MOVEMENT,
  'stats': COMP_TYPE.COMBAT_ATTRIBUTE,
  'equipment': COMP_TYPE.EQUIPMENT,
  'combat': COMP_TYPE.SKILL,       // 前端 combat 组件包含技能逻辑
  'statusEffect': COMP_TYPE.BUFF,
  'name': 0,                        // 前端独有，后端无对应
  'sprite': 0,                      // 前端独有，后端无对应
  'inventory': 0                    // 前端独有，后端无对应
};

/** 后端数字类型 → 前端组件字符串名 映射 */
export const COMP_TYPE_TO_NAME = {
  [COMP_TYPE.POSITION]: 'transform',
  [COMP_TYPE.MOVEMENT]: 'movement',
  [COMP_TYPE.COMBAT_ATTRIBUTE]: 'stats',
  [COMP_TYPE.EQUIPMENT]: 'equipment',
  [COMP_TYPE.SKILL]: 'combat',
  [COMP_TYPE.BUFF]: 'statusEffect'
};

/** 技能目标模式（与后端 TargetMode 一致） */
export const TARGET_MODE = {
  SINGLE: 0,      // TargetSingle
  FAN: 1,          // TargetFan
  CIRCLE: 2,       // TargetCircle
  RECTANGLE: 3     // TargetRectangle
};

/** 技能目标模式：字符串 → 数字 */
export const TARGET_MODE_FROM_STRING = {
  'single': TARGET_MODE.SINGLE,
  'fan': TARGET_MODE.FAN,
  'circle': TARGET_MODE.CIRCLE,
  'rectangle': TARGET_MODE.RECTANGLE
};

/** 技能阶段（与后端 SkillPhase 一致） */
export const SKILL_PHASE = {
  WINDUP: 0,       // 前摇
  HIT: 1,          // 命中检测
  SETTLE: 2,       // 伤害结算
  RECOVERY: 3      // 后摇
};

/** 打断策略（与后端 InterruptPolicy 一致） */
export const INTERRUPT_POLICY = {
  CANCEL: 0,       // 取消剩余阶段
  CONTINUE: 1      // 继续执行
};

/** 装备品质（与后端 EquipmentQuality 一致） */
export const EQUIPMENT_QUALITY = {
  NORMAL: 0,
  RARE: 1,
  EPIC: 2,
  LEGENDARY: 3
};

/**
 * 后端伤害公式（与 arena.go calcDamage 一致）
 * base = attack - defense * 0.5, min 1
 * variance = 0.85 ~ 1.15
 * @param {number} attack - 攻击力
 * @param {number} defense - 防御力
 * @returns {number} 伤害值
 */
export function calcDamage(attack, defense) {
  let base = attack - defense * 0.5;
  if (base < 1) base = 1;
  return base * (0.85 + Math.random() * 0.3);
}

/**
 * 暴击判定
 * @param {number} critRate - 暴击率 (0~1)
 * @param {number} critDamage - 暴击倍率 (默认1.5)
 * @param {number} baseDamage - 基础伤害
 * @returns {{ damage: number, isCrit: boolean }}
 */
export function applyCrit(critRate, critDamage, baseDamage) {
  const isCrit = Math.random() < critRate;
  return {
    damage: isCrit ? Math.round(baseDamage * critDamage) : Math.round(baseDamage),
    isCrit
  };
}
