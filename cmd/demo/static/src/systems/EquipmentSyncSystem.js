/**
 * EquipmentSyncSystem - 后端装备数据同步到前端 ECS 组件
 *
 * 将后端 JSON 装备格式转换为前端 EquipmentComponent 所需格式，
 * 并完成装备/背包的分配。
 *
 * 使用方式：
 *   EquipmentSyncSystem.loadFromBackend(playerEntity, backendEquips)
 */
export class EquipmentSyncSystem {
    // 后端 slot_type -> 前端 slot 映射
    static SLOT_MAP = {
        'weapon': 'mainhand',
        'helmet': 'helmet',
        'armor': 'armor',
        'boots': 'boots',
        'ammo': 'offhand'
    };

    // 后端 quality -> 前端 rarity 数值映射
    static QUALITY_MAP = {
        'normal': 0,
        'rare': 2,
        'epic': 3,
        'legendary': 4
    };

    // 职业默认武器属性
    static WEAPON_PROPS = {
        'warrior': { attackSpeed: 2.0, attackRange: 90, attackDistance: 100 },
        'archer':  { attackSpeed: 3.0, attackRange: 30, attackDistance: 250 }
    };

    /**
     * 将后端装备数据加载到玩家实体的 EquipmentComponent / InventoryComponent
     * @param {Object} playerEntity - 玩家 ECS 实体（需有 equipment、inventory 组件）
     * @param {Array}  backendEquips - 后端装备数组
     */
    static loadFromBackend(playerEntity, backendEquips) {
        if (!playerEntity || !backendEquips || backendEquips.length === 0) return;

        const equipment = playerEntity.getComponent('equipment');
        if (!equipment) return;

        const charClass = playerEntity.class || 'warrior';
        const { SLOT_MAP, QUALITY_MAP, WEAPON_PROPS } = EquipmentSyncSystem;

        // ── 弹药合并处理 ──────────────────────────────────────────
        let totalAmmo = 0;
        let ammoTemplate = null;
        for (const eq of backendEquips) {
            if (eq.slot_type !== 'ammo') continue;
            totalAmmo += (eq.quantity || 1);
            if (!ammoTemplate) {
                ammoTemplate = {
                    id: eq.def.icon_id || eq.def.id,
                    name: eq.def.name,
                    type: 'ammo', subType: 'ammo', maxStack: 99,
                    rarity: QUALITY_MAP[eq.def.quality] ?? 0,
                    level: eq.def.level,
                    stats: { attack: eq.def.attack || 0, defense: 0, maxHp: 0, speed: 0 }
                };
            }
        }
        if (ammoTemplate) {
            const equipQty = Math.min(totalAmmo, 99);
            equipment.equip('offhand', { ...ammoTemplate, quantity: equipQty });
            const remaining = totalAmmo - equipQty;
            if (remaining > 0) {
                const inventory = playerEntity.getComponent('inventory');
                inventory?.addItem({ ...ammoTemplate }, remaining);
            }
        }

        // ── 其他装备 ──────────────────────────────────────────────
        for (const eq of backendEquips) {
            const def = eq.def;
            if (!def || eq.slot_type === 'ammo') continue;

            const frontendSlot = SLOT_MAP[eq.slot_type];
            if (!frontendSlot) continue;

            const item = {
                id: def.icon_id || def.id,
                name: def.name,
                type: eq.slot_type,
                subType: frontendSlot,
                rarity: QUALITY_MAP[def.quality] ?? 0,
                level: def.level,
                stats: {
                    attack: def.attack || 0,
                    defense: def.defense || 0,
                    maxHp: def.hp || 0,
                    speed: def.speed || 0
                }
            };

            if (eq.slot_type === 'weapon') {
                const wp = WEAPON_PROPS[charClass] || WEAPON_PROPS['warrior'];
                item.attackSpeed    = def.attack_interval || wp.attackSpeed;
                item.attackRange    = def.attack_range    || wp.attackRange;
                item.attackDistance = def.attack_distance || wp.attackDistance;
                item.pierce         = def.pierce     || 0;
                item.multiArrow     = def.multi_arrow || 0;
                if (charClass === 'archer' || def.pierce > 0 || def.multi_arrow > 0) {
                    item.ranged  = true;
                    item.subType = 'bow';
                }
            }

            equipment.equip(frontendSlot, item);
        }

        console.log('EquipmentSyncSystem: 加载完成', backendEquips.map(e => e.def?.name));
    }
}
