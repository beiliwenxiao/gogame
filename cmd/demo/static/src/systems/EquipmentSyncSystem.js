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
        if (!playerEntity) return;

        const equipment = playerEntity.getComponent('equipment');
        if (!equipment) return;

        // 先清空所有装备槽（后端权威，完全替换）
        for (const slot in equipment.slots) {
            equipment.slots[slot] = null;
        }
        equipment.recalculateBonusStats();

        if (!backendEquips || backendEquips.length === 0) return;

        const charClass = playerEntity.class || 'warrior';
        const { SLOT_MAP, QUALITY_MAP, WEAPON_PROPS } = EquipmentSyncSystem;

        // ── 弹药类型标记处理（数量由 loadInventoryFromBackend 管理） ──
        let ammoTemplate = null;
        for (const eq of backendEquips) {
            if (eq.slot_type !== 'ammo') continue;
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
            // 副手只记录箭矢类型标记，不存数量
            equipment.equip('offhand', { ...ammoTemplate, quantity: null });
        }

        // ── 其他装备 ──────────────────────────────────────────────
        for (const eq of backendEquips) {
            const def = eq.def;
            if (!def || eq.slot_type === 'ammo') continue;

            const frontendSlot = SLOT_MAP[eq.slot_type];
            if (!frontendSlot) continue;

            const item = {
                id: def.icon_id || def.id,
                defId: def.id,  // 后端装备定义 ID，用于重新装备时同步
                name: def.name,
                type: 'equipment',  // 统一用 'equipment'，方便背包装备逻辑识别
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

    /**
     * 将后端背包数据加载到玩家实体的 InventoryComponent（完全替换）
     */
    static loadInventoryFromBackend(playerEntity, backendInventory) {
        if (!playerEntity) return;
        const inventory = playerEntity.getComponent('inventory');
        if (!inventory) return;

        inventory.clear();

        if (!backendInventory || backendInventory.length === 0) return;
        const { QUALITY_MAP, WEAPON_PROPS } = EquipmentSyncSystem;
        const charClass = playerEntity.class || 'warrior';

        for (const item of backendInventory) {
            const def = item.def;
            if (!def) continue;
            const isAmmo = def.slot_type === 'ammo';
            const isWeapon = def.slot_type === 'weapon';
            const isRanged = charClass === 'archer' || def.pierce > 0 || def.multi_arrow > 0;
            const frontendItem = {
                id: def.icon_id || String(def.id),
                defId: def.id,
                name: def.name,
                type: isAmmo ? 'ammo' : 'equipment',
                subType: isAmmo ? 'ammo' : (isWeapon ? (isRanged ? 'bow' : 'mainhand') : def.slot_type),
                rarity: QUALITY_MAP[def.quality] ?? 0,
                level: def.level,
                maxStack: isAmmo ? 99 : 1,
                stats: { attack: def.attack || 0, defense: def.defense || 0, maxHp: def.hp || 0, speed: def.speed || 0 }
            };
            if (isWeapon) {
                const wp = WEAPON_PROPS[charClass] || WEAPON_PROPS['warrior'];
                frontendItem.attackSpeed    = def.attack_interval || wp.attackSpeed;
                frontendItem.attackRange    = def.attack_range    || wp.attackRange;
                frontendItem.attackDistance = def.attack_distance || wp.attackDistance;
                frontendItem.pierce         = def.pierce     || 0;
                frontendItem.multiArrow     = def.multi_arrow || 0;
                frontendItem.ranged         = isRanged;
            }
            inventory.addItem(frontendItem, item.quantity);
        }
    }
}
