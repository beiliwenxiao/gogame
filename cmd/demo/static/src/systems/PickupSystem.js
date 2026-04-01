/**
 * PickupSystem.js
 * 拾取系统 - 处理玩家拾取地面物品、装备、武器
 */

import { Entity } from '../ecs/Entity.js';
import { TransformComponent } from '../ecs/components/TransformComponent.js';
import { NameComponent } from '../ecs/components/NameComponent.js';

export class PickupSystem {
    constructor(config = {}) {
        this.pickupRadius    = config.pickupRadius    ?? 75;
        this.pickupCooldown  = config.pickupCooldown  ?? 300;
        this.pickupKey       = config.pickupKey       ?? 'e';
        this.lastPickupTime  = 0;
        this.inputManager        = null;
        this.floatingTextManager = null;
        this.weaponRenderer      = null;
    }

    init({ inputManager, floatingTextManager, weaponRenderer } = {}) {
        this.inputManager        = inputManager        ?? null;
        this.floatingTextManager = floatingTextManager ?? null;
        this.weaponRenderer      = weaponRenderer      ?? null;
    }

    update(playerEntity, pickupItems = [], equipmentItems = [], allEntities = []) {
        const removedEntities = [];
        if (!playerEntity || playerEntity.dead) return { removedEntities };

        const pt = playerEntity.getComponent('transform');
        if (!pt) return { removedEntities };

        const now = Date.now();
        const ePressed = this.inputManager?.isKeyPressed(this.pickupKey);
        if (!ePressed) return { removedEntities };
        if (now - this.lastPickupTime < this.pickupCooldown) return { removedEntities };

        const px = pt.position.x, py = pt.position.y;
        const r2 = this.pickupRadius * this.pickupRadius;

        for (const item of pickupItems) {
            if (item._picked) continue;
            const t = item.getComponent('transform');
            if (!t) continue;
            const dx = t.position.x - px, dy = t.position.y - py;
            if (dx * dx + dy * dy > r2) continue;
            item._picked = true;
            this.lastPickupTime = now;
            removedEntities.push(item);
            const inventory = playerEntity.getComponent('inventory');
            if (inventory && item._itemData) inventory.addItem(item._itemData, 1);
            if (this.floatingTextManager && item._itemData) {
                this.floatingTextManager.addText(px, py - 20, `+1 ${item._itemData.name}`, '#ffffff');
            }
        }

        for (const item of equipmentItems) {
            if (item._picked) continue;
            const t = item.getComponent('transform');
            if (!t) continue;
            const dx = t.position.x - px, dy = t.position.y - py;
            if (dx * dx + dy * dy > r2) continue;
            item._picked = true;
            this.lastPickupTime = now;
            removedEntities.push(item);
            const equipment = playerEntity.getComponent('equipment');
            if (equipment && item._equipData) equipment.equip(item._equipData.slotType, item._equipData);
            if (this.floatingTextManager && item._equipData) {
                this.floatingTextManager.addText(px, py - 20, `拾取 ${item._equipData.name}`, '#ffdd88');
            }
        }

        return { removedEntities };
    }

    checkWeaponPickup(playerEntity) {
        if (!this.weaponRenderer || !playerEntity) return;
        const thrown = this.weaponRenderer.thrownWeapon;
        if (!thrown || thrown.flying) return;
        const pt = playerEntity.getComponent('transform');
        if (!pt) return;
        const dx = thrown.x - pt.position.x;
        const dy = thrown.y - pt.position.y;
        if (dx * dx + dy * dy <= this.pickupRadius * this.pickupRadius) {
            this.weaponRenderer.recallWeapon?.();
        }
    }

    spawnLootItems(position, lootItems = []) {
        const entities = [];
        for (const loot of lootItems) {
            const entity = new Entity();
            const t = new TransformComponent();
            t.position.x = position.x + (Math.random() - 0.5) * 40;
            t.position.y = position.y + (Math.random() - 0.5) * 20;
            entity.addComponent(t);
            if (loot.name) entity.addComponent(new NameComponent(loot.name));
            entity._itemData  = loot.type === 'consumable' ? loot : null;
            entity._equipData = loot.type !== 'consumable' ? loot : null;
            entity._picked    = false;
            entities.push(entity);
        }
        return entities;
    }
}
