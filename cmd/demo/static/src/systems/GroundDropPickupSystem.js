/**
 * PickupSystem - 地面掉落物拾取系统
 *
 * 处理玩家拾取地面掉落物的逻辑：背包操作、装备操作、浮动文字反馈。
 *
 * 使用方式：
 *   PickupSystem.pickup(playerEntity, drop, floatingTextManager)
 */
export class GroundDropPickupSystem {
    /**
     * 拾取掉落物
     * @param {Object} playerEntity - 玩家 ECS 实体
     * @param {Object} drop - 掉落物数据 { dropType, dropName, dropCount, ... }
     * @param {Object} [floatingTextManager] - 浮动文字管理器（可选）
     */
    static pickup(playerEntity, drop, floatingTextManager) {
        if (!playerEntity || playerEntity.dead) return;
        drop.picked = true;

        const transform = playerEntity.getComponent('transform');
        const inventory = playerEntity.getComponent('inventory');
        const px = transform?.position.x ?? 0;
        const py = transform?.position.y ?? 0;

        switch (drop.dropType) {
            case 'health_potion':
                if (inventory) {
                    inventory.addItem({
                        id: 'health_potion', name: '红瓶', type: 'consumable',
                        subType: 'health_potion', maxStack: 20, usable: true, rarity: 0,
                        effect: { type: 'heal', value: 50 }, stats: {}
                    }, 1);
                }
                floatingTextManager?.addText(px, py - 20, '+1 红瓶', '#ff4444');
                break;

            case 'mana_potion':
                if (inventory) {
                    inventory.addItem({
                        id: 'mana_potion', name: '蓝瓶', type: 'consumable',
                        subType: 'mana_potion', maxStack: 20, usable: true, rarity: 0,
                        effect: { type: 'restore_mana', value: 30 }, stats: {}
                    }, 1);
                }
                floatingTextManager?.addText(px, py - 20, '+1 蓝瓶', '#4488ff');
                break;

            case 'iron_arrow':
            case 'wood_arrow': {
                const equipment = playerEntity.getComponent('equipment');
                const inventory = playerEntity.getComponent('inventory');
                if (!equipment) break;
                const count = drop.dropCount || 1;
                const isIron = drop.dropType === 'iron_arrow';
                const arrowName = isIron ? '铁箭' : '木箭';
                const arrowId = isIron ? 'iron_arrow' : 'wooden_arrow';

                // 判断主手是否为近战武器（非弓）
                const mainhand = equipment.getEquipment('mainhand');
                const isMeleeWeapon = mainhand && mainhand.subType !== 'bow';

                // 箭矢始终放入背包（99支一组）
                if (inventory) {
                    inventory.addItem({
                        id: arrowId, name: arrowName, type: 'ammo', subType: 'ammo',
                        rarity: 0, level: 1, maxStack: 99,
                        stats: { attack: 0, defense: 0, maxHp: 0, speed: 0 }
                    }, count);
                }

                // 仅在非近战武器时，才自动装备到副手（副手无箭时）
                if (!isMeleeWeapon) {
                    const offhand = equipment.getEquipment('offhand');
                    if (!offhand || offhand.subType !== 'ammo') {
                        // 从背包取出一组装备到副手
                        if (inventory) {
                            const ammoSlot = inventory.getAllItems().find(
                                ({ slot }) => slot.item.id === arrowId
                            );
                            if (ammoSlot) {
                                const qty = Math.min(ammoSlot.slot.quantity, 99);
                                inventory.removeFromSlot(ammoSlot.index, qty);
                                equipment.equip('offhand', {
                                    id: arrowId, name: arrowName, type: 'ammo', subType: 'ammo',
                                    rarity: 0, level: 1, quantity: qty, maxStack: 99,
                                    stats: { attack: 0, defense: 0, maxHp: 0, speed: 0 }
                                });
                            }
                        }
                    } else {
                        // 副手已有箭矢，直接叠加数量
                        offhand.quantity = (offhand.quantity || 0) + count;
                        // 同时从背包移除刚加入的那部分（已叠加到副手）
                        if (inventory) inventory.removeItem(arrowId, count);
                    }
                }

                floatingTextManager?.addText(px, py - 20, `+${count} ${arrowName}`, '#88ccff');
                break;
            }
        }
    }
}
