/**
 * PickupSystem - 地面掉落物拾取系统
 *
 * 处理玩家拾取地面掉落物的逻辑：背包操作、装备操作、浮动文字反馈。
 *
 * 使用方式：
 *   PickupSystem.pickup(playerEntity, drop, floatingTextManager)
 */
export class PickupSystem {
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
                if (!equipment) break;
                const count = drop.dropCount || 1;
                const isIron = drop.dropType === 'iron_arrow';
                const arrowName = isIron ? '铁箭' : '木箭';
                const arrowId = isIron ? 'iron_arrow' : 'wooden_arrow';
                const offhand = equipment.getEquipment('offhand');
                if (offhand && offhand.subType === 'ammo') {
                    offhand.quantity = (offhand.quantity || 0) + count;
                } else {
                    equipment.equip('offhand', {
                        id: arrowId, name: arrowName, type: 'ammo', subType: 'ammo',
                        rarity: 0, level: 1, quantity: count,
                        stats: { attack: 0, defense: 0, maxHp: 0, speed: 0 }
                    });
                }
                floatingTextManager?.addText(px, py - 20, `+${count} ${arrowName}`, '#88ccff');
                break;
            }
        }
    }
}
