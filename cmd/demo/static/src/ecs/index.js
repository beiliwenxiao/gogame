/**
 * index.js
 * ECS模块导出
 */

// 核心
export { Component } from './Component.js';
export { Entity } from './Entity.js';
export { EntityFactory } from './EntityFactory.js';

// 组件类型常量（对齐后端）
export {
  COMP_TYPE, COMP_NAME_TO_TYPE, COMP_TYPE_TO_NAME,
  TARGET_MODE, TARGET_MODE_FROM_STRING,
  SKILL_PHASE, INTERRUPT_POLICY,
  EQUIPMENT_QUALITY,
  calcDamage, applyCrit
} from './ComponentTypes.js';

// 组件
export { TransformComponent } from './components/TransformComponent.js';
export { StatsComponent } from './components/StatsComponent.js';
export { SpriteComponent } from './components/SpriteComponent.js';
export { CombatComponent } from './components/CombatComponent.js';
export { MovementComponent } from './components/MovementComponent.js';
export { EquipmentComponent } from './components/EquipmentComponent.js';
export { StatusEffectComponent, StatusEffect, StatusEffectType, StatusEffectData } from './components/StatusEffectComponent.js';
export { NameComponent } from './components/NameComponent.js';
export { InventoryComponent } from './components/InventoryComponent.js';
